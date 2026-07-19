# Changelog

Shipped work, newest first. Roadmap and open ideas live in
[BACKLOG.md](BACKLOG.md); the code layout is described in
[../ARCHITECTURE.md](../ARCHITECTURE.md).

## v0.17.0 — screenshot decodes once + set_battery on real devices + list_gradle_projects

A performance fix and two backlog items.

**Perf — `screenshot` decodes the PNG once, not twice.** `CaptureScreen` decoded
each frame in `isMostlyBlack` and *again* in `downscalePNG` (and re-decoded on
every black-retry). Measured: **~85 ms + 18 MB per `png.Decode`** of a 2076×2152
frame. It now decodes once into an `image.Image` and shares it between the
black-check and the downscale — roughly halving the CPU/allocs of the
most-called tool (screenshot runs after every action in the driving loop). The
byte-in wrappers are kept for the tested undecodable-input path; verified live
that full-res and downscaled captures still decode correctly.

**`set_battery` works on physical devices** (closes the enhancement backlog).
The emulator path (`adb emu power`) is unchanged; a physical device now forces
the values through the framework (`dumpsys battery set level/ac`). Those persist
until you clear them, so a new `reset` option restores automatic reporting
(`dumpsys battery reset`) on either. Verified live that `set`/`reset` round-trip.

**New — `list_gradle_projects`** (XcodeBuildMCP parity — deeper project
discovery). Runs `gradlew projects` and returns the module paths of a
multi-module build (`:app`, `:core`, `:feature:login`) so you can point
`gradle_build`/`list_gradle_variants` at the right module or address a task with
`:module:task`. Complements `list_gradle_variants` (which lists a module's build
variants). Parser unit-tested against the standard `gradlew projects` tree.

## v0.16.0 — foldable screenshot fix + app_state + has_biometric_enrolled + run_sequence

Four field-feedback items, each reproduced and verified on a live emulator
(`emulator-5554`, including a `Pixel_10_Pro_Fold` AVD for the foldable case).

