# adb_mcp — roadmap

The Android counterpart to [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP).
This file is the lean hub — details live in linked docs so it stays readable.

**Current:** v0.11.1 · 53 tools + 4 guide resources · [tool reference in README](README.md#tools)
Installed + smoke-tested live on an emulator (last_crash verified against a real recorded crash; launch_app failure/component echo verified; screenshot black-frame detection verified).
Core parity with [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP) reached; remaining gaps below.

## Map

| Doc | What's in it |
|---|---|
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | Everything shipped, newest first (v0.1.0 → v0.10.0) |
| [docs/BACKLOG.md](docs/BACKLOG.md) | Open ideas + the conventions to follow when adding a tool |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Two-layer mirror layout + how to add a tool (with diagram) |

## Next up

Pulled from [docs/BACKLOG.md](docs/BACKLOG.md) — see there for full context.

**XcodeBuildMCP parity gaps** (priority order)
- [ ] `build_and_run` — one-shot build → install → launch (highest value / lowest lift)
- [ ] Deeper project discovery — `list_gradle_variants` + module/build-info dump
- [ ] Project scaffolding — new Android project from a template (biggest lift)

**Field feedback** (see BACKLOG.md — most items shipped in v0.8.0–v0.10.0)
- [ ] `launch_app` dev-server deep link for Expo/RN dev clients (`open_url` is a stopgap today)

**Field feedback rounds 3–4** (2026-07-17 — most items shipped in v0.11.0, see CHANGELOG; still open:)
- [ ] App/bundle state probe — Metro vs embedded bundle, pid/uptime, HMR connected (most expensive gap)
- [ ] `tap_element(resource_id)` — re-resolve at tap time; overlays can eat coordinate taps
- [ ] Verify `reload_app`/`open_dev_menu` on a real Expo dev client
- [ ] DECISION: Maestro integration (`run_maestro_flow`) — deliberate yes/no, see BACKLOG.md
- [ ] DECISION: `run_sequence` batching (steps + sleeps ± if-present guard) — decide together with Maestro

**Enhancements**
- [ ] Multi-touch / pinch-zoom (needs `sendevent`; single-pointer `drag` already shipped) — parked, no reliable cross-device approach yet

## Ground rules

- Every device-facing tool takes an optional `serial`; single-device sessions omit it.
- `internal/android` stays pure/testable; `internal/tools` stays a thin MCP binding (see [ARCHITECTURE.md](ARCHITECTURE.md)).
- Unit-test any new pure logic (parsers, coordinate math, arg parsing).
