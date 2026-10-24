#!/bin/sh
# Builds the six adb-mcp binaries the Claude plugin bundles into
# plugin/adb-mcp/bin/<os>-<arch>/ and writes checksums.txt beside them.
# Same flags as the release archives in release.yml. Run it in the release
# commit only (`make plugin-bins`), so the binaries enter history once per release.
set -eu

bin_dir=plugin/adb-mcp/bin
version=$(tr -d ' \t\n' < VERSION)

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

rm -rf "$bin_dir"/darwin-* "$bin_dir"/linux-* "$bin_dir"/windows-* "$bin_dir/checksums.txt"
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os=${target%/*}; arch=${target#*/}
  name=adb-mcp; [ "$os" = windows ] && name=adb-mcp.exe
  echo "building $os-$arch"
  mkdir -p "$bin_dir/$os-$arch"
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$bin_dir/$os-$arch/$name" ./cmd/adb-mcp
done

(cd "$bin_dir" && for f in */adb-mcp*; do printf '%s  %s\n' "$(sha256_of "$f")" "$f"; done > checksums.txt)
scripts/check-plugin-bins.sh "$bin_dir" "$version"
du -sh "$bin_dir"
