# Release Checklist

Step-by-step guide for publishing a new version.

## 1. Bump version

Update the version string in **4 files** (5 occurrences total):

| File | Location | Count |
|------|----------|-------|
| `cmd/claude-notifications/main.go` | `const version = "X.Y.Z"` | 1 |
| `.claude-plugin/plugin.json` | `"version": "X.Y.Z"` | 1 |
| `.claude-plugin/marketplace.json` | `"version": "X.Y.Z"` | 2 |
| `.codex-plugin/plugin.json` | `"version": "X.Y.Z"` | 1 |

Quick check — all occurrences should match:

```bash
grep -rn '1\.[0-9]\+\.[0-9]\+' cmd/claude-notifications/main.go .claude-plugin/plugin.json .claude-plugin/marketplace.json .codex-plugin/plugin.json
```

`TestCodexManifestContract` in `cmd/claude-notifications` fails the build when any
of the five occurrences drift apart.

Codex note: `CN_CODEX_MIN_VERSION` in `bin/codex-hook-wrapper.sh` must equal the
version of the FIRST release that ships Codex support and must never change after
that release. If the first Codex release number changes during planning, update
that constant in the same release.

## 2. Update CHANGELOG.md

Add a new section at the top following [Keep a Changelog](https://keepachangelog.com/) format:

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Added
- ...

### Changed
- ...

### Fixed
- ...
```

## 3. Run tests

```bash
make test-race
make lint
```

## 4. Commit, push, and wait for CI

```bash
git add -A
git commit -m "release: vX.Y.Z"
git push origin main
```

**Wait for ALL CI checks to pass before tagging:**

```bash
gh run list --limit 5          # check status
gh run watch <run-id>          # wait for a specific run
```

All three workflows must be green: Ubuntu CI, macOS CI, Windows CI. If any fail — fix, push again, and wait. Do NOT create the tag until CI is green.

## 5. Tag and release

```bash
git tag vX.Y.Z
git push origin --tags
```

The `release.yml` workflow triggers on tag push and builds binaries for all platforms automatically.

Verify at: https://github.com/777genius/claude-notifications-go/releases

## ClaudeNotifier.app (macOS)

ClaudeNotifier.app is **automatically built, signed, and notarized** by the `release.yml`
workflow as a `build-notifier` job. It runs in parallel with Go binary builds and the
resulting `ClaudeNotifier.app.zip` is included in the same GitHub Release.

The CI workflow:
1. Imports the Apple Developer certificate from GitHub Secrets
2. Builds a universal binary (arm64 + x86_64)
3. Signs with **Developer ID Application** + hardened runtime
4. Notarizes via `xcrun notarytool` and staples the ticket
5. Uploads `ClaudeNotifier.app.zip` as a release asset

### Required GitHub Secrets

| Secret | Description |
|--------|-------------|
| `APPLE_CERTIFICATE` | Base64-encoded .p12 export of Developer ID Application cert |
| `APPLE_CERTIFICATE_PASSWORD` | Password for the .p12 file |
| `APPLE_ID` | Apple ID email for notarization |
| `APPLE_PASSWORD` | App-specific password for notarization |
| `APPLE_TEAM_ID` | Apple Developer Team ID |

### Local build (optional)

```bash
make build-notifier                                      # ad-hoc or local cert signing
cd swift-notifier && bash scripts/build-app.sh --ci      # Developer ID + notarization (needs env vars)
```

## 6. Update release description

The auto-generated release description is minimal. Edit it with a human-readable summary:

```bash
gh release edit vX.Y.Z --notes "$(cat <<'NOTES_EOF'
## Bug Fixes

### Title ([#N](link))
Description of what was broken and how it was fixed.

## New Features

### Title ([#N](link))
Description of what was added and why.

---

📦 **[Installation](https://github.com/777genius/claude-notifications-go#installation)** · 🔄 **[Updating](https://github.com/777genius/claude-notifications-go#updating)**

**Full Changelog**: https://github.com/777genius/claude-notifications-go/compare/vPREV...vX.Y.Z
NOTES_EOF
)"
```

## 7. Notify relevant issues/PRs

Comment on fixed issues and merged PRs with a link to the release:

```bash
gh issue comment N --body "Fixed in [vX.Y.Z](https://github.com/777genius/claude-notifications-go/releases/tag/vX.Y.Z)."
gh pr comment N --body "Released in [vX.Y.Z](https://github.com/777genius/claude-notifications-go/releases/tag/vX.Y.Z)."
```

## How auto-update works

Users don't need to manually download binaries after a plugin update:

1. User updates the plugin via `/plugin` menu
2. This updates `plugin.json` with the new version
3. On the next hook invocation, `bin/hook-wrapper.sh` compares the installed binary version with `plugin.json`
4. If versions differ, it runs `install.sh --force` to download the matching binary from GitHub Releases
5. User sees a `[claude-notifications] Updated to vX.Y.Z` message
