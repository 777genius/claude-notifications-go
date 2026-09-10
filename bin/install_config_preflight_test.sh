#!/bin/bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/test-env.sh"
test_env_enter "$0" "$@"
# Disposable adapter tests. The helper models the CLI protocol; native Store
# identity behavior is covered by internal/config/preflight_test.go.
set -eo pipefail
root=$(cd "$(dirname "$0")" && pwd)
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT
test_env_setup "$sandbox"
python3 - "$root" "$sandbox" <<'PY'
import json, os, pathlib, shlex, shutil, subprocess, sys
root, box = map(pathlib.Path, sys.argv[1:])
functions = box/'functions.sh'
functions.write_text((root/'install.sh').read_text().replace('main "$@"', ''))
helper = box/'helper'
helper.write_text("""#!/usr/bin/env python3
import json,os,sys
if sys.argv[1:]==['--version']:
    print('claude-notifications 0.0.1'); sys.exit(0)
assert sys.argv[1:]==['config','preflight-update','--stdin','--json']
r=json.load(sys.stdin)
with open(os.environ['TRACE'],'a') as f: f.write(json.dumps(r)+'\\n')
e=os.environ.get('AGENT_NOTIFICATIONS_CONFIG','')
status='safe'; code=''
if e and not os.path.isabs(e): status='invalid-config'; code='ConfigOverrideInvalid'
elif e:
    e=os.path.realpath(e)
    for p in r['refreshDirs']:
        assert os.path.isabs(p)
        p=os.path.realpath(p)
        if e==p or e.startswith(p+os.sep) or (os.path.isfile(e) and os.path.isfile(p) and os.path.samefile(e,p)):
            status='unsafe-target'; code='ConfigUnsafeTarget'; break
print(json.dumps(dict(status=status,diagnostics=[dict(code=code,path='SECRET-CANARY')])))
sys.exit(0 if status=='safe' else 1)
""")
helper.chmod(0o755)
q=lambda p: shlex.quote(str(p))
# Fail safely even if a regression removes a guard.
isolation="""
curl() { echo 'unexpected network request' >&2; return 99; }
wget() { echo 'unexpected network request' >&2; return 99; }
python3() {
    if [ "$1" = -I ]; then command python3 "$@"; return; fi
    echo 'unexpected Python environment creation' >&2
    return 99
}
"""

def run(name, body, e=None, ok=True):
    case=box/name; case.mkdir()
    env=dict(os.environ, INSTALL_TARGET_DIR=str(case), TRACE=str(case/'trace'))
    if e is not None: env['AGENT_NOTIFICATIONS_CONFIG']=str(e)
    script='source '+q(functions)+'\ndetect_platform\nINSTALL_CONFIG_HELPER='+q(helper)+'\n'+isolation+body
    r=subprocess.run(['bash','-c',script],env=env,capture_output=True,timeout=30)
    assert (r.returncode==0)==ok,(name,r.returncode,r.stderr.decode())
    assert b'SECRET-CANARY' not in r.stdout+r.stderr
    print('PASS:',name)
    return case,r

venv=pathlib.Path(os.environ['HOME'])/'.claude/claude-notifications-go/iterm2-venv'
venv.mkdir(parents=True)
config=venv/'config.json'; config.write_text('SECRET-CANARY')
iterm="""
uname() { echo Darwin; }
tmux() { :; }
TERM_PROGRAM=iTerm.app
setup_iterm2_venv
"""
run('iterm-overlap',iterm,config,False)
assert config.read_text()=='SECRET-CANARY'
alias=box/'venv-alias'; alias.symlink_to(venv,target_is_directory=True)
run('iterm-alias',iterm,alias/'config.json',False)
hard=box/'config-hardlink'; os.link(config,hard)
run('hardlink-target','guard_install_paths '+q(hard),config,False)
for name,body,target in [
    ('modern-app','download_terminal_notifier_modern', 'ClaudeNotifier.app/config.json'),
    ('legacy-app','download_terminal_notifier', 'terminal-notifier.app/config.json'),
    ('icon-app','create_claude_notifications_app', 'ClaudeNotifications.app/Contents/Info.plist'),
]:
    (box/'claude_icon.png').write_bytes(b'fixture')
    run(name,body,box/name/target,False)
