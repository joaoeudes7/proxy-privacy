#!/bin/sh
set -e

REPO_URL="${REPO_URL:-https://github.com/joaoeudes7/proxy-privacy}"
BIN_NAME="${BIN_NAME:-proxy-privacy}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)

echo "  == Proxy Privacy Installer =="
echo

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)

case "$os" in
    linux) goos="linux" ;;
    darwin) goos="darwin" ;;
    mingw*|cygwin*|msys*) goos="windows" ;;
    *) goos="" ;;
esac

case "$arch" in
    x86_64|amd64) goarch="amd64" ;;
    aarch64|arm64) goarch="arm64" ;;
    *) goarch="" ;;
esac

try_download() {
    suffix="$goos-$goarch"
    if [ "$goos" = "windows" ]; then
        suffix="${suffix}.exe"
    fi

    if [ "$VERSION" = "latest" ]; then
        url="$REPO_URL/releases/latest/download/proxy-privacy-$suffix"
    else
        url="$REPO_URL/releases/download/$VERSION/proxy-privacy-$suffix"
    fi

    echo "  Downloading pre-built binary for $goos/$goarch..."
    if command -v curl >/dev/null 2>&1; then
        http_code=$(curl -fsSL -w "%{http_code}" -o "$BIN_NAME" "$url" 2>/dev/null || echo "000")
    else
        http_code=$(wget -q "$url" -O "$BIN_NAME" 2>/dev/null && echo "200" || echo "000")
    fi

    if [ "$http_code" = "200" ]; then
        chmod +x "$BIN_NAME"
        echo "  Installing to $INSTALL_DIR/$BIN_NAME..."
        if [ "$(id -u)" -ne 0 ]; then
            sudo mv "$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
        else
            mv "$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
        fi
        echo
        echo "  Installed! Run: proxy-privacy --help"
        exit 0
    fi

    echo "  No pre-built binary available for $goos/$goarch."
    [ -f "$BIN_NAME" ] && rm -f "$BIN_NAME"
    return 1
}

# Try pre-built binary first
if [ -n "$goos" ] && [ -n "$goarch" ]; then
    try_download
fi

# Fall back to building from source
echo "  Building from source..."

if ! command -v go >/dev/null 2>&1; then
    echo "  Go is not installed."
    echo "  Install Go 1.26+ first:"
    echo "    macOS: brew install go"
    echo "    Linux: https://go.dev/dl/"
    echo
    os=$(uname -s)
    case "$os" in
        Darwin)
            if command -v brew >/dev/null 2>&1; then
                echo "  Installing Go via Homebrew..."
                brew install go
            else
                echo "  Install Homebrew from https://brew.sh or download Go from https://go.dev/dl/"
                exit 1
            fi
            ;;
        Linux)
            echo "  Downloading Go 1.26+ for Linux..."
            arch=$(uname -m)
            case "$arch" in
                x86_64|amd64) goarch="amd64" ;;
                aarch64|arm64) goarch="arm64" ;;
                *) echo "Unsupported arch: $arch"; exit 1 ;;
            esac
            curl -fsSL "https://go.dev/dl/go1.26.0.linux-$goarch.tar.gz" | sudo tar -C /usr/local -xz
            export PATH=$PATH:/usr/local/go/bin
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
            echo "  Go installed. Restart your shell or run: export PATH=\$PATH:/usr/local/go/bin"
            ;;
    esac
fi

echo "  Checking Go version..."
go_version=$(go version 2>/dev/null | sed 's/.*go\([0-9]\.[0-9]*\).*/\1/')
if [ -z "$go_version" ]; then
    echo "  Error: Go not found after install attempt."
    exit 1
fi
echo "  Go $go_version detected."

BUILD_DIR=""
TMP_DIR=""

if [ -f "$SCRIPT_DIR/go.mod" ] && [ -d "$SCRIPT_DIR/cmd/proxy-privacy" ]; then
    BUILD_DIR="$SCRIPT_DIR"
    echo "  Using local repository at $BUILD_DIR"
else
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT
    BUILD_DIR="$TMP_DIR"

    echo "  Cloning repository..."
    git clone --depth 1 "$REPO_URL" "$BUILD_DIR"
fi

echo "  Building..."
cd "$BUILD_DIR"
go build -o "$BIN_NAME" ./cmd/proxy-privacy/

echo "  Installing to $INSTALL_DIR/$BIN_NAME..."
if [ "$(id -u)" -ne 0 ]; then
    sudo mv "$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
else
    mv "$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
fi

echo
echo "  Installed! Run: proxy-privacy --help"
