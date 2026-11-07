#!/bin/sh
# Checks the pin the Claude plugin's launcher runs: release.json names the same
# version as plugin.json and pins a SHA-256 for each build target. With a
# third argument, the release's plugin-checksums.txt, every pin must also equal
# the checksum the release workflow recorded for that build.
#
#   scripts/check-plugin-pin.sh [LAUNCHER_DIR] [PLUGIN_JSON] [EXPECTED_CHECKSUMS]
set -eu

launcher_dir="${1:-plugin/adb-mcp/launcher}"
plugin_json="${2:-plugin/adb-mcp/.claude-plugin/plugin.json}"
expected="${3:-}"
# The targets are the os/arch pairs release.yml builds, read from its `for p in`
# list so there is no second copy to keep in step.
workflow="${RELEASE_WORKFLOW:-$(dirname "$0")/../.github/workflows/release.yml}"
targets=$(sed -n 's/.*for p in \(.*\); do.*/\1/p' "$workflow" | head -n 1 | tr '/' '-')
[ -n "$targets" ] || { echo "plugin pin: no build targets found in $workflow" >&2; exit 1; }
want_count=$(printf '%s\n' $targets | wc -l | tr -d ' ')
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
[ "$count" -eq "$want_count" ] || problem "release.json pins $count builds, want $want_count"

for target in $targets; do
  hash=$(jq -r --arg t "$target" '.assets[$t] // empty' "$pin")
  if ! printf '%s' "$hash" | grep -Eq '^[0-9a-f]{64}$'; then problem "$target has no valid sha256 pin"; continue; fi
  if [ -n "$expected" ]; then
    # sha256sum writes "*name" in binary mode and "name" in text mode.
    want=$(awk -v t="$target" '{ n = $2; sub(/^\*/, "", n) } n == t { print $1 }' "$expected")
    [ "$hash" = "$want" ] || problem "$target pins $hash, the release recorded '${want:-nothing}'"
  fi
done

[ "$fail" -eq 0 ] && echo "plugin pin OK ($pinned_version)"
exit "$fail"
