#!/bin/bash
# install.sh - Auto-installer for claude-notifications binaries
# Downloads the appropriate binary from GitHub Releases

set -e

# Colors and formatting
BOLD='\033[1m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Get target directory
# Priority: 1) INSTALL_TARGET_DIR env var (set by notifications-init.md)
#           2) Script's own directory (normal case)
if [ -n "$INSTALL_TARGET_DIR" ]; then
    SCRIPT_DIR="$INSTALL_TARGET_DIR"
else
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

# Ensure target directory exists
mkdir -p "$SCRIPT_DIR" 2>/dev/null || true

# Lockfile to prevent parallel installations
LOCKFILE="${SCRIPT_DIR}/.install.lock"

# Network settings
MAX_RETRIES=3
RETRY_DELAY=2
CURL_TIMEOUT=60
WGET_TIMEOUT=60
CONNECT_TIMEOUT=20
CURL_EXTRA_OPTS=()
CURL_COMPAT_OPTS=()

# GitHub repository (can be overridden via env for testing)
REPO="777genius/claude-notifications-go"
RELEASES_BASE_URL="${RELEASES_BASE_URL:-https://github.com/${REPO}/releases}"
LATEST_RELEASE_API_URL="${LATEST_RELEASE_API_URL:-https://api.github.com/repos/${REPO}/releases/latest}"
DEFAULT_RELEASE_URL="${RELEASES_BASE_URL}/latest/download"
DEFAULT_CHECKSUMS_URL="${DEFAULT_RELEASE_URL}/checksums.txt"
DEFAULT_MODERN_NOTIFIER_URL="${DEFAULT_RELEASE_URL}/ClaudeNotifier.app.zip"
RELEASE_URL="${RELEASE_URL:-${DEFAULT_RELEASE_URL}}"
CHECKSUMS_URL="${CHECKSUMS_URL:-${DEFAULT_CHECKSUMS_URL}}"
# ClaudeNotifier.app is built, signed with Developer ID, and notarized in CI.
# It ships alongside Go binaries in each release.
MODERN_NOTIFIER_URL="${MODERN_NOTIFIER_URL:-${DEFAULT_MODERN_NOTIFIER_URL}}"
PINNED_RELEASE_TAG=""

# Parse command line arguments
FORCE_UPDATE=false
WINDOWS_NATIVE_HOOKS_NEED_UPDATE=false
for arg in "$@"; do
  case $arg in
    --force|-f)
      FORCE_UPDATE=true
      ;;
  esac
done

# Detect platform and architecture
detect_platform() {
    local os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    local arch="$(uname -m)"

    case "$os" in
        darwin)
            PLATFORM="darwin"
            ;;
        linux)
            PLATFORM="linux"
            ;;
        mingw*|msys*|cygwin*)
            PLATFORM="windows"
            ;;
        *)
            echo -e "${RED}✗ Unsupported OS: $os${NC}" >&2
            exit 1
            ;;
    esac

    case "$arch" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        *)
            echo -e "${RED}✗ Unsupported architecture: $arch${NC}" >&2
            exit 1
            ;;
    esac

    # Construct binary names
    if [ "$PLATFORM" = "windows" ]; then
        BINARY_NAME="claude-notifications-${PLATFORM}-${ARCH}.exe"
        SOUND_PREVIEW_NAME="sound-preview-${PLATFORM}-${ARCH}.exe"
        LIST_DEVICES_NAME="list-devices-${PLATFORM}-${ARCH}.exe"
        LIST_SOUNDS_NAME="list-sounds-${PLATFORM}-${ARCH}.exe"
    else
        BINARY_NAME="claude-notifications-${PLATFORM}-${ARCH}"
        SOUND_PREVIEW_NAME="sound-preview-${PLATFORM}-${ARCH}"
        LIST_DEVICES_NAME="list-devices-${PLATFORM}-${ARCH}"
        LIST_SOUNDS_NAME="list-sounds-${PLATFORM}-${ARCH}"
    fi

    BINARY_PATH="${SCRIPT_DIR}/${BINARY_NAME}"
    SOUND_PREVIEW_PATH="${SCRIPT_DIR}/${SOUND_PREVIEW_NAME}"
    LIST_DEVICES_PATH="${SCRIPT_DIR}/${LIST_DEVICES_NAME}"
    LIST_SOUNDS_PATH="${SCRIPT_DIR}/${LIST_SOUNDS_NAME}"
    CHECKSUMS_PATH="${SCRIPT_DIR}/.checksums.txt"

    configure_curl_options
}

curl_supports_option() {
    local option="$1"
    curl --help all 2>/dev/null | grep -q -- "$option"
}

configure_curl_options() {
    CURL_EXTRA_OPTS=()
    CURL_COMPAT_OPTS=()

    if ! command -v curl &>/dev/null; then
        return 0
    fi

    if [ "$PLATFORM" = "windows" ] && curl_supports_option "--ssl-no-revoke"; then
        # Git for Windows typically uses Schannel. Corporate TLS inspection often breaks
        # revocation checks for GitHub Releases, while the rest of GitHub still works.
        CURL_EXTRA_OPTS+=(--ssl-no-revoke)
    fi

    if curl_supports_option "--http1.1"; then
        CURL_COMPAT_OPTS+=(--http1.1)
    fi
}

clean_download_error_output() {
    printf '%s\n' "$1" | tr '\r' '\n' | sed -E \
        -e 's#(https?://)[^/@[:space:]]+:[^@[:space:]]+@#\1***:***@#g' \
        -e 's/^[[:space:]]+//; s/[[:space:]]+$//' \
        -e '/^$/d' \
        -e '/^% Total/d' \
        -e '/^  % Total/d' \
        -e '/^#+$/d' \
        -e '/^[#=[:space:]]*[0-9.]+%$/d'
}

print_download_error_details() {
    local error_text="$1"
    local cleaned
    local filtered

    cleaned=$(clean_download_error_output "$error_text")
    if [ -z "$cleaned" ]; then
        return 1
    fi

    filtered=$(printf '%s\n' "$cleaned" | grep -iE "error|fail|refused|timeout|resolve|ssl|certificate|connect|proxy|schannel|tls|handshake|reset|closed|abort|denied|host|http/2|revoke" | head -5 || true)
    if [ -n "$filtered" ]; then
        printf '%s\n' "$filtered" >&2
    else
        printf '%s\n' "$cleaned" | head -5 >&2
    fi

    return 0
}

get_proxy_env_names() {
    local names=()
    local var_name

    for var_name in HTTPS_PROXY https_proxy HTTP_PROXY http_proxy ALL_PROXY all_proxy; do
        if [ -n "${!var_name}" ]; then
            names+=("$var_name")
        fi
    done

    printf '%s' "${names[*]}"
}

print_windows_proxy_hint() {
    local proxy_envs

    [ "$PLATFORM" = "windows" ] || return 0

    proxy_envs=$(get_proxy_env_names)
    if [ -n "$proxy_envs" ]; then
        echo -e "${YELLOW}→ Proxy environment detected: ${proxy_envs}${NC}" >&2
        echo -e "${YELLOW}  If your proxy inspects TLS, ensure Git Bash curl trusts the corporate CA.${NC}" >&2
    else
        echo -e "${YELLOW}→ Windows/Git Bash downloads can fail behind corporate proxies or TLS inspection.${NC}" >&2
    fi
}

