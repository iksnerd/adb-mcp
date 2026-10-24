#!/bin/sh
# Tests scripts/check-plugin-bins.sh against stub binaries.
set -u

here=$(cd "$(dirname "$0")" && pwd)
check="$here/check-plugin-bins.sh"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
failures=0

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# fixture DIR VERSION: six stub binaries; each prints the version the way the real one does.
fixture() {
  dir=$1; ver=$2
  rm -rf "$dir"; : > /dev/null
  for t in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64 windows-arm64; do
    name=adb-mcp; case $t in windows-*) name=adb-mcp.exe ;; esac
    mkdir -p "$dir/$t"
    printf '#!/bin/sh\necho "adb-mcp %s"\n' "$ver" > "$dir/$t/$name"
    chmod 755 "$dir/$t/$name"
  done
  (cd "$dir" && for f in */adb-mcp*; do printf '%s  %s\n' "$(sha256_of "$f")" "$f"; done > checksums.txt)
}

expect() { # expect WANT_EXIT NAME DIR VERSION [STDERR_PATTERN]
  want=$1; name=$2; dir=$3; ver=$4; pat=${5:-}
  out=$("$check" "$dir" "$ver" 2>&1); got=$?
  if [ "$got" -ne "$want" ]; then echo "FAIL $name: exit $got, want $want ($out)"; failures=$((failures + 1)); return; fi
  if [ -n "$pat" ] && ! printf '%s' "$out" | grep -q "$pat"; then echo "FAIL $name: output lacks '$pat' ($out)"; failures=$((failures + 1)); return; fi
  echo "ok   $name"
}

d="$work/good"; fixture "$d" 1.2.3
expect 0 "complete, matching bundle passes" "$d" 1.2.3

d="$work/missing"; fixture "$d" 1.2.3; rm "$d/linux-arm64/adb-mcp"
expect 1 "a missing binary fails" "$d" 1.2.3 "linux-arm64/adb-mcp is missing"

d="$work/tampered"; fixture "$d" 1.2.3; echo '# x' >> "$d/windows-amd64/adb-mcp.exe"
expect 1 "a changed binary fails" "$d" 1.2.3 "windows-amd64/adb-mcp.exe checksum"

d="$work/unlisted"; fixture "$d" 1.2.3; grep -v darwin-amd64 "$d/checksums.txt" > "$d/c" && mv "$d/c" "$d/checksums.txt"
expect 1 "an unlisted binary fails" "$d" 1.2.3 "not listed"

d="$work/notexec"; fixture "$d" 1.2.3; chmod 644 "$d/darwin-arm64/adb-mcp" "$d/linux-amd64/adb-mcp"
expect 1 "a non-executable binary fails" "$d" 1.2.3 "not executable"

d="$work/oldver"; fixture "$d" 1.2.2
expect 1 "a binary built for another version fails" "$d" 1.2.3 "want 'adb-mcp 1.2.3'"

d="$work/nolist"; fixture "$d" 1.2.3; rm "$d/checksums.txt"
expect 1 "a missing checksums.txt fails" "$d" 1.2.3 "checksums.txt is missing"

exit "$failures"
