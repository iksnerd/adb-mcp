#!/bin/sh
# Tests scripts/check-plugin-pin.sh against fixtures.
set -u

here=$(cd "$(dirname "$0")" && pwd)
check="$here/check-plugin-pin.sh"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
failures=0
# The targets come from the workflow, as in the script under test.
workflow="$here/../.github/workflows/release.yml"
targets=$(sed -n 's/.*for p in \(.*\); do.*/\1/p' "$workflow" | head -n 1 | tr '/' '-')
[ -n "$targets" ] || { echo "FAIL: no targets in release.yml"; exit 1; }
want_count=$(printf '%s\n' $targets | wc -l | tr -d ' ')

hash_for() { printf '%s' "$1" | shasum -a 256 | cut -d' ' -f1; }

# fixture DIR VERSION: a launcher dir with a pin, a plugin.json and the release's checksums.
fixture() {
  d=$1; v=$2
  mkdir -p "$d/launcher"
  : > "$d/sums.txt"
  assets=
  for t in $targets; do
    h=$(hash_for "$t")
    printf '%s  %s\n' "$h" "$t" >> "$d/sums.txt"
    assets="$assets\"$t\":\"$h\","
  done
  printf '{"version":"%s","assets":{%s}}\n' "$v" "${assets%,}" > "$d/launcher/release.json"
  printf '{"name":"adb-mcp","version":"%s"}\n' "$v" > "$d/plugin.json"
}

expect() { # expect WANT_EXIT NAME PATTERN DIR [EXPECTED]
  want=$1; name=$2; pat=$3; d=$4; exp=${5:-}
  out=$("$check" "$d/launcher" "$d/plugin.json" $exp 2>&1); got=$?
  if [ "$got" -ne "$want" ]; then echo "FAIL $name: exit $got, want $want ($out)"; failures=$((failures + 1)); return; fi
  if [ -n "$pat" ] && ! printf '%s' "$out" | grep -q "$pat"; then echo "FAIL $name: output lacks '$pat' ($out)"; failures=$((failures + 1)); return; fi
  echo "ok   $name"
}

d="$work/good"; fixture "$d" 1.2.3
expect 0 "a pin that matches plugin.json and the release passes" "plugin pin OK" "$d" "$d/sums.txt"
expect 0 "the pin passes without a release to compare to" "plugin pin OK" "$d"

d="$work/nopin"; fixture "$d" 1.2.3; rm "$d/launcher/release.json"
expect 1 "a missing release.json fails" "release.json is missing" "$d"

d="$work/skew"; fixture "$d" 1.2.3; printf '{"name":"adb-mcp","version":"1.2.4"}\n' > "$d/plugin.json"
expect 1 "plugin.json ahead of the pin fails" "pins 1.2.3, plugin.json says 1.2.4" "$d"

d="$work/short"; fixture "$d" 1.2.3; jq 'del(.assets["linux-arm64"])' "$d/launcher/release.json" > "$d/p" && mv "$d/p" "$d/launcher/release.json"
expect 1 "a missing target fails" "pins $((want_count - 1)) builds" "$d"

d="$work/badhash"; fixture "$d" 1.2.3; jq '.assets["darwin-arm64"]="xyz"' "$d/launcher/release.json" > "$d/p" && mv "$d/p" "$d/launcher/release.json"
expect 1 "a malformed hash fails" "darwin-arm64 has no valid sha256" "$d"

d="$work/drift"; fixture "$d" 1.2.3; sed -i.bak 's/^[0-9a-f]\{64\}  linux-amd64/0000000000000000000000000000000000000000000000000000000000000000  linux-amd64/' "$d/sums.txt"
expect 1 "a pin that differs from the release fails" "linux-amd64 pins" "$d" "$d/sums.txt"

d="$work/notjson"; fixture "$d" 1.2.3; echo '{' > "$d/launcher/release.json"
expect 1 "invalid JSON fails" "not valid JSON" "$d"

# sha256sum writes "*name" in binary mode; the check must read it.
d="$work/star"; fixture "$d" 1.2.3; sed -i.bak 's/  / */' "$d/sums.txt"
expect 0 "a binary-mode checksums file is read" "plugin pin OK" "$d" "$d/sums.txt"

# pin-plugin.sh against a local checksums file, text mode and binary mode.
for mode in text binary; do
  d="$work/pin-$mode"; fixture "$d" 1.2.3
  s="$d/stage"
  mkdir -p "$s/scripts" "$s/.github/workflows" "$s/plugin/adb-mcp/launcher" "$s/plugin/adb-mcp/.claude-plugin"
  cp "$here/pin-plugin.sh" "$check" "$s/scripts/"
  cp "$workflow" "$s/.github/workflows/"
  printf '{"name":"adb-mcp","version":"1.2.3"}\n' > "$s/plugin/adb-mcp/.claude-plugin/plugin.json"
  [ "$mode" = binary ] && sed -i.bak 's/  / */' "$d/sums.txt"
  out=$(cd "$s" && sh scripts/pin-plugin.sh v1.2.3 "$d/sums.txt" 2>&1); got=$?
  if [ "$got" -eq 0 ] && printf '%s' "$out" | grep -q "plugin pin OK" &&
    [ "$(jq -r '.assets | length' "$s/plugin/adb-mcp/launcher/release.json")" -eq "$want_count" ]; then
    echo "ok   pin-plugin.sh reads a $mode-mode checksums file"
  else
    echo "FAIL pin-plugin.sh $mode-mode: exit $got ($out)"; failures=$((failures + 1))
  fi
done

exit "$failures"
