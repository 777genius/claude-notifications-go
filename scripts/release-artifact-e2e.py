#!/usr/bin/env python3
"""Exercise an already-built release executable in disposable profiles.

No agent CLI, external webhook, desktop session, or user configuration is used.
This proves CLI/config/webhook delivery; actual agent and GUI tests are separate.
"""
import argparse
import hashlib
import http.server
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--version', required=True)
    args = parser.parse_args()
    binary = args.binary.resolve()
    deliveries = []

    class Sink(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_POST(self):
            deliveries.append((self.path, self.rfile.read(int(self.headers['Content-Length'])).decode()))
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'ok')

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Sink)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    results = []
    try:
        with tempfile.TemporaryDirectory(prefix='notification-release-e2e-') as scratch:
            root = Path(scratch).resolve()
            for case in ('fresh', 'legacy', 'explicit'):
                home = root / case
                home.mkdir()
                env = {key: os.environ[key] for key in
                       ('PATH', 'SystemRoot', 'SYSTEMROOT', 'WINDIR', 'COMSPEC', 'PATHEXT')
                       if key in os.environ}
                env.update(HOME=str(home), USERPROFILE=str(home), GIT_CONFIG_NOSYSTEM='1')
                for key in ('CODEX_HOME', 'CLAUDE_HOME', 'CLAUDE_CONFIG_DIR',
                            'APPDATA', 'LOCALAPPDATA', 'XDG_CONFIG_HOME', 'XDG_CACHE_HOME',
                            'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_RUNTIME_DIR', 'TMPDIR', 'TMP', 'TEMP'):
                    folder = home / key
                    folder.mkdir(mode=0o700)
                    env[key] = str(folder)
                bundle = home / 'bundle'
                bundle.mkdir()
                env['PLUGIN_ROOT'] = str(bundle)
                env['CLAUDE_PLUGIN_ROOT'] = str(bundle)

                def run(*command, data=None, success=True):
                    result = subprocess.run([str(binary), *command], cwd=home, env=env,
                                            input=data, text=True, capture_output=True, timeout=30)
                    assert (result.returncode == 0) == success, (case, command, result.returncode, result.stderr)
                    return result

                assert run('version').stdout.strip() == 'claude-notifications ' + args.version
                legacy = home / '.claude' / 'claude-notifications-go' / 'config.json'
                if case == 'explicit':
                    env['AGENT_NOTIFICATIONS_CONFIG'] = str(home / 'custom' / 'config.json')
                if case == 'legacy':
                    legacy.parent.mkdir(parents=True, mode=0o700)
                    legacy.write_text('{}')
                    legacy.chmod(0o600)
                selected = Path(run('config', 'path').stdout.strip())
                if case == 'legacy':
                    assert selected == legacy
                elif case == 'explicit':
                    assert selected == Path(env['AGENT_NOTIFICATIONS_CONFIG'])
                else:
                    assert selected != legacy
                run('config', 'init')
                assert selected.exists()
                if case != 'legacy':
                    assert not legacy.exists()
                document = {
                    'notifications': {
                        'desktop': {'enabled': False, 'sound': False, 'terminalBell': False},
                        'webhook': {'enabled': True, 'preset': 'slack',
                                    'url': f'http://127.0.0.1:{server.server_port}/shared'},
                    },
                    'future': {'preserved': 9007199254740993},
                }
                if case != 'legacy':
                    document.update(schemaVersion=2, agents={
                        product: {'notifications': {'webhook': {
                            'url': f'http://127.0.0.1:{server.server_port}/{product}'}}}
                        for product in ('claude', 'codex')
                    })
                selected.write_text(json.dumps(document))
                before = selected.read_bytes()
                run('config', 'init')
                run('config', 'inspect', '--json')
                assert selected.read_bytes() == before
                for product in ('claude', 'codex'):
                    marker = f'release-{case}-{product}'
                    if product == 'codex':
                        event = 'Stop'
                        payload = {'hook_event_name': event, 'session_id': marker, 'turn_id': marker,
                                   'cwd': str(home), 'last_assistant_message': marker, 'stop_hook_active': False}
                    else:
                        event = 'Notification'
                        payload = {'hook_event_name': event, 'session_id': marker,
                                   'notification_type': 'permission_prompt', 'message': marker}
                    count = len(deliveries)
                    run('handle-hook', event, '--product', product, data=json.dumps(payload))
                    assert len(deliveries) == count + 1, (case, product, deliveries)
                    expected = '/shared' if case == 'legacy' else '/' + product
                    assert deliveries[-1][0] == expected and marker in deliveries[-1][1], deliveries[-1]
                    if product == 'codex':
                        assert marker in json.loads(deliveries[-1][1])['attachments'][0]['text']
                payload.update(session_id=case + '-literal-help', turn_id='literal-help',
                               last_assistant_message='--help')
                count = len(deliveries)
                run('handle-hook', 'Stop', '--product', 'codex', data=json.dumps(payload))
                assert len(deliveries) == count + 1
                assert '--help' in json.loads(deliveries[-1][1])['attachments'][0]['text']
                assert selected.read_bytes() == before
                selected.write_text('{invalid-config')
                count = len(deliveries)
                run('config', 'inspect', '--json', success=False)
                result = run('handle-hook', 'Stop', '--product', 'codex', data=json.dumps(payload))
                assert result.stdout == result.stderr == ''
                assert len(deliveries) == count
                assert selected.read_text() == '{invalid-config'
                results.append(case)
        print(json.dumps({'result': 'PASS', 'version': args.version,
                          'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
                          'profiles': results, 'webhook_deliveries': len(deliveries)}))
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == '__main__':
    main()