print_curl_failure_guidance() {
    local http_code="$1"
    local curl_exit_code="$2"
    local curl_error="$3"

    if [ "$http_code" = "000" ] || [ -z "$http_code" ]; then
        echo -e "${YELLOW}→ No HTTP response received from the release server (curl exit code: ${curl_exit_code:-unknown}).${NC}" >&2
    fi

    case "$curl_exit_code" in
        5|6)
            echo -e "${YELLOW}→ DNS resolution failed. Check your network and DNS settings.${NC}" >&2
            ;;
        7)
            echo -e "${YELLOW}→ Connection to the release host failed. Check firewall, proxy, or antivirus settings.${NC}" >&2
            ;;
        18|56)
            echo -e "${YELLOW}→ The connection was interrupted mid-download. A proxy or TLS filter may be interfering.${NC}" >&2
            ;;
        28)
            echo -e "${YELLOW}→ Connection timed out. GitHub may be slow or blocked from this network.${NC}" >&2
            ;;
        35|51|58|60|77)
            echo -e "${YELLOW}→ TLS/certificate validation failed while contacting GitHub Releases.${NC}" >&2
            ;;
        92)
            echo -e "${YELLOW}→ HTTP/2 transport failed. The installer retried with HTTP/1.1 compatibility mode.${NC}" >&2
            ;;
    esac

    if echo "$curl_error" | grep -qi "resolve" && [ "$curl_exit_code" != "5" ] && [ "$curl_exit_code" != "6" ]; then
        echo -e "${YELLOW}→ DNS resolution failed. Check your internet connection.${NC}" >&2
    elif echo "$curl_error" | grep -qiE "ssl|certificate|tls|schannel|revoke|revocation" && [ "$curl_exit_code" != "35" ] && [ "$curl_exit_code" != "51" ] && [ "$curl_exit_code" != "58" ] && [ "$curl_exit_code" != "60" ] && [ "$curl_exit_code" != "77" ]; then
        echo -e "${YELLOW}→ SSL/TLS validation failed. Update system certificates or trust your corporate CA.${NC}" >&2
    elif echo "$curl_error" | grep -qi "timeout" && [ "$curl_exit_code" != "28" ]; then
        echo -e "${YELLOW}→ Connection timed out. GitHub may be slow or blocked.${NC}" >&2
    elif echo "$curl_error" | grep -qiE "refused|connect|proxy|407" && [ "$curl_exit_code" != "7" ] && [ "$curl_exit_code" != "56" ]; then
        echo -e "${YELLOW}→ Connection was blocked before the download started. Check proxy and firewall settings.${NC}" >&2
    fi

    if [ "$PLATFORM" = "windows" ] && { [ "$http_code" = "000" ] || echo "$curl_error" | grep -qiE "schannel|ssl|tls|certificate|proxy|connect|407|revoke|revocation"; }; then
        print_windows_proxy_hint
    fi
}

# Acquire lock to prevent parallel installations
acquire_lock() {
    # Use mkdir for atomic lock (works on all platforms)
    if ! mkdir "$LOCKFILE" 2>/dev/null; then
        # Check if lock is stale (older than 10 minutes)
        if [ -d "$LOCKFILE" ]; then
            local lock_age=0
            if stat -f%m "$LOCKFILE" &>/dev/null; then
                lock_age=$(($(date +%s) - $(stat -f%m "$LOCKFILE")))
            elif stat -c%Y "$LOCKFILE" &>/dev/null; then
                lock_age=$(($(date +%s) - $(stat -c%Y "$LOCKFILE")))
            fi

            if [ "$lock_age" -gt 600 ]; then
                echo -e "${YELLOW}⚠ Removing stale lock (${lock_age}s old)${NC}"
                rm -rf "$LOCKFILE"
                mkdir "$LOCKFILE" 2>/dev/null || true
            else
                echo -e "${RED}✗ Another installation is in progress${NC}" >&2
                echo -e "${YELLOW}If this is incorrect, remove: ${LOCKFILE}${NC}" >&2
                return 1
            fi
        fi
    fi

    # Set trap to release lock on exit
    trap 'rm -rf "$LOCKFILE" 2>/dev/null' EXIT INT TERM
    return 0
}

# Check if we have write permissions
check_write_permissions() {
    if [ ! -d "$SCRIPT_DIR" ]; then
        echo -e "${RED}✗ Directory does not exist: ${SCRIPT_DIR}${NC}" >&2
        return 1
    fi

    if [ ! -w "$SCRIPT_DIR" ]; then
        echo -e "${RED}✗ No write permission to: ${SCRIPT_DIR}${NC}" >&2
        echo -e "${YELLOW}Try running with sudo or check directory permissions${NC}" >&2
        return 1
    fi

    return 0
}

# Check required tools
check_required_tools() {
    local missing_tools=()

    # curl or wget required
    if ! command -v curl &>/dev/null && ! command -v wget &>/dev/null; then
        missing_tools+=("curl or wget")
    fi

    # unzip required for terminal-notifier on macOS
    if [ "$(uname -s | tr '[:upper:]' '[:lower:]')" = "darwin" ]; then
        if ! command -v unzip &>/dev/null; then
            missing_tools+=("unzip")
        fi
    fi

    if [ ${#missing_tools[@]} -gt 0 ]; then
        echo -e "${RED}✗ Missing required tools: ${missing_tools[*]}${NC}" >&2
        return 1
    fi

    return 0
}

# Retry wrapper for network operations
retry_download() {
    local url="$1"
    local output="$2"
    local description="$3"
    local attempt=1

    while [ $attempt -le $MAX_RETRIES ]; do
        if [ $attempt -gt 1 ]; then
            echo -e "${YELLOW}Retry ${attempt}/${MAX_RETRIES} after ${RETRY_DELAY}s...${NC}"
            sleep $RETRY_DELAY
        fi

        local temp_file="${output}.tmp.$$"
        local success=false

        if command -v curl &>/dev/null; then
            if curl -fsSL "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --max-time "$CURL_TIMEOUT" "$url" -o "$temp_file" 2>/dev/null; then
                success=true
            fi
        elif command -v wget &>/dev/null; then
            if wget -q --timeout=$WGET_TIMEOUT "$url" -O "$temp_file" 2>/dev/null; then
                success=true
            fi
        fi

        if [ "$success" = true ] && [ -f "$temp_file" ] && [ "$(get_file_size "$temp_file")" -gt 0 ]; then
            mv "$temp_file" "$output"
            return 0
        fi

        rm -f "$temp_file" 2>/dev/null
        attempt=$((attempt + 1))
    done

    echo -e "${RED}✗ Failed to download ${description} after ${MAX_RETRIES} attempts${NC}" >&2
    return 1
}

# Get file size with multiple fallbacks
get_file_size() {
    local file="$1"

    # Try BSD stat (macOS)
    if stat -f%z "$file" 2>/dev/null; then
        return 0
    fi

    # Try GNU stat (Linux)
    if stat -c%s "$file" 2>/dev/null; then
        return 0
    fi

    # Fallback to wc -c (universal)
    wc -c < "$file" 2>/dev/null || echo "0"
}

fetch_url_to_stdout() {
    local url="$1"

    if command -v curl &>/dev/null; then
        curl -fsSL "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --max-time "$CURL_TIMEOUT" "$url" 2>/dev/null
        return $?
    fi

    if command -v wget &>/dev/null; then
        wget -qO- "$url" 2>/dev/null
        return $?
    fi

    return 1
}

get_latest_release_tag() {
    local response=""
    local tag=""

    response=$(fetch_url_to_stdout "$LATEST_RELEASE_API_URL") || return 1
    tag=$(printf '%s\n' "$response" | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"[^"]+"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')

    [ -n "$tag" ] || return 1
    printf '%s\n' "$tag"
}

pin_release_urls() {
    [ "$RELEASE_URL" = "$DEFAULT_RELEASE_URL" ] || return 0

    local tag=""
    tag=$(get_latest_release_tag || true)

    if [ -z "$tag" ]; then
        echo -e "${YELLOW}⚠ Could not resolve latest release tag, using /releases/latest fallback${NC}"
        return 0
    fi

    PINNED_RELEASE_TAG="$tag"
    RELEASE_URL="${RELEASES_BASE_URL}/download/${PINNED_RELEASE_TAG}"

    if [ "$CHECKSUMS_URL" = "$DEFAULT_CHECKSUMS_URL" ]; then
        CHECKSUMS_URL="${RELEASE_URL}/checksums.txt"
    fi

    if [ "$MODERN_NOTIFIER_URL" = "$DEFAULT_MODERN_NOTIFIER_URL" ]; then
        MODERN_NOTIFIER_URL="${RELEASE_URL}/ClaudeNotifier.app.zip"
    fi

    echo -e "${BLUE}Release:${NC}  ${PINNED_RELEASE_TAG}"
}

get_file_magic_hex() {
    if command -v od &>/dev/null; then
        od -An -tx1 -N 16 "$1" 2>/dev/null | tr -d ' \n'
    fi
}

get_payload_text_sample() {
    LC_ALL=C head -c 256 "$1" 2>/dev/null | tr '\000' ' ' | tr '\r' '\n'
}

print_unexpected_payload_diagnostics() {
    local file="$1"
    local magic=""
    local sample=""
    local file_desc=""

    [ -f "$file" ] || return 0

    magic=$(get_file_magic_hex "$file")
    sample=$(get_payload_text_sample "$file")

    if command -v file &>/dev/null; then
        file_desc=$(file -b "$file" 2>/dev/null || true)
    fi

    if [ -n "$file_desc" ]; then
        echo -e "${YELLOW}Detected payload:${NC} ${file_desc}" >&2
    fi

    case "$magic" in
        1f8b08*)
            echo -e "${YELLOW}→ Payload looks like gzip-compressed data, not a raw executable. A proxy/CDN may have re-encoded the binary in transit.${NC}" >&2
            return 0
            ;;
        504b0304*|504b0506*|504b0708*)
            echo -e "${YELLOW}→ Payload looks like a ZIP archive instead of the requested raw executable.${NC}" >&2
            return 0
            ;;
        7f454c46*)
            if [ "$PLATFORM" = "darwin" ] || [ "$PLATFORM" = "windows" ]; then
                echo -e "${YELLOW}→ Payload is a Linux ELF executable, which suggests the wrong asset or a bad cache response was returned.${NC}" >&2
            fi
            return 0
            ;;
        4d5a*)
            if [ "$PLATFORM" != "windows" ]; then
                echo -e "${YELLOW}→ Payload is a Windows PE executable, which suggests the wrong asset or a bad cache response was returned.${NC}" >&2
            fi
            return 0
            ;;
        cffaedfe*|cefaedfe*|feedfacf*|cafebabe*)
            if [ "$PLATFORM" != "darwin" ]; then
                echo -e "${YELLOW}→ Payload is a macOS Mach-O executable, which suggests the wrong asset or a bad cache response was returned.${NC}" >&2
            fi
            return 0
            ;;
    esac

    if printf '%s\n' "$sample" | grep -qiE '<!doctype|<html|<head|<body'; then
        echo -e "${YELLOW}→ Payload looks like an HTML page. A proxy/login page/CDN error likely replaced the binary.${NC}" >&2
        return 0
    fi

    if printf '%s\n' "$sample" | grep -qiE '^[[:space:]]*[{[]|\"message\"|\"error\"'; then
        echo -e "${YELLOW}→ Payload looks like JSON/text instead of a raw executable. The release endpoint may be returning an API or error response.${NC}" >&2
        return 0
    fi

    if [ -n "$file_desc" ] && printf '%s\n' "$file_desc" | grep -qiE 'text|ascii|unicode'; then
        echo -e "${YELLOW}→ Payload looks like text instead of a raw executable.${NC}" >&2
    fi
}

