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
            echo -e "${RED}✗ Unsupported OS: $os${NC}"
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
            echo -e "${RED}✗ Unsupported architecture: $arch${NC}"
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

# Download binary with progress bar
download_binary() {
    local url="${RELEASE_URL}/${BINARY_NAME}"

    echo -e "${BLUE}📦 Downloading ${BOLD}${BINARY_NAME}${NC}${BLUE}...${NC}"
    echo -e "${BLUE}   From: ${url}${NC}"
    echo ""

    # Try curl first (with progress bar)
    if command -v curl &> /dev/null; then
        if curl -fL --progress-bar "$url" -o "$BINARY_PATH"; then
            echo ""
            return 0
        else
            echo -e "${RED}✗ Download failed${NC}"
            rm -f "$BINARY_PATH"
            return 1
        fi
    # Fallback to wget
    elif command -v wget &> /dev/null; then
        if wget --show-progress -q "$url" -O "$BINARY_PATH"; then
            echo ""
            return 0
        else
            echo -e "${RED}✗ Download failed${NC}"
            rm -f "$BINARY_PATH"
            return 1
        fi
    else
        echo -e "${RED}✗ Error: curl or wget required for installation${NC}"
        echo -e "${YELLOW}Please install curl or wget and try again${NC}"
        return 1
    fi
}

# Verify downloaded binary
verify_binary() {
    if [ ! -f "$BINARY_PATH" ]; then
        echo -e "${RED}✗ Binary file not found after download${NC}"
        return 1
    fi

    local size=$(stat -f%z "$BINARY_PATH" 2>/dev/null || stat -c%s "$BINARY_PATH" 2>/dev/null)
    if [ "$size" -lt 1000000 ]; then
        echo -e "${RED}✗ Downloaded file too small (${size} bytes)${NC}"
        echo -e "${YELLOW}This might be an error page. Check your internet connection.${NC}"
        rm -f "$BINARY_PATH"
        return 1
    fi

    return 0
}

# Make binary executable
make_executable() {
    chmod +x "$BINARY_PATH"
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
        echo -e "${GREEN}✓ No download needed${NC}"
        echo ""
        return 0
    fi

    # Download
    if ! download_binary; then
        echo ""
        echo -e "${RED}========================================${NC}"
        echo -e "${RED} Installation Failed${NC}"
        echo -e "${RED}========================================${NC}"
        echo ""
        echo -e "${YELLOW}Troubleshooting:${NC}"
        echo -e "  1. Check your internet connection"
        echo -e "  2. Verify the release exists at:"
        echo -e "     https://github.com/${REPO}/releases"
        echo -e "  3. Try manual download from the link above"
        echo ""
        exit 1
    fi

    # Verify
    if ! verify_binary; then
        echo ""
        echo -e "${RED}========================================${NC}"
        echo -e "${RED} Verification Failed${NC}"
        echo -e "${RED}========================================${NC}"
        echo ""
        exit 1
    fi

    # Make executable
    make_executable

    # Success message
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}✓ Installation Complete!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "${GREEN}✓${NC} Binary downloaded: ${BOLD}${BINARY_NAME}${NC}"
    echo -e "${GREEN}✓${NC} Location: ${SCRIPT_DIR}/"
    echo -e "${GREEN}✓${NC} Ready to use!"
    echo ""
}

# Run main function
main "$@"
