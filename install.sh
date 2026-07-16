#!/bin/sh
# Installs the latest adb-mcp release binary for this machine.
#
#   curl -fsSL https://raw.githubusercontent.com/iksnerd/adb_mcp/main/install.sh | sh
#
# Options (env vars):
#   BIN_DIR   install destination (default: ~/.local/bin)
#   VERSION   release tag to install, e.g. v0.10.1 (default: latest)
set -eu

REPO="iksnerd/adb_mcp"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *) echo "error: unsupported OS '$os' — on Windows, download the zip from https://github.com/$REPO/releases/latest" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "error: unsupported architecture '$arch'" >&2; exit 1 ;;
esac

tag="${VERSION:-}"
if [ -z "$tag" ]; then
  tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')
  [ -n "$tag" ] || { echo "error: could not determine the latest release tag" >&2; exit 1; }
fi

archive="adb-mcp_${tag}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$tag/$archive"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "downloading $url"
curl -fsSL -o "$tmp/$archive" "$url"

echo "verifying checksum"
curl -fsSL -o "$tmp/checksums.txt" "https://github.com/$REPO/releases/download/$tag/checksums.txt"
expected=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$expected" ] || { echo "error: $archive not found in checksums.txt" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
else
  actual=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
fi
[ "$actual" = "$expected" ] || { echo "error: checksum mismatch for $archive" >&2; exit 1; }

tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$BIN_DIR"
mv "$tmp/adb-mcp" "$BIN_DIR/adb-mcp"
chmod +x "$BIN_DIR/adb-mcp"

echo "installed adb-mcp $tag to $BIN_DIR/adb-mcp"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "note: $BIN_DIR is not on your PATH — add it, e.g.: export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac
