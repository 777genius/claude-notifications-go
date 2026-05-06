# Troubleshooting

Common installation and runtime issues.

## macOS: VS Code click-to-focus focuses the wrong window

### Symptom

Clicking a notification activates VS Code but raises the wrong window (or the last-active window) instead of the project-specific one.

### Why it happens

VS Code window focus requires **Screen Recording** permission (macOS 10.15+) to read window titles across all Spaces. Without it, the binary falls back to plain app activation.

### Fix

On first use the binary requests Screen Recording access automatically — a macOS dialog will appear. If you dismissed it:

1. Open **System Settings → Privacy & Security → Screen Recording**
2. Enable access for the `claude-notifications` binary (or the terminal running Claude Code)
3. Click the notification again

Once granted, the correct VS Code window will be raised even if it is on a different Space.

## Ubuntu 24.04: `EXDEV: cross-device link not permitted` during `/plugin install`

### Symptom

Plugin installation fails with an error similar to:

```
EXDEV: cross-device link not permitted, rename '.../.claude/plugins/cache/...' -> '/tmp/claude-plugin-temp-...'
```

### Why it happens

Claude Code's plugin installer attempts to move a plugin directory from `~/.claude/...` into `/tmp/...` using `rename()`.
On many Linux systems (including Ubuntu 24.04), `/tmp` is mounted as `tmpfs` (a different filesystem/device), so cross-device `rename()` fails with `EXDEV`.

### Fix (recommended)

Set a temporary directory on the same filesystem as your `~/.claude` (usually under `$HOME`) and start Claude Code from that environment:

```bash
mkdir -p "$HOME/.claude/tmp"
TMPDIR="$HOME/.claude/tmp" claude
```

Then retry:

```text
/plugin install claude-notifications-go@claude-notifications-go
```

### Diagnostics (optional)

```bash
df -T "$HOME" /tmp
mount | grep -E ' on /tmp | on /home '
```

If `/tmp` is `tmpfs` (or otherwise on a different device) and `$HOME` is on `ext4/btrfs/...`, the error is expected without the `TMPDIR` workaround.

## Linux: click-to-focus opens the wrong window

### Symptom

Clicking a notification focuses the wrong terminal window, a stale Terminator window, or does nothing.

### Quick diagnostics

Reproduce the failed click first, then run:

```bash
curl -fsSL https://raw.githubusercontent.com/777genius/claude-notifications-go/main/scripts/linux-focus-debug.sh | bash
```

The script generates a report file in the current directory with:

- `XDG_SESSION_TYPE`, `DISPLAY`, `WAYLAND_DISPLAY`, `TERM_PROGRAM`, `TERMINATOR_UUID`, `WINDOWID`
- installed plugin version/path and marketplace source
- available focus tools like `xdotool`, `wmctrl`, and `remotinator`
- active-window data, `wmctrl` window lists, `xdotool` searches, and recent plugin log lines

Review the file before posting it publicly, because it may include local file paths and window titles.

### Why this helps

Linux click-to-focus behavior depends on the session type, terminal, window manager, and available focus tools. The diagnostic script captures the exact environment needed to explain why the plugin focused the wrong window or could not focus anything at all.

## Windows: install issues related to `%TEMP%` / `%TMP%` location

If your temp directory is on a different drive than your user profile (or where Claude stores plugin cache), you may see similar cross-device move issues.

### Fix

Make sure `%TEMP%` and `%TMP%` point to a directory on the same drive as `%USERPROFILE%` (or where Claude stores its plugin directories), then restart your terminal/app.

## Windows: hooks do not fire or notifications are silent

### Symptom

The plugin is installed and the Windows executable exists, but Claude Code does not show notifications for `Stop`, `ExitPlanMode`, `AskUserQuestion`, or `permission_prompt`. Claude Code debug logs may show hook command failures, or the hook may fail silently.

### Why it happens

The bundled plugin hook configuration uses `bin/hook-wrapper.sh`. On some Windows 11 Claude Code environments, bash/shebang resolution, `${CLAUDE_PLUGIN_ROOT}` expansion, or Unix-only paths like `/dev/tty` are not reliable enough for command hooks.

Claude Code supports PowerShell command hooks on Windows via `"shell": "powershell"`, so the safer Windows workaround is to call the native `.exe` directly with an absolute path.

### Fix

