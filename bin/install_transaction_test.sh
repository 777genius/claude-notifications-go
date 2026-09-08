#!/bin/bash
# Focused runtime promotion regressions. No network, builds, or real profiles.
set -euo pipefail
root=$(cd "$(dirname "$0")" && pwd)
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT
export HOME="$sandbox/home" XDG_DATA_HOME="$sandbox/home/data" TMPDIR="$sandbox"
mkdir -p "$HOME"
sed '/^main "\$@"$/d' "$root/install.sh" > "$sandbox/functions.sh"
for scenario in offline fresh_offline download checksum missing_checksum executable interrupt desktop fresh_desktop success fresh_success optional_interrupt; do
    case_dir="$sandbox/$scenario"
    mkdir -p "$case_dir"
    (
        export INSTALL_TARGET_DIR="$case_dir"
        source "$sandbox/functions.sh"
        detect_platform() {
            PLATFORM=darwin ARCH=amd64
            BINARY_NAME=claude-notifications-darwin-amd64
            BINARY_PATH="$SCRIPT_DIR/$BINARY_NAME"
            CHECKSUMS_PATH="$SCRIPT_DIR/.checksums.txt"
            FOCUS_HANDLER_NAME='' FOCUS_HANDLER_PATH=''
        }
        detect_platform
        FORCE_UPDATE=true
        [ "$scenario" != desktop ] || FORCE_UPDATE=false
        printf '#!/bin/bash\necho old-version\n' > "$BINARY_PATH"
        chmod +x "$BINARY_PATH"
        mkdir -p "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS"
        printf '#!/bin/bash\necho old-notifier\n' > "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
        chmod +x "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
        ln -s "$BINARY_NAME" "$SCRIPT_DIR/claude-notifications"
        printf utility > "$SCRIPT_DIR/sound-preview"
        if [[ "$scenario" == fresh_* ]]; then
            rm "$BINARY_PATH" "$SCRIPT_DIR/claude-notifications"
            rm -rf "$SCRIPT_DIR/ClaudeNotifier.app"
        elif [ "$scenario" = desktop ]; then
            rm -rf "$SCRIPT_DIR/ClaudeNotifier.app"
        fi
        abort_if_wsl_environment() { :; }
        check_required_tools() { :; }
        configure_curl_options() { :; }
        check_github_availability() { OFFLINE_MODE=false; [[ "$scenario" != *offline ]] || OFFLINE_MODE=true; }
        pin_release_urls() { :; }
        eval "$(declare -f download_binary | sed '1s/download_binary/real_download_binary/')"
        MAX_RETRIES=1 RETRY_DELAY=0
        download_checksums() {
            printf '#!/bin/bash\necho claude-notifications-new-version\nexit 0\n' > "$SCRIPT_DIR/payload"
            [ "$scenario" != executable ] || printf '#!/bin/bash\nexit 1\n' > "$SCRIPT_DIR/payload"
            head -c 1000000 /dev/zero >> "$SCRIPT_DIR/payload"
            [ "$scenario" != missing_checksum ] || return 1
            local sum
            if command -v shasum >/dev/null 2>&1; then
                sum=$(shasum -a 256 "$SCRIPT_DIR/payload")
            else
                sum=$(sha256sum "$SCRIPT_DIR/payload")
            fi
            printf '%s  %s\n' "${sum%% *}" "$BINARY_NAME" > "$CHECKSUMS_PATH"
            [ "$scenario" != checksum ] || printf 'bad  %s\n' "$BINARY_NAME" > "$CHECKSUMS_PATH"
        }
        download_binary() {
            if [ "$scenario" = download ]; then
                curl() { printf 404; return 22; }
                real_download_binary
                return $?
            fi
            cp "$SCRIPT_DIR/payload" "$BINARY_PATH"
            if [ "$scenario" = interrupt ]; then kill -TERM "$BASHPID"; fi
        }
        download_terminal_notifier_modern() {
            [ "$scenario" = fresh_success ] || return 1
            mkdir -p "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS"
            printf '#!/bin/bash\necho new-notifier\n' > "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
            chmod +x "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
        }
        download_terminal_notifier() { return 1; }
        download_utilities() {
            # Optional downloads cannot start until the live runtime is complete.
            desktop_runtime_usable
            "$BINARY_PATH" --version | grep -q new-version
            [ "$scenario" != optional_interrupt ] || kill -TERM "$BASHPID"
        }
        create_claude_notifications_app() { :; }
        setup_iterm2_venv() { :; }
        main
    ) > "$case_dir/output" 2>&1 && status=0 || status=$?
    binary="$case_dir/claude-notifications-darwin-amd64"
    case "$scenario" in
        success|fresh_success|optional_interrupt)
            "$binary" | grep -q new-version
            [ -x "$case_dir/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern" ] ;;
        fresh_desktop|fresh_offline) [ ! -e "$binary" ]; [ "$status" != 0 ] ;;
        *) "$binary" | grep -q old-version
           "$case_dir/claude-notifications" | grep -q old-version ;;
    esac
    [ "$(cat "$case_dir/sound-preview")" = utility ]
    if [[ "$scenario" != fresh_* && "$scenario" != desktop ]]; then
        [ "$("$case_dir/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern")" = old-notifier ]
    fi
    [ -z "$(find "$case_dir" -name '.install-stage.*' -o -name '.install.lock')" ]
    echo "PASS: $scenario (status $status)"
done
