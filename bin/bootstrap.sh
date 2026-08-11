#!/bin/bash
# bootstrap.sh - One-command install/update for claude-notifications plugin
# Usage: curl -fsSL https://raw.githubusercontent.com/777genius/claude-notifications-go/main/bin/bootstrap.sh | bash

set -euo pipefail

# Colors and formatting
BOLD='\033[1m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Constants
REPO="777genius/claude-notifications-go"
MARKETPLACE_SOURCE="${BOOTSTRAP_MARKETPLACE_SOURCE:-$REPO}"
MARKETPLACE_NAME="claude-notifications-go"
PLUGIN_NAME="claude-notifications-go"
PLUGIN_KEY="${PLUGIN_NAME}@${MARKETPLACE_NAME}"
INSTALL_SCRIPT_URL="${INSTALL_SCRIPT_URL:-https://raw.githubusercontent.com/${REPO}/main/bin/install.sh}"

# Paths — CLAUDE_CONFIG_DIR is the official Claude Code env var;
# CLAUDE_HOME is a legacy fallback; default to ~/.claude
CLAUDE_HOME="${CLAUDE_CONFIG_DIR:-${CLAUDE_HOME:-$HOME/.claude}}"
if [ -z "$CLAUDE_HOME" ]; then
    CLAUDE_HOME="$HOME/.claude"
fi
INSTALLED_JSON="${CLAUDE_HOME}/plugins/installed_plugins.json"
CACHE_DIR="${CLAUDE_HOME}/plugins/cache/${MARKETPLACE_NAME}"
MARKETPLACE_DIR="${CLAUDE_HOME}/plugins/marketplaces/${MARKETPLACE_NAME}"
MARKETPLACE_PLUGIN_JSON="${MARKETPLACE_DIR}/.claude-plugin/plugin.json"

# State
PLUGIN_ROOT=""
_BOOTSTRAP_TMP=""  # temp file path for trap (set -u safe)

# ──────────────────────────────────────────────

print_header() {
    echo ""
    echo -e "${BOLD}============================================${NC}"
    echo -e "${BOLD} Claude Notifications — Bootstrap Installer${NC}"
    echo -e "${BOLD}============================================${NC}"
    echo ""
}

# ──────────────────────────────────────────────

is_wsl_environment() {
    case "${CLAUDE_NOTIFICATIONS_ALLOW_WSL:-}" in
        1|true|TRUE|yes|YES) return 1 ;;
    esac

    local os
    os="$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]' || true)"
    [ "$os" = "linux" ] || return 1

    [ -n "${WSL_DISTRO_NAME:-}" ] && return 0
    [ -n "${WSL_INTEROP:-}" ] && return 0

    if [ -r /proc/version ] && grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
        return 0
    fi

    return 1
}

abort_if_wsl_environment() {
    is_wsl_environment || return 0

    echo -e "${RED}✗ WSL environment detected${NC}" >&2
    echo "" >&2
    echo -e "${YELLOW}This command is running inside WSL, so it would install Linux binaries under /home instead of Windows binaries.${NC}" >&2
    echo -e "${YELLOW}If you started this from PowerShell or Windows Terminal, your bash command is probably WSL bash, not Git Bash.${NC}" >&2
    echo "" >&2
    echo -e "${YELLOW}For Windows Claude Code, open Git Bash from the Start menu and run:${NC}" >&2
    echo -e "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/bin/bootstrap.sh | bash" >&2
    echo "" >&2
    echo -e "${YELLOW}If you intentionally use Claude Code inside WSL, rerun with:${NC}" >&2
    echo -e "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/bin/bootstrap.sh | env CLAUDE_NOTIFICATIONS_ALLOW_WSL=1 bash" >&2
    echo "" >&2
    exit 1
}

# ──────────────────────────────────────────────

check_prerequisites() {
    if ! command -v claude &>/dev/null; then
        echo -e "${RED}✗ claude CLI not found in PATH${NC}" >&2
        echo "" >&2
        echo -e "${YELLOW}Install Claude Code first:${NC}" >&2
        echo -e "  npm install -g @anthropic-ai/claude-code" >&2
        echo "" >&2
        exit 1
    fi
    echo -e "${GREEN}✓${NC} claude CLI found"

    if ! command -v curl &>/dev/null && ! command -v wget &>/dev/null; then
        echo -e "${RED}✗ curl or wget required${NC}" >&2
        exit 1
    fi
}

# ──────────────────────────────────────────────

detect_platform() {
    local os
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"

    case "$os" in
        darwin)  PLATFORM="macOS" ;;
        linux)   PLATFORM="Linux" ;;
        mingw*|msys*|cygwin*) PLATFORM="Windows (Git Bash)" ;;
        *)       PLATFORM="$os" ;;
    esac

    echo -e "${BLUE}Platform:${NC} ${PLATFORM}"
}

# ──────────────────────────────────────────────

