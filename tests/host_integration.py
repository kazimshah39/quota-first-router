"""Synthetic integration against the installed CPA 8.1.0 binary.
Runs in a disposable network-isolated container; no personal credentials.
"""
import argparse
import concurrent.futures
import hashlib
import http.server
import http.client
import json
import os
from pathlib import Path
import signal
import ssl
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request

PLUGIN = 'quota-first-router'
BASE = '/v0/management/plugins/' + PLUGIN
MODEL = 'gpt-6-luna'
IMAGE = 'golang:1.26-bookworm@sha256:a688600ca24f8a4d3ca77f95b0dd40704a9fc787c826660eb7ba0b641b8b175d'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--cpa', required=True)
    parser.add_argument('--library', required=True)
    args = parser.parse_args()
    if os.environ.get('QFR_ISOLATED') != '1':
        root = Path(__file__).resolve().parents[1]
        ui_fixture = os.environ.get('QFR_UI_FIXTURE') == '1'
        network = ['--network', 'bridge', '-p', '127.0.0.1:18319:18318', '-p', '127.0.0.1:18444:443'] if ui_fixture else ['--network', 'none']
        subprocess.run(['docker', 'run', '--rm', *network,
                        '-e', 'QFR_UI_FIXTURE=' + ('1' if ui_fixture else '0'),
                        '--add-host', 'chatgpt.com:127.0.0.1',
                        '-e', 'QFR_ISOLATED=1', '-e', 'SOAK_SECONDS=' + os.environ.get('SOAK_SECONDS', '0'),
                        '-v', f'{root}:/project:ro',
                        '-v', f'{Path(args.cpa).resolve()}:/cpa:ro',
                        '-w', '/project', IMAGE, 'python3', 'tests/host_integration.py',
                        '--cpa', '/cpa', '--library', '/project/' + str(Path(args.library))], check=True)
        return
    accounts = {'a-later': [20, 120], 'b-sooner': [20, 20], 'c-third': [20, 60], 'd-backup': [10, 10]}
    exhausted = set()
    weekly = set()
    failures = set()
    failure_status = 429
    rss_samples = []
    quota_error = False
    messages = []
    quota_calls = []
    errors = []
    upstream_requests = []
    cancellation_started = threading.Event()

    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def send(self, code, payload, content_type='application/json'):
            raw = json.dumps(payload).encode() if not isinstance(payload, bytes) else payload
            self.send_response(code)
            self.send_header('Content-Type', content_type)
            self.send_header('Content-Length', str(len(raw)))
            try:
                self.end_headers()
                self.wfile.write(raw)
            except (BrokenPipeError, ConnectionResetError, ssl.SSLEOFError):
                pass  # Expected when the synthetic cancellation client disconnects.

        def account(self):
            account = self.headers.get('Authorization', '').removeprefix('Bearer synthetic-')
            if account not in accounts:
                errors.append('unknown account')
            return account

        def do_GET(self):
            if self.path != '/backend-api/wham/usage':
                errors.append('unexpected quota path ' + self.path)
                return self.send(404, {})
            account = self.account()
            quota_calls.append(account)
            if quota_error:
                return self.send(503, {'error': 'fixture'})
            return self.send(200, {'plan_type': 'plus', 'rate_limit': {
                'allowed': True,
                'primary_window': {'used_percent': 100 if account in exhausted else 20,
                                   'limit_window_seconds': 18000,
                                   'reset_at': int(time.time()) + accounts[account][1] * 60},
                'secondary_window': {'used_percent': 100 if account in weekly else 10,
                                     'limit_window_seconds': 604800,
                                     'reset_after_seconds': 86400}}})

        def do_POST(self):
            nonlocal quota_error
            if self.path == '/fixture' and os.environ.get('QFR_UI_FIXTURE') == '1':
                command = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                quota_error = command.get('quota_error', False)
                exhausted.clear(); exhausted.update(command.get('exhausted', []))
                return self.send(200, {'ok':True})
            if self.path != '/backend-api/codex/responses':
                errors.append('unexpected request path ' + self.path)
                return self.send(404, {})
            account = self.account()
            request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            messages.append(account)
            upstream_requests.append(request)
            if 'QFR_CANCEL' in json.dumps(request.get('input')):
                cancellation_started.set()
                time.sleep(1)
            if account in failures:
                return self.send(failure_status, {'error': {'type': 'usage_limit_reached' if failure_status == 429 else 'server_error', 'message': 'fixture failure', 'resets_at': int(time.time()) + 60}})
            response = {'id': 'resp_fixture', 'object': 'response', 'status': 'completed', 'model': MODEL,
                        'output': [{'id': 'msg_fixture', 'type': 'message', 'role': 'assistant', 'status': 'completed',
                                    'content': [{'type': 'output_text', 'text': account, 'annotations': []}]}],
                        'usage': {'input_tokens': 1, 'output_tokens': 1, 'total_tokens': 2}}
            events = [
                {'type': 'response.created', 'response': dict(response, status='in_progress', output=[])},
                {'type': 'response.output_text.delta', 'output_index': 0, 'content_index': 0, 'delta': account},
                {'type': 'response.completed', 'response': response}]
            body = ''.join('data: ' + json.dumps(event) + '\n\n' for event in events).encode()
            return self.send(200, body, 'text/event-stream')

    with tempfile.TemporaryDirectory(prefix='qfr-integration-') as directory:
        root = Path(directory)
        cert, key = root / 'cert.pem', root / 'key.pem'
        subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
                        '-keyout', str(key), '-out', str(cert), '-subj', '/CN=chatgpt.com',
                        '-addext', 'subjectAltName=DNS:chatgpt.com'], check=True, capture_output=True)
        server = http.server.ThreadingHTTPServer(('127.0.0.1', 443), Fixture)
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(cert, key)
        server.socket = tls.wrap_socket(server.socket, server_side=True)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        auths = root / 'auths'
        plugins = root / 'plugins'
        auths.mkdir()
        plugins.mkdir()
        for account, (priority, _) in accounts.items():
            (auths / (account + '.json')).write_text(json.dumps({
                'type': 'codex', 'access_token': 'synthetic-' + account, 'account_id': account,
                'email': account + '@example.invalid', 'expired': '2099-01-01T00:00:00Z',
                'disabled': False, 'priority': priority}))
        (plugins / (PLUGIN + '.so')).write_bytes(Path(args.library).read_bytes())
        subprocess.run(['gcc', '-shared', '-fPIC', '/project/tests/competing_scheduler.c', '-o', str(plugins / 'test-scheduler.so')], check=True)
        config = root / 'config.yaml'
        config.write_text(f'''config-version: 8
server:
  host: {'0.0.0.0' if os.environ.get('QFR_UI_FIXTURE') == '1' else '127.0.0.1'}
  port: 18318
management:
  allow-remote: {'true' if os.environ.get('QFR_UI_FIXTURE') == '1' else 'false'}
  secret-key: synthetic-management-key
  disable-control-panel: true
oauth:
  auth-dir: {auths}
access:
  api-keys: [synthetic-client-key]
routing:
  strategy: fill-first
  session-affinity: false
  retry:
    request-retry: 0
    max-retry-interval: 1
plugins:
  enabled: true
  dir: {plugins}
  configs:
    {PLUGIN}:
      enabled: true
      priority: 100
    test-scheduler:
      enabled: false
      priority: 200
''')
        env = dict(os.environ, SSL_CERT_FILE=str(cert), GOMEMLIMIT='256MiB', GOGC='20')
        for k in list(env):
            if k.lower().endswith('_proxy'):
                del env[k]
        log = (root / 'host.log').open('w+')
        proc = subprocess.Popen([args.cpa, '--config', str(config), '--local-model'], cwd=root,
                                env=env, stdout=log, stderr=subprocess.STDOUT)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

        def api(path, body=None, method=None, management=True):
            token = 'synthetic-management-key' if management else 'synthetic-client-key'
            request = urllib.request.Request('http://127.0.0.1:18318' + path,
                data=None if body is None else json.dumps(body).encode(),
                headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'},
                method=method or ('GET' if body is None else 'POST'))
            with opener.open(request, timeout=15) as response:
                return response.read()

        def status():
            return json.loads(api(BASE + '/status'))

        def wait(check, label, timeout=30):
            deadline = time.monotonic() + timeout
            last = None
            while time.monotonic() < deadline:
                try:
                    last = check()
                    if last:
                        return last
                except (urllib.error.URLError, ValueError) as exc:
                    last = repr(exc)
                if proc.poll() is not None:
                    raise AssertionError('CPA exited: ' + label)
                time.sleep(.1)
            raise AssertionError(label + ': ' + repr(last))

        def ready():
            s = status()
            return len(s['accounts']) == 4 and all(a.get('quota') for a in s['accounts'].values())

        generation = 0

        def configure():
            nonlocal generation
            generation += 1
            interval = '2m' if generation % 2 else '1m'
            api(BASE + '/config', {'poll_interval': interval}, 'PATCH')
            wait(lambda: ready() and status()['poll_interval'] == interval + '0s', 'quota refresh')

        def chat(stream=False):
            raw = api('/v1/chat/completions', {'model': MODEL, 'messages': [{'role':'user','content':'Synthetic chat check'}], 'stream':stream}, management=False)
            if stream: return raw.decode()
            result=json.loads(raw); return result['choices'][0]['message']['content']

        def rss():
            lines=Path(f'/proc/{proc.pid}/status').read_text().splitlines()
            return int(next(l.split()[1] for l in lines if l.startswith('VmRSS:')))

        def message(stream=False):
            try:
                raw = api('/v1/responses', {'model': MODEL, 'input': 'Synthetic routing check', 'stream': stream}, management=False)
            except urllib.error.HTTPError as exc:
                print('REQUEST FAILURE', exc.code, exc.read().decode(), file=sys.stderr)
                raise
            return raw.decode() if stream else json.loads(raw)['output'][0]['content'][0]['text']

        try:
            wait(ready, 'startup')
            wait(lambda: any(m['id'] == MODEL for m in json.loads(api('/v1/models', management=False))['data']), 'model registration')
            # Native resource menu and protected JSON must remain separate.
            for path in [BASE + '/status', BASE + '/monitor']:
                try:
                    opener.open('http://127.0.0.1:18318' + path)
                    raise AssertionError('unauthenticated diagnostics allowed')
                except urllib.error.HTTPError as exc:
                    assert exc.code == 401
            resource = '/v0/resource/plugins/' + PLUGIN + '/monitor'
            response = opener.open('http://127.0.0.1:18318' + resource)
            assert response.headers.get('Cache-Control')=='no-store' and response.headers.get('X-Content-Type-Options')=='nosniff'
            assert "frame-ancestors 'self'" in response.headers.get('Content-Security-Policy','')
            shell = response.read().decode()
            assert 'Routing monitor' in shell and 'synthetic-client-key' not in shell and 'a-later.json' not in shell
            listing = json.loads(api('/v0/management/plugins'))
            registered = next(p for p in listing['plugins'] if p['id'] == PLUGIN)
            assert any(menu['path'] == resource for menu in registered['menus']), registered
            assert all(field['name'] != 'mode' for field in registered['config_fields']), 'legacy mode still advertised'
            before_monitor = len(quota_calls), len(upstream_requests), status()['picks']
            for _ in range(10):
                monitor = json.loads(api(BASE + '/monitor'))
                assert monitor['version'] and len(monitor['accounts']) == 4
            assert before_monitor == (len(quota_calls), len(upstream_requests), status()['picks']), 'monitor GET triggered polling, inference or selection'
            for method,path in [('POST',resource),('POST',BASE+'/monitor')]:
                try:api(path,{},method);raise AssertionError('Invalid method accepted')
                except urllib.error.HTTPError as exc:assert exc.code in (404,405)
            epoch=monitor['history']['epoch']
            if os.environ.get('QFR_UI_FIXTURE') == '1':
                configure()
                assert message() == 'b-sooner'
                print('UI_FIXTURE_READY', flush=True)
                stopped = threading.Event()
                signal.signal(signal.SIGTERM, lambda *_: stopped.set())
                stopped.wait()
                return
            # Management routes must retain host authentication.
            try:
                opener.open('http://127.0.0.1:18318' + BASE + '/status')
                raise AssertionError('unauthenticated status allowed')
            except urllib.error.HTTPError as exc:
                assert exc.code == 401
            assert message() == 'b-sooner', 'enabled router did not select by quota without mode settings'
            assert status()['last_decision']['auth_id'] == 'b-sooner.json'
            assert len(status()['last_decision']['candidates']) == 4, 'all tiers not offered'
            configure()
            assert json.loads(api(BASE+'/monitor'))['history']['epoch']!=epoch,'History epoch did not reset'
            # Explicit operator pinning wins over plugin ordering.
            verify = json.loads(api('/v8/management/credentials/verify', {'name': 'a-later.json', 'model': MODEL}))
            assert verify['verified'], verify
            assert messages[-1] == 'a-later', 'plugin overrode credential pin'
            api('/v0/management/auth-files/status', {'name': 'b-sooner.json', 'disabled': True}, 'PATCH')
            wait(lambda: message() == 'c-third', 'disabled account host filter')
            api('/v0/management/auth-files/status', {'name': 'b-sooner.json', 'disabled': False}, 'PATCH')
            wait(lambda: message() == 'b-sooner', 'reenabled account')
            # Model exclusions must be applied by the host before selection.
            bfile = auths / 'b-sooner.json'
            bstored = json.loads(bfile.read_text())
            bfile.write_text(json.dumps(dict(bstored, excluded_models=[MODEL])))
            wait(lambda: message() == 'c-third', 'model eligibility filter')
            bfile.write_text(json.dumps(bstored))
            wait(lambda: message() == 'b-sooner', 'model eligibility restored')
            assert chat() == 'b-sooner', 'chat completion routing'
            assert 'b-sooner' in chat(True), 'chat streaming routing'
            before = len(quota_calls)
            assert message() == 'b-sooner', 'active reset ranking'
            assert 'b-sooner' in message(True), 'streaming'
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                assert list(pool.map(lambda _: message(), range(24))) == ['b-sooner'] * 24
            assert len(quota_calls) == before, 'quota fetched in request path'
            soak_seconds = int(os.environ.get('SOAK_SECONDS', '0'))
            end, soak_requests = time.monotonic() + soak_seconds, 0
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                while time.monotonic() < end:
                    replies=list(pool.map(lambda i: chat(i%2==0) if i%3==0 else message(i%2==0), range(8)))
                    assert all(any(account in r for account in ['a-later', 'b-sooner', 'c-third']) for r in replies), 'soak selected outside usable top tier'
                    if not all('b-sooner' in r for r in replies):
                        print('SOAK fallback observation', json.dumps(status()), flush=True)
                    raw_monitor=api(BASE + '/monitor')
                    assert len(raw_monitor)<=8*1024*1024,'Monitor response exceeds cap'
                    monitor = json.loads(raw_monitor)
                    assert not monitor['worker_failed'], 'worker failed during soak'
                    assert len(monitor['history']['decisions']) <= 200 and len(monitor['history']['incidents']) <= 200
                    rss_samples.append(rss())
                    soak_requests += 8
                    time.sleep(.2)
            if soak_seconds:
                assert len(quota_calls) > before, 'background polling did not occur during soak'
                assert max(rss_samples) < 512*1024, 'RSS exceeded 512MiB budget'
                assert max(rss_samples[-min(100,len(rss_samples)):])-min(rss_samples[:min(100,len(rss_samples))]) < 128*1024, 'RSS growth exceeded 128MiB'
                assert monitor['history']['decision_evictions'] > 0, 'decision history did not roll over'
                print(f'PASS monitor rollover {monitor["history"]["decision_evictions"]} evicted decisions')
                print(f'PASS soak: {soak_requests} mixed protocol/stream requests in {soak_seconds} seconds; background polling continued; RSS KiB min={min(rss_samples)} max={max(rss_samples)} final={rss_samples[-1]}')
            exhausted.add('b-sooner')
            configure()
            assert message() == 'c-third', 'within-tier exhaustion'
            weekly.add('c-third')
            exhausted.add('a-later')
            configure()
            assert message() == 'd-backup', 'top tier did not fall through'
            exhausted.add('d-backup')
            configure()
            count = len(messages)
            try:
                message()
                raise AssertionError('all exhausted succeeded')
            except urllib.error.HTTPError as exc:
                assert exc.code in (429, 503), exc.read()
            assert len(messages) == count, 'known-empty account reached upstream'
            exhausted.clear()
            weekly.clear()
            configure()
            assert message() == 'b-sooner', 'higher tier recovery'
            accounts['b-sooner'][1] = 240
            configure()
            assert message() == 'c-third', 'reset rollover'
            quota_error = True
            api(BASE + '/config', {'poll_interval': '3m'}, 'PATCH')
            wait(lambda: all(s.get('last_error') == 'quota_http_503' for s in status()['accounts'].values()), 'quota error')
            assert message() == 'a-later', 'unknown quota did not delegate to highest tier'
            quota_error = False
            accounts['b-sooner'][1] = 20
            configure()
            api('/v0/management/plugins/test-scheduler/enabled', {'enabled': True}, 'PATCH')
            wait(lambda: message() == 'a-later', 'higher-priority competing scheduler')
            api('/v0/management/plugins/test-scheduler/enabled', {'enabled': False}, 'PATCH')
            wait(lambda: message() == 'b-sooner', 'scheduler competition rollback')
            # CPA must preserve tool definitions/outputs; the scheduler only
            # chooses credentials, never rewrites the protocol payload.
            raw = api('/v1/responses', {'model':MODEL, 'input':[
                {'type':'function_call','call_id':'synthetic-call','name':'echo','arguments':'{}'},
                {'type':'function_call_output','call_id':'synthetic-call','output':'OK'}],
                'tools':[{'type':'function','name':'echo','parameters':{'type':'object','properties':{}}}],
                'stream':False}, management=False)
            assert json.loads(raw)['status']=='completed'
            assert upstream_requests[-1]['tools'][0]['name']=='echo'
            assert any(item.get('type')=='function_call_output' for item in upstream_requests[-1]['input'])
            conn=http.client.HTTPConnection('127.0.0.1',18318,timeout=5)
            conn.request('POST','/v1/responses',json.dumps({'model':MODEL,'input':'QFR_CANCEL','stream':True}),
                         {'Authorization':'Bearer synthetic-client-key','Content-Type':'application/json'})
            assert cancellation_started.wait(5), 'cancel request did not reach fixture'
            conn.close()
            assert message()=='b-sooner', 'client cancellation harmed subsequent routing'
            for code in [500, 502, 503, 429, 401]:
                failure_status=code
                failures.add('b-sooner')
                assert message() == 'c-third', f'upstream {code} did not retry next account'
                failures.clear()
                if code == 401:
                    print('PASS 401 failover; CPA retains unauthorized state after verification, no automatic recovery claimed')
                    break
                # Explicit operator verification releases the synthetic failed account
                # and proves recovery without waiting for provider-sized cooldowns.
                verified=json.loads(api('/v8/management/credentials/verify', {'name':'b-sooner.json','model':MODEL}))
                assert verified['verified'], verified
                configure()
                wait(lambda: message() == 'b-sooner', f'upstream {code} recovery', timeout=75)
            api(BASE + '/enabled', {'enabled': False}, 'PATCH')
            wait(lambda: message() == 'a-later', 'disable restores CPA fill-first')
            assert next(p for p in json.loads(api('/v0/management/plugins'))['plugins'] if p['id'] == PLUGIN)['effective_enabled'] is False
            api(BASE + '/config', {'mode': 'shadow'}, 'PATCH')
            api(BASE + '/enabled', {'enabled': True}, 'PATCH')
            assert message() == 'a-later', 'unmigrated shadow configuration started routing'
            api(BASE + '/config', {'mode': None, 'enabled': False}, 'PATCH')
            assert 'mode' not in json.loads(api(BASE + '/config'))
            assert message() == 'a-later', 'disabled migration started routing'
            api(BASE + '/enabled', {'enabled': True}, 'PATCH')
            wait(ready, 'reenable starts quota polling')
            assert message() == 'c-third', 'reenable did not select next usable account after synthetic 401'
            for account, (priority, _) in accounts.items():
                stored = json.loads((auths / (account + '.json')).read_text())
                assert stored['priority'] == priority and not stored['disabled'], 'credentials changed'
            assert not errors, errors
            log.flush()
            log.seek(0)
            logs = log.read()
            assert 'synthetic-a-later' not in logs, 'credential leaked'
            print('PASS CPA 8.1.0: authenticated status, enabled routing, all-tier candidates, streaming, '
                  '24 concurrent requests, no request-path polling, five-hour/weekly exhaustion, '
                  'lower-tier failover, all-empty no upstream, recovery, rollover, unknown quota fallback, '
                  '500/502/503/429/401 failover, tool payloads, client cancellation, pinning, model eligibility, disable/reenable filtering, competing scheduler precedence, '
                  'disable rollback, priorities preserved, no token logging.')
            print('CPA SHA256:', hashlib.sha256(Path(args.cpa).read_bytes()).hexdigest())
        except BaseException:
            log.flush()
            log.seek(0)
            print(log.read(), file=sys.stderr)
            raise
        finally:
            proc.send_signal(signal.SIGTERM)
            try:
                proc.wait(timeout=15)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()
                raise AssertionError('host shutdown timeout')
            log.close()
            server.shutdown()


if __name__ == '__main__':
    main()
