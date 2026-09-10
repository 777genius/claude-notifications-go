#!/bin/bash
# Real protocol fixture, including Git Bash cygpath -> native Python -> Go.
TEST_ENV_HANDOFF_GOMODCACHE=1
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/test-env.sh"
test_env_enter "$0" "$@"
set -eo pipefail
root=$(cd "$(dirname "$0")" && pwd)
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT
test_env_setup "$sandbox"
(cd "$root/.." && go build -o "$sandbox/helper.exe" ./cmd/claude-notifications)
sed '/^main "\$@"$/d' "$root/install.sh" > "$sandbox/functions.sh"
export INSTALL_TARGET_DIR="$sandbox/live"
source "$sandbox/functions.sh"
# Sourcing the installer installs its own trap.
trap 'cleanup_install_config; rm -rf "$sandbox"' EXIT
detect_platform
INSTALL_CONFIG_HELPER="$sandbox/helper.exe"
printf '{}' > "$sandbox/config.json"
export AGENT_NOTIFICATIONS_CONFIG="$sandbox/config.json"
if [ "$PLATFORM" = windows ]; then
    AGENT_NOTIFICATIONS_CONFIG=$(cygpath -aw "$AGENT_NOTIFICATIONS_CONFIG")
fi
guard_install_paths "$SCRIPT_DIR"
echo 'PASS: real native helper accepts independent config'
if (guard_install_paths "$sandbox/config.json"); then
    echo 'FAIL: accepted selected file mutation' >&2; exit 1
else
    [ "$?" = 78 ]
fi
[ "$(cat "$sandbox/config.json")" = '{}' ]
echo 'PASS: real native helper rejects selected file mutation'
export AGENT_NOTIFICATIONS_CONFIG="$SCRIPT_DIR/missing.json"
if [ "$PLATFORM" = windows ]; then
    AGENT_NOTIFICATIONS_CONFIG=$(cygpath -aw "$AGENT_NOTIFICATIONS_CONFIG")
fi
if (guard_install_paths "$SCRIPT_DIR"); then
    echo 'FAIL: accepted missing selected child' >&2; exit 1
else
    [ "$?" = 78 ]
fi
[ ! -e "$SCRIPT_DIR/missing.json" ]
echo 'PASS: real native helper rejects missing selected child'
# Reverse child aliases are safe to unlink only after private construction.
# POSIX symlink creation is optional on Windows; protocol tests above are native.
if [ "$PLATFORM" != windows ]; then
    export AGENT_NOTIFICATIONS_CONFIG="$sandbox/config.json"
    venv="$HOME/.claude/claude-notifications-go/iterm2-venv"
    uname() { echo Darwin; }
    tmux() { :; }
    TERM_PROGRAM=iTerm.app
    python3() {
        if [ "$1" = -I ]; then command python3 "$@"; return; fi
        mkdir -p "$3/bin"
        printf created > "$3/pyvenv.cfg"
        printf '#!/bin/sh\nexit 0\n' > "$3/bin/pip"
        chmod +x "$3/bin/pip"
    }
    for child in pyvenv.cfg bin; do
        rm -rf "$venv"
        mkdir -p "$venv"
        if [ "$child" = bin ]; then
            mkdir -p "$sandbox/protected-bin"
            printf '{}' > "$sandbox/protected-bin/python3"
            AGENT_NOTIFICATIONS_CONFIG="$sandbox/protected-bin/python3"
            ln -s "$sandbox/protected-bin" "$venv/bin"
        else
            AGENT_NOTIFICATIONS_CONFIG="$sandbox/config.json"
            ln -s "$AGENT_NOTIFICATIONS_CONFIG" "$venv/pyvenv.cfg"
        fi
        setup_iterm2_venv
        [ "$(cat "$AGENT_NOTIFICATIONS_CONFIG")" = '{}' ]
        [ ! -L "$venv/$child" ]
        echo "PASS: real helper and private venv preserve reverse $child alias"
    done
    # Build a real offline venv and a local stand-in for the iterm2 package.
    # This verifies that moved Python and rewritten entry points still run.
    python3() {
        if [ "$1" = -I ]; then command python3 "$@"; return; fi
        command python3 -m venv --without-pip "$3" || return 1
        local site
        site=$("$3/bin/python3" -c 'import sysconfig; print(sysconfig.get_path("purelib"))')
        printf '# local test module\n' > "$site/iterm2.py"
        printf '#!%s/bin/python3\nimport sys\nsys.exit(0)\n' "$3" > "$3/bin/pip"
        chmod +x "$3/bin/pip"
    }
    rm -rf "$venv"
    setup_iterm2_venv
    "$venv/bin/python3" -c 'import iterm2'
    "$venv/bin/pip" --version
    grep -F "$venv" "$venv/bin/activate" >/dev/null
    echo 'PASS: real offline venv Python, entry point, and activation survive promotion'

fi
