#!/usr/bin/env bash
# ABOUTME: Manual Warp click-to-focus check. Run this inside a Warp pane.
# ABOUTME: `open` proves Warp's session URL; `notify` sends a clickable banner.

set -euo pipefail

usage() {
	cat <<'EOF'
Usage: scripts/warp-focus-check.sh <env|open|notify>

Run these from Warp, not from Cursor/iTerm.

  env     Print WARP_FOCUS_URL / session UUID
  open    Focus THIS Warp pane via the session URL
  notify  Send a desktop notification whose click opens THIS pane

Two-tab check:
  1. In tab A:  scripts/warp-focus-check.sh notify
  2. Switch to tab B (or another Warp window)
  3. Click the notification
  4. Tab A (and its agent chat) should come to the front
EOF
}

focus_url="${WARP_FOCUS_URL:-}"
uuid="${WARP_TERMINAL_SESSION_UUID:-}"

require_warp() {
	if [[ -z "$focus_url" ]]; then
		echo "WARP_FOCUS_URL is empty. This shell is not a Warp pane (or Warp is older than v0.2026.05.27)." >&2
		echo "TERM_PROGRAM=${TERM_PROGRAM:-}  __CFBundleIdentifier=${__CFBundleIdentifier:-}" >&2
		exit 1
	fi
	case "$focus_url" in
	warp://session/* | warppreview://session/* | warposs://session/*) ;;
	*)
		echo "WARP_FOCUS_URL is not a session deep link: $focus_url" >&2
		echo "Action/conversation/settings URLs cannot focus an existing pane." >&2
		exit 1
		;;
	esac
}

cmd="${1:-}"
case "$cmd" in
env | "")
	require_warp
	echo "TERM_PROGRAM=${TERM_PROGRAM:-}"
	echo "__CFBundleIdentifier=${__CFBundleIdentifier:-}"
	echo "WARP_TERMINAL_SESSION_UUID=${uuid:-}"
	echo "WARP_FOCUS_URL=$focus_url"
	if [[ "${1:-}" == "" ]]; then
		echo
		usage
	fi
	;;
open)
	require_warp
	echo "Opening $focus_url"
	open "$focus_url"
	;;
notify)
	require_warp
	execute_cmd="open '$focus_url'"
	echo "Sending notification."
	echo "Click action: $execute_cmd"
	echo "Switch to another Warp tab/window, then click the banner."

	if command -v terminal-notifier >/dev/null 2>&1; then
		terminal-notifier \
			-title "Warp focus check" \
			-message "Click to return to this Warp pane/chat" \
			-execute "$execute_cmd"
		exit 0
	fi

	plugin_root="$(cd "$(dirname "$0")/.." && pwd)"
	notifier_bin="$plugin_root/bin/ClaudeNotifier.app/Contents/MacOS/terminal-notifier-modern"
	if [[ -x "$notifier_bin" ]]; then
		"$notifier_bin" \
			-title "Warp focus check" \
			-message "Click to return to this Warp pane/chat" \
			-execute "$execute_cmd" \
			-nosound
		exit 0
	fi

	echo "No terminal-notifier / ClaudeNotifier.app found." >&2
	echo "Falling back to Warp URL only (no banner). This still proves session focus:" >&2
	open "$focus_url"
	echo "If this pane just came to the front, Warp's deep link works. For a clickable banner, brew install terminal-notifier and rerun: $0 notify" >&2
	;;
-h | --help | help)
	usage
	;;
*)
	usage >&2
	exit 2
	;;
esac
