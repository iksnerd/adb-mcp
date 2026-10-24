#!/bin/sh
# Pins the Claude plugin to a published release: writes the version and the
# SHA-256 of each build into plugin/adb-mcp/launcher/release.json, then checks it
# against plugin.json (whose version `make docs` keeps equal to VERSION) and the
# release. Run it after the release workflow has finished, and commit the result
# as its own commit.
#
#   scripts/pin-plugin.sh v0.24.3
set -eu

tag="${1:?usage: scripts/pin-plugin.sh vX.Y.Z}"
bare="${tag#v}"
repo=iksnerd/adb-mcp
launcher_dir=plugin/adb-mcp/launcher
plugin_json=plugin/adb-mcp/.claude-plugin/plugin.json

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSL "https://github.com/$repo/releases/download/$tag/plugin-checksums.txt" -o "$work/plugin-checksums.txt"

jq -n --arg v "$bare" --rawfile sums "$work/plugin-checksums.txt" '
  {version: $v,
   assets: ($sums | split("\n") | map(select(length > 0) | split("  ") | {(.[1]): .[0]}) | add)}' \
  > "$launcher_dir/release.json"

scripts/check-plugin-pin.sh "$launcher_dir" "$plugin_json" "$work/plugin-checksums.txt"
