# adb_mcp landing page — spec

A single static page for `adb_mcp`, in the spirit of
[xcodebuildmcp.com](https://xcodebuildmcp.com) but scaled to what this project
actually has: one repo, one maintainer, no video demos, no CLI product line.
Same *job* (convert a visiting developer into an install in under a minute),
smaller page.

Goal: someone lands here from a GitHub search, a tweet, or the XcodeBuildMCP
README's "see also" link, and within one scroll understands what it does, that
it's trustworthy (real releases, tests, MIT), and how to install it.

Non-goals: no blog, no docs hosting (docs/TOOLS.md etc. stay on GitHub), no
analytics dashboards, no marketing claims not already true in the README.

## Content rule

Every section's copy is pulled from or directly derived from README.md — no
new claims. Where a paragraph below is a suggested rewrite, the source line(s)
in README.md are cited so copy stays truthful as the README evolves.

## Page structure (top to bottom)

### 1. Nav (sticky, minimal)

- Left: Android-head mark (`assets/android-head_flat.svg`) + wordmark `adb_mcp`.
- Right: `Tools` `Install` `Docs` (anchor links to sections 4/6 and out to
  `docs/TOOLS.md`) · GitHub icon+star-count link · **Get Started** button
  (scrolls to Install).
- Collapses to a hamburger under ~640px; nav links become a single dropdown.

### 2. Hero

- Eyebrow chip: `Android counterpart to XcodeBuildMCP` → links to the
  XcodeBuildMCP repo. (Source: README line 21-23.)
- H1: **Drive Android emulators and devices with AI.**
- Subhead: "An MCP server that lets an agent boot an AVD, screenshot, read the
  UI hierarchy, tap/swipe/type, and manage app lifecycle over `adb`." (Source:
  README lines 17-19.)
- Install one-liner in a code block with a copy button:
  ```
  curl -fsSL https://raw.githubusercontent.com/iksnerd/adb_mcp/main/install.sh | sh
  ```
- CTA row: **Get Started** (primary, scrolls to Install) · **View on GitHub**
  (secondary, outbound).
- Badge row, mirroring the four README shields exactly (pull live from
  shields.io, don't hardcode the numbers): Release version · CI status ·
  Go 1.26+ · MCP stdio.

### 3. Why (3 cards)

Directly from the README "Why" section (lines 31-46) — pick the three most
concrete, non-generic claims so this doesn't read like AI-generated feature
soup:

| Card | Copy |
|---|---|
| **No more stale coordinates** | `describe_ui` returns each element's center in true device pixels, computed fresh — so a tap lands where you mean it to, no guessing off a downscaled screenshot. |
| **Screenshots that actually work** | Captured via `exec-out screencap` (no CRLF corruption) and auto-downscaled to fit the image reader — plus black-frame retry and a diagnosis (FLAG_SECURE vs. a sleeping display) when a capture really is black. |
| **The runbook ships with the server** | The driving workflow — observe→act loop, PIN/lock handling, crash triage — is baked in as MCP resources the agent can pull up mid-task, not tribal knowledge you have to re-explain every session. |

### 4. Tool areas (grid, 9 cards)

One card per README tool-category bullet (lines 137-145), title + one-line
gloss + tool count chip if easy to derive. Grid: `auto-fit, minmax(260px, 1fr)`,
3 columns desktop / 2 tablet / 1 mobile.

1. **Emulator / device** — boot, list, wait-for-boot, shut down, Wi-Fi connect, port forwarding.
2. **Observe** — screenshot (multi-display aware) and `describe_ui` with true-pixel centers and top-window awareness.
3. **Interact** — tap, swipe, drag, type, key combos, PIN pads, `run_sequence` batching.
4. **Lock / Keystore / Biometrics** — secure lock screen, `fingerprint_touch`, biometric enrollment checks.
5. **Extended Controls** — SMS, phone calls, battery, cellular conditions, sensors, AVD snapshots.
6. **App lifecycle** — install/launch/stop, `app_state` (Metro-vs-embedded bundle detection), permissions, deep links.
7. **Logs & capture** — one-shot/streaming logcat, filters, screen recording.
8. **Environment & diagnostics** — dark mode, mock location, `stay_awake`, `doctor`.
9. **Gradle build & test** — build, unit/instrumented tests, variant/module discovery, `build_and_run`.

Header above the grid: **"{N} tools across nine areas"** where `{N}` is
pulled from `TODO.md`'s current line (`70 tools`, `73 tools`, …) at build
time (see Content freshness below) rather than hardcoded.

Footer link under the grid: "Full reference → docs/TOOLS.md" (outbound to GitHub).

### 5. The core loop (small diagram strip)

Four-step horizontal flow, from README lines 151-156:

```
screenshot → describe_ui → tap / swipe / type → screenshot
   (see)      (locate)         (act)             (confirm)
```

One sentence under it: "Read `android://guide/driving` (an MCP resource, not
a URL — ask your agent to fetch it) for the full loop and the gotchas that
waste turns."

### 6. Install (the conversion section)

Tabbed, three tabs (default: Quick install):

- **Quick install** — the `curl | sh` one-liner (same as hero, repeated here
  intentionally — this is the section people scroll back to), plus a note on
  `BIN_DIR`/`VERSION` overrides and `adb-mcp update`.
- **Manual download** — link to the Releases page; one line noting SHA-256
  checksums are published per-platform (macOS/Linux/Windows, amd64/arm64).
- **From source** — the three build commands from README lines 123-129
  (`make install` / `go build` / `go install .../cmd/adb-mcp@latest`), with a
  note that Go is *not* required for the first two install paths.

