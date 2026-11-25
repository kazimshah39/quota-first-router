"""Explicitly authorized, bounded shadow -> active -> disable live canary.
Never prints secrets or consumes the shared usage queue. Correlates requests with
read-only Oh My CPA usage records by a unique User-Agent.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import shutil
import sqlite3
import time
import urllib.error
import urllib.request
import uuid

import yaml

PLUGIN = 'quota-first-router'
BASE = '/v0/management/plugins/' + PLUGIN


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--execute-authorized-canary', action='store_true', required=True)
    p.add_argument('--cpa-config', required=True)
    p.add_argument('--omc-env', required=True)
    p.add_argument('--omc-db', required=True)
    p.add_argument('--model', default='gpt-6-luna')
    args = p.parse_args()
    project = Path(__file__).resolve().parents[1]
    config_path = Path(args.cpa_config)
    original = yaml.safe_load(config_path.read_text())
    env = {}
    for line in Path(args.omc_env).read_text().splitlines():
        if '=' in line and not line.startswith('#'):
            k, v = line.split('=', 1)
            env[k] = v.strip().strip('\"\'')
    base = env['OMCPA_CPA_BASE_URL']
    management_key = env['OMCPA_CPA_MANAGEMENT_KEY']
    keys = original.get('access', {}).get('api-keys') or original.get('api-keys')
    client_key = keys[0] if isinstance(keys[0], str) else keys[0]['key']
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    run_id = 'qfr-canary-' + uuid.uuid4().hex
    backup = project / '.local' / run_id
    backup.mkdir(parents=True, mode=0o700)
    os.chmod(backup.parent, 0o700)
    shutil.copyfile(config_path, backup / 'config.yaml')
    os.chmod(backup / 'config.yaml', 0o600)
    report = {'run_id': run_id, 'started_at': dt.datetime.now(dt.timezone.utc).isoformat(),
              'backup_sha256': hashlib.sha256(config_path.read_bytes()).hexdigest(), 'model': args.model}

    def api(path, body=None, method=None, client=False, phase=''):
        req = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(),
            headers={'Authorization': 'Bearer ' + (client_key if client else management_key),
                     'Content-Type': 'application/json', 'User-Agent': run_id + '-' + phase},
            method=method or ('GET' if body is None else 'POST'))
        with opener.open(req, timeout=120 if client else 15) as response:
            return json.load(response)

    def wait(check, label, timeout=100):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            try:
                result = check()
                if result:
                    return result
            except (urllib.error.URLError, KeyError, ValueError):
                pass
            time.sleep(1)
        raise AssertionError('Timed out: ' + label)

    def routing_fields():
        return {p.name: {k: d.get(k, False if k == 'disabled' else 0) for k in ['priority', 'disabled']}
                for p in Path(original['oauth']['auth-dir']).expanduser().glob('codex-*.json')
                for d in [json.loads(p.read_text())]}

    before = routing_fields()
    status = lambda: api(BASE + '/status')
    initial_plugins = api('/v0/management/plugins')
    report['existing_plugins'] = [x['id'] for x in initial_plugins['plugins']]
    report['requests'] = []
    touched = False

    def inference(phase):
        result = api('/v1/responses', {'model': args.model,
                    'input': 'Reply with exactly OK.', 'stream': False,
                    'reasoning': {'effort': 'low'}, 'max_output_tokens': 32}, client=True, phase=phase)
        assert result.get('status') == 'completed', 'Incomplete model response'
        # Wait for OMC's existing collector; never pop the shared CPA usage queue.
        def observation():
            db = sqlite3.connect('file:' + args.omc_db + '?mode=ro', uri=True, timeout=5)
            try:
                rows = db.execute('SELECT auth_index,failed,request_id,model FROM usage_events WHERE user_agent=? ORDER BY id DESC LIMIT 1',
                                  (run_id + '-' + phase,)).fetchall()
                return rows[0] if rows else None
            finally:
                db.close()
        row = wait(observation, 'usage attribution ' + phase, 100)
        assert not row[1], 'Usage event reports failed request'
        return {'phase': phase, 'actual_auth_index': row[0], 'request_id': row[2], 'model': row[3], 'completed': True}

    try:
        # Validate an already installed artifact: copying over a mapped native
        # library is unsafe even when the plugin has been disabled.
        installed = next(p for p in initial_plugins['plugins'] if p['id'] == PLUGIN)
        destination = Path(installed['path'])
        assert destination.is_file(), 'Install a verified release before the canary'
        report['artifact_sha256'] = hashlib.sha256(destination.read_bytes()).hexdigest()
        settings = {'enabled': True, 'priority': 100, 'mode': 'shadow', 'poll_interval': '1m', 'max_age': '5m', 'request_timeout': '10s'}
        global_proxy = original.get('requests', {}).get('proxy-url')
        if global_proxy:
            settings['proxy_url'] = global_proxy
        touched = True
        api(BASE + '/config', settings, 'PATCH')
        def ready():
            s = status()
            return s if s.get('mode') == 'shadow' and s.get('accounts') and all(a.get('quota') for a in s['accounts'].values()) else None
        s = wait(ready, 'shadow quota discovery', 150)
        report['quota_observations'] = s['accounts']
        shadow = inference('shadow')
        s = status()
        shadow['proposal'] = s['last_decision']
        assert s['picks'] > 0 and not s['routed'], 'Shadow changed routing or scheduler not invoked'
        report['requests'].append(shadow)
        assert shadow['proposal'].get('auth_id'), 'Shadow has no trustworthy proposed account'
        api(BASE + '/config', {'mode': 'active'}, 'PATCH')
        wait(lambda: status()['mode'] == 'active' and all(a.get('quota') for a in status()['accounts'].values()), 'active ready')
        active = inference('active')
        s = status()
        active['proposal'] = s['last_decision']
        chosen = active['proposal'].get('auth_id')
        assert chosen and active['actual_auth_index'] == s['accounts'][chosen]['auth_index'], 'Actual account differs from plugin proposal'
        assert s['routed'] > 0, 'Active plugin did not route'
        # Independent ordering check against the offered candidate observations.
        candidates = active['proposal']['candidates']
        eligible = [c for c in candidates if c['reason'] == 'ready']
        top = max(c['priority'] for c in eligible)
        ranked = sorted((c for c in eligible if c['priority'] == top),
                        key=lambda c: (c.get('five_hour_reset') is None, c.get('five_hour_reset') or '', c['auth_id']))
        assert chosen == ranked[0]['auth_id'], 'Independent reset order check failed'
        report['requests'].append(active)
    finally:
        if touched:
            api(BASE + '/enabled', {'enabled': False}, 'PATCH')
            def disabled():
                items = api('/v0/management/plugins')['plugins']
                return any(p['id'] == PLUGIN and not p['effective_enabled'] for p in items)
            wait(disabled, 'disabled rollback')
            report['rollback_disabled'] = True
            try:
                report['requests'].append(inference('rollback'))
            except Exception as exc:
                report['rollback_request_error'] = type(exc).__name__
            report['priorities_and_enabled_preserved'] = routing_fields() == before
        report['finished_at'] = dt.datetime.now(dt.timezone.utc).isoformat()
        (backup / 'report.json').write_text(json.dumps(report, indent=2))
        os.chmod(backup / 'report.json', 0o600)
    assert report['priorities_and_enabled_preserved'], 'Routing fields changed'
    assert 'rollback_request_error' not in report, 'Rollback request failed'
    print('PASS shadow/active/rollback live canary; report:', backup / 'report.json')
    print('Plugin remains installed but disabled; account tiers unchanged.')


if __name__ == '__main__':
    main()
