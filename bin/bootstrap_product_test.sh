#!/bin/bash
# Isolated unit/adapter fixtures: no public network, real host CLIs or Go builds.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
SANDBOX=$(mktemp -d /tmp/bootstrap-products-XXXXXX)
trap 'rm -rf "$SANDBOX"' EXIT
export HOME="$SANDBOX/home space" CODEX_HOME="$SANDBOX/codex space"
export XDG_CONFIG_HOME="$SANDBOX/config" XDG_CACHE_HOME="$SANDBOX/cache" TMPDIR="$SANDBOX/tmp"
export CLAUDE_CONFIG_DIR="$SANDBOX/claude config" CLAUDE_HOME="$SANDBOX/claude home"
export USERPROFILE="$HOME" APPDATA="$SANDBOX/appdata" LOCALAPPDATA="$SANDBOX/localappdata"
export XDG_DATA_HOME="$SANDBOX/data" XDG_STATE_HOME="$SANDBOX/state" XDG_RUNTIME_DIR="$SANDBOX/run"
export TMP="$TMPDIR" TEMP="$TMPDIR"
mkdir -p "$HOME" "$CODEX_HOME" "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$TMPDIR"
sed '/^main "\$@"$/d' "$ROOT/bin/bootstrap.sh" > "$SANDBOX/functions.sh"
source "$SANDBOX/functions.sh"
for product in claude codex both; do
    PRODUCT=""; select_product --product "$product"; [ "$PRODUCT" = "$product" ]
done
for args in '--product invalid' '--product' '--unknown' '--product claude --product codex'; do
    if ( PRODUCT=""; select_product $args ); then echo "accepted $args"; exit 1; fi
done
for tag in v1.42.0 v1.43.2 v2.0.0; do
    BOOTSTRAP_RELEASE_TAG="$tag" resolve_bootstrap_release
    [ "$BOOTSTRAP_TAG" = "$tag" ]
done
for tag in v1.41.0 v0.99.0 v1.42.0-rc1 v01.42.0 v1.042.0 v1.42.00 v99999999999999999999.0.0 main; do
    if BOOTSTRAP_RELEASE_TAG="$tag" resolve_bootstrap_release; then exit 1; fi
done
# Dispatch tests preserve shared bundle state and isolate CN_PRODUCT.
print_header() { :; }; abort_if_wsl_environment() { :; }
check_prerequisites() { :; }; detect_platform() { :; }
resolve_bootstrap_release() { BOOTSTRAP_TAG=v1.43.2; }
install_claude() { [ "$CN_PRODUCT" = claude ]; PLUGIN_ROOT='bundle space'; echo claude >> "$SANDBOX/calls"; }
install_codex() { [ "${CN_PRODUCT:-}" = sentinel ]; echo codex >> "$SANDBOX/calls"; }
export CN_PRODUCT=sentinel
for product in claude codex both; do
    : > "$SANDBOX/calls"
    PRODUCT=""; main --product "$product"
    case "$product" in
        claude) [ "$(cat "$SANDBOX/calls")" = claude ] ;;
        codex) [ "$(cat "$SANDBOX/calls")" = codex ] ;;
        both) [ "$(cat "$SANDBOX/calls")" = "$(printf 'claude\ncodex')" ]; [ "$PLUGIN_ROOT" = 'bundle space' ] ;;
    esac
done
install_codex() { return 1; }
if ( PRODUCT=""; main --product both ); then exit 1; fi
# main installs its own trap; restore test-owned sandbox cleanup.
trap 'rm -rf "$SANDBOX"' EXIT
printf 'bootstrap product unit fixtures passed\n'
# Local HTTP and controlling-PTY integration. Installer/registration are explicit
# fake adapters here; the fetched bootstrap, archive extraction and curl are real.
python3 - "$ROOT" "$SANDBOX" <<'PY'
import functools, http.server, io, os, pathlib, select, shlex, shutil, subprocess, sys, tarfile, threading, time
if os.name != "nt":
    import pty
