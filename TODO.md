# adb_mcp — roadmap

The Android counterpart to [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP).
This file is the lean hub — only what's **open**. Shipped work lives in the
CHANGELOG; details for ideas live in the BACKLOG.

**Current:** v0.14.0 · 65 tools + 4 guide resources · [tool reference in README](README.md#tools)
Core parity with [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP) reached; remaining gaps below.

## Map

| Doc | What's in it |
|---|---|
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | Everything shipped, newest first (v0.1.0 → v0.14.0) |
| [docs/BACKLOG.md](docs/BACKLOG.md) | Open ideas + the conventions to follow when adding a tool |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Package layout (sdk/uiauto/adb/gradle/tools) + how to add a tool (with diagram) |

## Recently shipped (v0.14.0)

See [CHANGELOG](docs/CHANGELOG.md) (v0.14.0): `list_gradle_variants` (the "list
schemes" analogue — buildable variants parsed from the `assemble*` tasks) and a
`tap` hit-test (`identify` reports which element a coordinate lands in, from the
NativeTabs no-op field report). Both pure logic, unit-tested with no device.

v0.13.0 before it: `cellular` + `set_sensor` Extended-Controls tools and
`launch_dev_client`.

## Next up

Pulled from [docs/BACKLOG.md](docs/BACKLOG.md) — see there for full context.

**XcodeBuildMCP parity gaps** (priority order)
- [ ] Deeper project discovery — module/build-info dump (the `list_gradle_variants` half shipped v0.14.0; the per-module `projects`/`properties` dump is still open)
- [ ] Project scaffolding — new Android project from a template (biggest lift)

**Field feedback** (open items; most rounds shipped in v0.8.0–v0.13.0, see CHANGELOG)
- [ ] App/bundle state probe — Metro vs embedded bundle, pid/uptime, HMR connected (most expensive gap)
- [ ] `biometric_auth` that discovers the enrolled finger id — parser needs live-emulator `dumpsys fingerprint` output to write honestly; **held for a device pass** (this box has no SDK/adb)
- [ ] Verify `reload_app`/`open_dev_menu` on a real Expo dev client
- [ ] Residual describe_ui auto-filter noise — single-child chain collapse (clickable/query/compact cover it today)
- [ ] Accessibility-action tap for native surfaces — coordinate `input tap` no-ops on Compose/RN `NativeTabs` bars where Maestro's `tapOn` (UiAutomator `ACTION_CLICK`) works (`android-mcp` #019f75a8). `tap identify` (v0.14.0) diagnoses it; the real fix needs a live-emulator pass (no simple adb command — likely a UiAutomator route)
- [ ] DECISION: Maestro integration (`run_maestro_flow`) — deliberate yes/no, see BACKLOG.md
- [ ] DECISION: `run_sequence` batching (steps + sleeps ± if-present guard, batch tap folds in) — decide together with Maestro

**Enhancements**
- [ ] Multi-touch / pinch-zoom (needs `sendevent`; single-pointer `drag` already shipped) — parked, no reliable cross-device approach yet
- [ ] Real-device `set_battery` path via `adb shell dumpsys battery set …` (the console tools are emulator-only)

## Ground rules

- Every device-facing tool takes an optional `serial`; single-device sessions omit it.
- Device commands are `adb.Client` methods; pure logic (parsing, geometry) lives in `internal/uiauto` or a plain func with its own test. `internal/tools` stays a thin MCP binding (see [ARCHITECTURE.md](ARCHITECTURE.md)).
- Unit-test any new logic: a command builder with a fake `Runner`, pure logic directly.