# Check if GitHub is accessible
# Returns 0 if accessible, 1 if not (but may set OFFLINE_MODE=true if binary exists)
check_github_availability() {
    OFFLINE_MODE=false

    # Allow tests to skip the real connectivity check
    if [ "${SKIP_CONNECTIVITY_CHECK:-}" = true ]; then
        return 0
    fi

    if command -v curl &> /dev/null; then
        if ! curl -s "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --max-time 10 -I https://github.com &> /dev/null; then
            # Network unavailable - check if we can use existing binary
            if [ -f "$BINARY_PATH" ]; then
                echo -e "${YELLOW}⚠ Cannot reach GitHub, but existing binary found${NC}"
                echo -e "${YELLOW}  Using offline mode with existing installation${NC}"
                OFFLINE_MODE=true
                return 0  # Don't fail - use existing
            fi

            echo -e "${RED}✗ Cannot reach GitHub${NC}" >&2
            echo -e "${YELLOW}Possible issues:${NC}" >&2
            echo -e "  - No internet connection" >&2
            echo -e "  - GitHub is down" >&2
            echo -e "  - Firewall/proxy blocking access" >&2
            echo ""
            echo -e "${YELLOW}Diagnostics:${NC}" >&2
            # Try to diagnose the issue (cross-platform ping)
            local ping_ok=false
            if [ "$PLATFORM" = "windows" ]; then
                # Windows ping: -n count, -w timeout_ms
                ping -n 1 -w 2000 8.8.8.8 &>/dev/null && ping_ok=true
            else
                # Unix ping: -c count, -W timeout (seconds on Linux, ms on macOS - 2 works for both)
                ping -c 1 -W 2 8.8.8.8 &>/dev/null && ping_ok=true
            fi

            if [ "$ping_ok" = false ]; then
                echo -e "  - No network connectivity (ping failed)" >&2
            elif [ "$PLATFORM" != "windows" ] && ! ping -c 1 -W 2 github.com &>/dev/null; then
                echo -e "  - DNS resolution may be failing" >&2
            else
                echo -e "  - GitHub may be blocked by firewall/proxy" >&2
            fi
            return 1
        fi
    fi
    return 0
}

# Check if binary already exists
check_existing() {
    if [ "$FORCE_UPDATE" = true ]; then
        echo -e "${BLUE}🔄 Force update requested, removing old files...${NC}"
        rm -f "$BINARY_PATH" "$SOUND_PREVIEW_PATH" "$LIST_DEVICES_PATH" "$LIST_SOUNDS_PATH" 2>/dev/null
        # Remove symlinks (Unix) and .bat wrappers (Windows)
        rm -f "${SCRIPT_DIR}/claude-notifications" "${SCRIPT_DIR}/sound-preview" "${SCRIPT_DIR}/list-devices" "${SCRIPT_DIR}/list-sounds" 2>/dev/null
        rm -f "${SCRIPT_DIR}/claude-notifications.bat" "${SCRIPT_DIR}/sound-preview.bat" "${SCRIPT_DIR}/list-devices.bat" "${SCRIPT_DIR}/list-sounds.bat" 2>/dev/null
        # Remove macOS apps for clean reinstall
        rm -rf "${SCRIPT_DIR}/terminal-notifier.app" "${SCRIPT_DIR}/ClaudeNotifier.app" "${SCRIPT_DIR}/ClaudeNotifications.app" 2>/dev/null
        rm -f "${SCRIPT_DIR}/README.markdown" 2>/dev/null
        return 1
    fi
    if [ -f "$BINARY_PATH" ]; then
        if windows_native_hooks_update_required; then
            WINDOWS_NATIVE_HOOKS_NEED_UPDATE=true
            echo -e "${YELLOW}⚠ Existing Windows binary cannot generate PowerShell hooks${NC}"
            echo -e "${YELLOW}  Updating ${BINARY_NAME} before rewriting hooks...${NC}"
            return 1
        fi

        echo -e "${GREEN}✓${NC} Binary already installed: ${BOLD}${BINARY_NAME}${NC}"
        echo ""
        return 0
    fi
    return 1
}

# Download a utility binary (sound-preview, list-devices)
download_utility() {
    local util_name="$1"
    local util_path="$2"
    local url="${RELEASE_URL}/${util_name}"

    # Skip if already exists
    if [ -f "$util_path" ]; then
        echo -e "${GREEN}✓${NC} ${util_name} already installed"
        return 0
    fi

    echo -e "${BLUE}📦 Downloading ${util_name}...${NC}"

    if command -v curl &> /dev/null; then
        if curl -fsSL "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --max-time "$CURL_TIMEOUT" "$url" -o "$util_path" 2>/dev/null; then
            if [ -f "$util_path" ] && [ "$(get_file_size "$util_path")" -gt 100000 ]; then
                chmod +x "$util_path" 2>/dev/null || true
                echo -e "${GREEN}✓${NC} ${util_name} downloaded"
                return 0
            fi
        fi
    elif command -v wget &> /dev/null; then
        if wget -q "$url" -O "$util_path" 2>/dev/null; then
            if [ -f "$util_path" ] && [ "$(get_file_size "$util_path")" -gt 100000 ]; then
                chmod +x "$util_path" 2>/dev/null || true
                echo -e "${GREEN}✓${NC} ${util_name} downloaded"
                return 0
            fi
        fi
    fi

    # Not critical - just warn
    rm -f "$util_path" 2>/dev/null
    echo -e "${YELLOW}⚠${NC} Could not download ${util_name} (optional utility)"
    return 1
}

# Download utility binaries (sound-preview, list-devices)
download_utilities() {
    echo ""
    echo -e "${BLUE}📦 Downloading utility binaries...${NC}"

    download_utility "$SOUND_PREVIEW_NAME" "$SOUND_PREVIEW_PATH" || true
    download_utility "$LIST_DEVICES_NAME" "$LIST_DEVICES_PATH" || true
    download_utility "$LIST_SOUNDS_NAME" "$LIST_SOUNDS_PATH" || true

    # Create symlinks for utilities (may fail if downloads failed - that's OK)
    create_utility_symlink "sound-preview" "$SOUND_PREVIEW_NAME" "$SOUND_PREVIEW_PATH" || true
    create_utility_symlink "list-devices" "$LIST_DEVICES_NAME" "$LIST_DEVICES_PATH" || true
    create_utility_symlink "list-sounds" "$LIST_SOUNDS_NAME" "$LIST_SOUNDS_PATH" || true
}

