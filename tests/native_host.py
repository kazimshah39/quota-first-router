"""Portable CPA acceptance for disposable native runners only.
Installs a temporary test CA in the current user's native trust store on macOS/
Windows, removes it in finally, and never uses personal credentials. Not for a
shared desktop/server. Linux uses SSL_CERT_FILE instead. No external inference.
"""
import argparse
import concurrent.futures
import datetime
import hashlib
import http.server
import importlib.util
import json
import os
import zipfile
import io
import urllib.parse
from pathlib import Path
import platform
import select
import socket
import ssl
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

MODEL='gpt-6-luna'
BASE='/v0/management/plugins/quota-first-router'

def native_target():
    goos={'Linux':'linux','Darwin':'darwin','Windows':'windows'}[platform.system()]
    arch={'x86_64':'amd64','AMD64':'amd64','aarch64':'arm64','arm64':'arm64','ARM64':'arm64'}[platform.machine()]
    return goos,arch

def main():
    p=argparse.ArgumentParser();p.add_argument('--cpa',required=True);p.add_argument('--library',required=True);p.add_argument('--archive',required=True);p.add_argument('--output',required=True);p.add_argument('--disposable-native-runner',action='store_true',required=True);p.add_argument('--seconds',type=int,default=75);a=p.parse_args()
    goos,arch=native_target();library=Path(a.library).resolve();cpa=Path(a.cpa).resolve()
    version_output=subprocess.run([str(cpa),'--version'],capture_output=True,text=True,timeout=15)
    assert '8.1.0' in version_output.stdout+version_output.stderr,'Wrong CPA host version'
    spec=importlib.util.spec_from_file_location('native_binary',Path(__file__).resolve().parents[1]/'scripts/binary.py');mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod);inspection=mod.inspect(library.read_bytes(),goos,arch)
    trust_added=False;proc=None;servers=[];accounts={'a':(20,120),'b':(20,20),'c':(10,10)};exhausted=set();polls=[];messages=[];stop=threading.Event()
    with tempfile.TemporaryDirectory(prefix='qfr-native-') as tmp:
        root=Path(tmp);cert=root/'cert.pem';key=root/'key.pem';serial=str(int(time.time_ns()))
        cert_config=root/'openssl.cnf'
        cert_config.write_text('[req]\ndistinguished_name=dn\nx509_extensions=extensions\nprompt=no\n[dn]\nCN=QFR Disposable Native Test '+serial+'\n[extensions]\nsubjectAltName=DNS:chatgpt.com\nbasicConstraints=critical,CA:TRUE\nkeyUsage=critical,digitalSignature,keyEncipherment,keyCertSign,cRLSign\nsubjectKeyIdentifier=hash\n')
        subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','1','-set_serial',serial,'-keyout',str(key),'-out',str(cert),'-config',str(cert_config)],check=True,capture_output=True,timeout=30)
        print('PASS generated isolated TLS certificate',flush=True)
        fingerprint=subprocess.check_output(['openssl','x509','-in',str(cert),'-noout','-fingerprint','-sha1'],text=True).strip().split('=')[1].replace(':','')
        def trust(add):
            if goos=='windows':
                certutil=str(Path(os.environ['SystemRoot'])/'System32/certutil.exe')
                cmd=[certutil,'-f','-addstore','Root',str(cert)] if add else [certutil,'-delstore','Root',fingerprint]
            elif goos=='darwin':
                keychain='/Library/Keychains/System.keychain'
                cmd=['sudo','-n','security','add-trusted-cert','-d','-r','trustRoot','-k',keychain,str(cert)] if add else ['sudo','-n','security','delete-certificate','-Z',fingerprint,keychain]
            else:return
            print('Native trust', 'add' if add else 'remove',goos,flush=True)
            subprocess.run(cmd,check=True,capture_output=True,timeout=30)
            if not add and goos=='darwin':
                # -t/remove-trusted-cert hangs on hosted macOS (runner-images
                # issue 12116). Delete the exact certificate and prove it is
                # absent; this disposable VM is destroyed after the job.
                remaining=subprocess.check_output(['security','find-certificate','-a','-Z',keychain],text=True,timeout=30)
                assert fingerprint not in remaining,'Temporary CA still present'
                print('PASS temporary system CA removed',flush=True)
        class Fixture(http.server.BaseHTTPRequestHandler):
            def log_message(self,*_):pass
            def answer(self,code,value,content='application/json'):
                raw=value if isinstance(value,bytes) else json.dumps(value).encode();self.send_response(code);self.send_header('Content-Type',content);self.send_header('Content-Length',str(len(raw)));self.end_headers();self.wfile.write(raw)
            def account(self):
                account=self.headers.get('Authorization','').removeprefix('Bearer synthetic-');assert account in accounts;return account
            def do_GET(self):
                assert self.path=='/backend-api/wham/usage';id=self.account();polls.append(id)
                self.answer(200,{'rate_limit':{'allowed':True,'primary_window':{'used_percent':100 if id in exhausted else 20,'limit_window_seconds':18000,'reset_at':int(time.time())+accounts[id][1]*60},'secondary_window':{'used_percent':10,'limit_window_seconds':604800,'reset_after_seconds':86400}}})
            def do_POST(self):
                assert self.path=='/backend-api/codex/responses';id=self.account();messages.append(id);self.rfile.read(int(self.headers['Content-Length']))
                response={'id':'resp_fixture','object':'response','status':'completed','model':MODEL,'output':[{'id':'msg_fixture','type':'message','role':'assistant','status':'completed','content':[{'type':'output_text','text':id,'annotations':[]}]}],'usage':{'input_tokens':1,'output_tokens':1,'total_tokens':2}}
                events=[{'type':'response.created','response':dict(response,status='in_progress',output=[])},{'type':'response.output_text.delta','output_index':0,'content_index':0,'delta':id},{'type':'response.completed','response':response}]
                self.answer(200,''.join('data: '+json.dumps(e)+'\n\n' for e in events).encode(),'text/event-stream')
        fixture=http.server.ThreadingHTTPServer(('127.0.0.1',0),Fixture);ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);ctx.load_cert_chain(cert,key);fixture.socket=ctx.wrap_socket(fixture.socket,server_side=True)
        class Proxy(http.server.BaseHTTPRequestHandler):
            def log_message(self,*_):pass
            def do_CONNECT(self):
                if self.path!='chatgpt.com:443':self.send_error(403);return
                with socket.create_connection(fixture.server_address,timeout=10) as upstream:
                    self.send_response(200);self.end_headers();self.connection.settimeout(30)
                    while not stop.is_set():
                        ready,_,_=select.select([self.connection,upstream],[],[],.5)
                        for src in ready:
                            try:
                                raw=src.recv(65536)
                                if not raw:return
                                (upstream if src is self.connection else self.connection).sendall(raw)
                            except (ConnectionResetError,BrokenPipeError):return
        proxy=http.server.ThreadingHTTPServer(('127.0.0.1',0),Proxy)
        for server in (fixture,proxy):threading.Thread(target=server.serve_forever,daemon=True).start();servers.append(server)
        auths=root/'auths';plugins=root/'plugins';auths.mkdir();plugins.mkdir()
        archive_bytes=Path(a.archive).read_bytes()
        with zipfile.ZipFile(io.BytesIO(archive_bytes)) as z:
            assert z.read(library.name)==library.read_bytes(),'Archive and library differ'
        # An invalid library must be rejected without taking down CPA.
        (plugins/('invalid-native-fixture'+library.suffix)).write_bytes(b'not a shared library')
        other='arm64' if arch=='amd64' else 'amd64'
        other_archive=Path(a.archive).with_name(Path(a.archive).name.replace('_'+arch+'.zip','_'+other+'.zip'))
        with zipfile.ZipFile(other_archive) as z:(plugins/('wrong-machine-fixture'+library.suffix)).write_bytes(z.read(library.name))
        class Store(http.server.BaseHTTPRequestHandler):
            def log_message(self,*_):pass
            def do_GET(self):
                if self.path=='/plugin.zip':body=archive_bytes
                elif self.path=='/registry.json':
                    body=json.dumps({'schema_version':2,'plugins':[{'id':'quota-first-router','name':'Quota First Router','description':'Local synthetic installation acceptance','author':'test','version':'0.1.2','repository':'https://github.com/kazimshah39/quota-first-router','install':{'type':'direct','artifacts':[{'goos':goos,'goarch':arch,'url':f'http://127.0.0.1:{self.server.server_port}/plugin.zip','sha256':hashlib.sha256(archive_bytes).hexdigest()}]}}]}).encode()
                else:self.send_error(404);return
                self.send_response(200);self.send_header('Content-Type','application/octet-stream' if self.path.endswith('.zip') else 'application/json');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
        store=http.server.ThreadingHTTPServer(('127.0.0.1',0),Store);threading.Thread(target=store.serve_forever,daemon=True).start();servers.append(store)
        source=f'http://127.0.0.1:{store.server_port}/registry.json'
        for id,(priority,_) in accounts.items():(auths/(id+'.json')).write_text(json.dumps({'type':'codex','access_token':'synthetic-'+id,'account_id':id,'email':id+'@example.invalid','expired':'2099-01-01T00:00:00Z','priority':priority,'disabled':False,'proxy_url':'http://127.0.0.1:'+str(proxy.server_port)}))
        with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
        proxyurl='http://127.0.0.1:'+str(proxy.server_port)
        # JSON is also a valid YAML document and quotes native Windows paths safely.
        config={'config-version':8,'server':{'host':'127.0.0.1','port':port},'management':{'secret-key':'synthetic-management-key','allow-remote':False,'disable-control-panel':True},'oauth':{'auth-dir':str(auths)},'access':{'api-keys':['synthetic-client-key']},'routing':{'strategy':'fill-first','session-affinity':False},'plugins':{'enabled':True,'dir':str(plugins),'configs':{'quota-first-router':{'enabled':True,'priority':100,'mode':'shadow','proxy_url':proxyurl}}}}
        config['plugins']['store-sources']=[source]
        config['plugins']['store-auth']=[{'match':f'http://127.0.0.1:{store.server_port}','allow-insecure':True}]
        configpath=root/'config.yaml';configpath.write_text(json.dumps(config));env=dict(os.environ,SSL_CERT_FILE=str(cert));env={k:v for k,v in env.items() if not k.lower().endswith('_proxy')};log=(root/'cpa.log').open('w+')
        opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
        def api(path,body=None,method=None,client=False):
            req=urllib.request.Request(f'http://127.0.0.1:{port}'+path,data=None if body is None else json.dumps(body).encode(),headers={'Authorization':'Bearer '+('synthetic-client-key' if client else 'synthetic-management-key'),'Content-Type':'application/json'},method=method or ('POST' if body is not None else 'GET'))
            try:
                with opener.open(req,timeout=20) as response:return response.read()
            except urllib.error.HTTPError as ex:
                raise RuntimeError(path+': '+str(ex.code)+' '+ex.read().decode()) from ex
        def status():return json.loads(api(BASE+'/status'))
        def wait(fn,label,seconds=60):
            until=time.monotonic()+seconds;last=None
            while time.monotonic()<until:
                try:
                    last=fn()
                    if last:return last
                except (OSError,ValueError,KeyError,RuntimeError) as ex:last=repr(ex)
                assert proc.poll() is None,'CPA exited';time.sleep(.2)
            raise AssertionError(label+': '+str(last))
        def ready(mode):
            s=status();return s['mode']==mode and len(s['accounts'])==3 and all(x.get('quota') for x in s['accounts'].values())
        generation=0
        def configure(mode='active'):
            nonlocal generation
            generation+=1;interval='1m' if generation%2 else '2m';api(BASE+'/config',{'mode':mode,'poll_interval':interval},'PATCH');wait(lambda:ready(mode) and status()['poll_interval']==interval+'0s','refresh')
        def inference(stream=False,chat=False):
            body={'model':MODEL,'stream':stream};body.update({'messages':[{'role':'user','content':'Synthetic'}]} if chat else {'input':'Synthetic'})
            raw=api('/v1/chat/completions' if chat else '/v1/responses',body,client=True)
            if stream:return raw.decode()
            value=json.loads(raw);return value['choices'][0]['message']['content'] if chat else value['output'][0]['content'][0]['text']
        try:
            trust(True);trust_added=goos!='linux';proc=subprocess.Popen([str(cpa),'--config',str(configpath),'--local-model'],cwd=root,env=env,stdout=log,stderr=subprocess.STDOUT)
            print('Native CPA started',flush=True)
            wait(lambda:json.loads(api('/v0/management/plugins')),'host startup')
            print('PASS native host startup',flush=True)
            print('Fetching local store catalog',flush=True)
            catalog=json.loads(api('/v0/management/plugin-store'));matches=[x for x in catalog['plugins'] if x['id']=='quota-first-router' and x['source_url']==source];assert matches, catalog.get('source_errors');entry=matches[0]
            print('Installing exact native package',flush=True)
            installed=json.loads(api('/v0/management/plugin-store/quota-first-router/install?'+urllib.parse.urlencode({'source':entry['source_id']}),{}))
            assert hashlib.sha256(Path(installed['path']).read_bytes()).hexdigest()==hashlib.sha256(library.read_bytes()).hexdigest(),'Installer library differs'
            print('Waiting for native plugin registration',flush=True)
            wait(lambda:ready('shadow'),'registration and polling')
            defaults=status();assert defaults['version']=='0.1.2' and defaults['poll_interval']=='1m0s' and defaults['max_age']=='5m0s',defaults
            listing=json.loads(api('/v0/management/plugins'));assert all(name in json.dumps(listing) for name in ('invalid-native-fixture','wrong-machine-fixture')),'Invalid fixtures were not scanned'
            failures=[v for v in listing['plugins'] if 'fixture' in v.get('path','')]
            assert len(failures)==2 and all(not v.get('registered') for v in failures),'Invalid libraries registered'
            wait(lambda:any(m['id']==MODEL for m in json.loads(api('/v1/models',client=True))['data']),'model registration')
            for token in (None,'bad'):
                req=urllib.request.Request(f'http://127.0.0.1:{port}'+BASE+'/status',headers={} if token is None else {'Authorization':'Bearer '+token})
                try:opener.open(req);raise AssertionError('unauthenticated status')
                except urllib.error.HTTPError as ex:assert ex.code==401
            assert inference()=='a' and status()['last_decision']['auth_id']=='b.json';configure();assert inference()=='b'
            count=len(polls)
            for chat in (False,True):
                for stream in (False,True):assert 'b' in inference(stream,chat)
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:assert list(pool.map(lambda _:inference(),range(24)))==['b']*24
            assert len(polls)==count,'request path polled'
            end=time.monotonic()+a.seconds;requests=0
            while time.monotonic()<end:
                actual=inference();assert actual=='b',{'actual':actual,'status':status(),'polls':polls,'auth_files':[p.name for p in auths.iterdir()]}
                requests+=1;time.sleep(.1)
            if a.seconds>=65:assert len(polls)>count,'natural polling did not continue'
            exhausted.update(['a','b']);configure();assert inference()=='c','tier descent'
            exhausted.clear();configure();assert inference()=='b','tier recovery'
            api(BASE+'/enabled',{'enabled':False},'PATCH');wait(lambda:inference()=='a','disable rollback');api(BASE+'/enabled',{'enabled':True},'PATCH');wait(lambda:ready('active'),'reenable');assert inference()=='b'
            for id,(priority,_) in accounts.items():stored=json.loads((auths/(id+'.json')).read_text());assert stored['priority']==priority and not stored['disabled']
            evidence={'goos':goos,'goarch':arch,'native':True,'passed':True,'cpa_version':'8.1.0','library_sha256':hashlib.sha256(library.read_bytes()).hexdigest(),'cpa_sha256':hashlib.sha256(cpa.read_bytes()).hexdigest(),'inspection':inspection,'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'soak_seconds':a.seconds,'soak_requests':requests,'quota_gets':len(polls),'local_store_install':True,'archive_sha256':hashlib.sha256(archive_bytes).hexdigest(),'host_os':platform.platform(),'host_machine':platform.machine(),'go_version':subprocess.check_output([os.environ.get('GO','go'),'version'],text=True).strip(),'invalid_library_rejected':True,'wrong_arch_library_rejected':True}
            Path(a.output).parent.mkdir(parents=True,exist_ok=True);Path(a.output).write_text(json.dumps(evidence,indent=2));print('PASS native CPA acceptance',goos,arch)
        except BaseException:
            log.flush();log.seek(0);print(log.read());raise
        finally:
            if proc and proc.poll() is None:
                proc.terminate()
                try:proc.wait(timeout=15)
                except subprocess.TimeoutExpired:proc.kill();proc.wait()
            stop.set()
            for server in servers:server.shutdown();server.server_close()
            log.close()
            if trust_added:trust(False)

if __name__=='__main__':main()
