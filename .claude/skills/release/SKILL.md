---
name: release
description: Cut an adb-mcp release — review what's landed since the last tag, verify the new tools actually work over a real stdio session, bump the four version files that CI hard-fails on, write the CHANGELOG section that becomes the release notes, replicate the release gate locally, then tag, push, and confirm the GitHub Release and MCP registry both flipped. Use when asked to tag/cut/ship a release, bump the version, or review recent work and release it if it's clean. Also covers the drift checks CI does NOT enforce (tool counts in README/docs/TOOLS.md, the private docs/personal/TODO.md banners).
---

# Cutting an adb-mcp release

Everything runs off one tag push. `.github/workflows/release.yml` triggers only
on `v*` — there is no push/PR CI — so the tag is simultaneously the build, the
test run, the GitHub Release, and the MCP registry publish. A bad tag is
therefore public before you find out.

The rule that follows from that: **replicate the gate locally before tagging.**
Re-tagging is the only fix, and the registry has already seen the old version.

## Order of operations

1. Review what landed since the last tag.
2. Verify the new tools live (see below — this is not optional here).
3. Fix what the review found. Do not tag around a known bug in unreleased code.
4. Bump the four version files + write the CHANGELOG section.
5. Update the drift-prone docs CI does not check.
6. Run the gate locally.
7. Tag, push, watch, confirm.

## 1. Review

```bash
git log --oneline "$(git describe --tags --abbrev=0)"..HEAD
git status --short          # uncommitted feature work is common here
git diff
```

Working-tree changes matter as much as commits: features often sit uncommitted
with their docs already written. Read `docs/BACKLOG.md` — entries are updated to
`[x]` with a "Verified…" note when something ships, and that note is a claim you
are about to put your name on.

## 2. Verify live, don't reason from the diff

A passing `go test` says the parser works. It does not say the tool works. Drive
the freshly built binary over real stdio JSON-RPC:

```bash
go build -o "$SCRATCH/adb-mcp" ./cmd/adb-mcp
python3 .claude/skills/release/mcp_drive.py "$SCRATCH/adb-mcp" \
  'session_set_defaults|{"project_dir":"/path/to/project"}' \
  'get_coverage_report|{}'
```

Exercise the **error paths too** (missing arg, not-found, ambiguous input), and
hand-check the numbers against the source rather than accepting a plausible
percentage.

Test the shape the feature is actually for. A Gradle tool verified only on a
single-module project is barely verified: most Android projects are
multi-module, and that is where `jacocoTestReport` fans out across modules and
per-tool assumptions break. The v0.22.0 coverage bug — reporting one module and
silently dropping the rest — passed every unit test and a single-module live
run.

## 3. Bump the four files CI hard-fails on

All four must equal the bare tag (`v0.22.0` → `0.22.0`):

| File | Shape |
|---|---|
| `VERSION` | bare version, single line |
| `cmd/adb-mcp/main.go` | `var version = "0.22.0"` — matched by an exact `sed` pattern |
| `server.json` | `.version` (no `v` prefix; the registry rejects one) |
| `docs/CHANGELOG.md` | a heading `## v0.22.0 — …` |

The CHANGELOG heading is load-bearing twice. The gate greps for the literal
`"## <tag> "` (tag followed by a space), and an `awk` step slices everything
between that heading and the next `## ` to use as the GitHub Release body. Get
the heading wrong and the gate fails; get the section wrong and the release
notes are wrong. Write it for a reader who has not seen the commits.

Bump minor for new tools, patch for fixes only.

## 4. Drift CI does not catch

These have gone stale before and nothing will stop you:

- **Tool counts** in `README.md` (two places: the intro line and the `## Tools`
  section) and `docs/TOOLS.md` (the "N tools + 4 guide resources" line).
- **`docs/personal/TODO.md`** — gitignored, so it never shows in `git status`.
  Its `**Current:** vX.Y.Z · N tools` banner, the Map table's version range, the
  "Recently shipped (vX.Y.Z)" heading *and its body*, and any parity checkboxes
  the release closes.

Counting tools — `docs/TOOLS.md` documents some tools in paired rows
(`` `install_app` / `uninstall_app` ``), so a raw row count under-reports.
Compare names, not totals:

```bash
grep -o '^\tadd(s, "[a-z_]*"' internal/tools/register.go | sed 's/.*"\(.*\)"/\1/' | sort > /tmp/reg.txt
grep -o '`[a-z_]*`' docs/TOOLS.md | tr -d '`' | sort -u > /tmp/doc.txt
comm -23 /tmp/reg.txt /tmp/doc.txt   # registered but undocumented — must be empty
wc -l < /tmp/reg.txt                 # the number that goes in the docs
```

## 5. Run the gate locally

Same checks as `release.yml`, before the tag exists:

```bash
set -euo pipefail
tag="v0.22.0"; bare="${tag#v}"; fail=0
[ "$(tr -d ' \t\n' < VERSION)" = "$bare" ] || { echo "VERSION"; fail=1; }
[ "$(sed -n 's/^var version = "\(.*\)"$/\1/p' cmd/adb-mcp/main.go)" = "$bare" ] || { echo "main.go"; fail=1; }
grep -qF "## $tag " docs/CHANGELOG.md || { echo "CHANGELOG heading"; fail=1; }
[ "$(jq -r '.version' server.json)" = "$bare" ] || { echo "server.json"; fail=1; }
[ "$fail" -eq 0 ] && echo "gate OK"

gofmt -l .            # must print nothing
go vet ./...
go test -race ./...   # CI uses -race; a clean `go test` is not the same run
mcp-publisher validate
```

`mcp-publisher validate` calls the live registry, so it needs network and the
registry being up — that is true in CI too, and a registry outage fails the gate
before anything publishes rather than after. Use the pinned version
(`PUBLISHER_VERSION` in `release.yml`, currently v1.8.1), not `:latest`.

## 6. Tag, push, confirm

```bash
git tag -a v0.22.0 -m "v0.22.0 — <same summary as the CHANGELOG heading>"
git push origin main && git push origin v0.22.0
gh run watch "$(gh run list --workflow=release.yml --limit 1 --json databaseId --jq '.[0].databaseId')" --exit-status
```

Pushing the tag publishes to GitHub **and** the MCP registry. Confirm with the
user before pushing unless they have already said to ship it.

The workflow publishes the registry entry **last**, after the GitHub Release, so
a failed build can never leave the registry advertising a release that does not
exist. Confirm both:

```bash
gh release view v0.22.0 --json tagName,assets --jq '{tagName, assets:[.assets[].name]}'
curl -fsS "https://registry.modelcontextprotocol.io/v0/servers?search=io.github.iksnerd/adb-mcp"
```

Expect 6 platform archives + `adb-mcp-bridge.apk` + `checksums.txt`. The
registry search returns every published version, so check that the new one is
present — not merely that the query returned something.

## Failure modes

| Symptom | Cause |
|---|---|
| Gate fails on one of the four files | They were bumped in separate commits; bump all four in one |
| Release notes are a bare compare link | The `## <tag> ` heading didn't match, so the `awk` slice came out empty |
| Registry lists the old version after a successful release | Publish step was skipped or failed after the Release — check the run's last step |
| `mcp-publisher validate` fails on a fresh field | Schema is versioned by the `$schema` date in `server.json`; validate against the pinned publisher, not a newer one |
| Tool count wrong in docs | Counted table rows instead of comparing tool names (paired rows) |