# Create symlink for a utility binary
create_utility_symlink() {
    local util_base="$1"
    local util_name="$2"
    local util_path="$3"

    if [ ! -f "$util_path" ]; then
        return 1
    fi

    local symlink_path="${SCRIPT_DIR}/${util_base}"

    # Remove old symlink if exists
    rm -f "$symlink_path" 2>/dev/null || true

    if [ "$PLATFORM" = "windows" ]; then
        # Windows: create .bat wrapper
        local bat_path="${symlink_path}.bat"
        cat > "$bat_path" << EOF
@echo off
setlocal
set SCRIPT_DIR=%~dp0
"%SCRIPT_DIR%${util_name}" %*
EOF
        return 0
    fi

    # Unix: create symlink
    if ln -s "$util_name" "$symlink_path" 2>/dev/null; then
        return 0
    fi

    # Fallback: copy
    cp "$util_path" "$symlink_path" 2>/dev/null || true
    chmod +x "$symlink_path" 2>/dev/null || true
}

# Download checksums file
download_checksums() {
    echo -e "${BLUE}📝 Downloading checksums...${NC}"

    if command -v curl &> /dev/null; then
        if curl -fsSL "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --max-time "$CURL_TIMEOUT" "$CHECKSUMS_URL" -o "$CHECKSUMS_PATH" 2>/dev/null; then
            return 0
        fi
    elif command -v wget &> /dev/null; then
        if wget -q "$CHECKSUMS_URL" -O "$CHECKSUMS_PATH" 2>/dev/null; then
            return 0
        fi
    fi

    # Checksums optional - just warn
    echo -e "${YELLOW}⚠ Could not download checksums (verification will be skipped)${NC}"
    return 1
}

# Download binary with progress bar
download_binary() {
    local url="${RELEASE_URL}/${BINARY_NAME}"
    local error_log="${TMPDIR:-${TEMP:-/tmp}}/install-error-$$.log"
    local http_code=""
    local curl_exit_code=0
    local curl_error=""
    local should_retry=false

    echo -e "${BLUE}📦 Downloading ${BOLD}${BINARY_NAME}${NC}${BLUE}...${NC}"
    echo -e "${BLUE}   From: ${url}${NC}"
    echo ""

    # Try curl first (with progress bar)
    if command -v curl &> /dev/null; then
        # Use a progress bar only for the first attempt; retry failures with clean stderr.
        http_code=$(curl -w "%{http_code}" -fL "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --progress-bar --max-time "$CURL_TIMEOUT" \
            "$url" -o "$BINARY_PATH" 2>"$error_log") || curl_exit_code=$?

        if [ "$curl_exit_code" -eq 0 ] && [ -f "$BINARY_PATH" ] && [ "$(get_file_size "$BINARY_PATH")" -gt 100000 ]; then
            rm -f "$error_log"
            echo ""
            return 0
        fi

        # Analyze failure
        rm -f "$BINARY_PATH"
        if [ -f "$error_log" ]; then
            curl_error=$(<"$error_log")
        fi

        case "$curl_exit_code" in
            5|6|7|18|28|35|52|56|60|77|92)
                should_retry=true
                ;;
        esac

        if [ "$http_code" = "000" ] || [ -z "$curl_error" ]; then
            should_retry=true
        fi

        if [ "$should_retry" = true ]; then
            echo -e "${YELLOW}  Retrying once with compatibility mode...${NC}"
            rm -f "$error_log" "$BINARY_PATH"

            curl_exit_code=0
            http_code=$(curl -w "%{http_code}" -fL "${CURL_EXTRA_OPTS[@]}" "${CURL_COMPAT_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" -sS --max-time "$CURL_TIMEOUT" \
                "$url" -o "$BINARY_PATH" 2>"$error_log") || curl_exit_code=$?

            if [ "$curl_exit_code" -eq 0 ] && [ -f "$BINARY_PATH" ] && [ "$(get_file_size "$BINARY_PATH")" -gt 100000 ]; then
                rm -f "$error_log"
                echo ""
                return 0
            fi

            rm -f "$BINARY_PATH"
            if [ -f "$error_log" ]; then
                curl_error=$(<"$error_log")
            fi
        fi

        rm -f "$error_log"

        echo ""
        if [ "$http_code" = "404" ]; then
            echo -e "${RED}✗ Binary not found (404)${NC}" >&2
            echo ""
            echo -e "${YELLOW}This usually means the release is still building.${NC}" >&2
            echo -e "${YELLOW}Check build status at:${NC}" >&2
            echo -e "  https://github.com/${REPO}/actions" >&2
            echo ""
            echo -e "${YELLOW}Wait a few minutes and try again.${NC}" >&2
        elif [ "$http_code" = "407" ]; then
            echo -e "${RED}✗ Proxy authentication required (407)${NC}" >&2
            print_windows_proxy_hint
        elif [ "$http_code" = "000" ] || [ -z "$http_code" ]; then
            echo -e "${RED}✗ Download failed before an HTTP response was received${NC}" >&2
            echo -e "${YELLOW}Error details:${NC}" >&2
            if ! print_download_error_details "$curl_error"; then
                echo -e "${YELLOW}(curl did not return any stderr output)${NC}" >&2
            fi
            print_curl_failure_guidance "$http_code" "$curl_exit_code" "$curl_error"
        elif echo "$http_code" | grep -qE "^5[0-9]{2}"; then
            echo -e "${RED}✗ GitHub server error (${http_code})${NC}" >&2
            echo -e "${YELLOW}GitHub may be experiencing issues. Try again later.${NC}" >&2
        elif [ -n "$curl_error" ]; then
            echo -e "${RED}✗ Download failed${NC}" >&2
            echo -e "${YELLOW}Error details:${NC}" >&2
            if ! print_download_error_details "$curl_error"; then
                echo -e "${YELLOW}(curl did not return any stderr output)${NC}" >&2
            fi
            print_curl_failure_guidance "$http_code" "$curl_exit_code" "$curl_error"
        else
            echo -e "${RED}✗ Download failed (HTTP ${http_code})${NC}" >&2
            echo -e "${YELLOW}Check your internet connection and try again.${NC}" >&2
        fi
        return 1

    # Fallback to wget
    elif command -v wget &> /dev/null; then
        # Capture wget errors
        if wget --show-progress --timeout=$WGET_TIMEOUT "$url" -O "$BINARY_PATH" 2>"$error_log"; then
            if [ -f "$BINARY_PATH" ] && [ "$(get_file_size "$BINARY_PATH")" -gt 100000 ]; then
                rm -f "$error_log"
                echo ""
                return 0
            fi
        fi

        local wget_error=$(cat "$error_log" 2>/dev/null)
        rm -f "$BINARY_PATH" "$error_log"

        echo ""
        echo -e "${RED}✗ Download failed${NC}" >&2
        if [ -n "$wget_error" ]; then
            echo -e "${YELLOW}Error details:${NC}" >&2
            if ! print_download_error_details "$wget_error"; then
                echo -e "${YELLOW}(wget did not return any stderr output)${NC}" >&2
            fi
        fi
        return 1

    else
        echo -e "${RED}✗ Error: curl or wget required for installation${NC}" >&2
        echo -e "${YELLOW}Please install curl or wget and try again${NC}" >&2
        return 1
    fi
}

# Verify checksum
verify_checksum() {
    if [ ! -f "$CHECKSUMS_PATH" ]; then
        echo -e "${YELLOW}⚠ Skipping checksum verification (checksums.txt not available)${NC}"
        return 0
    fi

    echo -e "${BLUE}🔒 Verifying checksum...${NC}"

    # Extract expected checksum for our binary
    local expected_sum=$(grep "$BINARY_NAME" "$CHECKSUMS_PATH" 2>/dev/null | awk '{print $1}')

    if [ -z "$expected_sum" ]; then
        echo -e "${YELLOW}⚠ Checksum not found for ${BINARY_NAME} (skipping)${NC}"
        return 0
    fi

    # Calculate actual checksum
    # Note: On Windows (MSYS2/Git Bash/Cygwin), sha256sum prefixes output with \
    # when the file path contains backslashes. awk sub() strips this prefix.
    # This is safe because SHA-256 hashes are hex-only [0-9a-f] and never contain \.
    local actual_sum=""
    if command -v shasum &> /dev/null; then
        actual_sum=$(shasum -a 256 "$BINARY_PATH" 2>/dev/null | awk '{sub(/^\\/, "", $1); print $1}')
    elif command -v sha256sum &> /dev/null; then
        actual_sum=$(sha256sum "$BINARY_PATH" 2>/dev/null | awk '{sub(/^\\/, "", $1); print $1}')
    else
        echo -e "${YELLOW}⚠ sha256sum not available (skipping checksum)${NC}"
        return 0
    fi

    if [ "$expected_sum" = "$actual_sum" ]; then
        echo -e "${GREEN}✓ Checksum verified${NC}"
        return 0
    else
        echo -e "${RED}✗ Checksum mismatch!${NC}" >&2
        echo -e "${RED}  Expected: ${expected_sum}${NC}" >&2
        echo -e "${RED}  Got:      ${actual_sum}${NC}" >&2
        print_unexpected_payload_diagnostics "$BINARY_PATH"
        echo -e "${YELLOW}The downloaded file may be corrupted. Try again.${NC}" >&2
        rm -f "$BINARY_PATH"
        return 1
    fi
}

# Verify downloaded binary
verify_binary() {
    if [ ! -f "$BINARY_PATH" ]; then
        echo -e "${RED}✗ Binary file not found after download${NC}" >&2
        return 1
    fi

    local size=$(get_file_size "$BINARY_PATH")

    # Check minimum size (1MB)
    if [ "$size" -lt 1000000 ]; then
        echo -e "${RED}✗ Downloaded file too small (${size} bytes)${NC}" >&2
        echo -e "${YELLOW}This might be an error page. Check your internet connection.${NC}" >&2
        print_unexpected_payload_diagnostics "$BINARY_PATH"
        rm -f "$BINARY_PATH"
        return 1
    fi

    echo -e "${GREEN}✓ Size check passed${NC} (${size} bytes)"

    # Verify checksum
    if ! verify_checksum; then
        return 1
    fi

    return 0
}

# Download and verify the main binary. Retry when verification fails after a
# nominally successful download, which can happen when a proxy/CDN returns an
# unexpected payload with HTTP 200.
download_and_verify_binary() {
    local attempt=1

    while [ $attempt -le $MAX_RETRIES ]; do
        if [ $attempt -gt 1 ]; then
            echo -e "${YELLOW}Retry ${attempt}/${MAX_RETRIES} after ${RETRY_DELAY}s (fresh download)...${NC}"
            sleep $RETRY_DELAY
        fi

        download_checksums || true

        if ! download_binary; then
            return 1
        fi

        if verify_binary; then
            return 0
        fi

        rm -f "$BINARY_PATH"

        if [ $attempt -lt $MAX_RETRIES ]; then
            echo -e "${YELLOW}⚠ Verification failed, retrying with a fresh download...${NC}"
            echo ""
        fi

        attempt=$((attempt + 1))
    done

    return 1
}

# Verify binary actually executes
verify_executable() {
    echo -e "${BLUE}🔍 Verifying binary executes...${NC}"

    # Make executable first
    chmod +x "$BINARY_PATH" 2>/dev/null || true

    # Try to run --version (should return 0 and output version info)
    local output
    output=$("$BINARY_PATH" --version 2>&1)
    local exit_code=$?

    if [ $exit_code -ne 0 ]; then
        echo -e "${RED}✗ Binary failed to execute (exit code: ${exit_code})${NC}" >&2
        echo -e "${RED}  Output: ${output}${NC}" >&2
        echo -e "${YELLOW}The downloaded file may be corrupted or incompatible.${NC}" >&2
        rm -f "$BINARY_PATH"
        return 1
    fi

    # Verify output contains expected string
    if ! echo "$output" | grep -qiE "claude-notifications|version"; then
        echo -e "${RED}✗ Binary output unexpected${NC}" >&2
        echo -e "${RED}  Output: ${output}${NC}" >&2
        echo -e "${YELLOW}This doesn't appear to be the correct binary.${NC}" >&2
        rm -f "$BINARY_PATH"
        return 1
    fi

    echo -e "${GREEN}✓ Binary executes correctly${NC}"
    return 0
}

# Make binary executable
make_executable() {
    chmod +x "$BINARY_PATH" 2>/dev/null || true
}

windows_hooks_path() {
    local plugin_root
    plugin_root="$(cd "$SCRIPT_DIR/.." 2>/dev/null && pwd)" || return 1
    printf '%s\n' "${plugin_root}/hooks/hooks.json"
}

windows_native_hooks_json() {
    [ "$PLATFORM" = "windows" ] || return 1
    [ -f "$BINARY_PATH" ] || return 1

    local exe_path="$BINARY_PATH"
    if command -v cygpath >/dev/null 2>&1; then
        exe_path="$(cygpath -w "$BINARY_PATH" 2>/dev/null || printf '%s' "$BINARY_PATH")"
    fi

    "$BINARY_PATH" windows-hooks --exe "$exe_path"
}

windows_native_hooks_supported() {
    local hooks_json
    hooks_json="$(windows_native_hooks_json 2>/dev/null)" || return 1
    printf '%s\n' "$hooks_json" | grep -qE '"shell"[[:space:]]*:[[:space:]]*"powershell"'
}

windows_native_hooks_update_required() {
    [ "$PLATFORM" = "windows" ] || return 1

    local hooks_path
    hooks_path="$(windows_hooks_path)" || return 1
    [ -f "$hooks_path" ] || return 1

    ! windows_native_hooks_supported
}

# Create symlink for hooks
create_symlink() {
    # On Windows, create a .bat wrapper instead of symlink
    if [ "$PLATFORM" = "windows" ]; then
        local bat_path="${SCRIPT_DIR}/claude-notifications.bat"

        # Remove old .bat file if exists
        rm -f "$bat_path" 2>/dev/null || true

        # Create .bat wrapper that calls the platform-specific binary
        cat > "$bat_path" << EOF
@echo off
REM claude-notifications Windows wrapper
REM Automatically runs the platform-specific binary

setlocal
set SCRIPT_DIR=%~dp0
"%SCRIPT_DIR%${BINARY_NAME}" %*
EOF

        if [ -f "$bat_path" ]; then
            echo -e "${GREEN}✓ Created wrapper${NC} claude-notifications.bat → ${BINARY_NAME}"
            return 0
        else
            echo -e "${YELLOW}⚠ Could not create .bat wrapper (hooks may not work)${NC}"
            return 1
        fi
    fi

    # Unix: create symlink or copy
    local symlink_path="${SCRIPT_DIR}/claude-notifications"

    # Remove old symlink if exists
    rm -f "$symlink_path" 2>/dev/null || true

    # Create symlink pointing to platform-specific binary
    if ln -s "$BINARY_NAME" "$symlink_path" 2>/dev/null; then
        echo -e "${GREEN}✓ Created symlink${NC} claude-notifications → ${BINARY_NAME}"
        return 0
    else
        # Fallback: copy if symlink fails (some systems don't support symlinks)
        if cp "$BINARY_PATH" "$symlink_path" 2>/dev/null; then
            chmod +x "$symlink_path" 2>/dev/null || true
            echo -e "${GREEN}✓ Created copy${NC} claude-notifications (symlink not supported)"
            return 0
        fi

        echo -e "${YELLOW}⚠ Could not create symlink/copy (hooks may not work)${NC}"
        return 1
    fi
}

configure_windows_native_hooks() {
    [ "$PLATFORM" = "windows" ] || return 0
    [ -f "$BINARY_PATH" ] || return 0

    local hooks_path
    hooks_path="$(windows_hooks_path)" || return 0
    [ -f "$hooks_path" ] || return 0

    local hooks_json
    if ! hooks_json="$(windows_native_hooks_json 2>/dev/null)"; then
        echo -e "${YELLOW}⚠ Could not generate Windows PowerShell hooks${NC}"
        return 0
    fi

    if ! printf '%s\n' "$hooks_json" | grep -qE '"shell"[[:space:]]*:[[:space:]]*"powershell"'; then
        echo -e "${YELLOW}⚠ Generated Windows hooks did not include PowerShell shell setting${NC}"
        return 0
    fi

    local tmp_hooks="${hooks_path}.tmp.$$"
    if printf '%s\n' "$hooks_json" > "$tmp_hooks" 2>/dev/null && mv "$tmp_hooks" "$hooks_path" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} Windows PowerShell hooks configured"
        echo -e "${YELLOW}  Restart Claude Code to apply the Windows hook update.${NC}"
    else
        rm -f "$tmp_hooks" 2>/dev/null || true
        echo -e "${YELLOW}⚠ Could not write Windows PowerShell hooks${NC}"
    fi

    return 0
}

