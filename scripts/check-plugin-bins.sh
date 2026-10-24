#!/bin/sh
# Checks the binaries bundled in the Claude plugin: all six targets are present,
# executable, and listed in checksums.txt with the right sha256, and the one
# this machine can run reports the expected version.
#
#   scripts/check-plugin-bins.sh [BIN_DIR] [VERSION]
#
# Defaults: plugin/adb-mcp/bin and the contents of VERSION. Used by `make
# plugin-bins-check` and the release gate.
set -eu

bin_dir="${1:-plugin/adb-mcp/bin}"
version="${2:-$(tr -d ' \t\n' < VERSION)}"
targets="darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64 windows-arm64"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

fail=0
problem() { echo "plugin bins: $*" >&2; fail=1; }

[ -f "$bin_dir/checksums.txt" ] || { echo "plugin bins: $bin_dir/checksums.txt is missing" >&2; exit 1; }

listed=$(grep -c . "$bin_dir/checksums.txt" || true)
[ "$listed" -eq 6 ] || problem "checksums.txt lists $listed files, want 6"

for target in $targets; do
  name=adb-mcp
  case "$target" in windows-*) name=adb-mcp.exe ;; esac
  rel="$target/$name"
  file="$bin_dir/$rel"
  if [ ! -f "$file" ]; then problem "$rel is missing"; continue; fi
  case "$target" in windows-*) ;; *) [ -x "$file" ] || problem "$rel is not executable" ;; esac
  want=$(awk -v f="$rel" '$2 == f { print $1 }' "$bin_dir/checksums.txt")
  if [ -z "$want" ]; then problem "$rel is not listed in checksums.txt"; continue; fi
  got=$(sha256_of "$file")
  [ "$got" = "$want" ] || problem "$rel checksum is $got, checksums.txt says $want"
done

# Only the build for this machine can be run; the others are covered by the
# checksums. CI runs this on linux/amd64, a maintainer's Mac on darwin.
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) host=linux-amd64 ;;
  Linux-aarch64|Linux-arm64) host=linux-arm64 ;;
  Darwin-x86_64) host=darwin-amd64 ;;
  Darwin-arm64) host=darwin-arm64 ;;
  *) host= ;;
esac
if [ -n "$host" ] && [ -x "$bin_dir/$host/adb-mcp" ]; then
  reported=$("$bin_dir/$host/adb-mcp" version 2>&1 || true)
  [ "$reported" = "adb-mcp $version" ] || problem "$host/adb-mcp reports '$reported', want 'adb-mcp $version'"
fi

[ "$fail" -eq 0 ] && echo "plugin bins OK ($version)"
exit "$fail"
