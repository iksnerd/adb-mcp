# Backlog & ideas

Open, unstarted work. Shipped history is in [CHANGELOG.md](CHANGELOG.md).

## XcodeBuildMCP parity gaps

Core driving/build/test/automate is at parity (and ahead on screen recording,
device-lock/Keystore, custom PIN pads, `tap_on_text`/`wait_for_text`,
`set_status_bar`). These are the remaining gaps vs XcodeBuildMCP:

- [x] **`build_and_run`** — one-shot `gradle_build` → `install_app` → `launch_app`. Shipped: installs the first APK the build produces; pass a variant-specific `task` to disambiguate multi-flavor projects.
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

## Field feedback, round 3 (biometric / lock-screen sessions, 2026-07-17)

Four new reports from live driving sessions (council-hub
`android-emulator-mcp-feedback`, messages #019f6f96). Recurring theme: the
tools report what they *did* well and what they *couldn't see* poorly —
occluded windows, filtered nodes, no-op actions all look identical to
"nothing there".

- [x] **No biometric simulation** (highest value). `enter_pin`/`set_device_lock` exist but nothing drives fingerprint, so agents can only ever test the PIN fallback. Emulator supports `adb emu finger touch <id>` directly. Add `fingerprint_touch`; document the enrollment workflow (Settings flow + finger touch during enroll).
- [x] **`describe_ui` is silent about system-window occlusion.** When BiometricPrompt (or a permission dialog / dev-client overlay) is up, the response is systemui's tree with no indication the target app is occluded — reads as "the app broke". Add a `top window:` line (`dumpsys window` focus) and a warning when it isn't the expected app.
- [x] **`describe_ui` filtering makes absence untrustworthy & payload is still noisy.** The "pure-layout containers are filtered out" claim doesn't hold (a tab screen returned ~24 elements, 2 clickable; 5-deep `navigation_bar_item_*` chains with identical bounds survive because they carry resource ids). And because *some* filtering happens, "not in the output" can't distinguish absent from filtered. Fix: `filter: auto|all|clickable` param, drop identical-bounds textless wrappers in auto, report a hidden-node count, and make the description honest.
- [x] **Action tools report success without effect.** `press_key(back)` returns success-shaped output while a BiometricPrompt eats the event — every action needs a describe_ui round-trip to learn if it did anything. Opt-in `verify_change` returning `ui_changed` (hierarchy signature before/after).
- [x] **No plain `wait`.** `wait_for_text` is condition-based; "background the app 18s to trip a native auth timer" has no tool. Add `wait{seconds}`.
- [x] **`logcat` has no time window.** `lines` is the wrong axis for "the user just hit this error" — on a chatty emulator 300 lines can be <10s. Add `since` (e.g. `"2m"`, device-clock based → `adb logcat -t '<time>'`). The paired `tag`/`priority` asks already shipped (v0.7.0+); this closes the remaining round-trip.
- [x] **Guide correction — PIN-pad visibility is pad-specific.** `android://guide/pin-and-lock` says pads are canvas-drawn/invisible; a native Kotlin `PinPadView` was fully visible to `describe_ui` (digits as `Button` text, Cancel by content-desc, **no view ids** — match by label). Split the guidance: RN/Skia pad → grid/coords; native pad → hierarchy match.
- [ ] **DECISION NEEDED — Maestro integration.** `run_maestro_flow{path, appId?, env?}` returning structured per-step pass/fail. The server owns every primitive and the emulator lifecycle Maestro flows need, but drops out the moment work becomes "run/debug the E2E suite". Counter: wrapping another tool's CLI is a slippery slope. Reporter explicitly asks for a deliberate yes/no rather than drift — decide before implementing.

## Field feedback, round 4 (back-gesture + re-lock sessions, 2026-07-17)

From council-hub `android-mcp-papercuts` (#019f6fad) and the
`android-emulator-mcp-feedback` addendum (#019f6fb4). Headline lesson from the
reporter: *"a tool that can't participate in a scripted sequence tends to get
abandoned wholesale"* — one missing primitive (a fixed sleep) pushed an entire
session into raw bash. Two of the session's wrong conclusions trace to
absence-of-logs being unverifiable (buffer rotation / embedded bundle).

- [x] **`clear_logcat`.** The press→observe loop needs "read only what THIS action produced"; with no clear, a filter hit may be 10 minutes old and a miss may be rotation (caused a false-negative theory). `since` (shipping with this round) covers most cases; an explicit clear is still the sharpest isolation primitive. Trivial: `adb logcat -c`.
- [x] **`describe_ui` query + compact mode.** Payload is ~10x the information needed for geometry work (~2k tokens for a 20-element screen vs a ~150-token `text | bounds` table). Add `query` (substring on text/content_desc/resource_id — answers "is X on screen?" cheaply, incl. with filter=all for trustworthy absence) and `compact: true` (one line per element).
- [x] **`adb_reverse` / port forwarding.** Nothing in the server touches emulator↔host networking; a dev client that can't reach Metro silently falls back to its embedded bundle — reporter burned most of a session testing code that was never running. Workaround was one command: `adb reverse tcp:8081 tcp:8081`.
- [ ] **App/bundle state probe (the most expensive gap).** No way to tell a Metro-connected process from one running its embedded bundle, or to see that presses and log-reads were hitting *different* processes. Proposal: extend `doctor`/`get_app_details` with per-app runtime state — pid, process uptime, install time, bundle source (Metro URL vs embedded), HMR connected. Heuristics: `HMRClient`/`DevSupport` presence in logs, `dumpsys package` lastUpdateTime.
- [x] **`tap_element(resource_id)`.** Shipped: mirrors `tap_on_text` but matches by resource_id (substring by default, `partial=false` for exact), re-resolving the element right before tapping to narrow the window where a stale coordinate lands on an overlay.
- [ ] **DECISION NEEDED — `run_sequence` batching.** `home → sleep 19 → launch → sleep 9 → if-present(cancel) → dump`: 6+ round trips today, and for native-timer flows (background token clears, biometric auto-fire on RESUME) the round trips *change the timing being tested*. Even a minimal steps+sleeps version (no branching) would move those flows back onto the server. Related to (but smaller than) the Maestro question above — decide together.
- [ ] **Verify `reload_app`/`open_dev_menu` against real Expo dev clients.** Reporter (on an older tool build) found keycode-82 and the RELOAD broadcast both no-ops on an Expo dev build; our v0.8.0 tools use the same mechanisms. Confirm they work on a current dev client, and document which reload path applies where.
- [x] **Guide: KEYCODE_HOME under automation may cold-start instead of backgrounding.** Backgrounding 18-19s produced the expected lifecycle transition only ~50% of the time; when the app "re-locked" it was actually a cold process start. Now noted in `android://guide/driving`.

## Field feedback, round 5 (biometric-loop + stale-install reports, 2026-07-17 afternoon)

From `android-mcp-papercuts` #019f709b and #019f70d1.

- [x] **`enter_pin` blind-tap guard.** With `grid`/`coords` it tapped straight into a BiometricPrompt (no pad on screen). Now refuses when a biometric window has focus, pointing at `fingerprint_touch` / cancel-to-PIN. (v0.11.2)
- [x] **Fingerprint id troubleshooting.** `emu finger touch 1` returns OK without authenticating when the enrolled id ≠ 1 (re-enrollments increment it). Tool description + pin-and-lock guide now cover: try ids 2..5, double-touch timing, deterministic re-enrollment. (v0.11.2)
- [x] **`doctor` reports the server version.** Reporter burned a session concluding v0.11.0 params "regressed" when their install was simply pre-v0.11.0. `doctor` now leads with the serving binary's version + the `adb-mcp update` pointer. (v0.11.2)
- [ ] **`biometric_auth` that knows the enrolled id.** The robust version of `fingerprint_touch`: discover enrolled finger ids (needs a verified probe — `dumpsys fingerprint`/`biometric` output varies by image) and touch the right one, maybe `success|fail` semantics. Needs a live-emulator verification pass before shipping.
- [ ] **Force-PIN path.** An `auth_prefer_pin`-style way to reliably reach the PIN pad instead of the biometric prompt. App-controlled in general (the app decides to auto-fire biometrics); may reduce to a documented cancel loop. Investigate before promising a tool.
- [ ] **Batch tap.** XcodeBuildMCP has a batched same-screen tap; each of ours is a round trip. Low severity; fold into the `run_sequence` decision rather than shipping a one-off.
- [ ] **Residual `auto`-filter noise.** The identical-bounds rule kills only part of Material's `navigation_bar_item_*` chain (nested wrappers have distinct sub-bounds). Remaining idea from round 3: collapse single-child layout chains to their meaningful leaf. `filter=clickable`/`query`/`compact` are the practical answer today.

## Conventions (read before adding a tool)

- Every device-facing tool takes an optional `serial`; single-device sessions can omit it.
- Keep the execution layers (`internal/adb` device client, `internal/uiauto` parsing, `internal/gradle` builds) pure/testable; `internal/tools` stays a thin MCP binding. Device commands are `adb.Client` methods; each `tools/<domain>.go` mirrors its execution file — see [../ARCHITECTURE.md](../ARCHITECTURE.md).
- Add unit tests for any new pure logic (parsers, coordinate math, arg parsing).
- Open a [GitHub issue](https://github.com/iksnerd/adb_mcp/issues) for feedback, bugs, or tool requests.