# Cleanup temporary files
cleanup() {
    rm -f "$CHECKSUMS_PATH" 2>/dev/null || true
}

# Download ClaudeNotifier for macOS (modern UNUserNotificationCenter, works on M4 Sequoia)
download_terminal_notifier_modern() {
    local MODERN_APP="${SCRIPT_DIR}/ClaudeNotifier.app"
    local MODERN_URL="${MODERN_NOTIFIER_URL:-${RELEASE_URL}/ClaudeNotifier.app.zip}"
    local TEMP_ZIP="${TMPDIR:-${TEMP:-/tmp}}/ClaudeNotifier-$$.zip"

    # Check if already installed
    if [ -d "$MODERN_APP" ] && [ -x "$MODERN_APP/Contents/MacOS/terminal-notifier-modern" ]; then
        echo -e "${GREEN}✓${NC} ClaudeNotifier already installed"
        return 0
    fi

    echo ""
    echo -e "${BLUE}📦 Installing ClaudeNotifier (modern notifications + click-to-focus)...${NC}"

    local attempt=1
    local downloaded=false

    while [ $attempt -le $MAX_RETRIES ]; do
        if [ $attempt -gt 1 ]; then
            echo -e "${YELLOW}Retry ${attempt}/${MAX_RETRIES}...${NC}"
            sleep $RETRY_DELAY
        fi

        if command -v curl &>/dev/null; then
            if curl -fsSL "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --max-time "$CURL_TIMEOUT" "$MODERN_URL" -o "$TEMP_ZIP" 2>/dev/null; then
                downloaded=true
                break
            fi
        elif command -v wget &>/dev/null; then
            if wget -q --timeout=$WGET_TIMEOUT "$MODERN_URL" -O "$TEMP_ZIP" 2>/dev/null; then
                downloaded=true
                break
            fi
        fi

        attempt=$((attempt + 1))
    done

    if [ "$downloaded" != true ]; then
        echo -e "${YELLOW}⚠ Could not download ClaudeNotifier, falling back to legacy${NC}"
        rm -f "$TEMP_ZIP" 2>/dev/null
        return 1
    fi

    # Verify zip
    if ! unzip -t "$TEMP_ZIP" &>/dev/null; then
        echo -e "${YELLOW}⚠ Downloaded file is not a valid zip, falling back to legacy${NC}"
        rm -f "$TEMP_ZIP"
        return 1
    fi

    # Extract
    if ! unzip -o -q "$TEMP_ZIP" -d "${SCRIPT_DIR}/" 2>&1; then
        echo -e "${YELLOW}⚠ Could not extract ClaudeNotifier${NC}"
        rm -f "$TEMP_ZIP"
        return 1
    fi

    rm -f "$TEMP_ZIP"

    # Verify extraction
    if [ -d "$MODERN_APP" ] && [ -x "$MODERN_APP/Contents/MacOS/terminal-notifier-modern" ]; then
        # Remove quarantine attribute (downloaded files are flagged by Gatekeeper)
        xattr -cr "$MODERN_APP" 2>/dev/null || true
        # Verify code signature (notarized builds have valid Developer ID signature)
        if codesign --verify --verbose "$MODERN_APP" 2>/dev/null; then
            echo -e "${GREEN}✓${NC} Code signature verified"
        else
            echo -e "${YELLOW}⚠${NC} Code signature verification failed (app may still work)"
        fi
        # Register with Launch Services
        /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$MODERN_APP" 2>/dev/null || true
        echo -e "${GREEN}✓${NC} ClaudeNotifier installed (modern notifications + click-to-focus)"
        return 0
    else
        echo -e "${YELLOW}⚠ ClaudeNotifier extraction incomplete, falling back to legacy${NC}"
        rm -rf "$MODERN_APP" 2>/dev/null
        return 1
    fi
}

# Download terminal-notifier for macOS (legacy, enables click-to-focus)
download_terminal_notifier() {
    local NOTIFIER_URL="${NOTIFIER_URL:-https://github.com/julienXX/terminal-notifier/releases/download/2.0.0/terminal-notifier-2.0.0.zip}"
    local NOTIFIER_APP="${SCRIPT_DIR}/terminal-notifier.app"
    local TEMP_ZIP="${TMPDIR:-${TEMP:-/tmp}}/terminal-notifier-$$.zip"

    # Check if already installed
    if [ -d "$NOTIFIER_APP" ]; then
        echo -e "${GREEN}✓${NC} terminal-notifier already installed"
        return 0
    fi

    echo ""
    echo -e "${BLUE}📦 Installing terminal-notifier (click-to-focus support)...${NC}"

    # Download with retry
    local attempt=1
    local downloaded=false

    while [ $attempt -le $MAX_RETRIES ]; do
        if [ $attempt -gt 1 ]; then
            echo -e "${YELLOW}Retry ${attempt}/${MAX_RETRIES}...${NC}"
            sleep $RETRY_DELAY
        fi

        if command -v curl &>/dev/null; then
            if curl -fsSL "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --max-time "$CURL_TIMEOUT" "$NOTIFIER_URL" -o "$TEMP_ZIP" 2>/dev/null; then
                downloaded=true
                break
            fi
        elif command -v wget &>/dev/null; then
            if wget -q --timeout=$WGET_TIMEOUT "$NOTIFIER_URL" -O "$TEMP_ZIP" 2>/dev/null; then
                downloaded=true
                break
            fi
        fi

        attempt=$((attempt + 1))
    done

    if [ "$downloaded" != true ]; then
        echo -e "${YELLOW}⚠ Could not download terminal-notifier (click-to-focus will be disabled)${NC}"
        rm -f "$TEMP_ZIP" 2>/dev/null
        return 1
    fi

    # Verify zip file is valid before extracting
    if ! unzip -t "$TEMP_ZIP" &>/dev/null; then
        echo -e "${YELLOW}⚠ Downloaded file is not a valid zip (click-to-focus will be disabled)${NC}"
        rm -f "$TEMP_ZIP"
        return 1
    fi

    # Extract (-o to overwrite without prompting)
    if ! unzip -o -q "$TEMP_ZIP" -d "${SCRIPT_DIR}/" 2>&1; then
        echo -e "${YELLOW}⚠ Could not extract terminal-notifier${NC}"
        rm -f "$TEMP_ZIP"
        return 1
    fi

    # Cleanup
    rm -f "$TEMP_ZIP"

    # Verify extraction
    if [ -d "$NOTIFIER_APP" ] && [ -x "$NOTIFIER_APP/Contents/MacOS/terminal-notifier" ]; then
        echo -e "${GREEN}✓${NC} terminal-notifier installed (click-to-focus enabled)"
        return 0
    else
        echo -e "${YELLOW}⚠ terminal-notifier extraction incomplete${NC}"
        rm -rf "$NOTIFIER_APP" 2>/dev/null
        return 1
    fi
}