# Extension CLI and downloads are explicit stubs with no desktop effects.
extension=pathlib.Path(os.environ['XDG_DATA_HOME'])/'gnome-shell/extensions/activate-window-by-title@lucaswerkmeister.de'
run('gnome-extension',"""
gnome-shell() { echo 'GNOME Shell 46.0'; }
gnome-extensions() { [ "$1" != install ] || touch "$SCRIPT_DIR/unexpected"; }
curl() { printf '{"download_url":"/fixture.zip"}'; }
install_gnome_activate_window_extension
""",extension/'config.json',False)
assert not (box/'gnome-extension/unexpected').exists()
# No-op exits must not require a protocol or contact a release server.
run('iterm-noop','CN_PRODUCT=codex; INSTALL_CONFIG_HELPER=/missing; '+iterm,config)
outside=box/'outside.json'; outside.write_text('{}')
for name,code in [
 ('runtime','guard_install_paths "$SCRIPT_DIR"; touch "$SCRIPT_DIR/mutated"'),
 ('utility','printf old > "$SCRIPT_DIR/sound-preview"; create_utility_symlink sound-preview x "$SCRIPT_DIR/sound-preview"'),
 ('windows-hooks','PLATFORM=windows; cygpath() { printf "%s\\n" "$2"; }; windows_hooks_path() { echo "$SCRIPT_DIR/hooks.json"; }; windows_native_hooks_json() { echo "{}"; }; windows_native_hooks_are_exec_form() { :; }; touch "$BINARY_PATH" "$SCRIPT_DIR/hooks.json"; configure_windows_native_hooks'),
 ('linux-desktop','PLATFORM=linux; install_linux_notification_desktop_entry'),
]:
    target=box/(name+'-reject')/('sound-preview' if name=='utility' else 'hooks.json' if name=='windows-hooks' else 'config.json')
    if name=='linux-desktop': target=pathlib.Path(os.environ['XDG_DATA_HOME'])/'applications/claude-notifications.desktop'
    run(name+'-reject',code,target,False)
    run(name+'-safe',code,outside)
run('relative','guard_install_paths "$SCRIPT_DIR"', 'relative.json',False)
run('absent','INSTALL_CONFIG_HELPER=/missing; guard_install_paths "$SCRIPT_DIR"')
run('missing-explicit-safe','guard_install_paths "$SCRIPT_DIR"',box/'missing.json')
# The old helper cannot authorize anything. Only verified staging may replace it.
stage_body="""
INSTALL_CONFIG_HELPER=/missing
pin_release_urls() { :; }
download_and_verify_binary() {
    cp @HELPER@ "$BINARY_PATH"
    printf checksum > "$CHECKSUMS_PATH"
}
verify_executable() { :; }
guard_install_paths "$SCRIPT_DIR"
[ -n "$INSTALL_CONFIG_STAGE" ]
[ -f "$INSTALL_STAGED_ASSETS/checksums.txt" ]
""".replace('@HELPER@',q(helper))
run('old-staged',stage_body,outside)
run('old-staged-reject',stage_body,box/'old-staged-reject/config.json',False)
run('old-unavailable','INSTALL_CONFIG_HELPER=/missing; pin_release_urls() { :; }; download_and_verify_binary() { return 1; }; guard_install_paths "$SCRIPT_DIR"',outside,False)
case,r=run('old-known-offline',"""
INSTALL_CONFIG_HELPER=/missing
OFFLINE_MODE=true
download_and_verify_binary() { touch "$SCRIPT_DIR/unexpected-download"; }
guard_install_paths "$SCRIPT_DIR"
""",outside,False)
assert not (case/'unexpected-download').exists()
# Force/offline keeps the old runtime without fetching a config helper.
run('offline-noop',"""
INSTALL_CONFIG_HELPER=/missing
FORCE_UPDATE=true
touch "$BINARY_PATH"
abort_if_wsl_environment() { :; }
check_required_tools() { :; }
check_github_availability() { OFFLINE_MODE=true; }
main
""",outside)
# Exercise the actual wrapper's lazy installer suppression and retained binary.
case=box/'lazy'; case.mkdir()
shutil.copy(root/'hook-wrapper.sh',case/'hook-wrapper.sh')
binary=case/'claude-notifications'
binary.write_text('#!/bin/sh\nif [ "$1" = version ]; then echo "claude-notifications 0.0.1"; fi\n')
binary.chmod(0o755)
plugin=box/'.claude-plugin'; plugin.mkdir()
(plugin/'plugin.json').write_text('{"version":"9.9.9"}')
installer=case/'install.sh'
installer.write_text('#!/bin/bash\nsource '+q(functions)+'\ndetect_platform\nINSTALL_CONFIG_HELPER='+q(helper)+'\n'+isolation+iterm)
installer.chmod(0o755)
env=dict(os.environ,AGENT_NOTIFICATIONS_CONFIG=str(config),TRACE=str(case/'trace'))
before=binary.read_bytes()
r=subprocess.run(['sh',str(case/'hook-wrapper.sh'),'Stop'],env=env,input=b'{}',capture_output=True,timeout=30)
assert r.returncode==0 and r.stdout==b'' and r.stderr==b'',r
assert binary.read_bytes()==before and config.read_text()=='SECRET-CANARY'
assert (case/'trace').exists()
print('PASS: actual hook-wrapper lazy rejection is silent and preserves runtime')
# Real transaction must use the new staged protocol and preserve live bytes.
for overlap in (True,False):
    name='promotion-reject' if overlap else 'promotion-safe'
    target=box/name/'claude-notifications-linux-amd64' if overlap else outside
    case,r=run(name,"""
detect_platform() {
    PLATFORM=linux ARCH=amd64 BINARY_NAME=claude-notifications-linux-amd64
    BINARY_PATH="$SCRIPT_DIR/$BINARY_NAME"
    CHECKSUMS_PATH="$SCRIPT_DIR/.checksums.txt"
}
detect_platform
printf old-runtime > "$BINARY_PATH"
INSTALL_CONFIG_HELPER=/old-without-protocol
download_and_verify_binary() { cp @HELPER@ "$BINARY_PATH"; }
verify_executable() { :; }
install_linux_notification_desktop_entry() { :; }
stage_and_promote_runtime
""".replace('@HELPER@',q(helper)),target,not overlap)
    assert (case/'claude-notifications-linux-amd64').read_bytes()==(b'old-runtime' if overlap else helper.read_bytes())
