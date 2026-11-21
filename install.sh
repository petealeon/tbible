#!/usr/bin/env bash
# tbible v2 installer for Linux and macOS.
#
# By default this installs a prebuilt release binary. Use --source to build
# from a local checkout with Go, or --local to install an already-built
# bin/tbible from this checkout.
set -euo pipefail

VERSION="${TBIBLE_VERSION:-2.1.0}"
REPO="petealeon/tbible"
BIN_DIR="${TBIBLE_BIN_DIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}"
MODE="binary"
DESKTOP=false
ROOT="$(cd "$(dirname "$0")" && pwd)"

usage() {
  cat <<EOF
tbible $VERSION installer

Usage: ./install.sh [options]

Options:
  --source          Build from source with Go instead of downloading a binary
  --local           Install ./bin/tbible from this checkout (run make build first)
  --version X       Install release version X (default: $VERSION)
  --bin-dir DIR     Install into DIR (default: $BIN_DIR)
  --desktop         Install a Linux desktop entry
  -h, --help        Show this help

Environment:
  TBIBLE_VERSION      Default release version
  TBIBLE_BIN_DIR      Default install directory
  TBIBLE_RELEASE_BASE Override the release asset URL base (for mirrors/tests)
  XDG_BIN_HOME        Standard XDG binary directory override
EOF
}

fail() {
  echo "install: $*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --source) MODE="source" ;;
    --local) MODE="local" ;;
    --version)
      [ "$#" -ge 2 ] || fail "--version needs a value"
      VERSION="$2"
      shift
      ;;
    --bin-dir)
      [ "$#" -ge 2 ] || fail "--bin-dir needs a value"
      BIN_DIR="$2"
      shift
      ;;
    --desktop) DESKTOP=true ;;
    -h|--help)
      usage
      exit 0
      ;;
    *) fail "unknown option: $1" ;;
  esac
  shift
done

detect_platform() {
  local kernel arch
  kernel="$(uname -s)"
  arch="$(uname -m)"
  case "$kernel" in
    Linux) OS="linux" ;;
    Darwin) OS="darwin" ;;
    *) fail "unsupported operating system: $kernel" ;;
  esac
  case "$arch" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) fail "unsupported architecture: $arch" ;;
  esac
}

download() {
  local url="$1" dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$dest" "$url"
  else
    fail "curl or wget is required to download binaries"
  fi
}

hash_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    fail "sha256sum or shasum is required to verify downloads"
  fi
}

verify_asset() {
  local asset="$1" checksums="$2" expected actual
  expected="$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1; exit }' "$checksums")"
  [ -n "$expected" ] || fail "no checksum found for $asset"
  actual="$(hash_of "$TMPDIR_DOWNLOAD/$asset")"
  [ "$actual" = "$expected" ] || fail "checksum mismatch for $asset"
}

TMPDIR_DOWNLOAD="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_DOWNLOAD"' EXIT

case "$MODE" in
  source)
    command -v go >/dev/null 2>&1 || fail "Go 1.27+ is required for --source"
    echo "== building tbible $VERSION from source =="
    (cd "$ROOT" && CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/petealeon/tbible/v2/internal/version.Version=$VERSION" \
      -o "$TMPDIR_DOWNLOAD/tbible" ./cmd/tbible)
    ;;
  local)
    [ -x "$ROOT/bin/tbible" ] || fail "$ROOT/bin/tbible not found; run make build first"
    cp "$ROOT/bin/tbible" "$TMPDIR_DOWNLOAD/tbible"
    ;;
  binary)
    detect_platform
    ASSET="tbible-$VERSION-$OS-$ARCH.tar.gz"
    CHECKSUMS="tbible-$VERSION-checksums.txt"
    BASE="${TBIBLE_RELEASE_BASE:-https://github.com/$REPO/releases/download/v$VERSION}"
    echo "== downloading tbible $VERSION for $OS/$ARCH =="
    download "$BASE/$ASSET" "$TMPDIR_DOWNLOAD/$ASSET"
    download "$BASE/$CHECKSUMS" "$TMPDIR_DOWNLOAD/$CHECKSUMS"
    verify_asset "$ASSET" "$TMPDIR_DOWNLOAD/$CHECKSUMS"
    tar -xzf "$TMPDIR_DOWNLOAD/$ASSET" -C "$TMPDIR_DOWNLOAD" tbible
    ;;
  *)
    fail "internal error: unknown mode $MODE"
    ;;
esac

chmod 0755 "$TMPDIR_DOWNLOAD/tbible"
mkdir -p "$BIN_DIR"
install -m 0755 "$TMPDIR_DOWNLOAD/tbible" "$BIN_DIR/tbible"

if [ "$DESKTOP" = true ]; then
  if [ "$(uname -s)" != "Linux" ]; then
    echo "install: --desktop is only used on Linux" >&2
  else
    APP_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
    mkdir -p "$APP_DIR"
    cat > "$APP_DIR/tbible.desktop" <<EOF
[Desktop Entry]
Version=1.0
Name=tbible
GenericName=Bible Reader
Comment=Terminal Bible (KJV) reader and search
Exec=tbible
Terminal=true
Type=Application
Categories=Utility;Office;Education;
StartupNotify=true
EOF
    echo "installed: $APP_DIR/tbible.desktop"
  fi
fi

echo "installed: $BIN_DIR/tbible"
"$BIN_DIR/tbible" --version || true

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Add $BIN_DIR to your PATH to run tbible from anywhere." ;;
esac
