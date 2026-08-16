# adb-mcp — roadmap

The Android counterpart to [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP).
This file is the lean hub — only what's **open**. Shipped work lives in the
CHANGELOG; details for ideas live in the BACKLOG.

**Current:** v0.21.1 · 73 tools + 4 guide resources · [tool reference in README](README.md#tools)
Core parity with [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP) reached; remaining gaps below.

## Map

| Doc | What's in it |
|---|---|
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | Everything shipped, newest first (v0.1.0 → v0.21.1) |
| [docs/BACKLOG.md](docs/BACKLOG.md) | Open ideas + the conventions to follow when adding a tool |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Package layout (sdk/uiauto/adb/gradle/scaffold/selfupdate/tools) + how to add a tool (with diagram) |

## Recently shipped (v0.21.1)

See [CHANGELOG](docs/CHANGELOG.md). v0.21.1: **`doctor`/`app_state`/`describe_ui`
fan out their independent adb probes concurrently** instead of one after
another (new shared `internal/concurrent` primitive; `-race`-tested in CI),
and **`describe_ui`'s reported top window** no longer comes back empty on
Android versions where `dumpsys window windows` lacks the focus line.

v0.21.0: **EXPERIMENTAL accessibility-click
bridge** — `tap_on_text`/`tap_element` gain `via_accessibility`, dispatching a
real `AccessibilityNodeInfo.performAction(ACTION_CLICK)` through a small
companion app (repo-root `bridge/`, one-time `adb-mcp bridge install` per
device) for native views (Compose/RN `NativeTabs` bars) a coordinate tap
can't reach. `doctor` reports its status. Closes the round-6 BACKLOG item
open since v0.14.0.

v0.20.1: **`list_gradle_projects`/
`list_gradle_variants` schemas no longer advertise the `json`/`args` params
they ignore** (contributed by @Ani07-05, fixes #2).

v0.20.0: **project renamed `adb_mcp` →
`adb-mcp`** (module path + GitHub repo — re-run `go install` under the new
path if you installed a previous version), **`gradle_project_properties`**
redacts secret-shaped values by default, **`list_gradle_variants`/
`list_gradle_tasks`** can now scope to a submodule via `task="<module>:tasks"`,
and **`wait_for_text`**'s timeout message now points at `scroll=true`.

v0.19.0: **`logcat`/capture `redact:true`**
(masks tokens/passwords/API keys before output), **`launch_dev_client`**
reports the app's registered URL scheme(s) on failure, **`describe_ui`**
gets an optional `package` occlusion check, and **`wait_for_text`** gets
opt-in `scroll` to reach off-screen `ScrollView` content.

v0.18.0: **`app_state` foreground detection**
(`foreground`/`top_activity`, and an optional `source_path` staleness check for
JS that Metro's watcher missed after a `git stash`/`checkout`), **`run_sequence`
gets `assert_foreground` + per-step `elapsed_ms`**, **`launch_dev_client` detects
the Metro-unreachable error screen**, **`gradle_project_properties`**,
**`scaffold_android_project`**, **`prefer_pin`** (biometric → PIN fallback), and
a **`describe_ui` single-child chain-collapse fix** for nested Material wrappers.

v0.17.1: `app_state` live-socket Metro fallback for builds whose logcat omits
the older HMR markers. v0.17.0: **screenshot decodes the PNG once**
(was twice — ~85ms/18MB saved per call), **`set_battery` on physical devices**
(dumpsys battery + `reset`), **`list_gradle_projects`** (module discovery).

v0.16.0, all reproduced live (incl. a `Pixel_10_Pro_Fold` AVD): **foldable
`screenshot` fix** (strip the multi-display `[Warning]` prefix that corrupted the
PNG header; optional `display` param for inner/cover), **`app_state`** (running
pid(s) + Metro-vs-embedded bundle — the "my edits aren't showing up" probe),
**`has_biometric_enrolled`** (count probe before a biometric flow), and
**`run_sequence`** (batch steps + guards in one call).

v0.15.0 before it: `stay_awake`, `wakeup`/`sleep` keys, `enter_pin` bouncer
retry. v0.14.0: `list_gradle_variants` + `tap identify`.

## Next up

Pulled from [docs/BACKLOG.md](docs/BACKLOG.md) — see there for full context.

**XcodeBuildMCP parity gaps** (priority order)
- [x] Deeper project discovery — **`list_gradle_projects` + `gradle_project_properties` shipped** (module map plus per-module evaluated properties).
- [x] Project scaffolding — **`scaffold_android_project` shipped**: creates a minimal Kotlin Android project in a new empty directory.
- [ ] Debugging — XcodeBuildMCP has 7 LLDB-backed tools (attach, breakpoints, continue, detach, raw command, stack, variables); adb-mcp has no attach/breakpoint/variable-inspection equivalent. Android's analogue is JDWP (`jdb`), a different mechanism — needs a deliberate scope decision, not silent drift. Re-audited 2026-08-13 against XcodeBuildMCP's actual source tree; see BACKLOG.md.
- [ ] Code coverage reporting — `get_coverage_report`/`get_file_coverage` (xcresult → per-target/per-function coverage) has no adb-mcp equivalent; `run_unit_tests`/`run_instrumented_tests` run tests but don't surface JaCoCo output.
- [ ] Session defaults — `session_set/show/clear_defaults` (pin a default project/scheme/device) has no analogue beyond the optional `serial`. Lower priority; no field report has asked for it.

**Field feedback** (open items; most rounds shipped in v0.8.0–v0.16.0, see CHANGELOG)
- [x] App/bundle state probe — **shipped v0.16.0**, strengthened after the 2026-08-05 field report: installed?/running? + pid(s), process uptime, install/update times, Metro-vs-embedded bundle heuristic over recent logcat, plus a live Metro-port socket fallback and `bundle_signals` evidence. Flags multiple live processes for one package.
- [x] Multi-display foldable `screenshot` corruption — **shipped v0.16.0**: strip the `screencap` multi-display `[Warning]` prefix before the PNG signature (robust, display-agnostic) + optional `display` selector (inner/cover/index/physical-id).
- [~] `biometric_auth` — **`has_biometric_enrolled` + `prefer_pin` shipped** (count probe and best-effort credential fallback). Still open: deterministic re-enroll that captures the assigned finger id from the enrollment HAL log; id-guessing remains unsafe.
- [~] Verify `reload_app`/`open_dev_menu` on a real Expo dev client — tool paths are documented; current Expo classic/bridgeless behavior still needs a live matrix pass.
- [x] Residual `describe_ui` auto-filter noise — auto now collapses unlabelled, non-clickable single-child layout chains as well as identical-bounds wrappers.
- [x] Accessibility-action tap for native surfaces — **shipped v0.21.0 (EXPERIMENTAL)**: coordinate `input tap` no-ops on Compose/RN `NativeTabs` bars where Maestro's `tapOn` (UiAutomator `ACTION_CLICK`) works (`android-mcp` #019f75a8). `tap identify` (v0.14.0) diagnoses it; `tap_on_text`/`tap_element`'s `via_accessibility` now fixes it via a companion `AccessibilityService` bridge (`adb-mcp bridge install`, see `bridge/README.md`).
- [x] DECISION: `run_sequence` batching — **shipped v0.16.0**. Steps + sleeps + if_present/if_absent guards + optional, over the existing client methods; returns per-step results + final hierarchy. Batch-tap folds in (a sequence of `tap` steps).
- [x] DECISION: Maestro integration (`run_maestro_flow`) — defer. `run_sequence` covers in-process batching/guards; keep Maestro as an external E2E runner until structured cross-tool reporting is a demonstrated requirement.

**Field feedback, round 9** (`android-mcp-papercuts` #019fdb7d, 2026-08-07 — shipped v0.18.0)
- [x] `app_state` foreground state — **shipped:** `foreground` + `top_activity` from `dumpsys activity activities`; `run_sequence` also supports `assert_foreground`.
- [x] `app_state` Metro staleness signal — optional `source_path` compares newest host source mtime with the latest epoch-timed Metro/HMR marker and reports `bundle_stale`, `source_mtime`, and `last_hmr_update`.
- [x] `run_sequence` foreground assertion/timing — **shipped:** `assert_foreground` plus per-step `elapsed_ms`.
- [x] `launch_dev_client` error activity — **shipped:** detects `DevLauncherErrorActivity` after launch and includes the visible error text when available.

**Enhancements**
- [ ] Multi-touch / pinch-zoom (needs `sendevent`; single-pointer `drag` already shipped) — parked, no reliable cross-device approach yet
- [x] Real-device `set_battery` path — **shipped v0.17.0**: physical devices go through `dumpsys battery set level/ac` (emulator still uses `emu power`), with a `reset` option (`dumpsys battery reset`) to restore automatic reporting. Verified live.
- [x] **Perf: `screenshot` decodes the PNG once** (v0.17.0) — was decoded in `isMostlyBlack` and again in `downscalePNG` (~85ms/18MB each on a full-res frame); now one decode shared between the black-check and downscale.

**Code health** (from an architecture-principles DRY pass, 2026-08-07)
- [x] Dedupe the "is this package installed?" check — **done**: extracted `isPackageInstalled(dump string) bool` in `internal/adb/packages.go`, used by both `GetAppStateWithSource` and `GetAppDetails`.
- [x] Unify the logcat-capture and screen-record session registries in `internal/adb/capture.go` — **done**: replaced the two duplicated `map[string]*T` + mutex pairs with one generic `sessionRegistry[T]` (start/take/stopAll), used for both log and recording sessions.

## Ground rules

- Every device-facing tool takes an optional `serial`; single-device sessions omit it.
- Device commands are `adb.Client` methods; pure logic (parsing, geometry) lives in `internal/uiauto` or a plain func with its own test. `internal/tools` stays a thin MCP binding (see [ARCHITECTURE.md](ARCHITECTURE.md)).
- Unit-test any new logic: a command builder with a fake `Runner`, pure logic directly.