Below the tabs, a second small tab group for **client registration** (README
lines 93-117):

- Claude Code — `claude mcp add adb -- adb-mcp`
- Cursor / VS Code — the two one-click install badges (image links, same
  URLs as README)
- Any other client — the generic `{"mcpServers": {"adb": {"command": "adb-mcp"}}}` JSON block

Closing line: "Confirm it's wired up — ask your agent to *boot an emulator and
take a screenshot*."

### 7. Positioning strip (thin, text-only)

One line, not a full section: "Built on the official
[Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) · Not affiliated
with Google or Sentry" — links to the MCP SDK and states the two disclaimers
the README already carries (Android trademark note, and XcodeBuildMCP being a
separate project) so the page doesn't imply endorsement it doesn't have.

### 8. Footer

- Left: Android-head mark + `adb_mcp` + `MIT License`.
- Center: link list — GitHub · Releases · Issues · docs/TOOLS.md · CHANGELOG.
- Right: the Android trademark disclaimer, verbatim from README lines 25-29
  (small print, required — don't paraphrase it).

## Visual design

**Palette** — pull from the badges already in the README rather than
inventing a new brand:

| Token | Hex | Use |
|---|---|---|
| `--android-green` | `#3DDC84` | primary accent — CTA buttons, links, active tab, tool-card icons |
| `--go-blue` | `#00ADD8` | secondary accent — code block accents, the Go badge callout |
| `--bg` | `#0D1117` (dark) / `#FFFFFF` (light) | page background |
| `--surface` | `#161B22` (dark) / `#F6F8FA` (light) | card backgrounds |
| `--text` | `#E6EDF3` (dark) / `#1F2328` (light) | body text |
| `--text-muted` | `#8B949E` | subheads, captions |

Default to **dark**, respect `prefers-color-scheme` for light, no manual
toggle needed for v1 (matches the dark GitHub-style shields already in use;
skip the added complexity of a toggle unless requested later).

**Type** — system sans stack (`-apple-system, "Segoe UI", Roboto, sans-serif`)
for body/headings; a monospace stack (`"JetBrains Mono", "SF Mono", Consolas,
monospace`) for every code block and inline command. No webfont loading — 
keeps the page a true static single-request-class asset.

**Layout** — centered container, `max-width: 1100px`, generous vertical
rhythm (`~96px` between major sections on desktop, `~56px` mobile). Cards use
a 1px border in `--surface` plus a subtle shadow on hover, not on rest —
keeps the page calm rather than "AI-generated SaaS" glossy.

**Motion** — minimal: hover lift on cards/buttons (`transform: translateY(-2px)`,
150ms), a copy-button state change on the install snippet, tab-switch fade.
No scroll-triggered animation, no autoplay video (there is none), respect
`prefers-reduced-motion`.

**Imagery** — the two existing logo assets (`android-head_flat.svg` for the
nav/footer mark, `android-head_3D.png` as the OG/social-card image) are the
only imagery needed. No screenshots/mockups exist yet — do not fabricate a
product screenshot; if one gets added later (e.g. a real `describe_ui`/tap
session), it belongs in the Hero as a right-column visual, not before.

## Content freshness (the thing that will actually break)

The tool count (`73 tools`) and version badges are the two numbers most
likely to drift and read as stale/wrong within a month. Two ways to handle
it, pick one at build time:

1. **Cheapest — badges only, no hardcoded count.** Use shields.io badges for
   version/CI/Go (already dynamic, zero maintenance) and drop the "N tools"
   number from copy entirely — say "dozens of tools across nine areas"
   instead. Simplest, never goes stale.
2. **More accurate — generate at deploy time.** A one-line build step greps
   `TODO.md`'s `**Current:**` line (or a small Go program that counts
   `add(s, "...", ...)` calls in `internal/tools/register.go`) and templates
   the number into `index.html` on deploy. Only worth it if this page is
   redeployed on every release anyway (see Hosting).

Recommendation: start with (1); revisit (2) only if the page gets a CI
pipeline of its own.

## Tech & hosting recommendation

**Stack:** one `index.html`, one `styles.css`, one small `main.js` (tab
switching + copy-to-clipboard only — no framework, no build step). This is a
one-page static site; a bundler/framework would be pure overhead for a
project this size and adds a dependency-update surface nobody will maintain.

**Hosting:** GitHub Pages from a `/site` directory in this repo (or a
`gh-pages` branch), with a custom domain later if wanted. It's free, needs no
new account, and keeps the landing page's source-of-truth next to the code it
describes — a docs/README change and a landing-page change can land in the
same PR when they're related (e.g. a new tool count). Cloudflare Pages is a
reasonable alternative if a custom domain + edge analytics matter more than
one-repo simplicity — flag if you want that instead.

**SEO/meta:** `<title>adb_mcp — drive Android with AI over MCP</title>`,
meta description = the Hero subhead, Open Graph + Twitter card image =
`android-head_3D.png`, canonical URL once a domain is chosen.

**Accessibility:** semantic heading order (one `h1` in Hero, `h2` per
section), all interactive elements keyboard-reachable with visible focus
rings, alt text on the logo mark, AA contrast on `--text`/`--text-muted`
against both backgrounds, `prefers-reduced-motion` respected.

## Open questions for you

1. **Domain** — GitHub Pages default (`iksnerd.github.io/adb_mcp`) or a
   custom domain? Changes the hosting recommendation above.
2. **Tool count in copy** — go with the no-hardcode approach (option 1
   above), or wire up the build-time count (option 2)?
3. Want me to build this now as the actual `index.html`/`styles.css`/`main.js`
   under a new `site/` directory, or review/adjust this spec first?
