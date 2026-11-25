"""Synthetic integration against the installed CPA 8.1.0 binary.
Runs in a disposable network-isolated container; no personal credentials.
"""
import argparse
import concurrent.futures
import hashlib
import http.server
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
        subprocess.run(['docker', 'run', '--rm', '--network', 'none',
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
    quota_error = False
    messages = []
    quota_calls = []
    errors = []

    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def send(self, code, payload, content_type='application/json'):
            raw = json.dumps(payload).encode() if not isinstance(payload, bytes) else payload
            self.send_response(code)
            self.send_header('Content-Type', content_type)
            self.send_header('Content-Length', str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

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
            if self.path != '/backend-api/codex/responses':
                errors.append('unexpected request path ' + self.path)
                return self.send(404, {})
            account = self.account()
            request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            messages.append(account)
            if account in failures:
                return self.send(429, {'error': {'type': 'usage_limit_reached', 'message': 'fixture exhausted', 'resets_at': int(time.time()) + 60}})
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
  host: 127.0.0.1
  port: 18318
management:
  allow-remote: false
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
      mode: shadow
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

        def ready(mode):
            s = status()
            return s['mode'] == mode and len(s['accounts']) == 4 and all(a.get('quota') for a in s['accounts'].values())

        generation = 0

        def configure(mode='active'):
            nonlocal generation
            generation += 1
            interval = '1m' if generation % 2 else '2m'
            api(BASE + '/config', {'mode': mode, 'poll_interval': interval}, 'PATCH')
            wait(lambda: ready(mode) and status()['poll_interval'] == interval + '0s', 'quota refresh and mode ' + mode)

        def message(stream=False):
            try:
                raw = api('/v1/responses', {'model': MODEL, 'input': 'Synthetic routing check', 'stream': stream}, management=False)
            except urllib.error.HTTPError as exc:
                print('REQUEST FAILURE', exc.code, exc.read().decode(), file=sys.stderr)
                raise
            return raw.decode() if stream else json.loads(raw)['output'][0]['content'][0]['text']

        try:
            wait(lambda: ready('shadow'), 'startup')
            wait(lambda: any(m['id'] == MODEL for m in json.loads(api('/v1/models', management=False))['data']), 'model registration')
            # Management routes must retain host authentication.
            try:
                opener.open('http://127.0.0.1:18318' + BASE + '/status')
                raise AssertionError('unauthenticated status allowed')
            except urllib.error.HTTPError as exc:
                assert exc.code == 401
            assert message() == 'a-later', 'shadow changed built-in fill-first'
            assert status()['last_decision']['auth_id'] == 'b-sooner.json'
            assert len(status()['last_decision']['candidates']) == 4, 'all tiers not offered'
            configure()
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
                    assert list(pool.map(lambda _: message(), range(8))) == ['b-sooner'] * 8
                    soak_requests += 8
                    time.sleep(.2)
            if soak_seconds:
                assert len(quota_calls) > before, 'background polling did not occur during soak'
                print(f'PASS soak: {soak_requests} requests in {soak_seconds} seconds; background polling continued')
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
            api(BASE + '/config', {'mode': 'active', 'poll_interval': '3m'}, 'PATCH')
            wait(lambda: all(s.get('last_error') == 'quota_http_503' for s in status()['accounts'].values()), 'quota error')
            assert message() == 'a-later', 'unknown quota did not delegate to highest tier'
            quota_error = False
            accounts['b-sooner'][1] = 20
            configure()
            api('/v0/management/plugins/test-scheduler/enabled', {'enabled': True}, 'PATCH')
            wait(lambda: message() == 'a-later', 'higher-priority competing scheduler')
            api('/v0/management/plugins/test-scheduler/enabled', {'enabled': False}, 'PATCH')
            wait(lambda: message() == 'b-sooner', 'scheduler competition rollback')
            failures.add('b-sooner')
            assert message() == 'c-third', 'upstream failure did not retry next account'
            api(BASE + '/enabled', {'enabled': False}, 'PATCH')
            wait(lambda: message() == 'a-later', 'disable rollback')
            for account, (priority, _) in accounts.items():
                stored = json.loads((auths / (account + '.json')).read_text())
                assert stored['priority'] == priority and not stored['disabled'], 'credentials changed'
            assert not errors, errors
            log.flush()
            log.seek(0)
            logs = log.read()
            assert 'synthetic-a-later' not in logs, 'credential leaked'
            print('PASS CPA 8.1.0: authenticated status, shadow, active, all-tier candidates, streaming, '
                  '24 concurrent requests, no request-path polling, five-hour/weekly exhaustion, '
                  'lower-tier failover, all-empty no upstream, recovery, rollover, unknown quota fallback, '
                  '429 retry, pinning, model eligibility, disable/reenable filtering, competing scheduler precedence, '
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
