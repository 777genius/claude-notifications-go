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

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# GitHub repository
REPO="777genius/claude-notifications-go"
RELEASE_URL="https://github.com/${REPO}/releases/latest/download"
CHECKSUMS_URL="${RELEASE_URL}/checksums.txt"

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

    # Construct binary name
    if [ "$PLATFORM" = "windows" ]; then
        BINARY_NAME="claude-notifications-${PLATFORM}-${ARCH}.exe"
    else
        BINARY_NAME="claude-notifications-${PLATFORM}-${ARCH}"
    fi

    BINARY_PATH="${SCRIPT_DIR}/${BINARY_NAME}"
    CHECKSUMS_PATH="${SCRIPT_DIR}/.checksums.txt"
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

# Check if GitHub is accessible
check_github_availability() {
    if command -v curl &> /dev/null; then
        if ! curl -s --max-time 5 -I https://github.com &> /dev/null; then
            echo -e "${RED}✗ Cannot reach GitHub${NC}" >&2
            echo -e "${YELLOW}Possible issues:${NC}" >&2
            echo -e "  - No internet connection" >&2
            echo -e "  - GitHub is down" >&2
            echo -e "  - Firewall/proxy blocking access" >&2
            return 1
        fi
    fi
    return 0
}

# Check if binary already exists
check_existing() {
    if [ -f "$BINARY_PATH" ]; then
        echo -e "${GREEN}✓${NC} Binary already installed: ${BOLD}${BINARY_NAME}${NC}"
        echo ""
        return 0
    fi
    return 1
}

# Download checksums file
download_checksums() {
    echo -e "${BLUE}📝 Downloading checksums...${NC}"

    if command -v curl &> /dev/null; then
        if curl -fsSL "$CHECKSUMS_URL" -o "$CHECKSUMS_PATH" 2>/dev/null; then
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

    echo -e "${BLUE}📦 Downloading ${BOLD}${BINARY_NAME}${NC}${BLUE}...${NC}"
    echo -e "${BLUE}   From: ${url}${NC}"
    echo ""

    # Try curl first (with progress bar)
    if command -v curl &> /dev/null; then
        # Capture HTTP status
        local http_code=$(curl -w "%{http_code}" -fL --progress-bar "$url" -o "$BINARY_PATH" 2>&1 | tail -1)

        if [ -f "$BINARY_PATH" ] && [ "$(get_file_size "$BINARY_PATH")" -gt 100000 ]; then
            echo ""
            return 0
        else
            # Analyze failure
            rm -f "$BINARY_PATH"

            if echo "$http_code" | grep -q "404"; then
                echo ""
                echo -e "${RED}✗ Binary not found (404)${NC}" >&2
                echo ""
                echo -e "${YELLOW}This usually means the release is still building.${NC}" >&2
                echo -e "${YELLOW}Check build status at:${NC}" >&2
                echo -e "  https://github.com/${REPO}/actions" >&2
                echo ""
                echo -e "${YELLOW}Wait a few minutes and try again.${NC}" >&2
            elif echo "$http_code" | grep -qE "^5[0-9]{2}"; then
                echo ""
                echo -e "${RED}✗ GitHub server error (${http_code})${NC}" >&2
                echo -e "${YELLOW}GitHub may be experiencing issues. Try again later.${NC}" >&2
            else
                echo ""
                echo -e "${RED}✗ Download failed${NC}" >&2
                echo -e "${YELLOW}Check your internet connection and try again.${NC}" >&2
            fi
            return 1
        fi

    # Fallback to wget
    elif command -v wget &> /dev/null; then
        if wget --show-progress -q "$url" -O "$BINARY_PATH" 2>&1; then
            if [ -f "$BINARY_PATH" ] && [ "$(get_file_size "$BINARY_PATH")" -gt 100000 ]; then
                echo ""
                return 0
            fi
        fi

        rm -f "$BINARY_PATH"
        echo ""
        echo -e "${RED}✗ Download failed${NC}" >&2
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
    local actual_sum=""
    if command -v shasum &> /dev/null; then
        actual_sum=$(shasum -a 256 "$BINARY_PATH" 2>/dev/null | awk '{print $1}')
    elif command -v sha256sum &> /dev/null; then
        actual_sum=$(sha256sum "$BINARY_PATH" 2>/dev/null | awk '{print $1}')
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

# Make binary executable
make_executable() {
    chmod +x "$BINARY_PATH" 2>/dev/null || true
}

# Create symlink for hooks
create_symlink() {
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

# Cleanup temporary files
cleanup() {
    rm -f "$CHECKSUMS_PATH" 2>/dev/null || true
}

# Main installation flow
main() {
    echo ""
    echo -e "${BOLD}========================================${NC}"
    echo -e "${BOLD} Claude Notifications - Binary Setup${NC}"
    echo -e "${BOLD}========================================${NC}"
    echo ""

    # Detect platform
    detect_platform
    echo -e "${BLUE}Platform:${NC} ${PLATFORM}-${ARCH}"
    echo -e "${BLUE}Binary:${NC}   ${BINARY_NAME}"
    echo ""

    # Check if already installed
    if check_existing; then
        # Even if binary exists, ensure symlink is created
        create_symlink
        echo -e "${GREEN}✓ No download needed${NC}"
        echo ""
        return 0
    fi

    # Check GitHub availability
    if ! check_github_availability; then
        echo ""
        exit 1
    fi

    # Download checksums (optional)
    download_checksums

    # Download
    if ! download_binary; then
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
        echo ""
        exit 1
    fi

    # Verify
    if ! verify_binary; then
        cleanup
        echo ""
        echo -e "${RED}========================================${NC}"
        echo -e "${RED} Verification Failed${NC}"
        echo -e "${RED}========================================${NC}"
        echo ""
        exit 1
    fi

    # Make executable
    make_executable

    # Create symlink for hooks to use
    create_symlink

    # Cleanup
    cleanup

    # Success message
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}✓ Installation Complete!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "${GREEN}✓${NC} Binary downloaded: ${BOLD}${BINARY_NAME}${NC}"
    echo -e "${GREEN}✓${NC} Location: ${SCRIPT_DIR}/"
    echo -e "${GREEN}✓${NC} Checksum verified"
    echo -e "${GREEN}✓${NC} Symlink created for hooks"
    echo -e "${GREEN}✓${NC} Ready to use!"
    echo ""
}

# Run main function
main "$@"
