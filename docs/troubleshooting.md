# Troubleshooting

Common installation and runtime issues.

## macOS: ClaudeNotifier.app not in the Accessibility list

### Symptom

After clicking the "Accessibility Access Needed" notification you land in **System Settings → Privacy & Security → Accessibility** but `ClaudeNotifier.app` is not in the list.

### Fix

Clicking the notification opens Settings and copies the path to `ClaudeNotifier.app` to your clipboard. In Finder press **⌘⇧G**, paste, then drag `ClaudeNotifier.app` into the Accessibility list and enable the toggle.

If you dismissed the notification before seeing this, trigger it again by deleting the marker file and clicking any notification:

```bash
rm ~/.claude/claude-notifications-go/.accessibility-prompted
```

## macOS: VS Code click-to-focus focuses the wrong window

### Symptom

Clicking a notification activates VS Code but raises the wrong window (or the last-active window) instead of the project-specific one.

### Why it happens

VS Code window focus requires **Screen Recording** permission (macOS 10.15+) to read window titles across all Spaces. Without it, the binary falls back to plain app activation.

### Fix

On first use you will receive a notification. Clicking it opens **System Settings → Privacy & Security → Screen Recording** and copies the path to `ClaudeNotifier.app` to your clipboard. In Finder press **⌘⇧G**, paste, then drag `ClaudeNotifier.app` into the list and enable the toggle.

If you dismissed the notification before seeing this, trigger it again by deleting the marker file and clicking any notification:

```bash
rm ~/.claude/claude-notifications-go/.screen-recording-prompted
```

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

## Windows: install issues related to `%TEMP%` / `%TMP%` location

If your temp directory is on a different drive than your user profile (or where Claude stores plugin cache), you may see similar cross-device move issues.

### Fix

Make sure `%TEMP%` and `%TMP%` point to a directory on the same drive as `%USERPROFILE%` (or where Claude stores its plugin directories), then restart your terminal/app.