Run the bootstrap installer or `/claude-notifications-go:init` again, then restart Claude Code. On Windows, the installer rewrites the plugin hook file to use PowerShell hooks with an absolute path to the native `.exe`, avoiding the Git Bash/shebang path.

If you need to inspect or apply the configuration manually, generate a PowerShell hook configuration from the installed executable. Replace `<arch>` with `amd64` or `arm64`:

```powershell
.\bin\claude-notifications-windows-<arch>.exe windows-hooks
```

If you downloaded the executable to a different location, pass it explicitly:

```powershell
.\bin\claude-notifications-windows-<arch>.exe windows-hooks --exe "C:\absolute\path\to\claude-notifications-windows-<arch>.exe"
```

To apply it manually, replace the installed plugin's `hooks/hooks.json` with the generated JSON and restart Claude Code. This command only prints JSON - it does not modify files.

### Manual fallback

If you cannot run `windows-hooks`, replace the installed plugin's `hooks/hooks.json` with this block and replace `<absolute-path-to-plugin>` with the actual plugin install directory:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "ExitPlanMode|AskUserQuestion",
        "hooks": [
          {
            "type": "command",
            "command": "$OutputEncoding = [System.Text.UTF8Encoding]::new($false); $input | & \"<absolute-path-to-plugin>\\bin\\claude-notifications-windows-<arch>.exe\" handle-hook PreToolUse",
            "timeout": 30,
            "shell": "powershell"
          }
        ]
      }
    ],
    "Notification": [
      {
        "matcher": "permission_prompt",
        "hooks": [
          {
            "type": "command",
            "command": "$OutputEncoding = [System.Text.UTF8Encoding]::new($false); $input | & \"<absolute-path-to-plugin>\\bin\\claude-notifications-windows-<arch>.exe\" handle-hook Notification",
            "timeout": 30,
            "shell": "powershell"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$OutputEncoding = [System.Text.UTF8Encoding]::new($false); $input | & \"<absolute-path-to-plugin>\\bin\\claude-notifications-windows-<arch>.exe\" handle-hook Stop",
            "timeout": 30,
            "shell": "powershell"
          }
        ]
      }
    ],
    "SubagentStop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$OutputEncoding = [System.Text.UTF8Encoding]::new($false); $input | & \"<absolute-path-to-plugin>\\bin\\claude-notifications-windows-<arch>.exe\" handle-hook SubagentStop",
            "timeout": 30,
            "shell": "powershell"
          }
        ]
      }
    ],
    "TeammateIdle": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$OutputEncoding = [System.Text.UTF8Encoding]::new($false); $input | & \"<absolute-path-to-plugin>\\bin\\claude-notifications-windows-<arch>.exe\" handle-hook TeammateIdle",
            "timeout": 30,
            "shell": "powershell"
          }
        ]
      }
    ]
  }
}
```

This workaround is based on confirmed Windows 11 behavior from [issue #73](https://github.com/777genius/claude-notifications-go/issues/73#issuecomment-4364271319).

### Note about beeep logs

If the log contains `beeep.Notify failed on windows: doc.LoadXml(tmpl)` but the toast still appears, treat it as a harmless Windows notifier false positive. Investigate it only if no popup appears.

## Windows / Git Bash: binary download fails from GitHub Releases

### Symptom

Bootstrap or `/claude-notifications-go:init` installs the plugin itself, but downloading `claude-notifications-windows-<arch>.exe` fails with an empty or generic network error.

### Why it happens

`raw.githubusercontent.com` and `github.com` may still work, but release assets are served from GitHub's release CDN. On corporate Windows machines, Git Bash `curl` often fails there because of:

- Proxy authentication or missing proxy environment variables
- TLS inspection with an untrusted corporate root CA
- Schannel certificate revocation checks blocking the request

### What to check

1. If your company requires a proxy, make sure the terminal running Claude Code or bootstrap has `HTTPS_PROXY`, `HTTP_PROXY`, or `ALL_PROXY` configured.
2. If your network inspects TLS traffic, ensure Git Bash `curl` trusts the corporate CA certificate.
3. Retry from another network or from WSL to confirm whether the issue is network-specific.
4. As a fallback, open the latest release page, download the matching `claude-notifications-windows-amd64.exe` or `claude-notifications-windows-arm64.exe`, place it into the plugin `bin` directory, and then re-run `/claude-notifications-go:init`.
