# Click-to-Focus

Clicking a notification activates your terminal window — no more hunting for the right window.

## Configuration

In `~/.claude/claude-notifications-go/config.json`:

```json
{
  "notifications": {
    "desktop": {
      "clickToFocus": true,
      "terminalBundleId": ""
    }
  }
}
```

| Option | Default | Description |
|--------|---------|-------------|
| `clickToFocus` | `true` | Enable click-to-focus on macOS and Linux |
| `terminalBundleId` | `""` | macOS only: override auto-detected terminal. Use bundle ID like `com.googlecode.iterm2` |

## macOS

Auto-detects your terminal via `TERM_PROGRAM` / `__CFBundleIdentifier`. Uses `terminal-notifier` (auto-installed via `/claude-notifications-go:init`).

| Terminal | Focus method |
|----------|-------------|
| Ghostty | Exact tab focus via Ghostty AppleScript, with AXDocument retry fallback |
| VS Code / Insiders / Cursor | AXTitle via focus-window subcommand |
| iTerm2 | Exact tab/pane targeting via iTerm2 Python API when available, otherwise app-level iTerm activation |
| Warp, kitty, WezTerm, Alacritty, Hyper, Apple Terminal | AXTitle via focus-window subcommand |
| Any other (custom `terminalBundleId`) | AXTitle via focus-window subcommand |

To find your terminal's bundle ID: `osascript -e 'id of app "YourTerminal"'`

### Permissions

All terminals with click-to-focus may require up to two permissions for window-level focus:

- **Accessibility** — to enumerate and raise the correct window via the AX API
- **Screen Recording** — to read window titles across Spaces (macOS 10.15+)

Screen Recording is requested automatically via system prompt on first use.
Accessibility is prompted via a one-time notification with a link to System Settings.

Without these permissions, clicking a notification still activates the terminal app,
but raises whichever window was last active rather than the project-specific one.

## Linux

Uses a background D-Bus daemon. Auto-detects terminal and compositor.

| Terminal | Supported compositors |
|----------|----------------------|
| VS Code | GNOME, KDE, Sway, X11 |
| GNOME Terminal, Konsole, Alacritty, kitty, WezTerm, Tilix, Terminator, XFCE4 Terminal, MATE Terminal | GNOME, KDE, Sway, X11 |
| Any other | Fallback by name |

Focus methods (tried in order):

1. **GNOME**: `activate-window-by-title` extension, Shell Eval, FocusApp (GNOME 45+)
2. **Sway / wlroots**: `wlrctl`
3. **KDE Plasma**: `kdotool`
4. **X11** (XFCE, MATE, Cinnamon, i3, bspwm): `xdotool`

Falls back to standard notifications if no focus tool is available.

### Diagnostics

If Linux click-to-focus focuses the wrong window, run the diagnostic script immediately after reproducing the failed click:

```bash
curl -fsSL https://raw.githubusercontent.com/777genius/claude-notifications-go/main/scripts/linux-focus-debug.sh | bash
```

It writes a report file in the current directory with:

- session type and terminal environment variables
- available focus tools (`xdotool`, `wmctrl`, `remotinator`, etc.)
- current window information and window lists
- installed plugin metadata and recent `notification-debug.log` lines

Review the file before sharing it publicly, because it may include local paths and window titles.

## Multiplexers

Clicking a notification switches to the correct session/pane/tab, on top of raising the window.

| Multiplexer | macOS | Linux |
|-------------|-------|-------|
| tmux | ✅ | — |
| zellij | ✅ (active tab) | ✅ (exact pane) |
| WezTerm | ✅ | ✅ |
| kitty | ✅ | — |

Where a multiplexer is unsupported the window is still raised; only the pane/tab switch is skipped.

On Linux, zellij is targeted by pane rather than by tab name: the hook records `$ZELLIJ_SESSION_NAME`
and `$ZELLIJ_PANE_ID`, and the click runs `zellij -s <session> action focus-pane-id <pane>`. That is
exact where a tab name is not — tab names need not be unique, and the tab that is *focused* when the
notification fires is whichever one you switched to, not the one Claude is running in.

**Requires zellij 0.44.1+**, which added `focus-pane-id`. On older versions the pane switch is
skipped with a logged error and the terminal window is still raised. This is why macOS keeps using
`go-to-tab-name`: switching it would raise that platform's zellij floor to 0.44.1 for everyone.

### iTerm2 + tmux Control Mode (-CC)

