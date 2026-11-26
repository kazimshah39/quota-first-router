"""Install the public release through an isolated CPA's real plugin-store API.
Only public network reads and synthetic management/client keys; no personal auths.
"""
import argparse
import hashlib
import json
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

ID = 'quota-first-router'
SOURCE = 'https://raw.githubusercontent.com/kazimshah39/quota-first-router/main/registry.json'


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--cpa', required=True)
    p.add_argument('--output-library', required=True)
    p.add_argument('--expected-version', default='0.1.1')
    args = p.parse_args()
    binary = str(Path(args.cpa).resolve())
    output = Path(args.output_library).resolve()
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with tempfile.TemporaryDirectory(prefix='qfr-store-') as tmp:
        root = Path(tmp)
        auths = root / 'auths'
        auths.mkdir()
        with socket.socket() as s:
            s.bind(('127.0.0.1', 0))
            port = s.getsockname()[1]
        config = root / 'config.yaml'
        config.write_text(f'''config-version: 8
server:
  host: 127.0.0.1
  port: {port}
management:
  allow-remote: false
  secret-key: synthetic-store-management
  disable-control-panel: true
oauth:
  auth-dir: {auths}
access:
  api-keys: [synthetic-store-client]
plugins:
  enabled: true
  dir: {root / 'plugins'}
  store-sources:
    - {SOURCE}
''')
        log = (root / 'cpa.log').open('w+')
        proc = subprocess.Popen([binary, '--config', str(config), '--local-model'], cwd=root, stdout=log, stderr=subprocess.STDOUT)

        def api(path, body=None, method=None):
            req = urllib.request.Request(f'http://127.0.0.1:{port}' + path,
                data=None if body is None else json.dumps(body).encode(),
                headers={'Authorization': 'Bearer synthetic-store-management', 'Content-Type': 'application/json'},
                method=method or ('GET' if body is None else 'POST'))
            try:
                with opener.open(req, timeout=120) as response:
                    return json.load(response)
            except urllib.error.HTTPError as exc:
                raise AssertionError(f'{path}: {exc.code}: {exc.read().decode()}') from exc

        def wait(check, name, seconds=40):
            end = time.monotonic() + seconds
            last = None
            while time.monotonic() < end:
                try:
                    last = check()
                    if last:
                        return last
                except (urllib.error.URLError, AssertionError, ValueError) as exc:
                    last = repr(exc)
                if proc.poll() is not None:
                    raise AssertionError('CPA exited during ' + name)
                time.sleep(.2)
            raise AssertionError(f'{name}: {last}')

        base = '/v0/management/plugins/' + ID
        try:
            wait(lambda: api('/v0/management/plugins'), 'startup')
            catalog = api('/v0/management/plugin-store')
            entry = next(x for x in catalog['plugins'] if x['id'] == ID and x['source_url'] == SOURCE)
            assert entry['repository'] == 'https://github.com/kazimshah39/quota-first-router'
            assert not entry['installed']
            query = urllib.parse.urlencode({'source': entry['source_id']})
            install_path = '/v0/management/plugin-store/' + ID + '/install?' + query
            result = api(install_path, {})
            assert result['version'] == args.expected_version, result
            installed = Path(result['path'])
            assert installed.is_file() and installed.name.endswith('.so')
            api(base + '/config', {'enabled': True, 'priority': 100}, 'PATCH')
            legacy = tuple(map(int, args.expected_version.split('.'))) < (0, 2, 0)
            status = wait(lambda: api(base + '/status'), 'plugin registration')
            assert status['mode'] == ('shadow' if legacy else 'active') and status['version'] == args.expected_version
            assert status['poll_interval'] == '1m0s' and status['max_age'] == '5m0s', status
            assert status['accounts'] == {}, 'Unexpected credentials in isolated instance'
            if legacy:
                api(base + '/config', {'mode': 'active'}, 'PATCH')
                wait(lambda: api(base + '/status')['mode'] == 'active', 'legacy active mode')
            api(base + '/enabled', {'enabled': False}, 'PATCH')
            wait(lambda: any(x['id'] == ID and not x['effective_enabled'] for x in api('/v0/management/plugins')['plugins']), 'disable')
            # Current CPA safely refuses replacement while a native artifact is still loaded.
            # Restart only this disposable test process to prove reinstall, not production.
            proc.send_signal(signal.SIGTERM)
            proc.wait(timeout=15)
            proc = subprocess.Popen([binary, '--config', str(config), '--local-model'], cwd=root, stdout=log, stderr=subprocess.STDOUT)
            wait(lambda: api('/v0/management/plugins'), 'isolated restart')
            second = api(install_path, {})
            assert second['version'] == args.expected_version
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_bytes(Path(second['path']).read_bytes())
            print(f'PASS public registry discovery, real CPA checksum-verified install, {args.expected_version} defaults '
                  '(1m/5m), disable, isolated restart and same-version reinstall.')
            print('Installed artifact SHA256:', hashlib.sha256(output.read_bytes()).hexdigest())
        except BaseException:
            log.flush()
            log.seek(0)
            print(log.read())
            raise
        finally:
            if proc.poll() is None:
                proc.send_signal(signal.SIGTERM)
                try:
                    proc.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()
            log.close()


if __name__ == '__main__':
    main()