**Fix — `screenshot` on multi-display foldables.** On a device with more than one
physical display, `screencap -p` (no `-d`) prints a `[Warning] Multiple displays
were found …` line to **stdout ahead of the PNG**, shifting the header ~250–350
bytes so nothing can decode it — the capture came back as `0x0` / "dimensions
could not be read", indistinguishable from a blank or FLAG_SECURE frame. On a
foldable the tool was effectively 100% unusable. Fix: strip any leading bytes
before the PNG signature (`\x89PNG`) from every screencap — robust and
display-agnostic (harmless on single-display, where the signature is already at
offset 0). Also added an optional `display` param (`"inner"`/`"primary"`,
`"cover"`/`"outer"`, an HWC index, or a raw physical id) to grab a specific
panel. Note: `screencap -d` keys off the **physical** display id from
`dumpsys SurfaceFlinger --display-id`, *not* the logical id `0`/`1` (passing the
logical id makes screencap fail outright) — `ResolveDisplay` handles the
mapping. (`android-emulator-mcp-feedback` #019f7abc.)

**New — `app_state`.** The most-requested gap: no way to tell a Metro-connected
dev process from one silently running its **embedded** bundle (which ignores
every JS edit), or to notice two live processes for one package (taps and log
reads hitting different pids). Reports installed?/running? + pid(s), main-process
uptime, first-install/last-update times, and a Metro-vs-embedded **bundle
source** heuristic over the app's recent logcat (HMRClient / Fast Refresh /
DevServer markers), with the evidence line it keyed on. Run it first when JS
edits seem to have no effect. (`android-mcp-papercuts` #019f6fad item 4.)

**New — `has_biometric_enrolled`.** Reports whether any fingerprint is enrolled
(and how many) from `dumpsys fingerprint`. Check it before a biometric flow:
with nothing enrolled, `fingerprint_touch` can never satisfy a BiometricPrompt —
it just sits on "Touch the sensor". Design settled by earlier live probing
(round 7): the framework exposes only an enrolled **count**, never the finger id,
and a wrong `fingerprint_touch` id trips a HAL lockout — so this is a count
probe, not runtime id-discovery. Verified live: an empty AVD reports 0, and 1
after enrolling one fingerprint. (`android-mcp-papercuts` #019f709b, reframed.)

**New — `run_sequence`** (resolves the round-4 DECISION). Runs several steps in
one call — `sleep`, `tap`, `tap_text`, `tap_element`, `key`, `text`, `swipe`,
`launch`, `stop`, `wait_text`, `describe_ui` — with `if_present`/`if_absent`
guards (the conditional-cancel idiom) and per-step `optional`. The point isn't
just fewer round-trips: for flows gated on native timing (a background-token
clear, a biometric prompt that auto-fires on resume) a per-step agent round-trip
perturbs the very timer being tested, so batching is the only faithful way to
reproduce them. Returns a per-step result (ok/skipped/error) plus the final
hierarchy; a non-optional step error stops the rest. Verified live end-to-end
(`home → launch → wait_text → guarded steps → describe_ui`). The larger Maestro
integration stays a separate open decision. (`android-mcp-papercuts` #019f6fad /
addendum #019f6fb4.)

**Docs & guides.** `driving` gains the foldable-capture note, an "edits not
showing up → check `app_state` for the embedded-bundle trap" gotcha, and a
`run_sequence` note for native-timer flows;
`pin-and-lock` leads the biometric section with `has_biometric_enrolled`; the
`fingerprint_touch` / `adb_reverse` / `reload_app` tool descriptions cross-link
the new tools.

## v0.15.0 — stay_awake + wakeup/sleep keys + enter_pin retry

Shipped from a live driving session on an Android 17 AVD — every item was
reproduced on the device first.

**New — `stay_awake`.** Keep the display from dozing during a driving session
(`svc power stayon true`). Fixes the case where `screenshot` keeps coming back
black with `screen_off:true` because a doze-happy emulator sleeps between steps —
`describe_ui` sees through it, but any screenshot/coordinate flow needs the screen
on. `enabled:false` restores the normal timeout.

**New — `wakeup` / `sleep` key names.** `press_key` gained `wakeup` (keycode 224,
turn the screen ON without toggling) and `sleep` (223). `power` toggles and can
sleep an already-awake screen; `wakeup` is the unambiguous "turn it on."

**Fix — `enter_pin` retries the hierarchy read.** The keyguard (system
lock-screen) PIN bouncer's digit buttons intermittently drop out of a
uiautomator dump — consecutive dumps of the same pad can disagree on whether a
key is present. `enter_pin` did a single settled read, and when it landed on an
empty moment it failed with a misleading *"the pad may be custom-drawn (RN/Skia)
and invisible"*. It now retries the read until the requested digits resolve
before giving up, and the error names the flaky-dump / covering-window cases too,
not just the canvas-pad one.

**Guides refreshed** (drift from v0.13–v0.14): `driving` now points at
`wakeup`/`stay_awake` for a black/sleeping frame; `getting-started` uses
`launch_dev_client` for RN/Expo dev builds; `pin-and-lock` notes the bouncer
dump-flicker and warns that a wrong `finger_id` counts toward the biometric HAL
lockout (so don't blind-sweep ids).

## v0.14.0 — list_gradle_variants + tap hit-test

**New — `list_gradle_variants`.** The Android analogue of XcodeBuildMCP's "list
schemes": parses the `assemble<Variant>` tasks out of `gradlew tasks` and returns
the buildable build variants (e.g. `freeDebug`, `paidRelease`). Each maps to an
`assemble<Variant>` / `install<Variant>` task, so the name feeds straight into
`gradle_build`/`build_and_run`'s `task=` arg to disambiguate a multi-flavor
project. Test-only APK tasks (`androidTest`/`unitTest`) are excluded. Closes the
"deeper project discovery" parity gap's variant half. Pure parsing, unit-tested.

**New — `tap` hit-test (`identify`).** From field feedback (`android-mcp`
#019f75a8): a coordinate `tap` on a native `NativeTabs` bar returned success but
navigated nowhere, and nothing distinguished "the tap missed" from "it landed
but did nothing." `tap` now takes `identify: true`, which reports which element
the coordinate lands in — or that it hit a **non-clickable wrapper** or **no
reported element at all** (an unseen overlay). Pairs with the existing
`verify_change` (did the UI change?) to tell the failure modes apart. The `tap`
description now also notes that some native surfaces (Compose/RN NativeTabs)
don't respond to coordinate taps — use `tap_on_text`/`tap_element` there.

Still open: the underlying accessibility-`ACTION_CLICK` tap path (what Maestro's
`tapOn` uses to reach those native views) needs a live-emulator pass before
shipping — logged in BACKLOG.

## v0.13.0 — cellular / sensors console controls + Expo dev-client launch

**New — more Extended Controls (emulator console).** Two tools extend the
`adb emu` console surface started in v0.12.0; both emulator-only:

- **`cellular`** — shape the emulated radio: `data`/`voice` registration state
  (unregistered/home/roaming/searching/denied/off/on), `signal` strength (0-4),
  and mobile-data `network_speed`/`network_delay` (named profiles like `lte`/
  `edge`, or raw `<up>:<down>` kbps / `<min>:<max>` ms). Test offline, roaming,
  weak-signal, and slow-network behaviour deterministically. Every field is
  optional; set at least one.
- **`set_sensor`** — set a hardware sensor value (`adb emu sensor set`): pass
  `x`/`y`/`z` for a multi-axis sensor (acceleration, gyroscope, magnetic-field,
  orientation) or just `x` for a single-value one (light, proximity, temperature,
  pressure, humidity). Drives shake/tilt/rotation and ambient-light/proximity
  handlers.

**New — `launch_dev_client`.** Launch an Expo dev build straight at a Metro dev
server, skipping the Dev Launcher's server-picker screen. Builds the
`<scheme>://expo-development-client/?url=http://host:port` deep link (from the
app's `scheme`; host/port default to `localhost:8081`) and opens it via
ACTION_VIEW. Closes the field-feedback gap where `open_url` with an `exp://` URL
was the stopgap. Run `adb_reverse tcp:8081` first, or the dev client falls back
to its embedded bundle.

Still open: `biometric_auth` (enrolled-id discovery) needs a live-emulator pass
to verify the `dumpsys fingerprint` format before shipping — see BACKLOG.

## v0.12.0 — Extended Controls tools + adb.Client refactor

**New — Extended Controls (emulator console).** Six tools drive the emulator's
Extended Controls panel, which lives in the emulator process's own window and is
invisible to `describe_ui`/`tap`. All route through the `adb emu` console bridge
(the same one `fingerprint_touch`/`set_location` already used) and are
emulator-only:

- **`send_sms`** — deliver an incoming SMS (`from`, `text`): the standard way to
  drive OTP / 2FA flows without a second phone.
- **`phone_call`** — ring or transition an emulated voice call
  (`action`: call/accept/cancel/busy/hold).
- **`set_battery`** — set battery `level` (0-100) and/or `charging` state, for
  low-battery and charging-only UI.
- **`rotate_screen`** — rotate the emulator to its next orientation.
- **`avd_snapshot`** — `save`/`load`/`delete`/`list` AVD snapshots: reset to a
  known state faster than a cold `wipe_data` boot.
- **`finger_remove`** — lift the simulated finger off the sensor (complement to
  `fingerprint_touch`).

**Architecture — `adb.Client` + package split (internal, no behavior change).**
`internal/android` became four inward-pointing packages: `internal/sdk` (SDK
paths/env), `internal/uiauto` (the `Element` model + pure parsing),
`internal/adb` (an `adb.Client` whose methods are the device commands over an
injectable `Runner`), and `internal/gradle` (build/APKs/test reports).
`internal/tools` stays the thin MCP layer. The `Runner` seam means command
builders are now unit-tested by asserting exact adb argv with **no device**.

**Fixes — `tap_element` / `build_and_run`** (from a review of the v0.11 additions):

- `build_and_run` selected the installed APK lexicographically, so a stale
  `androidTest` or wrong-variant APK could be installed and launched. It now
  picks the newest-by-mtime non-test APK, and `FindAPKs` prunes
  `node_modules`/dot-dirs from its walk.
- `tap_element` searched an auto-filtered snapshot, which drops the unlabeled
  id-carrying wrapper nodes it exists to find; it now searches `filter=all`.
- An empty/whitespace `text`/`resource_id` no longer substring-matches (and taps)
  an arbitrary element.
- `install_app`/`build_and_run` now error on an adb `Failure [...]` printed with
  a zero exit code (older platform-tools), instead of reporting a phantom success.
- Both tap tools gained `verify_change`.

## v0.11.2 — biometric-loop fixes + doctor version reporting

Round-5 field feedback (`android-mcp-papercuts` #019f709b / #019f70d1).

- **`enter_pin` blind-tap guard** — with `grid`/`coords` it happily tapped
  into a focused BiometricPrompt (no pad on screen). It now refuses when a
  biometric window has focus and says how to proceed (`fingerprint_touch`, or
  cancel to fall back to the PIN pad).
- **`doctor` leads with the serving binary's version** + an `adb-mcp update`
  pointer — a reporter concluded shipped params had "regressed" when their
  install was simply stale; now one call answers "is this install current?".
- **Fingerprint id troubleshooting** — `emu finger touch` reports OK even for
  an id that matches nothing enrolled. The `fingerprint_touch` description and
  `android://guide/pin-and-lock` now cover: re-enrollments increment the id
  (try 2..5), double-touch timing, and deterministic re-enrollment at session
  start.

## v0.11.1 — guides & descriptions caught up with v0.10–v0.11 reality

Doc-drift audit after the v0.11.0 visibility round; no behavior changes.

- **`android://guide/crash-triage` rewritten** — it still taught the pre-v0.10
  substring-only workflow. Now: `last_crash` first for crashes; isolate before
  reading (`since` window, `clear_logcat` → act → read, or the capture flow);
  filter by the right axis (`tags`/`priority`/substring); and "an empty result
  is only meaningful if you isolated".
- **`android://guide/getting-started`** — new "RN/Expo dev build? `adb_reverse`
  8081 first" section covering the silent embedded-bundle fallback.
- **`android://guide/driving`** — loop-economy note: `compact`/`query` for
  cheap re-observes, `verify_change` instead of a full re-observe,
  `wait` for elapsed-time conditions.
- **`enter_pin` description** — pad visibility is pad-specific: check
  `describe_ui` first; only canvas-drawn pads need `grid`/`coords`.
- **`reload_app` description** — states the `adb_reverse` prerequisite so a
  "successful" reload doesn't land back on the embedded bundle.

## v0.11.0 — visibility round: occlusion, trustworthy absence, biometrics, self-update

Driven by field-feedback rounds 3–4 (council-hub, 2026-07-17). Theme: the
tools reported what they *did* well and what they *couldn't see* poorly —
occluded windows, filtered nodes, no-op actions, rotated buffers all looked
identical to "nothing there". 49 → 53 tools.

- **`describe_ui` tells you what you're looking at.** The response header now
  states the focused **top window** (a systemui biometric prompt / permission
  dialog gets an explicit "the app underneath is occluded" warning) and how
  many nodes the filter hid. New params: `filter` (`auto`/`clickable`/`all` —
  `all` returns every bounded node, so absence finally *proves* an element
  isn't in the hierarchy), `query` (cheap "is X on screen?"), and `compact`
  (one line per element, ~10x fewer tokens). The `auto` filter also drops
  label-less wrappers whose bounds equal their parent's (Material's 5-deep
  `navigation_bar_item_*` chains), and the tool description no longer
  overpromises what is filtered.
- **`fingerprint_touch`** (new tool) — simulate a fingerprint touch on an
  emulator (`adb emu finger touch`), so agents can drive the real biometric
  unlock path (and enrollment) instead of cancelling into the PIN fallback
  every run. Enrollment workflow documented in `android://guide/pin-and-lock`.
- **`press_key`/`tap` `verify_change`** — opt-in `ui_changed: true/false`
  (hierarchy fingerprint before/after), so a key press silently consumed by an
  overlay no longer looks identical to one that worked.
- **`wait`** (new tool) — plain sleep for time-based conditions (backgrounding
  an app past a native auth timer) that `wait_for_text` can't express. Field
  report: this single gap pushed entire sessions into raw bash.
- **`logcat` `since`** — time-window dumps ("2m", "90s", device clock via
  `logcat -t '<time>'`), the right axis for "the user just hit an error";
  **`clear_logcat`** (new tool) — clear → act → read isolates what one action
  logged, killing the rotated-buffer false negative.
- **`adb_reverse`** (new tool) — device→host port forwarding; without
  `tcp:8081` an RN/Expo dev client silently falls back to its embedded bundle
  and ignores every edit (cost a reporter most of a session).
- **`adb-mcp update`** (new subcommand, plus `version`) — self-update from
  GitHub releases: resolves the latest tag, downloads the right OS/arch
  archive, verifies its SHA-256 against `checksums.txt`, and atomically
  replaces the running binary (old binary kept aside until the swap lands).
  Stdlib only; verified live 0.1.0 → 0.10.1. Pure parts unit-tested.
- **Guides corrected from the field:** PIN-pad visibility is pad-specific
  (native Kotlin pads ARE fully in the hierarchy — match by label; only
  canvas-drawn RN/Skia pads need `grid`/`coords`); system-window occlusion and
  the KEYCODE_HOME cold-start-instead-of-background trap added to
  `android://guide/driving`; biometrics-on-emulator section added to
  `android://guide/pin-and-lock`.

## v0.10.0 — last_crash, bounded capture, clearer launch_app

Clears the rest of the actionable field feedback.

- **`last_crash`** (new tool) — returns the most recent app crash from the
  system DropBox (`dumpsys dropbox --print`, JVM/RN and native), full header +
  stack in one call, optionally filtered to a package. Keeps the whole fatal
  together even after it's rotated out of the logcat ring buffer. DropBox
  parsing is pure/unit-tested; live-verified against a real recorded crash.
- **`stop_logcat_capture` output is bounded by default** — capped to the last
  500 lines (override with `tail`), so a long capture stops blowing the token
  budget and force-spilling to a file.
- **`launch_app` gives a clear failure** — a missing/uninstalled package or
  no-launcher-activity now returns a plain error instead of a raw `monkey`
  arg-dump, and on success echoes the resolved component.
- **`logcat` buffer-rotation hint** — an empty one-shot dump now points at
  `start_logcat_capture`/`stop_logcat_capture` or `last_crash` for a fatal that
  already scrolled off.
- **Driving guide** documents the `screenshot`/`describe_ui` state-skew during
  transitions and the black-screenshot → `describe_ui` fallback.

## v0.9.0 — screenshot black-frame detection & diagnosis

From field feedback: `screenshot` returned a bare black PNG in two
different situations with no hint why, causing repeated misdiagnosis.

- **`screenshot` now detects an all-black frame and says why.** It retries an
  all-black grab a couple of times (screencap intermittently returns black for
  a perfectly normal screen — the reported reliability bug), and if it stays
  black, diagnoses the likely cause: a `FLAG_SECURE` window (e.g. a native PIN
  pad, which the OS blanks to black) or a sleeping display. The result carries
  a compact status (`{all_black, secure_window, screen_off, attempts}`) and
  points the caller at `describe_ui`, which works even when a screenshot is
  blanked. Live-verified: normal screens aren't flagged; a screen-off frame is
  detected and labelled. Black detection (`isMostlyBlack`) is pure/unit-tested;
  the secure-window/screen-off probes are best-effort `dumpsys` reads.

## v0.8.0 — reload_app, open_dev_menu, richer log filtering

From real field feedback on two long real-world Expo dev-client debugging
sessions — the two items flagged as costing the most back-and-forth.

- **`reload_app`** — best-effort Metro/JS reload via the classic React Native
  `<package>.RELOAD_APP_ACTION` broadcast. Live-verified against a real Expo
  dev client (`com.example.devclient`): the broadcast triggered an actual
  reload attempt (it surfaced Metro's "couldn't load script" error, since
  Metro wasn't running in the test — confirming the receiver fired). Not
  guaranteed on newer bridgeless-mode RN architectures that don't register
  the receiver; falls back to `open_dev_menu`.
- **`open_dev_menu`** — opens the RN dev menu via `KEYCODE_MENU`, for driving
  Reload/Debug JS Remotely/etc. by hand (`tap_on_text`/`describe_ui`) when
  `reload_app`'s broadcast doesn't apply.
- **Richer log filtering** — `logcat` and `stop_logcat_capture` gained
  `priority` (V/D/I/W/E/F, keeps that level and more severe) and `tags`
  (case-insensitive, OR'd) filters alongside the existing substring filter,
  cutting down on the 89k–327k-char buffer spills the field feedback
  reported. Shared, unit-tested filtering logic (`LogFilter`) now backs both
  tools.

## v0.7.0 — richer status bar, deeper test-report insight, key-combo presets

- **`set_status_bar` — richer demo controls.** Added `network_type`
  (wifi/mobile/none) with `mobile_level`, `data_type` (lte/4g/5g/...), and
  `carrier` for mobile, plus `notifications_visible`/`notification_icon`.
  The `notification_icon` broadcast is best-effort — an obscure,
  version-dependent SystemUI internal that may silently no-op on some SDK
  images; the network/carrier/data-type controls are well-established demo
  mode commands and are the primary value here.
- **Deeper test-report insight.** `run_unit_tests`/`run_instrumented_tests`
  now report per-suite timing and full failure stack traces (previously only
  the first message line), and accept `json=true` for a structured summary.
  Fixed a related bug along the way: a `<testsuites>` wrapper's child suites
  were being flattened into one combined suite before aggregation, which is
  exactly what made per-suite timing impossible — each child suite now stays
  distinct.
- **`input_key_combo` presets.** Added named shortcuts (`select_all`, `copy`,
  `paste`, `cut`, `undo`, `redo`, `save`, `find`) via `preset=`, so callers
  don't need to know the underlying keycodes.

## v0.6.0 — renamed to adb_mcp

- **Renamed the project from `AndroidEmulatorMCP` to `adb_mcp`.** Google's
  [Android brand guidelines](https://developer.android.com/distribute/marketing-tools/brand-guidelines)
  don't allow "Android" (or anything confusingly similar) to lead a product
  name — it has to read as "X for Android," not "Android X." Go module path,
  all internal imports, the binary (`android-emulator-mcp` → `adb-mcp`), the
  MCP server identifier, `.mcp.json`, and the Makefile all moved together.
- Added a trademark-attribution line and brand-compliant tagline to the README
  ("an MCP server *for* Android," not "an Android MCP server").
- Set the Go module path to its public repo URL (`github.com/iksnerd/adb_mcp`)
  so the server is `go install`-able.

### Bug fixes

- **`open_url` with a package target was broken.** A bare package name was
  appended as a positional argument to `am start`, which `am` parses as the
  intent *data URI* — silently clobbering the `-d <url>` and opening the wrong
  thing. Now passed correctly as `-p <package>`.
- **`boot_emulator` could return the wrong serial.** If the pre-boot device
  listing errored, the "new device" snapshot was empty and any already-attached
  emulator was mistaken for the freshly-booted one. That error is now surfaced
  instead of silently driving the wrong device.
- **`swipe` schema now marks `x2`/`y2` as required** (they always were), so the
  calling model isn't misled into omitting the end point.
- **Test-report parsing no longer drops failing-test names** from a
  `<testsuites>` wrapper that also carries aggregate counts on its root element
  (regression test added).
- **The `logcat` "chatty" filter is now precise** — it only drops chatty dedup
  spam, not any line that merely contains the word "chatty" (an app tag,
  package name, or message).

### Repo / OSS readiness

- Added `LICENSE` (MIT), `CONTRIBUTING.md`, `SECURITY.md`, `CODEOWNERS`, and a
  tag-triggered GitHub Actions release workflow that gates on
  `gofmt`/`vet`/`test`, then cross-compiles binaries and publishes a Release.
- Removed a stale internal planning doc; replaced a private feedback-room
  reference with GitHub Issues; hardened `.gitignore`.

## v0.5.1 — clean process shutdown

- **Fix capture-session leak on client disconnect.** The server shut down via `log.Fatalf`, which `os.Exit`s and skips deferred cleanup — so on the normal stdin-EOF path (the MCP client closing), `StopAllCaptures()` never ran and a live `adb logcat`/`screenrecord` process plus its temp file leaked. Cleanup now runs explicitly on every exit path.
- **Exit cleanly on a normal disconnect.** A cancelled context (SIGINT/SIGTERM) or closed stdin now exits 0 quietly instead of logging a fatal "server error"; only genuinely unexpected errors are fatal (`isCleanShutdown` helper — the go-sdk folds `io.EOF` into a string with no exported sentinel).
- Verified live: the server exits when the client closes or is SIGKILL'd (no orphan), and capture sessions are torn down on both the EOF and signal paths. The `boot_emulator` emulator stays up by design (detached; stop it with `shutdown_emulator`).

## v0.5.0 — architecture split, bug-fix pass, gesture/status/report tools (46 tools)

**Refactor — domain-mirrored layout**
- Split the two monolith files (`tools/register.go` 997 lines, `android/device.go` 552 lines) into a domain-mirrored layout: each `tools/<domain>.go` adapter maps to an `android/<domain>.go` execution file, and `register.go` is now just the tool catalog. See [ARCHITECTURE.md](../ARCHITECTURE.md) and [architecture.mmd](architecture.mmd).

**Bug fixes (from code review)**
- `screenshot`: `max_dim:0` now actually disables downscaling (was silently remapped to the 760 default, making the documented full-res path unreachable). Arg is `*int`: omit → 760, `0`/negative → full resolution.
- `commandEnv`: match the `PATH` key case-insensitively and preserve its value — the old uppercase-only check appended a duplicate `PATH=` on Windows (case-insensitive keys), clobbering the system path.
- `boot_emulator`: honor the caller's `timeout_s` for the boot-wait phase instead of passing `WaitForBoot` a `<=0` remainder, which it silently treated as its own 120s default.
- `input_text`: single-quote the argument for the device shell so `$`, backtick, quotes, and other metacharacters type literally (the old escaper handled only a subset). Unit-tested.
- Capture sessions: `StopAllCaptures()` drains running logcat/screen-record sessions on shutdown so detached adb processes and temp files don't leak.
- `downscalePNG`: average/store in a consistent premultiplied color space (correctness for translucent PNGs; no change for opaque screenshots).

**New tools**
- `drag` — press-hold-move-release drag (`input draganddrop`, Android 11+), distinct from the fling of `swipe`.
- `input_key_combo` — chorded keys (`input keycombination`, Android 11+), e.g. `["ctrl","a"]`; added modifier + a-z key names.
- `set_status_bar` — SystemUI demo mode for clean doc screenshots (fixed clock, full signal, chosen battery, no notification icons).
- `run_unit_tests` / `run_instrumented_tests` now parse the JUnit XML and report a structured pass/fail/error/skipped summary with the failing tests — on both success and failure.

## v0.4.0 — backlog cleared

- `set_device_lock` optional `old_value` to change an existing lock in one call
- `connect_wireless` (`adb pair`/`connect`) for wireless-adb devices
- `get_app_details` (dumpsys package → version, launchable activity)
- `wait_for_text` (poll describe_ui until a label appears — kills manual sleep+screenshot)
- emulator factory reset — shipped as `boot_emulator {wipe_data:true}` (`-wipe-data`)

## v0.3.0 — XcodeBuildMCP-parity tool batches (40 tools total)

**Batch A — app-lifecycle & interaction completeness**
- `long_press` (input swipe same-point, long duration)
- `uninstall_app` (`adb uninstall`)
- `clear_app_data` (`pm clear`) — reset to clean state
- `grant_permission` / `revoke_permission` (`pm grant`/`revoke`) — skip runtime dialogs
- `open_url` (`am start -a android.intent.action.VIEW -d`) — deep links
- `push_file` / `pull_file` (`adb push`/`pull`) — test data & artifacts

**Batch B — build & test (true XcodeBuildMCP parity)**
- `gradle_build` (`./gradlew assembleDebug` → APK path)
- `run_unit_tests` (`./gradlew test`)
- `run_instrumented_tests` (`./gradlew connectedAndroidTest`)
- `list_gradle_tasks` (`./gradlew tasks`)

**Batch C — environment & session polish**
- `start_logcat_capture` / `stop_logcat_capture` (streaming session vs one-shot dump)
- `set_dark_mode` (`cmd uimode night yes/no`)
- `set_location` (`emu geo fix <lon> <lat>`)
- `start_screen_record` / `stop_screen_record` (`screenrecord` → mp4 pull)
- `doctor` (check adb/emulator/SDK/AVDs and report)

## v0.2.0 — fixes from live E2E feedback

- `boot_emulator`: launch emulator in its own session (`Setsid`) so it survives the server exit
- `enter_pin`: `grid`/`coords` fallback for canvas-drawn (RN/Skia) pads invisible to uiautomator
- `swipe`: accept `x`/`y` aliases for `x1`/`y1`; clearer missing-arg error
- `describe_ui`: dump-twice-and-settle guard against stale mid-refresh trees
- Guides: RN/Skia limitation + lock-then-restart + Google-Play-image notes
- Install: `rm` + ad-hoc `codesign` in Makefile (fixes macOS "Killed: 9" SIGKILL)
- Versioning: `-version` flag + build-time `-ldflags -X main.version` (git/VERSION)

## v0.1.0 — core driving (21 tools + 4 guide resources)

- Emulator mgmt: `list_avds`, `boot_emulator`, `list_devices`, `wait_for_boot`, `shutdown_emulator`
- Observe: `screenshot` (auto-downscale), `describe_ui` (true-pixel centers)
- Interact: `tap`, `tap_on_text`, `swipe`, `input_text`, `press_key`, `enter_pin`
- Device lock: `set_device_lock`, `clear_device_lock`, `is_device_secure`
- Logs: `logcat`
- App: `list_packages`, `install_app`, `launch_app`, `stop_app`
- Resources: `android://guide/{getting-started,driving,pin-and-lock,crash-triage}`
