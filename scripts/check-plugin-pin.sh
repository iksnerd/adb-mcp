#!/bin/sh
# Checks the pin the Claude plugin's launcher runs: release.json names the same
# version as plugin.json and pins a SHA-256 for each of the six targets. With a
# third argument, the release's plugin-checksums.txt, every pin must also equal
# the checksum the release workflow recorded for that build.
#
#   scripts/check-plugin-pin.sh [LAUNCHER_DIR] [PLUGIN_JSON] [EXPECTED_CHECKSUMS]
set -eu

launcher_dir="${1:-plugin/adb-mcp/launcher}"
plugin_json="${2:-plugin/adb-mcp/.claude-plugin/plugin.json}"
expected="${3:-}"
targets="darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64 windows-arm64"
pin="$launcher_dir/release.json"

fail=0
problem() { echo "plugin pin: $*" >&2; fail=1; }

[ -f "$pin" ] || { echo "plugin pin: $pin is missing; run scripts/pin-plugin.sh <tag> after the release" >&2; exit 1; }
jq -e . "$pin" >/dev/null 2>&1 || { echo "plugin pin: $pin is not valid JSON" >&2; exit 1; }

pinned_version=$(jq -r '.version // empty' "$pin")
plugin_version=$(jq -r '.version // empty' "$plugin_json")
[ -n "$pinned_version" ] || problem "$pin has no version"
[ "$pinned_version" = "$plugin_version" ] || problem "release.json pins $pinned_version, plugin.json says $plugin_version"

count=$(jq -r '.assets | length' "$pin")
[ "$count" -eq 6 ] || problem "release.json pins $count builds, want 6"

for target in $targets; do
  hash=$(jq -r --arg t "$target" '.assets[$t] // empty' "$pin")
  if ! printf '%s' "$hash" | grep -Eq '^[0-9a-f]{64}$'; then problem "$target has no valid sha256 pin"; continue; fi
  if [ -n "$expected" ]; then
    want=$(awk -v t="$target" '$2 == t { print $1 }' "$expected")
    [ "$hash" = "$want" ] || problem "$target pins $hash, the release recorded '${want:-nothing}'"
  fi
done

[ "$fail" -eq 0 ] && echo "plugin pin OK ($pinned_version)"
exit "$fail"
