#!/usr/bin/env bash
#
# build-all.sh - Cross-compile dermoscope-helper for all supported platforms
#
# This script builds the dermoscope-helper binary for:
# - Windows (amd64)
# - macOS (amd64 + arm64)
# - Linux (amd64)
#
# Usage:
#   ./scripts/build-all.sh [--clean] [--version VERSION] [--skip-failed]
#
# Options:
#   --clean        Remove dist/ directory before building
#   --version      Override the version string (default: 1.0.0)
#   --skip-failed  Continue building other platforms if one fails (useful for CGO)
#
# Cross-compilation Note:
# This project uses CGO (via gousb for USB support). Cross-compilation requires:
# - For Windows from macOS: mingw-w64 cross-compiler (brew install mingw-w64)
# - For Linux from macOS: musl-cross (brew install FiloSottile/musl-cross/musl-cross)
# Native builds on each platform work without additional setup.
# For CI/CD, prefer building on native runners for each target platform.

set -uo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Defaults
VERSION="${VERSION:-1.0.0}"
CLEAN=false
SKIP_FAILED=false
BINARY_NAME="dermoscope-helper"
DIST_DIR="$PROJECT_DIR/dist"
LDFLAGS="-s -w -X main.Version=${VERSION}"

# Track results
SUCCESSFUL_BUILDS=()
FAILED_BUILDS=()

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --clean)
            CLEAN=true
            shift
            ;;
        --version)
            VERSION="$2"
            LDFLAGS="-s -w -X main.Version=${VERSION}"
            shift 2
            ;;
        --skip-failed)
            SKIP_FAILED=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [--clean] [--version VERSION] [--skip-failed]"
            echo ""
            echo "Options:"
            echo "  --clean        Remove dist/ directory before building"
            echo "  --version      Override the version string (default: 1.0.0)"
            echo "  --skip-failed  Continue building other platforms if one fails"
            echo ""
            echo "Note: Cross-compilation requires CGO cross-compilers for non-native platforms."
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Change to project directory
cd "$PROJECT_DIR"

# Clean if requested
if [[ "$CLEAN" == true ]]; then
    echo "Cleaning dist/ directory..."
    rm -rf "$DIST_DIR"
fi

# Create dist directory
mkdir -p "$DIST_DIR"

echo "Building dermoscope-helper v${VERSION} for all platforms..."
echo ""

# Build function
build_platform() {
    local goos="$1"
    local goarch="$2"
    local output_name="$3"

    echo "Building for ${goos}/${goarch}..."

    if GOOS="$goos" GOARCH="$goarch" go build -ldflags "$LDFLAGS" \
        -o "${DIST_DIR}/${output_name}" \
        ./cmd/dermoscope-helper 2>&1; then

        if [[ -f "${DIST_DIR}/${output_name}" ]]; then
            local size
            size=$(ls -lh "${DIST_DIR}/${output_name}" | awk '{print $5}')
            echo "  -> ${output_name} (${size})"
            SUCCESSFUL_BUILDS+=("${goos}/${goarch}")
            return 0
        fi
    fi

    echo "  FAILED: ${goos}/${goarch} (CGO cross-compilation may require additional setup)"
    FAILED_BUILDS+=("${goos}/${goarch}")

    if [[ "$SKIP_FAILED" == false ]]; then
        return 1
    fi
    return 0
}

# Build for all platforms
build_platform "windows" "amd64" "${BINARY_NAME}-${VERSION}-windows-amd64.exe"
build_platform "darwin" "amd64" "${BINARY_NAME}-${VERSION}-darwin-amd64"
build_platform "darwin" "arm64" "${BINARY_NAME}-${VERSION}-darwin-arm64"
build_platform "linux" "amd64" "${BINARY_NAME}-${VERSION}-linux-amd64"

echo ""
echo "============================================"
echo "Build Summary"
echo "============================================"

if [[ ${#SUCCESSFUL_BUILDS[@]} -gt 0 ]]; then
    echo ""
    echo "Successful builds (${#SUCCESSFUL_BUILDS[@]}):"
    for build in "${SUCCESSFUL_BUILDS[@]}"; do
        echo "  - $build"
    done
fi

if [[ ${#FAILED_BUILDS[@]} -gt 0 ]]; then
    echo ""
    echo "Failed builds (${#FAILED_BUILDS[@]}):"
    for build in "${FAILED_BUILDS[@]}"; do
        echo "  - $build"
    done
    echo ""
    echo "Note: Cross-compilation with CGO requires platform-specific toolchains."
    echo "For production builds, use native runners in CI/CD."
fi

if [[ ${#SUCCESSFUL_BUILDS[@]} -gt 0 ]]; then
    echo ""
    echo "Binaries in ${DIST_DIR}/:"
    ls -lh "$DIST_DIR/" 2>/dev/null || echo "  (no files)"
fi

# Exit with error if any builds failed and not skipping
if [[ ${#FAILED_BUILDS[@]} -gt 0 && "$SKIP_FAILED" == false ]]; then
    exit 1
fi

exit 0