root, sandbox = map(pathlib.Path, sys.argv[1:])
web = sandbox / 'http'; web.mkdir()
(web / 'bootstrap.sh').write_bytes((root / 'bin/bootstrap.sh').read_bytes())
(web / 'latest').write_text('{"tag_name":"v1.42.0"}')
installer = '''#!/bin/bash
set -eu
[ "$CN_PRODUCT" = codex ]
[ "$1" = --force ]
curl -fsSL "$RELEASE_URL/binary" -o "$INSTALL_TARGET_DIR/claude-notifications"
chmod +x "$INSTALL_TARGET_DIR/claude-notifications"
cp "$INSTALL_TARGET_DIR/claude-notifications" "$INSTALL_TARGET_DIR/claude-notifications-windows-amd64.exe"
'''
binary = '''#!/bin/bash
set -eu
[ "$CN_PRODUCT" = codex ]
if [ "$1" = --version ]; then echo 'claude-notifications v1.42.0'; exit; fi
[ "$1" = setup-codex ]
[ "${FAIL_REGISTER:-0}" = 0 ] || exit 1
[ "${4:-}" != --dry-run ] || exit 0
mkdir -p "$CODEX_HOME"
printf registered > "$CODEX_HOME/fixture-registration"
'''
for tag in ['v1.42.0', 'v1.43.0']:
    with tarfile.open(web / (tag + '.tar.gz'), 'w:gz') as archive:
        for name, data in {'bin/install.sh': installer, '.claude-plugin/plugin.json': '{"version":"'+tag[1:]+'"}'}.items():
            data = data.encode(); entry = tarfile.TarInfo('bundle/' + name); entry.size = len(data); entry.mode = 0o755
            archive.addfile(entry, io.BytesIO(data))
    dest = web / 'download' / tag; dest.mkdir(parents=True)
    (dest / 'binary').write_text(binary.replace('v1.42.0', tag))
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *args): pass
server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(web)))
threading.Thread(target=server.serve_forever, daemon=True).start()
base = 'http://127.0.0.1:' + str(server.server_port)
env = os.environ.copy()
env.update(BOOTSTRAP_LATEST_RELEASE_API_URL=base+'/latest', BOOTSTRAP_SOURCE_BASE_URL=base, BOOTSTRAP_RELEASES_BASE_URL=base)
cli = sandbox / 'clis'; cli.mkdir()
(cli / 'codex').write_text('#!/bin/sh\nexit 99\n'); (cli / 'codex').chmod(0o755)
bash = shutil.which('bash'); assert bash
env['PATH'] = str(cli) + os.pathsep + (os.environ['PATH'] if os.name == 'nt' else '/usr/bin:/bin')
script = str(root / 'bin/bootstrap.sh')
def run(args, expected=0, extra=None):
    result = subprocess.run([bash, script]+args, env=dict(env, **(extra or {})), stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, start_new_session=True, timeout=20)
    assert (result.returncode == 0) == (expected == 0), result.stdout.decode()
    return result.stdout.decode()
assert 'No controlling TTY' in run([], 1)
assert 'claude CLI not found' in run(['--product', 'both'], 1)
(cli / 'codex').rename(cli / 'absent')
assert 'codex CLI not found' in run(['--product', 'codex'], 1)
(cli / 'absent').rename(cli / 'codex')
run(['--product', 'codex']); run(['--product', 'codex'])
run(['--product', 'codex'], extra={'BOOTSTRAP_RELEASE_TAG':'v1.43.0'})
registration = pathlib.Path(env['CODEX_HOME']) / 'fixture-registration'
before = registration.read_bytes()
run(['--product', 'codex'], 1, {'FAIL_REGISTER':'1'})
run(['--product', 'codex'], 1, {'BOOTSTRAP_SOURCE_BASE_URL':base+'/missing'})
assert registration.read_bytes() == before
# Matching Claude manifest with stale runtime must force a fresh staged binary.
live = sandbox / 'live claude'; (live / 'bin').mkdir(parents=True); (live / '.claude-plugin').mkdir()
(live / '.claude-plugin/plugin.json').write_text('{"version":"1.42.0"}')
(live / 'bin/install.sh').write_text(installer)
(live / 'bin/claude-notifications').write_text('stale')
command = 'source '+shlex.quote(str(sandbox/'functions.sh'))+'; PRODUCT=both; PLUGIN_ROOT='+shlex.quote(str(live))+'; BOOTSTRAP_TAG=v1.42.0; install_cleanup_traps; install_codex'
r = subprocess.run([bash,'-c',command],env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=20)
assert r.returncode == 0, r.stdout.decode()
assert (live/'bin/claude-notifications').read_text() == 'stale'
for choice, success in ([('2',True), ('invalid',False)] if os.name != 'nt' else []):
    pid, fd = pty.fork()
    if pid == 0:
        os.execve(bash,[bash,'-c', 'curl -fsSL '+base+'/bootstrap.sh | bash'],env)
    output = b''; sent = False; deadline = time.monotonic()+20
    while time.monotonic() < deadline:
        if select.select([fd],[],[],0.1)[0]:
            try: chunk = os.read(fd,65536)
            except OSError: break
            if not chunk: break
            output += chunk
            if b'Choice:' in output and not sent:
                os.write(fd,(choice+'\n').encode()); sent=True
    else:
        os.kill(pid,9); raise AssertionError('PTY timeout')
    _, status = os.waitpid(pid,0); os.close(fd)
    assert sent and (os.waitstatus_to_exitcode(status)==0)==success, output.decode()
assert not list(pathlib.Path(env['TMPDIR']).glob('bootstrap-codex-*'))
assert not list(pathlib.Path(env['TMPDIR']).glob('bootstrap-release-*'))
server.shutdown(); server.server_close()
print('local HTTP / curl-pipe PTY adapter fixtures passed (fake installer and binary)')
PY
