# adb_mcp — roadmap

The Android counterpart to [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP).
This file is the lean hub — only what's **open**. Shipped work lives in the
CHANGELOG; details for ideas live in the BACKLOG.

**Current:** v0.17.0 · 70 tools + 4 guide resources · [tool reference in README](README.md#tools)
Core parity with [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP) reached; remaining gaps below.

## Map

| Doc | What's in it |
|---|---|
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | Everything shipped, newest first (v0.1.0 → v0.15.0) |
| [docs/BACKLOG.md](docs/BACKLOG.md) | Open ideas + the conventions to follow when adding a tool |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Package layout (sdk/uiauto/adb/gradle/tools) + how to add a tool (with diagram) |

## Recently shipped (v0.16.0)

See [CHANGELOG](docs/CHANGELOG.md). v0.17.0: **screenshot decodes the PNG once**
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
- [~] Deeper project discovery — **`list_gradle_projects` shipped v0.17.0** (the `gradlew projects` module map; `list_gradle_variants` shipped v0.14.0). Still open: a per-module build-info/`properties` dump if it proves useful.
- [ ] Project scaffolding — new Android project from a template (biggest lift)

**Field feedback** (open items; most rounds shipped in v0.8.0–v0.16.0, see CHANGELOG)
- [x] App/bundle state probe — **shipped v0.16.0 as `app_state`**: installed?/running? + pid(s), process uptime, install/update times, Metro-vs-embedded bundle heuristic over recent logcat (HMRClient/Fast Refresh/DevServer) with the evidence line. Flags multiple live processes for one package.
- [x] Multi-display foldable `screenshot` corruption — **shipped v0.16.0**: strip the `screencap` multi-display `[Warning]` prefix before the PNG signature (robust, display-agnostic) + optional `display` selector (inner/cover/index/physical-id).
- [~] `biometric_auth` — **`has_biometric_enrolled` shipped v0.16.0** (count>0 probe; verified 0→1 on a live enroll). Still open: a deterministic re-enroll that *captures* the assigned finger id from the enrollment HAL log (id-guessing stays out — a wrong `finger_id` trips a HAL lockout, and `dumpsys fingerprint` never exposes the id).
- [ ] Verify `reload_app`/`open_dev_menu` on a real Expo dev client
- [ ] Residual describe_ui auto-filter noise — single-child chain collapse (clickable/query/compact cover it today)
- [ ] Accessibility-action tap for native surfaces — coordinate `input tap` no-ops on Compose/RN `NativeTabs` bars where Maestro's `tapOn` (UiAutomator `ACTION_CLICK`) works (`android-mcp` #019f75a8). `tap identify` (v0.14.0) diagnoses it; the real fix needs a live-emulator pass (no simple adb command — likely a UiAutomator route)
- [x] DECISION: `run_sequence` batching — **shipped v0.16.0**. Steps + sleeps + if_present/if_absent guards + optional, over the existing client methods; returns per-step results + final hierarchy. Batch-tap folds in (a sequence of `tap` steps).
- [ ] DECISION: Maestro integration (`run_maestro_flow`) — deliberate yes/no, see BACKLOG.md (decided separately from run_sequence now that batching shipped)

**Enhancements**
- [ ] Multi-touch / pinch-zoom (needs `sendevent`; single-pointer `drag` already shipped) — parked, no reliable cross-device approach yet
- [x] Real-device `set_battery` path — **shipped v0.17.0**: physical devices go through `dumpsys battery set level/ac` (emulator still uses `emu power`), with a `reset` option (`dumpsys battery reset`) to restore automatic reporting. Verified live.
- [x] **Perf: `screenshot` decodes the PNG once** (v0.17.0) — was decoded in `isMostlyBlack` and again in `downscalePNG` (~85ms/18MB each on a full-res frame); now one decode shared between the black-check and downscale.

## Ground rules

- Every device-facing tool takes an optional `serial`; single-device sessions omit it.
- Device commands are `adb.Client` methods; pure logic (parsing, geometry) lives in `internal/uiauto` or a plain func with its own test. `internal/tools` stays a thin MCP binding (see [ARCHITECTURE.md](ARCHITECTURE.md)).
- Unit-test any new logic: a command builder with a fake `Runner`, pure logic directly.