is_iterm2_detected() {
    [ "$(uname -s)" = "Darwin" ] || return 1

    [ "${TERM_PROGRAM:-}" = "iTerm.app" ] && return 0
    [ "${__CFBundleIdentifier:-}" = "com.googlecode.iterm2" ] && return 0
    [ -d "/Applications/iTerm.app" ] && return 0
    [ -d "$HOME/Applications/iTerm.app" ] && return 0

    return 1
}

# ──────────────────────────────────────────────

print_iterm2_python_api_notice() {
    is_iterm2_detected || return 0

    echo ""
    echo -e "${YELLOW}────────────────────────────────────────────${NC}"
    echo -e "${YELLOW}⚠${NC} ${BOLD}iTerm2 detected${NC}"
    echo -e "  To open the ${BOLD}exact iTerm2 tab / split pane${NC} on notification click:"
    echo -e "  1. Open ${BOLD}iTerm2${NC}"
    echo -e "  2. Go to ${BOLD}Settings → General → Magic${NC}"
    echo -e "  3. Enable ${BOLD}Python API${NC}"
    echo -e "  4. If you just toggled it, ${BOLD}restart iTerm2 once${NC}"
    echo -e "${YELLOW}────────────────────────────────────────────${NC}"
}

# ──────────────────────────────────────────────

setup_marketplace() {
    echo ""
    echo -e "${BLUE}📦 Setting up marketplace...${NC}"

    local output
    # Try adding marketplace — if already added, update instead
    # </dev/null prevents stdin conflicts when running via `curl | bash`
    if output=$(claude plugin marketplace add "$MARKETPLACE_SOURCE" </dev/null 2>&1); then
        echo -e "${GREEN}✓${NC} Marketplace added"
    else
        if echo "$output" | grep -qi "already"; then
            echo -e "${BLUE}  Marketplace already added, updating...${NC}"
            if claude plugin marketplace update "$MARKETPLACE_NAME" </dev/null 2>&1; then
                echo -e "${GREEN}✓${NC} Marketplace updated"
            else
                # Update may fail if already up-to-date — that's OK
                echo -e "${GREEN}✓${NC} Marketplace is up to date"
            fi
        else
            echo -e "${YELLOW}⚠ Marketplace add output: ${output}${NC}"
            echo -e "${YELLOW}  Continuing anyway...${NC}"
        fi
    fi
}

# ──────────────────────────────────────────────

get_manifest_version() {
    local manifest_path="$1"
    [ -f "$manifest_path" ] || return 0
    grep -Eo '"version"[[:space:]]*:[[:space:]]*"[0-9]+\.[0-9]+\.[0-9]+"' "$manifest_path" 2>/dev/null \
        | head -n 1 \
        | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+' || true
}