# Create ClaudeNotifications.app for custom notification icon
create_claude_notifications_app() {
    local APP_DIR="${SCRIPT_DIR}/ClaudeNotifications.app"
    local PLUGIN_ROOT="$(dirname "$SCRIPT_DIR")"
    local ICON_SRC="${PLUGIN_ROOT}/claude_icon.png"

    # Check if already created
    if [ -d "$APP_DIR" ]; then
        echo -e "${GREEN}✓${NC} ClaudeNotifications.app already exists"
        return 0
    fi

    # Check if icon exists
    if [ ! -f "$ICON_SRC" ]; then
        echo -e "${YELLOW}⚠ Claude icon not found at ${ICON_SRC}${NC}"
        return 1
    fi

    echo -e "${BLUE}🎨 Creating ClaudeNotifications.app (notification icon)...${NC}"

    # Create app structure
    mkdir -p "$APP_DIR/Contents/MacOS"
    mkdir -p "$APP_DIR/Contents/Resources"

    # Create iconset from PNG
    local ICONSET_DIR="${TMPDIR:-${TEMP:-/tmp}}/claude-$$.iconset"
    mkdir -p "$ICONSET_DIR"

    # Generate different icon sizes (silence sips stdout/stderr)
    sips -z 16 16 "$ICON_SRC" --out "$ICONSET_DIR/icon_16x16.png" >/dev/null 2>&1
    sips -z 32 32 "$ICON_SRC" --out "$ICONSET_DIR/icon_16x16@2x.png" >/dev/null 2>&1
    sips -z 32 32 "$ICON_SRC" --out "$ICONSET_DIR/icon_32x32.png" >/dev/null 2>&1
    sips -z 64 64 "$ICON_SRC" --out "$ICONSET_DIR/icon_32x32@2x.png" >/dev/null 2>&1
    sips -z 128 128 "$ICON_SRC" --out "$ICONSET_DIR/icon_128x128.png" >/dev/null 2>&1
    sips -z 256 256 "$ICON_SRC" --out "$ICONSET_DIR/icon_128x128@2x.png" >/dev/null 2>&1
    sips -z 256 256 "$ICON_SRC" --out "$ICONSET_DIR/icon_256x256.png" >/dev/null 2>&1
    sips -z 512 512 "$ICON_SRC" --out "$ICONSET_DIR/icon_256x256@2x.png" >/dev/null 2>&1
    cp "$ICON_SRC" "$ICONSET_DIR/icon_512x512.png" >/dev/null 2>&1

    # Convert to icns
    if ! iconutil -c icns "$ICONSET_DIR" -o "$APP_DIR/Contents/Resources/AppIcon.icns" 2>/dev/null; then
        echo -e "${YELLOW}⚠ Could not create app icon${NC}"
        rm -rf "$ICONSET_DIR" "$APP_DIR"
        return 1
    fi

    rm -rf "$ICONSET_DIR"

    # Create Info.plist
    cat > "$APP_DIR/Contents/Info.plist" << 'PLIST_EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>claude-notify</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundleIdentifier</key>
    <string>com.claude.notifications</string>
    <key>CFBundleName</key>
    <string>Claude Notifications</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleVersion</key>
    <string>1.0</string>
    <key>LSUIElement</key>
    <true/>
</dict>
</plist>
PLIST_EOF

    # Create minimal executable
    cat > "$APP_DIR/Contents/MacOS/claude-notify" << 'EXEC_EOF'
#!/bin/bash
exit 0
EXEC_EOF
    chmod +x "$APP_DIR/Contents/MacOS/claude-notify"

    # Register with Launch Services
    /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$APP_DIR" 2>/dev/null || true

    echo -e "${GREEN}✓${NC} ClaudeNotifications.app created (Claude icon in notifications)"
    return 0
}

