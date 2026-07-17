# adb_mcp — roadmap

The Android counterpart to [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP).
This file is the lean hub — details live in linked docs so it stays readable.

**Current:** v0.11.2 · 53 tools + 4 guide resources · [tool reference in README](README.md#tools)
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
- [x] `build_and_run` — one-shot build → install → launch
- [ ] Deeper project discovery — `list_gradle_variants` + module/build-info dump
- [ ] Project scaffolding — new Android project from a template (biggest lift)

**Field feedback** (see BACKLOG.md — most items shipped in v0.8.0–v0.10.0)
- [ ] `launch_app` dev-server deep link for Expo/RN dev clients (`open_url` is a stopgap today)

**Field feedback rounds 3–5** (2026-07-17 — most items shipped in v0.11.0–v0.11.2, see CHANGELOG; still open:)
- [ ] App/bundle state probe — Metro vs embedded bundle, pid/uptime, HMR connected (most expensive gap)
- [ ] `biometric_auth` that discovers the enrolled finger id (needs live-emulator verification)
- [x] `tap_element(resource_id)` — re-resolve at tap time; overlays can eat coordinate taps
- [ ] Verify `reload_app`/`open_dev_menu` on a real Expo dev client
- [ ] Residual describe_ui auto-filter noise — single-child chain collapse (clickable/query/compact cover it today)
- [ ] DECISION: Maestro integration (`run_maestro_flow`) — deliberate yes/no, see BACKLOG.md
- [ ] DECISION: `run_sequence` batching (steps + sleeps ± if-present guard, batch tap folds in) — decide together with Maestro

**Enhancements**
- [ ] Multi-touch / pinch-zoom (needs `sendevent`; single-pointer `drag` already shipped) — parked, no reliable cross-device approach yet

**Emulator Extended-Controls surface** (via the emulator console — `adb emu <cmd>`, no auth token needed; the same bridge `fingerprint_touch`/`set_location` already use). The Extended Controls panel is the emulator's own Qt window, invisible to `describe_ui`/`tap`, so these must go through the console, not screen taps. Emulator-only (physical devices have no console) unless an `adb shell` path exists. Candidate tools, in rough priority for testing auth/2FA/telephony flows:
- [ ] `battery` — `adb emu power capacity <pct>` / `power ac on|off` / `power status`; also has a real-device `adb shell dumpsys battery set …` path worth using when available
- [ ] `send_sms` — `adb emu sms send <number> <text>` (inject an incoming SMS — OTP/2FA flows)
- [ ] `phone_call` — `adb emu gsm call|accept|busy|cancel <number>` (incoming call / interruption testing)
- [ ] `cellular` — `adb emu gsm data|voice|signal`, `network speed|delay` (signal loss, throttling)
- [ ] `sensors` / `rotate` — `adb emu sensor set <name> <x> <y> <z>`, `adb emu rotate`
- [ ] `finger_remove` — `adb emu finger remove` (complement to `fingerprint_touch`)
- [ ] Snapshots — `adb emu avd snapshot save|load|list`, `avd pause|stop` (deterministic session reset)
- Note: all follow the exact `FingerTouch`/`SetLocation` pattern in `internal/android/emulator.go`/`environment.go` — small, uniform additions.

**Code review round** (2026-07-17, 8-angle review of the layout move + `tap_element` + `build_and_run`; all fixed same day)
- [x] `build_and_run` could install a stale/androidTest APK: `FindAPKs` sorted lexically despite its "newest first" comment. Now sorts by mtime newest-first, prunes `node_modules`/dot-dirs from the walk, and `PickAPK` skips androidTest APKs — all unit-tested.
- [x] `tap_element` searched a `FilterAuto` snapshot, which drops unlabeled id-carrying wrapper nodes (parent-equal bounds) — the exact elements the tool targets. Now uses `filter=all`.
- [x] Empty/whitespace `resource_id` (or `text`) substring-matched *every* element → arbitrary tap reported as success. Guarded in `FindByText`/`FindByResourceID` and at the tool layer.
- [x] `InstallApp` trusted adb's exit code only; older platform-tools print `Failure [INSTALL_FAILED_…]` with exit 0. Output is now scanned, so `install_app` and `build_and_run` both fail loudly.
- [x] `tap_on_text`/`tap_element` and `gradle_build`/`build_and_run` were copy-paste pairs → shared `findAndTap` and `buildAPKs` helpers; `FindByText`/`FindByResourceID` share `findFirst`. Both tap tools gained `verify_change`.
- [x] `go install github.com/iksnerd/adb_mcp@latest` (advertised in the changelog) broke with the `cmd/` move — README now documents the `…/cmd/adb-mcp@latest` path.
- [x] Test fixtures loaded via a panicking package-level var → `readTestdata(t, …)` with `t.Fatalf`.

**Go layout / structure** (2026-07-17 audit against Go directory conventions)
- [x] Move `main.go` → `cmd/adb-mcp/main.go`. Turned out lower-risk than the audit assumed: `install.sh` only downloads prebuilt release archives by binary name, no `go install module@latest` path is advertised anywhere, so only the Makefile, README build line, and release.yml needed the `./cmd/adb-mcp` path update.
- [x] Move uiauto XML fixtures from string literals in `uiauto_test.go` to `internal/android/testdata/*.xml`
- [x] Refactor package-level `runAdb`/`adbPath`/`commandEnv` into an `adb.Client` with an injectable `Runner` — done. Every device command is now a method on `*adb.Client`; `New(serial)` wires real adb, tests wire a fake runner and assert exact argv. New `client_test.go` covers the builders (tap/swipe/drag/keycombo/input-text escaping/install failure-scan/launch parse/finger guard/lock/…) with **no device**.
- [x] Split `internal/android` into a clean package graph — done (chose the 4-package layout): `internal/sdk` (paths/env), `internal/uiauto` (pure parsing + Element model), `internal/adb` (device Client), `internal/gradle` (build/APKs/test-report). Deps point inward only: `tools → adb, gradle, uiauto → sdk`; no cycles, no MCP import below `tools`. ARCHITECTURE.md + diagram + README updated.
- Fine as-is: no `pkg/` dir, `internal/` everywhere, embedded guides, module name (renaming `adb_mcp` → `adb-mcp` breaks all imports; not worth it post-release)

## Ground rules

- Every device-facing tool takes an optional `serial`; single-device sessions omit it.
- `internal/android` stays pure/testable; `internal/tools` stays a thin MCP binding (see [ARCHITECTURE.md](ARCHITECTURE.md)).
- Unit-test any new pure logic (parsers, coordinate math, arg parsing).
