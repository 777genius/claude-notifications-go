#!/bin/bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/test-env.sh"
test_env_enter "$0" "$@"
set -eo pipefail
root=$(cd "$(dirname "$0")" && pwd)
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT
sed '/^main "\$@"$/d' "$root/install.sh" > "$sandbox/functions.sh"
source "$sandbox/functions.sh"
# Only launcher creation is exercised. All mutations stay in a disposable bundle.
SCRIPT_DIR="$sandbox/bundle"
mkdir -p "$SCRIPT_DIR"
guard_install_paths() { :; }
PLATFORM=linux
BINARY_NAME=claude-notifications-linux-amd64
BINARY_PATH="$SCRIPT_DIR/$BINARY_NAME"
printf '#!/bin/sh\nprintf "fixture-binary\\n"\n' > "$BINARY_PATH"
chmod +x "$BINARY_PATH"
create_symlink
[ "$(readlink "$SCRIPT_DIR/agent-notifications")" = "$BINARY_NAME" ]
[ "$(readlink "$SCRIPT_DIR/claude-notifications")" = "$BINARY_NAME" ]
[ "$("$SCRIPT_DIR/agent-notifications")" = fixture-binary ]
[ "$("$SCRIPT_DIR/claude-notifications")" = fixture-binary ]
PLATFORM=windows
BINARY_NAME=claude-notifications-windows-amd64.exe
create_symlink
for name in agent-notifications claude-notifications; do
 grep -F '"%SCRIPT_DIR%claude-notifications-windows-amd64.exe" %*' "$SCRIPT_DIR/$name.bat"
 grep -F "set AGENT_NOTIFICATIONS_LAUNCHER=$name" "$SCRIPT_DIR/$name.bat"
done
echo 'PASS: dual launchers preserve the platform binary and Windows identity'