# A config rejection from the optional downloader must not become success.
run('optional-rejection',"""
download_utility() ( guard_install_paths "$SCRIPT_DIR"; )
download_utilities
""",box/'optional-rejection/config.json',False)
# Drive main with network/platform effects replaced, retaining real resource code.
run('direct-main',"""
abort_if_wsl_environment() { :; }
check_required_tools() { :; }
check_existing() { return 0; }
create_symlink() { :; }
configure_windows_native_hooks() { :; }
download_utilities() { :; }
create_claude_notifications_app() { :; }
detect_platform() { PLATFORM=darwin; ARCH=amd64; BINARY_NAME=fixture; BINARY_PATH="$SCRIPT_DIR/fixture"; }
uname() { echo Darwin; }
tmux() { :; }
TERM_PROGRAM=iTerm.app
main
""",config,False)
assert config.read_text()=='SECRET-CANARY'
# Real venv setup/failed-pip cleanup with only Python creation and pip stubbed.
run('pip-failure-safe',"""
uname() { echo Darwin; }
tmux() { :; }
TERM_PROGRAM=iTerm.app
python3() {
    if [ "$1" = -I ]; then command python3 "$@"; return; fi
    mkdir -p "$3/bin"
    printf '#!/bin/sh\\nexit 1\\n' > "$3/bin/pip"
    chmod +x "$3/bin/pip"
}
setup_iterm2_venv
""",outside)
assert not venv.exists()
print('PASS: non-overlap pip failure follows existing cleanup behavior')
# Change a directory alias during pip, then require a fresh preflight before rm.
late=box/'late-alias'; late.symlink_to(box/'initially-missing',target_is_directory=True)
body="""
uname() { echo Darwin; }
tmux() { :; }
TERM_PROGRAM=iTerm.app
python3() {
    if [ "$1" = -I ]; then command python3 "$@"; return; fi
    mkdir -p "$3/bin"
    printf '#!/bin/sh\\nrm @ALIAS@\\nln -s @VENV@ @ALIAS@\\nprintf protected > @VENV@/config.json\\nexit 1\\n' > "$3/bin/pip"
    chmod +x "$3/bin/pip"
}
setup_iterm2_venv
""".replace('@ALIAS@',str(late)).replace('@VENV@',str(venv))
run('pip-late-alias',body,late/'config.json',False)
assert (venv/'config.json').read_text()=='protected'
print('PASS: post-pip overlap blocks recursive cleanup')


PY