get_installed_plugin_version() {
    [ -f "$INSTALLED_JSON" ] || return 0

    if command -v jq &>/dev/null; then
        PLUGIN_KEY="$PLUGIN_KEY" jq -r '
          (.plugins[env.PLUGIN_KEY] // []) as $entries
          | ($entries | map(select((.installPath // "") != ""))) as $with_paths
          | (if ($with_paths | length) == 0 then $entries else $with_paths end)
          | sort_by((.version // "0.0.0") | split(".") | map(tonumber? // 0))
          | if length == 0 then {} else .[-1] end
          | .version // empty
        ' "$INSTALLED_JSON" 2>/dev/null || true
        return 0
    fi

    if command -v python3 &>/dev/null; then
        python3 - "$INSTALLED_JSON" "$PLUGIN_KEY" <<'PYEOF' 2>/dev/null || true
import json, sys
def ver_tuple(value):
    try:
        parts = str(value or "0.0.0").split(".")
        parts = (parts + ["0", "0", "0"])[:3]
        return tuple(int(p) for p in parts)
    except Exception:
        return (0, 0, 0)
try:
    with open(sys.argv[1]) as f:
        d = json.load(f)
    entries = d.get('plugins', {}).get(sys.argv[2], [])
    if entries:
        with_paths = [e for e in entries if isinstance(e, dict) and e.get('installPath')]
        candidates = with_paths or [e for e in entries if isinstance(e, dict)]
        if candidates:
            best = max(candidates, key=lambda e: ver_tuple(e.get('version')))
            print(best.get('version', '') or '')
except Exception:
    pass
PYEOF
        return 0
    fi

    if command -v node &>/dev/null; then
        PLUGIN_KEY="$PLUGIN_KEY" node - "$INSTALLED_JSON" <<'JSEOF' 2>/dev/null || true
const fs = require('fs');
function parseVersion(value) {
  return String(value || '0.0.0')
    .split('.')
    .slice(0, 3)
    .map((part) => {
      const n = parseInt(part, 10);
      return Number.isFinite(n) ? n : 0;
    });
}
function compareVersions(a, b) {
  const av = parseVersion(a && a.version);
  const bv = parseVersion(b && b.version);
  for (let i = 0; i < 3; i += 1) {
    if (av[i] !== bv[i]) return av[i] - bv[i];
  }
  return 0;
}
try {
  const installedPath = process.argv[2];
  const pluginKey = process.env.PLUGIN_KEY;
  const data = JSON.parse(fs.readFileSync(installedPath, 'utf8'));
  const entries = ((data.plugins || {})[pluginKey] || []).filter((entry) => entry && typeof entry === 'object');
  const candidates = entries.filter((entry) => entry.installPath) || entries;
  const pool = candidates.length > 0 ? candidates : entries;
  if (pool.length > 0) {
    const best = pool.slice().sort(compareVersions).pop();
    process.stdout.write(String((best && best.version) || ''));
  }
} catch (_) {}
JSEOF
        return 0
    fi

    grep -A6 "\"${PLUGIN_KEY}\"" "$INSTALLED_JSON" 2>/dev/null \
        | grep -Eo '"version"[[:space:]]*:[[:space:]]*"[0-9]+\.[0-9]+\.[0-9]+"' \
        | tail -n 1 \
        | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+' || true
}

get_installed_plugin_root() {
    [ -f "$INSTALLED_JSON" ] || return 0

    if command -v jq &>/dev/null; then
        PLUGIN_KEY="$PLUGIN_KEY" jq -r '
          (.plugins[env.PLUGIN_KEY] // [])
          | map(select((.installPath // "") != ""))
          | sort_by((.version // "0.0.0") | split(".") | map(tonumber? // 0))
          | if length == 0 then {} else .[-1] end
          | .installPath // empty
        ' "$INSTALLED_JSON" 2>/dev/null || true
        return 0
    fi

    if command -v python3 &>/dev/null; then
        python3 - "$INSTALLED_JSON" "$PLUGIN_KEY" <<'PYEOF' 2>/dev/null || true
import json, sys
def ver_tuple(value):
    try:
        parts = str(value or "0.0.0").split(".")
        parts = (parts + ["0", "0", "0"])[:3]
        return tuple(int(p) for p in parts)
    except Exception:
        return (0, 0, 0)
try:
    with open(sys.argv[1]) as f:
        d = json.load(f)
    entries = d.get('plugins', {}).get(sys.argv[2], [])
    candidates = [e for e in entries if isinstance(e, dict) and e.get('installPath')]
    if candidates:
        best = max(candidates, key=lambda e: ver_tuple(e.get('version')))
        print(best.get('installPath', '') or '')
except Exception:
    pass
PYEOF
        return 0
    fi

    if command -v node &>/dev/null; then
        PLUGIN_KEY="$PLUGIN_KEY" node - "$INSTALLED_JSON" <<'JSEOF' 2>/dev/null || true
const fs = require('fs');
function parseVersion(value) {
  return String(value || '0.0.0')
    .split('.')
    .slice(0, 3)
    .map((part) => {
      const n = parseInt(part, 10);
      return Number.isFinite(n) ? n : 0;
    });
}
function compareVersions(a, b) {
  const av = parseVersion(a && a.version);
  const bv = parseVersion(b && b.version);
  for (let i = 0; i < 3; i += 1) {
    if (av[i] !== bv[i]) return av[i] - bv[i];
  }
  return 0;
}
try {
  const installedPath = process.argv[2];
  const pluginKey = process.env.PLUGIN_KEY;
  const data = JSON.parse(fs.readFileSync(installedPath, 'utf8'));
  const entries = ((data.plugins || {})[pluginKey] || [])
    .filter((entry) => entry && typeof entry === 'object' && entry.installPath);
  if (entries.length > 0) {
    const best = entries.slice().sort(compareVersions).pop();
    process.stdout.write(String((best && best.installPath) || ''));
  }
} catch (_) {}
JSEOF
        return 0
    fi

    grep -o '"installPath"[[:space:]]*:[[:space:]]*"[^"]*'"${MARKETPLACE_NAME}"'[^"]*"' "$INSTALLED_JSON" 2>/dev/null \
        | tail -n 1 \
        | sed 's/"installPath"[[:space:]]*:[[:space:]]*"//;s/"$//' || true
}

sync_marketplace_checkout() {
    echo ""
    echo -e "${BLUE}🔄 Syncing marketplace checkout...${NC}"

    if [ "$MARKETPLACE_SOURCE" != "$REPO" ]; then
        echo -e "${BLUE}  Using custom marketplace source; skipping direct git sync${NC}"
        return 0
    fi

    if [ ! -d "$MARKETPLACE_DIR/.git" ]; then
        echo -e "${YELLOW}  Marketplace checkout not found yet; continuing${NC}"
        return 0
    fi

    if ! command -v git &>/dev/null; then
        echo -e "${YELLOW}  git not found; skipping marketplace checkout sync${NC}"
        return 0
    fi

    local remote_url=""
    remote_url=$(git -C "$MARKETPLACE_DIR" remote get-url origin 2>/dev/null || true)
    case "$remote_url" in
        *"${REPO}"*)
            ;;
        *)
            echo -e "${YELLOW}  Marketplace remote does not match ${REPO}; skipping direct sync${NC}"
            return 0
            ;;
    esac

    local before_version=""
    local after_version=""
    before_version=$(get_manifest_version "$MARKETPLACE_PLUGIN_JSON")

    local is_shallow="false"
    is_shallow=$(git -C "$MARKETPLACE_DIR" rev-parse --is-shallow-repository 2>/dev/null || echo "false")

    if [ "$is_shallow" = "true" ]; then
        if git -C "$MARKETPLACE_DIR" fetch --unshallow origin main >/dev/null 2>&1; then
            :
        elif git -C "$MARKETPLACE_DIR" fetch --deepen=50 origin main >/dev/null 2>&1; then
            :
        elif ! git -C "$MARKETPLACE_DIR" fetch origin main >/dev/null 2>&1; then
            echo -e "${YELLOW}  Could not refresh shallow marketplace checkout; keeping existing checkout${NC}"
            return 0
        fi
    elif ! git -C "$MARKETPLACE_DIR" fetch origin main >/dev/null 2>&1; then
        echo -e "${YELLOW}  Could not fetch marketplace checkout; keeping existing checkout${NC}"
        return 0
    fi

    if git -C "$MARKETPLACE_DIR" checkout -q main >/dev/null 2>&1 && \
       git -C "$MARKETPLACE_DIR" merge --ff-only FETCH_HEAD >/dev/null 2>&1; then
        after_version=$(get_manifest_version "$MARKETPLACE_PLUGIN_JSON")
        if [ -n "$after_version" ] && [ "$after_version" != "$before_version" ]; then
            echo -e "${GREEN}✓${NC} Marketplace checkout updated to v${after_version}"
        elif [ -n "$after_version" ]; then
            echo -e "${GREEN}✓${NC} Marketplace checkout already at v${after_version}"
        else
            echo -e "${GREEN}✓${NC} Marketplace checkout synced"
        fi
        return 0
    fi

    local current_head=""
    local fetched_head=""
    current_head=$(git -C "$MARKETPLACE_DIR" rev-parse --short HEAD 2>/dev/null || true)
    fetched_head=$(git -C "$MARKETPLACE_DIR" rev-parse --short FETCH_HEAD 2>/dev/null || true)

    if [ -n "$current_head" ] && [ "$current_head" = "$fetched_head" ]; then
        after_version=$(get_manifest_version "$MARKETPLACE_PLUGIN_JSON")
        if [ -n "$after_version" ]; then
            echo -e "${GREEN}✓${NC} Marketplace checkout already at v${after_version}"
        else
            echo -e "${GREEN}✓${NC} Marketplace checkout already current"
        fi
        return 0
    fi

    echo -e "${YELLOW}  Marketplace checkout fetch succeeded but fast-forward merge failed; keeping existing checkout${NC}"
    return 0
}

verify_installed_plugin_version() {
    local expected_version="$1"
    [ -n "$expected_version" ] || return 0

    local installed_version=""
    installed_version=$(get_installed_plugin_version)
    if [ "$installed_version" = "$expected_version" ]; then
        return 0
    fi

    # Claude Code can record a successfully installed plugin with an unknown or
    # empty registry version. In that case, verify the files at the selected
    # installPath instead of treating the registry placeholder as authoritative.
    case "$installed_version" in
        ""|unknown)
            local installed_root=""
            local manifest_version=""
            installed_root=$(get_installed_plugin_root)
            if [ -n "$installed_root" ]; then
                manifest_version=$(get_manifest_version "${installed_root}/.claude-plugin/plugin.json")
            fi
            [ "$manifest_version" = "$expected_version" ]
            ;;
        *)
            return 1
            ;;
    esac
}

# ──────────────────────────────────────────────

clear_plugin_cache() {
    if [ -n "$CACHE_DIR" ] && [ "$CACHE_DIR" != "/" ] && [ -d "$CACHE_DIR" ]; then
        echo -e "${BLUE}  Clearing plugin cache...${NC}"
        rm -rf "$CACHE_DIR" 2>/dev/null || true
    fi
}

# ──────────────────────────────────────────────

install_plugin() {
    echo ""
    echo -e "${BLUE}📦 Installing plugin...${NC}"

    # Remember old version directories before clearing cache.
    # After install, we create lightweight "shim" dirs for old versions that
    # forward hook-wrapper.sh to the currently installed version.
    #
    # Why shims (not symlinks)?
    # - Symlinks are unreliable on Windows (permissions / developer mode / Git settings)
    # - Shims are cross-platform and don't require special FS features
    #
    # This keeps a running Claude Code instance working until restart, even if it
    # cached the old version path in memory.
    local version_dir="${CACHE_DIR}/${MARKETPLACE_NAME}"
    local old_versions=()
    if [ -d "$version_dir" ]; then
        for d in "$version_dir"/*/; do
            # Skip symlinks from previous bootstrap runs, only collect real dirs
            [ -d "$d" ] && [ ! -L "${d%/}" ] && old_versions+=("$(basename "$d")")
        done
    fi

    # Migrate config to stable location before cache clear (#30)
    local stable_config_dir="${CLAUDE_HOME}/claude-notifications-go"
    if [ -d "$version_dir" ]; then
        # Collect version dirs using glob (no ls parsing, Bash 3.2 safe)
        local ver_dirs=()
        for d in "$version_dir"/*/; do
            [ -d "$d" ] && [ ! -L "${d%/}" ] && ver_dirs+=("$d")
        done
        # Search in reverse glob order (lexicographic — sufficient when only one version dir exists)
        local newest_config=""
        local i
        for (( i=${#ver_dirs[@]}-1; i>=0; i-- )); do
            d="${ver_dirs[$i]}"
            if [ -f "${d}config/config.json" ]; then
                newest_config="${d}config/config.json"
                break
            fi
        done
        if [ -n "$newest_config" ] && [ ! -f "$stable_config_dir/config.json" ]; then
            if mkdir -p "$stable_config_dir" 2>/dev/null; then
                # Atomic copy: tmp + mv (safe on interrupt)
                cp "$newest_config" "$stable_config_dir/config.json.tmp" 2>/dev/null && \
                    mv "$stable_config_dir/config.json.tmp" "$stable_config_dir/config.json" 2>/dev/null && \
                    echo -e "${BLUE}  Migrated config.json to stable location${NC}"
                rm -f "$stable_config_dir/config.json.tmp" 2>/dev/null
            fi
        fi
    fi

    local expected_version=""
    expected_version=$(get_manifest_version "$MARKETPLACE_PLUGIN_JSON")

    local installed_before=""
    installed_before=$(get_installed_plugin_version)

    local output
    if [ -n "$installed_before" ]; then
        if output=$(claude plugin update "$PLUGIN_KEY" </dev/null 2>&1); then
            echo -e "${GREEN}✓${NC} Plugin updated"
        else
            echo -e "${YELLOW}  Plugin update failed, will attempt recovery reinstall${NC}"
            echo -e "${YELLOW}  Output: ${output}${NC}"
        fi
    else
        clear_plugin_cache
        if output=$(claude plugin install "$PLUGIN_KEY" </dev/null 2>&1); then
            echo -e "${GREEN}✓${NC} Plugin installed"
        else
            if echo "$output" | grep -qi "already installed"; then
                echo -e "${GREEN}✓${NC} Plugin already installed"
            else
                echo -e "${RED}✗ Plugin install failed${NC}" >&2
                echo -e "${YELLOW}Output: ${output}${NC}" >&2
                exit 1
            fi
        fi
    fi

    if [ -n "$expected_version" ] && ! verify_installed_plugin_version "$expected_version"; then
        echo -e "${YELLOW}  Installed plugin version does not match marketplace v${expected_version}; reinstalling...${NC}"

        claude plugin uninstall "$PLUGIN_KEY" </dev/null >/dev/null 2>&1 || true
        clear_plugin_cache

        if output=$(claude plugin install "$PLUGIN_KEY" </dev/null 2>&1); then
            echo -e "${GREEN}✓${NC} Plugin reinstalled"
        else
            echo -e "${RED}✗ Plugin reinstall failed${NC}" >&2
            echo -e "${YELLOW}Output: ${output}${NC}" >&2
            exit 1
        fi
    fi

    if [ -n "$expected_version" ] && ! verify_installed_plugin_version "$expected_version"; then
        local installed_after=""
        installed_after=$(get_installed_plugin_version)
        echo -e "${RED}✗ Plugin version mismatch after install/update${NC}" >&2
        echo -e "${YELLOW}Expected: v${expected_version}${NC}" >&2
        echo -e "${YELLOW}Installed: v${installed_after:-unknown}${NC}" >&2
        exit 1
    fi

    # Create shim dirs for old version paths so running Claude Code instances
    # don't break before restart.
    #
    # Each shim contains only: <old>/bin/hook-wrapper.sh
    # The shim does NOT hardcode the target version; it reads installed_plugins.json
    # on each invocation and forwards to the currently installed installPath.
    if [ -d "$version_dir" ] && [ ${#old_versions[@]} -gt 0 ]; then
        # Determine the newly installed version dir name (first real dir).
        local new_version=""
        for d in "$version_dir"/*/; do
            [ -d "$d" ] && [ ! -L "${d%/}" ] && new_version="$(basename "$d")" && break
        done

        if [ -n "$new_version" ]; then
            for old_ver in "${old_versions[@]}"; do
                # Skip if it matches current version (shouldn't happen, but be safe)
                [ "$old_ver" = "$new_version" ] && continue

                # If something already exists at that path (directory, file, symlink), don't overwrite.
                if [ -e "$version_dir/$old_ver" ]; then
                    continue
                fi

                # Create minimal shim directory structure
                mkdir -p "$version_dir/$old_ver/bin" 2>/dev/null || true

                # Write shim hook-wrapper.sh (POSIX sh) atomically
                local shim_path="$version_dir/$old_ver/bin/hook-wrapper.sh"
                local tmp_path="${shim_path}.tmp.$$"
                cat > "$tmp_path" <<'SHIMEOF' 2>/dev/null || true
#!/bin/sh
# claude-notifications-go shim: forwards old cached hook path to current plugin installPath.
# This file is auto-generated by bootstrap.sh and is safe to delete after restarting Claude Code.
#
# Behavior:
# - Find current installPath from ~/.claude/plugins/installed_plugins.json
# - Set CLAUDE_PLUGIN_ROOT to that installPath
# - Exec the real hook-wrapper.sh from the current install
#
# IMPORTANT: Must never fail the hook (exit 0 on any error).

CLAUDE_HOME="${CLAUDE_CONFIG_DIR:-${CLAUDE_HOME:-$HOME/.claude}}"
if [ -z "$CLAUDE_HOME" ]; then
  CLAUDE_HOME="$HOME/.claude"
fi

INSTALLED_JSON="${CLAUDE_HOME}/plugins/installed_plugins.json"
MARKETPLACE_NAME="claude-notifications-go"
PLUGIN_KEY="claude-notifications-go@claude-notifications-go"
PLUGIN_ROOT=""

if [ -f "$INSTALLED_JSON" ]; then
  # Prefer robust JSON parsing; fall back to grep/sed only if needed.
  if command -v jq >/dev/null 2>&1; then
    PLUGIN_ROOT=$(PLUGIN_KEY="$PLUGIN_KEY" jq -r '
      (.plugins[env.PLUGIN_KEY] // [])
      | map(select((.installPath // "") != ""))
      | sort_by((.version // "0.0.0") | split(".") | map(tonumber? // 0))
      | if length == 0 then {} else .[-1] end
      | .installPath // empty
    ' "$INSTALLED_JSON" 2>/dev/null) || true
  fi

  if [ -z "$PLUGIN_ROOT" ] && command -v python3 >/dev/null 2>&1; then
    PLUGIN_ROOT=$(python3 - "$INSTALLED_JSON" "$PLUGIN_KEY" <<'PYEOF' 2>/dev/null || true
import json, sys
def ver_tuple(value):
    try:
        parts = str(value or "0.0.0").split(".")
        parts = (parts + ["0", "0", "0"])[:3]
        return tuple(int(p) for p in parts)
    except Exception:
        return (0, 0, 0)
try:
    with open(sys.argv[1]) as f:
        d = json.load(f)
    entries = [e for e in d.get('plugins', {}).get(sys.argv[2], []) if isinstance(e, dict) and e.get('installPath')]
    if entries:
        best = max(entries, key=lambda e: ver_tuple(e.get('version')))
        print(best.get('installPath', '') or '')
except Exception:
    pass
PYEOF
)
  fi

  # Node is very likely present because Claude Code is a Node app.
  if [ -z "$PLUGIN_ROOT" ] && command -v node >/dev/null 2>&1; then
    PLUGIN_ROOT=$(PLUGIN_KEY="$PLUGIN_KEY" node - "$INSTALLED_JSON" <<'JSEOF' 2>/dev/null || true
const fs = require('fs');
function parseVersion(value) {
  return String(value || '0.0.0')
    .split('.')
    .slice(0, 3)
    .map((part) => {
      const n = parseInt(part, 10);
      return Number.isFinite(n) ? n : 0;
    });
}
function compareVersions(a, b) {
  const av = parseVersion(a && a.version);
  const bv = parseVersion(b && b.version);
  for (let i = 0; i < 3; i += 1) {
    if (av[i] !== bv[i]) return av[i] - bv[i];
  }
  return 0;
}
try {
  const p = process.argv[2];
  const k = process.env.PLUGIN_KEY;
  const d = JSON.parse(fs.readFileSync(p, 'utf8'));
  const entries = ((d.plugins && d.plugins[k]) || []).filter((entry) => entry && typeof entry === 'object' && entry.installPath);
  if (entries.length > 0) {
    const e = entries.slice().sort(compareVersions).pop();
    process.stdout.write((e && e.installPath) ? String(e.installPath) : '');
  }
} catch (_) {}
JSEOF
)
  fi

  if [ -z "$PLUGIN_ROOT" ]; then
    # Best-effort fallback: extract first installPath containing the marketplace name.
    PLUGIN_ROOT=$(grep -o '"installPath"[[:space:]]*:[[:space:]]*"[^"]*'"${MARKETPLACE_NAME}"'[^"]*"' "$INSTALLED_JSON" 2>/dev/null \
      | tail -n 1 \
      | sed 's/"installPath"[[:space:]]*:[[:space:]]*"//;s/"$//' 2>/dev/null) || true
  fi
fi

# Last-resort fallback: try to find any sibling version dir with a real hook-wrapper.sh
if [ -z "$PLUGIN_ROOT" ]; then
  _self_dir="$(cd "$(dirname "$0")" 2>/dev/null && pwd)"
  _ver_dir="$(cd "$_self_dir/.." 2>/dev/null && pwd)"      # <old>/bin
  _ver_root="$(cd "$_ver_dir/.." 2>/dev/null && pwd)"      # <old>
  _parent="$(cd "$_ver_root/.." 2>/dev/null && pwd)"       # .../claude-notifications-go/<versions>
  for d in "$_parent"/*/; do
    [ -d "$d" ] || continue
    if [ -f "${d}bin/hook-wrapper.sh" ]; then
      PLUGIN_ROOT="${d%/}"
      break
    fi
  done
fi

# Extra fallback: stable pointer written by hook-wrapper.sh at runtime
if [ -z "$PLUGIN_ROOT" ]; then
  _PTR_FILE="${CLAUDE_HOME}/claude-notifications-go/plugin-root"
  if [ -f "$_PTR_FILE" ]; then
    IFS= read -r PLUGIN_ROOT < "$_PTR_FILE" 2>/dev/null || true
  fi
fi

if [ -n "$PLUGIN_ROOT" ] && [ -f "$PLUGIN_ROOT/bin/hook-wrapper.sh" ]; then
  export CLAUDE_PLUGIN_ROOT="$PLUGIN_ROOT"
  exec "$PLUGIN_ROOT/bin/hook-wrapper.sh" "$@" || true
fi

exit 0
SHIMEOF
                mv "$tmp_path" "$shim_path" 2>/dev/null || true
                rm -f "$tmp_path" 2>/dev/null || true
                chmod +x "$shim_path" 2>/dev/null || true
                echo -e "${BLUE}  Shim: ${old_ver} → current install (for running session)${NC}"
            done
        fi
    fi
}

# ──────────────────────────────────────────────

find_plugin_root() {
    echo ""
    echo -e "${BLUE}🔍 Locating plugin directory...${NC}"

    if [ ! -f "$INSTALLED_JSON" ]; then
        echo -e "${RED}✗ installed_plugins.json not found at ${INSTALLED_JSON}${NC}" >&2
        echo -e "${YELLOW}  Try restarting Claude Code and running this script again.${NC}" >&2
        exit 1
    fi

    # Try jq first (clean JSON parsing)
    if command -v jq &>/dev/null; then
        PLUGIN_ROOT=$(get_installed_plugin_root)
        if [ "$PLUGIN_ROOT" = "null" ]; then
            PLUGIN_ROOT=""
        fi
    fi

    # Fallback: python3 (available on macOS and most Linux)
    # Pass paths as arguments to avoid shell injection in python code
    if [ -z "$PLUGIN_ROOT" ]; then
        PLUGIN_ROOT=$(get_installed_plugin_root)
    fi

    # Fallback: grep + sed (works everywhere)
    if [ -z "$PLUGIN_ROOT" ]; then
        # Find the installPath that's inside the claude-notifications-go cache dir
        # Note: JSON may have whitespace after colon — "installPath": "..." or "installPath":"..."
        PLUGIN_ROOT=$(grep -o '"installPath"[[:space:]]*:[[:space:]]*"[^"]*'"${MARKETPLACE_NAME}"'[^"]*"' "$INSTALLED_JSON" 2>/dev/null \
            | tail -n 1 \
            | sed 's/"installPath"[[:space:]]*:[[:space:]]*"//;s/"$//' || true)
    fi

    if [ -z "$PLUGIN_ROOT" ] || [ ! -d "$PLUGIN_ROOT" ]; then
        echo -e "${RED}✗ Could not find plugin install path${NC}" >&2
        echo -e "${YELLOW}  installed_plugins.json may not contain the plugin entry yet.${NC}" >&2
        echo -e "${YELLOW}  Try: claude plugin install ${PLUGIN_KEY}${NC}" >&2
        exit 1
    fi

    echo -e "${GREEN}✓${NC} Plugin root: ${PLUGIN_ROOT}"
}

# ──────────────────────────────────────────────

download_binary() {
    echo ""
    echo -e "${BLUE}📦 Downloading notification binary...${NC}"

    local target_dir="${PLUGIN_ROOT}/bin"
    if ! mkdir -p "$target_dir" 2>/dev/null; then
        echo -e "${RED}✗ Cannot create directory: ${target_dir}${NC}" >&2
        exit 1
    fi

    # Download install.sh to a temp file, verify it's non-empty, then run
    # Set trap BEFORE mktemp to avoid race condition on Ctrl+C
    trap 'rm -f "$_BOOTSTRAP_TMP" 2>/dev/null' EXIT INT TERM
    # Validate TMPDIR exists; fall back to /tmp if it doesn't
    local tmp_base="${TMPDIR:-/tmp}"
    if [ ! -d "$tmp_base" ]; then
        tmp_base="/tmp"
    fi
    _BOOTSTRAP_TMP="$(mktemp "${tmp_base}/bootstrap-install-XXXXXX")"
    local tmp_script="$_BOOTSTRAP_TMP"

    local downloaded=false
    if command -v curl &>/dev/null; then
        curl -fsSL "$INSTALL_SCRIPT_URL" -o "$tmp_script" 2>/dev/null && downloaded=true
    elif command -v wget &>/dev/null; then
        wget -q "$INSTALL_SCRIPT_URL" -O "$tmp_script" 2>/dev/null && downloaded=true
    fi

    if [ "$downloaded" != true ] || [ ! -s "$tmp_script" ]; then
        echo -e "${RED}✗ Failed to download install.sh${NC}" >&2
        echo -e "${YELLOW}  URL: ${INSTALL_SCRIPT_URL}${NC}" >&2
        exit 1
    fi

    # </dev/null prevents stdin conflicts when running via `curl | bash`
    local install_exit=0
    INSTALL_TARGET_DIR="$target_dir" bash "$tmp_script" </dev/null || install_exit=$?

    if [ $install_exit -ne 0 ]; then
        echo -e "${RED}✗ Binary installation failed (exit code: ${install_exit})${NC}" >&2
        exit 1
    fi
}

# ──────────────────────────────────────────────

setup_iterm2_venv() {
    # Only relevant on macOS
    [ "$(uname -s)" = "Darwin" ] || return 0

    is_iterm2_detected || return 0

    # Use $HOME/.claude explicitly (not $CLAUDE_HOME) — the Go code resolves
    # the venv path via os.UserHomeDir()/.claude/..., so the venv must be there.
    local VENV_DIR="$HOME/.claude/claude-notifications-go/iterm2-venv"

    # Skip if venv already exists and is functional
    if [ -x "$VENV_DIR/bin/python3" ] && \
       "$VENV_DIR/bin/python3" -c "import iterm2" 2>/dev/null; then
        echo -e "${GREEN}  ✓${NC} iTerm2 Python API venv already set up"
        return 0
    fi

    # Find Python 3
    local python3_path=""
    command -v python3 &>/dev/null && python3_path="$(command -v python3)"

    if [ -z "$python3_path" ]; then
        echo ""
        echo -e "${YELLOW}  ⚠ Python 3 not found. iTerm2 tmux -CC click-to-focus unavailable.${NC}"
        echo -e "${YELLOW}    Install Python 3 and re-run bootstrap to enable.${NC}"
        return 0
    fi

    echo ""
    echo -e "${BLUE}  Setting up iTerm2 Python API support...${NC}"

    if ! "$python3_path" -m venv "$VENV_DIR" 2>/dev/null; then
        echo -e "${YELLOW}  ⚠ Could not create Python venv, skipping${NC}"
        return 0
    fi

    if "$VENV_DIR/bin/pip" install --quiet iterm2 2>/dev/null; then
        echo -e "${GREEN}  ✓${NC} iTerm2 Python API support installed"
        echo -e "${BLUE}    Enable 'Python API' in iTerm2 → Settings → General → Magic${NC}"
    else
        echo -e "${YELLOW}  ⚠ Could not install iterm2 module${NC}"
        rm -rf "$VENV_DIR" 2>/dev/null
    fi
}

# ──────────────────────────────────────────────

print_success() {
    echo ""
    echo -e "${GREEN}============================================${NC}"
    echo -e "${GREEN} ✓ Bootstrap Complete!${NC}"
    echo -e "${GREEN}============================================${NC}"
    echo ""
    echo -e "${BOLD}Next steps:${NC}"
    echo -e "  1. ${YELLOW}Restart Claude Code${NC} (exit and reopen)"
    echo -e "  2. Run ${BOLD}/claude-notifications-go:settings${NC} to configure sounds"
    if is_iterm2_detected; then
        echo -e "  3. In ${BOLD}iTerm2${NC}, enable ${BOLD}Settings → General → Magic → Python API${NC}"
    fi
    echo ""
    print_iterm2_python_api_notice
    echo ""
    echo -e "${BLUE}One-liner to update in the future (same as install):${NC}"
    echo -e "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/bin/bootstrap.sh | bash"
    echo ""
    echo -e "${YELLOW}────────────────────────────────────────────${NC}"
    echo -e "${YELLOW}★${NC} ${BOLD}Boost your productivity${NC}"
    echo -e "  Check out the advanced task manager for Claude"
    echo -e "  with a convenient UI, from the creator of this plugin:"
    echo -e "  ${GREEN}https://github.com/777genius/claude_agent_teams_ui${NC}"
    echo -e "${YELLOW}────────────────────────────────────────────${NC}"
    echo ""
}

# ──────────────────────────────────────────────

main() {
    print_header
    abort_if_wsl_environment
    check_prerequisites
    detect_platform
    setup_marketplace
    sync_marketplace_checkout
    install_plugin
    find_plugin_root
    download_binary
    setup_iterm2_venv
    print_success
}

main "$@"