# Set up iTerm2 Python API venv for tmux -CC click-to-focus (macOS only)
setup_iterm2_venv() {
    # Only relevant on macOS
    [ "$(uname -s)" = "Darwin" ] || return 0

    # Check if user has iTerm2 (current terminal or installed)
    local is_iterm=false
    [ "${TERM_PROGRAM:-}" = "iTerm.app" ] && is_iterm=true
    [ "${__CFBundleIdentifier:-}" = "com.googlecode.iterm2" ] && is_iterm=true
    [ "$is_iterm" = false ] && [ -d "/Applications/iTerm.app" ] && is_iterm=true
    [ "$is_iterm" = true ] || return 0

    # tmux must be installed
    command -v tmux &>/dev/null || return 0

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
        echo -e "${YELLOW}    Install Python 3 and re-run installer to enable.${NC}"
        return 0
    fi

    echo ""
    echo -e "${BLUE}  Setting up iTerm2 tmux -CC support...${NC}"

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

# Install GNOME activate-window-by-title extension for Linux click-to-focus
install_gnome_activate_window_extension() {
    local EXTENSION_ID=5021
    local EXTENSION_UUID="activate-window-by-title@lucaswerkmeister.de"

    # Check if GNOME Shell is available
    if ! command -v gnome-shell &>/dev/null; then
        echo -e "${YELLOW}⚠ Skipping GNOME extension (gnome-shell not found)${NC}"
        return 1
    fi

    # Check if gnome-extensions CLI exists
    if ! command -v gnome-extensions &>/dev/null; then
        echo -e "${YELLOW}⚠ Skipping GNOME extension (gnome-extensions not found)${NC}"
        return 1
    fi

    # Check if extension is already enabled
    if gnome-extensions list --enabled 2>/dev/null | grep -q "$EXTENSION_UUID"; then
        echo -e "${GREEN}✓${NC} GNOME activate-window extension already enabled"
        return 0
    fi

    echo ""
    echo -e "${BLUE}📦 Installing GNOME activate-window extension (click-to-focus support)...${NC}"

    # Get GNOME Shell version
    local gnome_version
    gnome_version=$(gnome-shell --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' || true)

    if [ -z "$gnome_version" ]; then
        echo -e "${YELLOW}⚠ Could not detect GNOME Shell version (click-to-focus will be disabled)${NC}"
        return 1
    fi

    echo -e "${BLUE}   GNOME Shell version: ${gnome_version}${NC}"

    # Query extensions.gnome.org for download URL
    local ext_info=""
    local attempt=1

    while [ $attempt -le $MAX_RETRIES ]; do
        if [ $attempt -gt 1 ]; then
            echo -e "${YELLOW}Retry ${attempt}/${MAX_RETRIES}...${NC}"
            sleep $RETRY_DELAY
        fi

        if command -v curl &>/dev/null; then
            ext_info=$(curl -sf "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --max-time "$CURL_TIMEOUT" \
                "https://extensions.gnome.org/extension-info/?pk=${EXTENSION_ID}&shell_version=${gnome_version}" 2>/dev/null) && break
        elif command -v wget &>/dev/null; then
            ext_info=$(wget -q --timeout=$WGET_TIMEOUT -O - \
                "https://extensions.gnome.org/extension-info/?pk=${EXTENSION_ID}&shell_version=${gnome_version}" 2>/dev/null) && break
        fi

        attempt=$((attempt + 1))
    done

    if [ -z "$ext_info" ]; then
        echo -e "${YELLOW}⚠ Could not fetch extension info from extensions.gnome.org${NC}"
        return 1
    fi

    # Extract download URL from JSON response
    local download_url
    download_url=$(echo "$ext_info" | sed -n 's/.*"download_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' || true)

    if [ -z "$download_url" ]; then
        echo -e "${YELLOW}⚠ No compatible extension version found for GNOME Shell ${gnome_version}${NC}"
        return 1
    fi

    # Download extension zip
    local temp_zip="${TMPDIR:-${TEMP:-/tmp}}/gnome-ext-$$.zip"

    echo -e "${BLUE}   Downloading extension...${NC}"

    local downloaded=false
    attempt=1

    while [ $attempt -le $MAX_RETRIES ]; do
        if [ $attempt -gt 1 ]; then
            echo -e "${YELLOW}Retry ${attempt}/${MAX_RETRIES}...${NC}"
            sleep $RETRY_DELAY
        fi

        if command -v curl &>/dev/null; then
            if curl -fsSL "${CURL_EXTRA_OPTS[@]}" --connect-timeout "$CONNECT_TIMEOUT" --max-time "$CURL_TIMEOUT" "https://extensions.gnome.org${download_url}" -o "$temp_zip" 2>/dev/null; then
                downloaded=true
                break
            fi
        elif command -v wget &>/dev/null; then
            if wget -q --timeout=$WGET_TIMEOUT "https://extensions.gnome.org${download_url}" -O "$temp_zip" 2>/dev/null; then
                downloaded=true
                break
            fi
        fi

        attempt=$((attempt + 1))
    done

    if [ "$downloaded" != true ]; then
        echo -e "${YELLOW}⚠ Could not download extension (click-to-focus will be disabled)${NC}"
        rm -f "$temp_zip" 2>/dev/null
        return 1
    fi

    # Install using gnome-extensions
    echo -e "${BLUE}   Installing extension...${NC}"

    if ! gnome-extensions install --force "$temp_zip" 2>/dev/null; then
        echo -e "${YELLOW}⚠ Could not install extension${NC}"
        rm -f "$temp_zip"
        return 1
    fi

    rm -f "$temp_zip"

    # Enable the extension
    echo -e "${BLUE}   Enabling extension...${NC}"

    if gnome-extensions enable "$EXTENSION_UUID" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} GNOME activate-window extension installed and enabled (click-to-focus)"
        return 0
    else
        echo -e "${YELLOW}⚠ Extension installed but could not be enabled immediately${NC}"
        echo -e "${YELLOW}  You may need to log out and log back in, then run:${NC}"
        echo -e "${YELLOW}  gnome-extensions enable ${EXTENSION_UUID}${NC}"
        return 1
    fi
}

# Install a hidden desktop entry used for GNOME/Wayland notifications.
# StartupNotify=false prevents GNOME Shell from creating an activation token
# that would otherwise leave a loading cursor spinning after our daemon has
# already focused the target window.
install_linux_notification_desktop_entry() {
    [ "$PLATFORM" = "linux" ] || return 0

    local data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
    local applications_dir="${data_home}/applications"
    local desktop_file="${applications_dir}/claude-notifications.desktop"
    local tmp_file="${desktop_file}.tmp.$$"

    if ! mkdir -p "$applications_dir" 2>/dev/null; then
        echo -e "${YELLOW}⚠ Could not create ${applications_dir} for notification desktop entry${NC}"
        return 1
    fi

    cat > "$tmp_file" << EOF
[Desktop Entry]
Name=Claude Notifications
Type=Application
Icon=utilities-terminal
Exec=/usr/bin/true
NoDisplay=true
StartupNotify=false
Terminal=false
EOF

    if mv "$tmp_file" "$desktop_file" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} Hidden desktop entry installed for GNOME/Wayland notifications"
        return 0
    fi

    rm -f "$tmp_file" 2>/dev/null || true
    echo -e "${YELLOW}⚠ Could not install hidden notification desktop entry${NC}"
    return 1
}

# Main installation flow
main() {
    echo ""
    echo -e "${BOLD}========================================${NC}"
    echo -e "${BOLD} Claude Notifications - Binary Setup${NC}"
    echo -e "${BOLD}========================================${NC}"
    echo ""

    # Pre-flight checks
    if ! check_required_tools; then
        exit 1
    fi

    if ! check_write_permissions; then
        exit 1
    fi

    if ! acquire_lock; then
        exit 1
    fi

    # Detect platform
    detect_platform
    echo -e "${BLUE}Platform:${NC} ${PLATFORM}-${ARCH}"
    echo -e "${BLUE}Binary:${NC}   ${BINARY_NAME}"
    echo ""

    # When force-updating, verify GitHub is reachable BEFORE deleting anything.
    # Otherwise a network outage leaves the user with no binary at all.
    if [ "$FORCE_UPDATE" = true ]; then
        if ! check_github_availability; then
            echo ""
            echo -e "${YELLOW}⚠ Keeping existing installation (GitHub unreachable)${NC}"
            return 0
        fi
    fi

    # Check if already installed
    if check_existing; then
        # Even if binary exists, ensure symlink is created
        create_symlink
        configure_windows_native_hooks

        # Download utility binaries (sound-preview, list-devices)
        download_utilities

        # On macOS, also check ClaudeNotifier (preferred) or legacy terminal-notifier
        if [ "$PLATFORM" = "darwin" ]; then
            download_terminal_notifier_modern || download_terminal_notifier
            # Icon app is optional - don't fail if icon not found
            create_claude_notifications_app || true
            # Set up iTerm2 Python API venv for tmux -CC click-to-focus
            setup_iterm2_venv || true
        fi

        # On Linux, also check GNOME activate-window extension
        if [ "$PLATFORM" = "linux" ]; then
            install_linux_notification_desktop_entry || true
            if install_gnome_activate_window_extension; then
                GNOME_EXT_INSTALLED=true
            fi
        fi

        echo -e "${GREEN}✓ Setup complete${NC}"
        echo ""
        return 0
    fi

    # Check GitHub availability (may set OFFLINE_MODE=true if binary exists)
    if ! check_github_availability; then
        echo ""
        exit 1
    fi

    # Handle offline mode - use existing binary without downloading
    if [ "$OFFLINE_MODE" = true ]; then
        echo ""
        echo -e "${YELLOW}Running in offline mode...${NC}"

        if [ "$WINDOWS_NATIVE_HOOKS_NEED_UPDATE" = true ]; then
            echo -e "${RED}✗ Existing Windows binary is too old for PowerShell hooks${NC}" >&2
            echo -e "${YELLOW}Restore network access and rerun the installer to download a compatible binary.${NC}" >&2
            exit 1
        fi

        # Verify existing binary still works
        if ! verify_executable; then
            echo -e "${RED}✗ Existing binary is corrupted or incompatible${NC}" >&2
            echo -e "${YELLOW}Please restore network access to download a fresh binary.${NC}" >&2
            exit 1
        fi

        # Ensure symlink exists
        create_symlink
        configure_windows_native_hooks

        echo ""
        echo -e "${GREEN}========================================${NC}"
        echo -e "${GREEN}✓ Offline Setup Complete${NC}"
        echo -e "${GREEN}========================================${NC}"
        echo ""
        echo -e "${YELLOW}Note: Running with cached binary (no updates)${NC}"
        echo -e "${YELLOW}Restore network access for full installation.${NC}"
        echo ""
        return 0
    fi

    pin_release_urls

    # Download and verify the main binary.
    if ! download_and_verify_binary; then
        cleanup
        echo ""
        echo -e "${RED}========================================${NC}"
        echo -e "${RED} Installation Failed${NC}"
        echo -e "${RED}========================================${NC}"
        echo ""
        echo -e "${YELLOW}Additional troubleshooting:${NC}"
        echo -e "  1. Wait a few minutes if release is building"
        echo -e "  2. Check: https://github.com/${REPO}/releases"
        echo -e "  3. Manual download: https://github.com/${REPO}/releases/latest"
        if [ "$PLATFORM" = "windows" ]; then
            echo -e "  4. Check proxy / TLS inspection settings in Git Bash or your corporate network"
        fi
        echo ""
        exit 1
    fi

    # Verify binary actually executes
    if ! verify_executable; then
        cleanup
        echo ""
        echo -e "${RED}========================================${NC}"
        echo -e "${RED} Binary Execution Failed${NC}"
        echo -e "${RED}========================================${NC}"
        echo ""
        echo -e "${YELLOW}Possible causes:${NC}"
        echo -e "  - Wrong architecture (try on different machine)"
        echo -e "  - Missing system libraries"
        echo -e "  - Corrupted download"
        echo ""
        exit 1
    fi

    # Make executable (already done in verify_executable, but ensure)
    make_executable

    # Create symlink for hooks to use
    create_symlink
    configure_windows_native_hooks

    # Download utility binaries (sound-preview, list-devices)
    download_utilities

    # On macOS, download ClaudeNotifier (preferred) or legacy terminal-notifier
    if [ "$PLATFORM" = "darwin" ]; then
        download_terminal_notifier_modern || download_terminal_notifier
        # Icon app is optional - don't fail if icon not found
        create_claude_notifications_app || true
        # Set up iTerm2 Python API venv for tmux -CC click-to-focus
        setup_iterm2_venv || true
    fi

    # On Linux, install GNOME activate-window-by-title extension for click-to-focus
    GNOME_EXT_INSTALLED=false
    if [ "$PLATFORM" = "linux" ]; then
        install_linux_notification_desktop_entry || true
        if install_gnome_activate_window_extension; then
            GNOME_EXT_INSTALLED=true
        fi
    fi

    # Cleanup
    cleanup

    # Success message
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}✓ Installation Complete!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "${GREEN}✓${NC} Binary downloaded: ${BOLD}${BINARY_NAME}${NC}"
    echo -e "${GREEN}✓${NC} Utilities: sound-preview, list-devices, list-sounds"
    echo -e "${GREEN}✓${NC} Location: ${SCRIPT_DIR}/"
    echo -e "${GREEN}✓${NC} Checksum verified"
    echo -e "${GREEN}✓${NC} Symlinks created"
    if [ "$PLATFORM" = "darwin" ]; then
        if [ -d "${SCRIPT_DIR}/ClaudeNotifier.app" ]; then
            echo -e "${GREEN}✓${NC} ClaudeNotifier installed (modern notifications + click-to-focus)"
        else
            echo -e "${GREEN}✓${NC} terminal-notifier installed (click-to-focus)"
        fi
        echo -e "${GREEN}✓${NC} Claude icon configured for notifications"
    fi
    if [ "$PLATFORM" = "linux" ]; then
        if [ "$GNOME_EXT_INSTALLED" = true ]; then
            echo -e "${GREEN}✓${NC} GNOME activate-window extension installed (click-to-focus)"
        else
            echo -e "${YELLOW}⚠${NC} GNOME extension not installed (click-to-focus requires manual setup)"
        fi
    fi
    echo -e "${GREEN}✓${NC} Ready to use!"
    echo ""
    echo -e "${YELLOW}────────────────────────────────────────${NC}"
    echo -e "${YELLOW}★${NC} ${BOLD}Boost your productivity${NC}"
    echo -e "  Check out the advanced task manager for Claude"
    echo -e "  with a convenient UI, from the creator of this plugin:"
    echo -e "  ${GREEN}https://github.com/777genius/claude_agent_teams_ui${NC}"
    echo -e "${YELLOW}────────────────────────────────────────${NC}"
    echo ""
}

# Run main function
main "$@"
