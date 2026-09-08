#!/bin/bash
# Focused runtime promotion regressions. No network, builds, or real profiles.
# Match installer nounset behavior: Bash 3.2 treats empty arrays as unset.
set -eo pipefail
root=$(cd "$(dirname "$0")" && pwd)
sandbox=$(mktemp -d)
trap 'result=$?; if [ "$result" != 0 ]; then echo "FAILED: ${scenario:-utility} (status $result)" >&2; [ ! -f "${case_dir:-}/output" ] || tail -n 25 "$case_dir/output" >&2; fi; rm -rf "$sandbox"' EXIT
export HOME="$sandbox/home" XDG_DATA_HOME="$sandbox/home/data" TMPDIR="$sandbox"
mkdir -p "$HOME"
sed '/^main "\$@"$/d' "$root/install.sh" > "$sandbox/functions.sh"
for scenario in offline fresh_offline download checksum missing_checksum executable interrupt desktop fresh_desktop success fresh_success optional_interrupt legacy_fallback retained_legacy failed_fallback; do
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
        case "$scenario" in
            legacy_fallback|retained_legacy|failed_fallback)
                # Git Bash recognizes shebang files as executable even after chmod -x.
                printf broken > "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
                chmod -x "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
                if [ "$scenario" = retained_legacy ]; then
                    mkdir -p "$SCRIPT_DIR/terminal-notifier.app/Contents/MacOS"
                    printf '#!/bin/bash\necho legacy-notifier\n' > "$SCRIPT_DIR/terminal-notifier.app/Contents/MacOS/terminal-notifier"
                    chmod +x "$SCRIPT_DIR/terminal-notifier.app/Contents/MacOS/terminal-notifier"
                fi ;;
        esac
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
            if [ "$scenario" = interrupt ]; then kill -TERM "$(sh -c 'echo "$PPID"')"; fi
        }
        download_terminal_notifier_modern() {
            [ "$scenario" = fresh_success ] || return 1
            mkdir -p "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS"
            printf '#!/bin/bash\necho new-notifier\n' > "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
            chmod +x "$SCRIPT_DIR/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
        }
        download_terminal_notifier() {
            [ "$scenario" = legacy_fallback ] || return 1
            mkdir -p "$SCRIPT_DIR/terminal-notifier.app/Contents/MacOS"
            printf '#!/bin/bash\necho legacy-notifier\n' > "$SCRIPT_DIR/terminal-notifier.app/Contents/MacOS/terminal-notifier"
            chmod +x "$SCRIPT_DIR/terminal-notifier.app/Contents/MacOS/terminal-notifier"
        }
        download_utilities() {
            # Optional downloads cannot start until the live runtime is complete.
            desktop_runtime_usable
            "$BINARY_PATH" --version | grep -q new-version
            [ "$scenario" != optional_interrupt ] || kill -TERM "$(sh -c 'echo "$PPID"')"
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
        legacy_fallback|retained_legacy)
            [ "$status" = 0 ]
            "$binary" | grep -q new-version
            # Match runtime discovery: prefer the modern path if it exists.
            selected="$case_dir/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
            [ -e "$selected" ] || selected="$case_dir/terminal-notifier.app/Contents/MacOS/terminal-notifier"
            [ "$("$selected")" = legacy-notifier ]
            [ ! -e "$case_dir/ClaudeNotifier.app" ] ;;
        fresh_desktop|fresh_offline) [ ! -e "$binary" ]; [ "$status" != 0 ] ;;
        *) "$binary" | grep -q old-version
           "$case_dir/claude-notifications" | grep -q old-version ;;
    esac
    [ "$(cat "$case_dir/sound-preview")" = utility ]
    if [[ "$scenario" != fresh_* && "$scenario" != desktop && "$scenario" != *legacy* && "$scenario" != *fallback ]]; then
        [ "$("$case_dir/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern")" = old-notifier ]
    fi
    if [ "$scenario" = failed_fallback ]; then
        [ "$status" != 0 ]
        [ -f "$case_dir/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern" ]
    fi
    [ -z "$(find "$case_dir" -name '.install-stage.*' -o -name '.install.lock')" ]
    echo "PASS: $scenario (status $status)"
done

# Exercise the real downloader with a curl stub; never touch a live utility.
(
    export INSTALL_TARGET_DIR="$sandbox/utilities"
    mkdir -p "$INSTALL_TARGET_DIR"
    source "$sandbox/functions.sh"
    utility="$INSTALL_TARGET_DIR/sound-preview-test"
    FORCE_UPDATE=true
    transfer=interrupt
    curl() {
        local output=''
        while [ "$#" -gt 0 ]; do
            if [ "$1" = -o ]; then output="$2"; shift; fi
            shift
        done
        [ "$output" != "$utility" ] || return 99
        printf partial > "$output"
        case "$transfer" in
            interrupt)
                # A child shell reports its actual parent, including on Bash 3.2.
                kill -TERM "$(sh -c 'echo "$PPID"')"
                return 1 ;;
            fail) return 22 ;;
            short) return 0 ;;
        esac
        printf '#!/bin/sh\necho new-utility\n' > "$output"
        head -c 100001 /dev/zero >> "$output"
    }
    download_utility test "$utility" && status=0 || status=$?
    [ "$status" = 143 ]
    [ ! -e "$utility" ]
    [ -z "$(find "$INSTALL_TARGET_DIR" -name '*.download.*')" ]
    transfer=success
    download_utility test "$utility"
    [ "$("$utility")" = new-utility ]
    cp "$utility" "$INSTALL_TARGET_DIR/expected"
    for transfer in interrupt fail short; do
        download_utility test "$utility" && status=0 || status=$?
        [ "$status" != 0 ]
        cmp "$utility" "$INSTALL_TARGET_DIR/expected"
        [ -z "$(find "$INSTALL_TARGET_DIR" -name '*.download.*')" ]
    done
    FORCE_UPDATE=false
    transfer=fail
    download_utility test "$utility" # usable existing file skips download
    for invalid in partial nonexecutable; do
        if [ "$invalid" = partial ]; then printf partial > "$utility";
        else head -c 100001 /dev/zero > "$utility"; chmod -x "$utility"; fi
        transfer=success
        download_utility test "$utility"
        utility_usable "$utility"
    done
    FORCE_UPDATE=true
    printf '#!/bin/sh\necho old-utility\n' > "$utility"
    head -c 100001 /dev/zero >> "$utility"
    chmod +x "$utility"
    download_utility test "$utility"
    [ "$("$utility")" = new-utility ]
    # Optional phase must not even request the required Windows focus asset.
    FOCUS_HANDLER_NAME=focus.exe FOCUS_HANDLER_PATH="$INSTALL_TARGET_DIR/focus.exe"
    SOUND_PREVIEW_NAME=sound LIST_DEVICES_NAME=devices LIST_SOUNDS_NAME=sounds
    SOUND_PREVIEW_PATH="$utility" LIST_DEVICES_PATH="$utility" LIST_SOUNDS_PATH="$utility"
    download_utility() { [ "$1" != focus.exe ] || exit 99; }
    create_utility_symlink() { :; }
    download_utilities
    echo 'PASS: real optional downloader interruption, preservation, repair, force, focus exclusion'
)
