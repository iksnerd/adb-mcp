# Backlog & ideas

Open, unstarted work. Shipped history is in [CHANGELOG.md](CHANGELOG.md).

## XcodeBuildMCP parity gaps

Core driving/build/test/automate is at parity (and ahead on screen recording,
device-lock/Keystore, custom PIN pads, `tap_on_text`/`wait_for_text`,
`set_status_bar`). These are the remaining gaps vs XcodeBuildMCP:

- [ ] **`build_and_run`** — one-shot `gradle_build` → `install_app` → `launch_app` (optionally on a chosen variant). Currently composable from three calls; XcodeBuildMCP exposes it as a single tool. Highest value / lowest lift.
- [ ] **Deeper project discovery** — no analogue of "list schemes / dump build settings". Add `list_gradle_variants` (parse `./gradlew tasks`/`app:properties` or the `assemble*` task list) and a module/build-info dump, complementing `list_gradle_tasks` + `get_app_details`.
- [ ] **Project scaffolding** — no "create a new Android project from a template" tool (XcodeBuildMCP has `scaffold`). Biggest lift; would need a bundled template + Gradle wrapper generation.
- [x] **Embedded runtime-crash telemetry (`last_crash`)** — shipped v0.10.0. `last_crash` pulls `dumpsys dropbox --print` (data_app_crash + native) so the whole fatal comes back in one call. A live streaming variant (vs. on-demand pull) is still open if it proves useful.

## Enhancements

- [ ] **Multi-touch / pinch-zoom gestures.** The single-pointer half shipped as `drag` (`input draganddrop`). True two-finger pinch/rotate needs the `sendevent` multi-touch protocol, which is device/kernel-specific (the `input` command has no multi-pointer verb) — parked until there's a reliable cross-device approach.

## Field feedback (real-world debugging sessions, 2026-07-15)

From real-world field feedback — real friction driving a React Native/Expo
dev-client app across several long debugging sessions. Most
items from these sessions have shipped (see CHANGELOG v0.8.0–v0.10.0); what's
left:

- [ ] **`launch_app` dev-server deep link.** For Expo/RN dev builds, `launch_app` lands on the Dev Launcher (which then needs Metro). A dedicated option to launch a dev-server URL directly would skip that hop. (`open_url` with the `exp://` / dev-client URL is a working stopgap today.) The "which surfaces are drivable vs opaque" docs are now covered in `android://guide/driving`.

## Conventions (read before adding a tool)

- Every device-facing tool takes an optional `serial`; single-device sessions can omit it.
- Keep `internal/android` pure/testable; `internal/tools` stays a thin MCP binding. Each `tools/<domain>.go` mirrors an `android/<domain>.go` — see [../ARCHITECTURE.md](../ARCHITECTURE.md).
- Add unit tests for any new pure logic (parsers, coordinate math, arg parsing).
- Open a [GitHub issue](https://github.com/iksnerd/adb_mcp/issues) for feedback, bugs, or tool requests.
