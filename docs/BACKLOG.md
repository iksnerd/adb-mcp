# Backlog & ideas

Open, unstarted work. Shipped history is in [CHANGELOG.md](CHANGELOG.md).

## XcodeBuildMCP parity gaps

Core driving/build/test/automate is at parity (and ahead on screen recording,
device-lock/Keystore, custom PIN pads, `tap_on_text`/`wait_for_text`,
`set_status_bar`). These are the remaining gaps vs XcodeBuildMCP:

- [x] **`build_and_run`** — one-shot `gradle_build` → `install_app` → `launch_app`. Shipped: installs the first APK the build produces; pass a variant-specific `task` to disambiguate multi-flavor projects.
- [x] **Deeper project discovery** — `list_gradle_projects` plus `gradle_project_properties` provide the module map and evaluated per-module properties.
- [x] **Project scaffolding** — `scaffold_android_project` creates a minimal Kotlin Android project in a new empty directory.
- [x] **Embedded runtime-crash telemetry (`last_crash`)** — shipped v0.10.0. `last_crash` pulls `dumpsys dropbox --print` (data_app_crash + native) so the whole fatal comes back in one call. A live streaming variant (vs. on-demand pull) is still open if it proves useful.

**Re-audited 2026-08-13 against `getsentry/XcodeBuildMCP`'s actual source tree** (`src/mcp/tools/`, ~79 tool files — their docs site loads the tool list via JS, so the tree is the ground truth, not the rendered page). Core driving/build/test/UI-automation/scaffolding is at parity or ahead; three category-level gaps remain, none previously tracked:

- [ ] **Debugging.** XcodeBuildMCP ships 7 LLDB-backed tools (`debug_attach_sim`, `debug_breakpoint_add`/`_remove`, `debug_continue`, `debug_detach`, `debug_lldb_command`, `debug_stack`, `debug_variables`). adb-mcp has no equivalent — no attach, breakpoints, or stack/variable inspection for a running app. Android's analogue would be JDWP-based (`jdb`, or driving Android Studio's debugger bridge) for Java/Kotlin, which is a different mechanism than LLDB and a substantial scope increase; needs a deliberate yes/no before starting, not silent drift.
- [ ] **Code coverage reporting.** `get_coverage_report`/`get_file_coverage` parse `xcresult` into per-target and per-function coverage. adb-mcp runs `run_unit_tests`/`run_instrumented_tests` but never surfaces JaCoCo coverage output — no gap in running tests, just in reporting what they covered.
- [ ] **Session defaults.** `session_set_defaults`/`session_show_defaults`/`session_clear_defaults`/`session_use_defaults_profile` let an agent pin a default project/scheme/simulator so later calls omit them. adb-mcp's `serial` being optional for a single device covers most of the need; there's no analogue for a default `project_dir`/variant across a multi-project or multi-flavor session. Lower priority than the two above — no field report has asked for it yet.

## Enhancements

- [ ] **Multi-touch / pinch-zoom gestures.** The single-pointer half shipped as `drag` (`input draganddrop`). True two-finger pinch/rotate needs the `sendevent` multi-touch protocol, which is device/kernel-specific (the `input` command has no multi-pointer verb) — parked until there's a reliable cross-device approach.
- [x] **Real-device `set_battery`.** Shipped v0.17.0. Emulator keeps the console path (`emu power`); a physical device forces values via `dumpsys battery set level/ac`, with a `reset` option (`dumpsys battery reset`) to restore automatic reporting. Verified live.
- [x] **Perf: decode the screenshot PNG once.** Shipped v0.17.0. `CaptureScreen` decoded each frame in `isMostlyBlack` and again in `downscalePNG` (~85ms/18MB per `png.Decode` on a 2076×2152 frame, plus a re-decode per black-retry). Now one decode is shared between the black-check and the downscale — roughly halves the CPU/allocs of the most-called tool.

## Field feedback (real-world debugging sessions, 2026-07-15)

From real-world field feedback — real friction driving a React Native/Expo
dev-client app across several long debugging sessions. Most
items from these sessions have shipped (see CHANGELOG v0.8.0–v0.10.0); what's
left:

