#!/bin/sh
# Install fixmyrouter from GitHub releases:
#   curl -fsSL https://raw.githubusercontent.com/kavinbm16/fixmyrouter/main/install.sh | sh
# Verifies the SHA-256 checksum before installing. Read it first if you like;
# it only downloads the release archive for your OS/CPU and copies one binary.
set -eu

REPO="kavinbm16/fixmyrouter"
VERSION="${FIXMYROUTER_VERSION:-latest}"
BASE="${FIXMYROUTER_BASE_URL:-}" # for testing against a local mirror

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $os. On Windows, download the zip from https://github.com/$REPO/releases" >&2; exit 1 ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "Unsupported CPU: $arch" >&2; exit 1 ;;
esac

if [ -z "$BASE" ]; then
  if [ "$VERSION" = latest ]; then
    BASE="https://github.com/$REPO/releases/latest/download"
  else
    BASE="https://github.com/$REPO/releases/download/$VERSION"
  fi
fi

file="fixmyrouter_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $file..."
curl -fsSL "$BASE/$file" -o "$tmp/$file"
curl -fsSL "$BASE/checksums.txt" -o "$tmp/checksums.txt"

want=$(grep " $file\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$file" | cut -d' ' -f1)
else
  got=$(shasum -a 256 "$tmp/$file" | cut -d' ' -f1)
fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then
  echo "Checksum mismatch for $file; not installing." >&2
  exit 1
fi

tar -xzf "$tmp/$file" -C "$tmp" fixmyrouter

dir="${FIXMYROUTER_INSTALL_DIR:-/usr/local/bin}"
if [ ! -w "$dir" ] && [ -z "${FIXMYROUTER_INSTALL_DIR:-}" ]; then
  dir="$HOME/.local/bin"
  mkdir -p "$dir"
fi
install -m 0755 "$tmp/fixmyrouter" "$dir/fixmyrouter"
echo "Installed $("$dir/fixmyrouter" version) to $dir/fixmyrouter"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "Add $dir to your PATH to run it from anywhere." ;;
esac
echo "Next: fixmyrouter check   (or: fixmyrouter ui)"