When using iTerm2's tmux integration (`tmux -CC`), standard `tmux select-window` doesn't switch iTerm2 tabs. The plugin detects control mode automatically and uses the iTerm2 Python API instead.

**Requirements:**
1. Python 3 installed
2. iTerm2 → Settings → General → Magic → **Enable Python API**
3. iterm2 venv (set up automatically by `bootstrap.sh` / `install.sh`)

**Manual setup** (if automatic setup failed):
```bash
python3 -m venv ~/.claude/claude-notifications-go/iterm2-venv
~/.claude/claude-notifications-go/iterm2-venv/bin/pip install iterm2
```

**Diagnostics:**
```bash
# Show the plugin root path (run inside Claude Code hook context)
echo "$CLAUDE_PLUGIN_ROOT"

# List all iTerm2 tabs with tmux pane mappings
~/.claude/claude-notifications-go/iterm2-venv/bin/python3 \
  "$CLAUDE_PLUGIN_ROOT/scripts/iterm2-select-tab.py" --list
```

If the Python API is not available, the plugin falls back to standard `tmux select-window` (which may not switch iTerm2 tabs in -CC mode). If you just toggled the setting, restart iTerm2 once. For plain iTerm2, the fallback is app-level activation instead of exact tab targeting.

## Windows

Clicking a notification raises the terminal **window** that started the task. Enabled by the same `clickToFocus` flag; no extra configuration.

How it works (no admin rights, no COM server):

1. When the notification fires, the plugin walks up the process tree to the terminal window hosting Claude (Windows Terminal, VS Code, conhost, ConEmu, …) and records its window handle, PID, title and project folder. When one process owns several windows (e.g. Windows Terminal's shared "monarch"), it prefers the window whose title contains the project folder name to tell them apart.
2. The toast is shown via [go-toast](https://git.sr.ht/~jackmordaunt/go-toast) with **protocol activation**, carrying that context in a `claude-notify-focus:` URI. A per-user handler for that scheme is registered under `HKCU\Software\Classes` (idempotent; refreshed if the binary moves), pointing at a second, GUI-subsystem build of the same binary (`claude-notifications-windows-amd64-focus.exe`) instead of the normal console-subsystem one — so the click never flashes a console window. If that sibling isn't present (older install), the handler falls back to the main binary.
3. Clicking the toast launches the URI, which re-runs the focus-handler binary's `focus-windows` subcommand. It re-finds the window (by handle, then PID, then title/folder) and raises it with `ShowWindow` + `SetForegroundWindow`.

### Scope: window-level only

Focus is **window-level**. Windows Terminal runs every tab and split pane inside one top-level window, and Win32 can only raise *windows*, not tabs — there is no public API to focus a specific WT tab by session (it's an open feature request on Windows Terminal). So:

- A single terminal window → raised reliably.
- Multiple **separate** windows → best effort (the foreground / most-recent window is chosen); it may not be the exact one when the session is in a background window.
- **Tabs and split panes** inside a window are not individually targetable.

### Terminal bell (tab indicator)

Separately from click-to-focus, the `terminalBell` option (on by default) now works on Windows. A Claude Code hook is spawned with its *own* hidden console (`CREATE_NO_WINDOW`), so writing to its own `CONOUT$` would ring a private, invisible console. Instead the plugin detaches that console and attaches to an ancestor's (`AttachConsole`) — the Claude session's console, which is the visible pane's ConPTY — then writes a BEL byte there. The BEL reaches the **originating** Windows Terminal pane and WT flags that tab's bell indicator.

Unlike click-to-focus, the bell **is** tab-accurate: because the BEL is written into that specific pane's console, Windows Terminal knows exactly which tab to flag — even with multiple tabs or split panes in one window. It's the one reliable per-tab "this session finished" signal.

It is best-effort and never blocks or fails a notification: if no ancestor console can be reached, the write is a silent, debug-logged no-op — the same way the Unix `/dev/tty` path degrades.

**Visibility (Windows Terminal `bellStyle`).** The background-tab bell glyph appears with WT's default settings — no configuration needed. For a more attention-grabbing cue that also flashes the window and taskbar (useful when the session is in a background window), set in WT settings:

```jsonc
// Settings → Profiles → Defaults → Advanced → Bell notification style
"bellStyle": "all"            // or ["audible", "window", "taskbar"]
```

Set `"terminalBell": false` in the plugin config to disable the bell entirely.