- [x] **`launch_app` dev-server deep link.** Shipped v0.13.0 as **`launch_dev_client`**: builds the `<scheme>://expo-development-client/?url=http://host:port` deep link and opens it via ACTION_VIEW, skipping the Dev Launcher's server picker. Host/port default to `localhost:8081` (pair with `adb_reverse tcp:8081`). Expo Go's plain `exp://` URL still goes through `open_url`. (Deep-link format built to Expo's documented spec; a live dev-build pass would confirm the scheme-resolution edge cases.)

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
- [x] **DECISION — Maestro integration deferred.** `run_sequence` now provides in-process batching, sleeps, guards, and per-step results. Keep Maestro external until there is a concrete requirement for structured cross-tool reporting; avoid adding a second E2E execution model to this server.

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
- [x] **App/bundle state probe (the most expensive gap).** **Shipped v0.16.0; strengthened after the 2026-08-05 field report.** Reports installed?/running? + pid(s), main-process uptime (`ps ETIME`), first-install/last-update (`dumpsys package`), and a Metro-vs-embedded bundle verdict from recent logcat markers. When modern Expo/RN logs omit those markers, it falls back to an established connection on common Metro/Expo ports and reports `bundle_signals` (`logcat` or `live_socket`). Flags multiple live processes for one package. Unit-tested with synthetic RN logs and `/proc/net/tcp` data.
- [x] **`tap_element(resource_id)`.** Shipped: mirrors `tap_on_text` but matches by resource_id (substring by default, `partial=false` for exact), re-resolving the element right before tapping to narrow the window where a stale coordinate lands on an overlay.
- [x] **`run_sequence` batching.** **Shipped v0.16.0.** Steps — `sleep`, `tap`, `tap_text`, `tap_element`, `key`, `text`, `swipe`, `launch`, `stop`, `wait_text`, `describe_ui` — run in one call over the existing client methods, with `if_present`/`if_absent` guards (the conditional-cancel idiom) and per-step `optional`. Returns per-step results (ok/skipped/error) + the final hierarchy; a non-optional error stops the rest. The timing argument was the real driver: a per-step agent round-trip perturbs native-timer flows (background-token clear, biometric auto-fire on RESUME), so batching is the only faithful reproduction. Batch-tap folds in as a run of `tap` steps. Verified live end-to-end. Decided separately from Maestro.
- [~] **Verify `reload_app`/`open_dev_menu` against real Expo dev clients.** The classic broadcast and dev-menu fallback are implemented and documented; a current Expo classic/bridgeless live matrix pass remains.
- [x] **Guide: KEYCODE_HOME under automation may cold-start instead of backgrounding.** Backgrounding 18-19s produced the expected lifecycle transition only ~50% of the time; when the app "re-locked" it was actually a cold process start. Now noted in `android://guide/driving`.

## Field feedback, round 5 (biometric-loop + stale-install reports, 2026-07-17 afternoon)

From `android-mcp-papercuts` #019f709b and #019f70d1.

- [x] **`enter_pin` blind-tap guard.** With `grid`/`coords` it tapped straight into a BiometricPrompt (no pad on screen). Now refuses when a biometric window has focus, pointing at `fingerprint_touch` / cancel-to-PIN. (v0.11.2)
- [x] **Fingerprint id troubleshooting.** `emu finger touch 1` returns OK without authenticating when the enrolled id ≠ 1 (re-enrollments increment it). Tool description + pin-and-lock guide now cover: try ids 2..5, double-touch timing, deterministic re-enrollment. (v0.11.2)
- [x] **`doctor` reports the server version.** Reporter burned a session concluding v0.11.0 params "regressed" when their install was simply pre-v0.11.0. `doctor` now leads with the serving binary's version + the `adb-mcp update` pointer. (v0.11.2)
- [~] **`biometric_auth` that knows the enrolled id.** The robust version of `fingerprint_touch`. The live-emulator pass is **done** (2026-07-18) and reframed the design — see round 7 below: `dumpsys fingerprint` gives only an enrolled count (no id), a wrong id trips a HAL lockout, so the tool is `has_biometric_enrolled` + deterministic re-enroll, not runtime id-discovery.
- [x] **Force-PIN path.** `prefer_pin` now selects common system credential-fallback buttons or sends BACK, then asks the caller to confirm the pad with `describe_ui`. App-controlled prompts may still suppress this path.
- [x] **Batch tap.** Folded into `run_sequence` in v0.16.0; a same-screen batch is a sequence of `tap` steps, so a one-off tool is unnecessary.
- [x] **Residual `auto`-filter noise.** Auto now also collapses unlabelled, non-clickable single-child layout chains; `filter=clickable`/`query`/`compact` remain available for compact or exact queries.

## Field feedback, round 6 (coordinate tap no-op on NativeTabs, 2026-07-18)

From `android-mcp` #019f75a8 (a client app, Pixel_9a).

- [x] **Tell the tap failure modes apart.** A coordinate `tap` on a native `NativeTabs` bottom bar (`expo-router/unstable-native-tabs`) returned success but navigated nowhere; three points inside the tab's clickable bounds, all no-ops, `describe_ui` after each byte-identical. Nothing distinguished "tap missed" from "landed but no effect" from "UI changed but describe_ui didn't see it". Shipped v0.14.0: `tap identify` reports which element the coordinate hit (or a non-clickable wrapper / no reported element); the existing `verify_change` answers "did the UI change?". Together they separate all three.
- [~] **Accessibility-action tap for native surfaces (the real fix).** Maestro's `tapOn` navigated the same bar first try — it clicks through UiAutomator (`AccessibilityNodeInfo.performAction(ACTION_CLICK)`), which reaches views that a low-level `input tap` coordinate event doesn't (Compose/RN `NativeTabs`, some overlays). The repository now records this as an explicit device-side blocker: there is no one-line adb equivalent, so implementation needs an accessibility/UiAutomator bridge and a live-emulator verification pass. Candidate: an accessibility-click path inside `tap_element`/`tap_on_text` with coordinate fallback.

## Field feedback, round 7 (live smoke test on Android 17 AVD, 2026-07-18)

From driving the shipped tools on `emulator-5554` (Pixel AVD, API 17) during the v0.14.0 verification pass.

- [x] **`press_key` had no wake/sleep alias.** Waking the screen meant a raw keycode (`224`); `power` toggles and can sleep an awake screen. Added `wakeup` (224) / `sleep` (223) name aliases.
- [x] **`stay_awake` / screen-timeout tool.** Shipped in v0.15.0 via `svc power stayon`; `enabled=false` restores normal timeout.
- [x] **`enter_pin` keyguard retry.** Shipped in v0.15.0: retries settled hierarchy reads and reports flaky dumps/covering windows distinctly from canvas-drawn pads.
- [~] **`biometric_auth` design settled by live probing (round 5 item reframed).** On this image `dumpsys fingerprint` reports only a per-user enrolled *count*, never the finger id; `emu finger touch <id>` reaches the ranchu HAL (`fpHash=<id>` in logcat) but `onAuthenticationFailed` unless the id matches the enrolled one — and the HAL locks out after ~2-3 wrong touches. So a runtime id-sweep is the wrong design (it degrades the device). The robust tool is `has_biometric_enrolled` (count>0, reliable) + a deterministic re-enroll that captures the assigned id from the enrollment HAL log — not id-guessing at auth time. **`has_biometric_enrolled` shipped v0.16.0** (JSON `"prints":[{"count":N}]` sum + legacy-text fallback; live-verified 0→1 by enrolling a fingerprint through the Settings wizard). Still open: the deterministic re-enroll that reads the id back from the enrollment HAL log.

## Field feedback, round 8 (foldable multi-display screenshot, 2026-07-19)

From `android-emulator-mcp-feedback` #019f7abc (a client app on a Pixel Fold AVD).

- [x] **`screenshot` corrupt on multi-display foldables.** **Shipped v0.16.0.** With >1 physical display, `screencap -p` (no `-d`) prints a `[Warning] Multiple displays were found …` line to stdout *before* the PNG, shifting the header so it won't decode — the tool returned `0x0` / undecodable, 100% unusable on a foldable. Fix: strip any bytes before the `\x89PNG` signature from every screencap (robust, display-agnostic, harmless single-display). Plus an optional `display` selector on `screenshot` (`inner`/`cover`/HWC-index/physical-id). Landmine confirmed live and NOT as the reporter guessed: `screencap -d` needs the **physical** display id from `dumpsys SurfaceFlinger --display-id` (e.g. `4619827259835644672`), *not* the logical id `0` — passing `-d 0` fails with "Failed to take the screenshot", so the byte-strip (not `-d`) is the primary fix.

## Field feedback, round 9 (foreground/staleness gaps from a timing-sensitive bug, 2026-08-07)

From `android-mcp-papercuts` #019fdb7d (a client app, `emulator-5554`, Android 17/API 37) — a ~3h session diagnosing an app-exit/reopen bug (an internal tracker ticket follow-up, fixed and shipped separately). Not yet actioned.

- [x] **`app_state` can't answer "is the app foregrounded?"** Shipped: `foreground` + `top_activity` from `dumpsys activity activities`; `run_sequence` also has `assert_foreground`.
- [x] **`app_state` Metro staleness signal.** Optional `source_path` compares newest host source mtime with the latest epoch-timed Metro/HMR marker and reports `bundle_stale`, `source_mtime`, and `last_hmr_update`.
- [x] **`run_sequence` assertion/timing gap.** Shipped: `assert_foreground` and per-step `elapsed_ms`.
- [x] **`launch_dev_client` reports success on `DevLauncherErrorActivity`.** Shipped: detects the error activity and includes its visible text when available.

Self-audit note from the same report, for calibration: most of the session's raw-`adb` use was the reporter's own habit, not a tool gap — `describe_ui(query=…, compact=true)` (round 4) and `start_logcat_capture`/`stop_logcat_capture` (the press→observe isolation from round 3) worked well when actually used; `run_sequence` (round 6's batching decision) went unused despite fitting the exact problem.

## Field feedback, round 10 (Gradle-introspection + UI-driving sessions, 2026-08-07 & 2026-08-11)

From `android-mcp-papercuts` #019fdd06 (2026-08-07, a client's multi-module Gradle build) and #019ff2df (2026-08-11, same project, emulator-driving). Postdates round 9's cutoff — not yet folded in until this pass (2026-08-13). A parallel `codex` session had already shipped the round's `logcat`/`describe_ui`/`launch_dev_client`/`wait_for_text` items as v0.19.0 (see CHANGELOG); the two Gradle-tool items below were the ones still unaddressed in the code.

- [x] **`gradle_project_properties` dumped injected secrets in plaintext, no redaction.** It runs Gradle's stock `properties` task, which returns a module's *entire* effective property set — including credentials injected via `~/.gradle/gradle.properties`/env for private Maven repo auth (this repo's case: Nexus creds for a private SDK, `*_USER`/`*_PASS` pairs), indistinguishable in the output from harmless entries like `compileSdkVersion`. **Shipped:** `redactSecretProperties` masks any property value whose key matches a secret-shaped pattern (password/token/key/credential, case-insensitive) before the tool returns it; the tool description now says so.
- [x] **`list_gradle_variants`/`list_gradle_tasks` couldn't be scoped to a submodule.** Unlike `gradle_project_properties` (takes `module`), these two always ran against the Gradle root — and for `list_gradle_tasks` the `task` param was silently ignored entirely (hardcoded `gradlew tasks`), confirmed by the report's repro (`task: ":app:tasks"` produced byte-identical output to omitting it). On a multi-module build where only a submodule applies the Android plugin, `list_gradle_variants` returned "No build variants found" even though `:app` (found seconds earlier via `list_gradle_projects`) plainly had them. **Shipped:** both tools now honor `task` — a bare `"tasks"` (default) or a module-qualified `":app:tasks"` reaches `gradlew` unchanged, matching the convention `gradle_build`'s `task=` already uses.
- [x] **`wait_for_text`'s viewport caveat didn't reach the timeout message.** Round 9's `scroll:true` shipped, and `describe_ui`'s "trustworthy absence" wording was qualified to the current viewport (v0.19.0) — but `wait_for_text`'s own timeout error (`"text %q did not appear within %s"`) still read as unconditional absence, which is exactly where the report said it cost 45s. **Shipped:** the timeout error now says so and points at `scroll=true` when the call didn't already use it.
- Minor, not actioned: `list_gradle_projects`/`list_gradle_variants` schema descriptions still carry copy-pasted `json=`/`run_unit_tests` references that don't apply to them — noise when reading the schema, not a functional bug. Low priority.

## Conventions (read before adding a tool)

- Every device-facing tool takes an optional `serial`; single-device sessions can omit it.
- Keep the execution layers (`internal/adb` device client, `internal/uiauto` parsing, `internal/gradle` builds) pure/testable; `internal/tools` stays a thin MCP binding. Device commands are `adb.Client` methods; each `tools/<domain>.go` mirrors its execution file — see [../ARCHITECTURE.md](../ARCHITECTURE.md).
- Add unit tests for any new pure logic (parsers, coordinate math, arg parsing).
- Open a [GitHub issue](https://github.com/iksnerd/adb-mcp/issues) for feedback, bugs, or tool requests.
