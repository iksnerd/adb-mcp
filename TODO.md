# adb_mcp — roadmap

The Android counterpart to [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP).
This file is the lean hub — only what's **open**. Shipped work lives in the
CHANGELOG; details for ideas live in the BACKLOG.

**Current:** v0.13.0 · 64 tools + 4 guide resources · [tool reference in README](README.md#tools)
Core parity with [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP) reached; remaining gaps below.

## Map

| Doc | What's in it |
|---|---|
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | Everything shipped, newest first (v0.1.0 → v0.13.0) |
| [docs/BACKLOG.md](docs/BACKLOG.md) | Open ideas + the conventions to follow when adding a tool |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Package layout (sdk/uiauto/adb/gradle/tools) + how to add a tool (with diagram) |

## Recently shipped (v0.13.0)

See [CHANGELOG](docs/CHANGELOG.md) (v0.13.0): two more Extended-Controls tools
(`cellular` — data/voice/signal/network throttling; `set_sensor` —
accelerometer/light/proximity/…) and `launch_dev_client` (Expo dev build →
Metro, skipping the Dev Launcher). All unit-tested against exact adb argv with
no device.

v0.12.0 before it: six Extended-Controls tools; the `adb.Client` refactor +
four-package split; the `tap_element`/`build_and_run` review fixes.

## Next up

Pulled from [docs/BACKLOG.md](docs/BACKLOG.md) — see there for full context.

**XcodeBuildMCP parity gaps** (priority order)
- [ ] Deeper project discovery — `list_gradle_variants` + module/build-info dump
- [ ] Project scaffolding — new Android project from a template (biggest lift)

**Field feedback** (open items; most rounds shipped in v0.8.0–v0.13.0, see CHANGELOG)
- [ ] App/bundle state probe — Metro vs embedded bundle, pid/uptime, HMR connected (most expensive gap)
- [ ] `biometric_auth` that discovers the enrolled finger id — parser needs live-emulator `dumpsys fingerprint` output to write honestly; **held for a device pass** (this box has no SDK/adb)
- [ ] Verify `reload_app`/`open_dev_menu` on a real Expo dev client
- [ ] Residual describe_ui auto-filter noise — single-child chain collapse (clickable/query/compact cover it today)
- [ ] DECISION: Maestro integration (`run_maestro_flow`) — deliberate yes/no, see BACKLOG.md
- [ ] DECISION: `run_sequence` batching (steps + sleeps ± if-present guard, batch tap folds in) — decide together with Maestro

**Enhancements**
- [ ] Multi-touch / pinch-zoom (needs `sendevent`; single-pointer `drag` already shipped) — parked, no reliable cross-device approach yet
- [ ] Real-device `set_battery` path via `adb shell dumpsys battery set …` (the console tools are emulator-only)

## Ground rules

- Every device-facing tool takes an optional `serial`; single-device sessions omit it.
- Device commands are `adb.Client` methods; pure logic (parsing, geometry) lives in `internal/uiauto` or a plain func with its own test. `internal/tools` stays a thin MCP binding (see [ARCHITECTURE.md](ARCHITECTURE.md)).
- Unit-test any new logic: a command builder with a fake `Runner`, pure logic directly.
