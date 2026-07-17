// Package tools binds the execution layers to MCP tools. Each handler is a thin
// adapter: resolve the target device into an *adb.Client, call one client
// method (or a gradle/uiauto function), and format the result. Keeping it thin
// means the real logic stays testable in internal/adb, internal/gradle, and
// internal/uiauto without any MCP dependency.
//
// This file is the tool CATALOG. Handlers and their argument types live in
// domain files that mirror the execution packages: emulator.go, observe.go,
// interact.go, lock.go, logs.go, apps.go, environment.go, gradle.go. Shared
// adapter helpers live in helpers.go.
package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register adds every Android tool to the server.
//
// Tool descriptions are written for the model that will call them: each says
// what the tool does, when to reach for it, and the one gotcha that most often
// wastes a turn. The overarching workflow lives in the android://guide/*
// resources — read android://guide/driving for the observe→act loop.
func Register(s *mcp.Server) {
	// --- Emulator / device management ---
	add(s, "list_avds",
		"List the Android Virtual Devices (AVDs) installed on this machine that can be booted. Start here when no emulator is running yet; the returned names feed boot_emulator.",
		listAVDs)
	add(s, "boot_emulator",
		"Boot an AVD by name and return its device serial (e.g. emulator-5554). Launches the emulator detached so it outlives this call, and waits for full boot (sys.boot_completed) by default. Use the returned serial for later tools if you boot more than one device. Booting can take 30-120s on a cold start.",
		bootEmulator)
	add(s, "list_devices",
		"List attached emulators/devices and their adb state (device = ready, offline, unauthorized). Use it to confirm a device is up before driving it, or to get the serial when several are attached.",
		listDevices)
	add(s, "wait_for_boot",
		"Block until a device finishes booting (sys.boot_completed=1). Only needed if you started an emulator elsewhere; boot_emulator already waits by default.",
		waitForBoot)
	add(s, "shutdown_emulator",
		"Power off a running emulator (adb emu kill). Use when you are done with a device you booted.",
		shutdownEmulator)

	// --- Observe ---
	add(s, "screenshot",
		"Capture the current screen as a PNG so you can SEE the UI state. Call it after every action to confirm the screen changed before acting again — driving blind chains taps onto the wrong screen. The image is auto-downscaled (default max 760px) so it is accepted by the image reader; this is for seeing only — derive tap coordinates from describe_ui, not from this image. Auto-retries an all-black frame (an intermittent capture glitch) and, if it stays black, says why (FLAG_SECURE content like a native PIN pad, or a sleeping display) — when black, use describe_ui instead.",
		screenshot)
	add(s, "describe_ui",
		"Read the on-screen UI hierarchy as a list of elements, each with its text, content_desc, resource_id, class, clickable flag, pixel bounds, and a precomputed center in TRUE DEVICE PIXELS. This is your source of truth for AIMING: pass an element's center straight to tap. Never guess coordinates from the screenshot (it is downscaled and you will miss). The response header states the FOCUSED WINDOW (if it's a system overlay — biometric prompt, permission dialog — the elements belong to that overlay, not your app) and how many nodes the filter hid. Default filter keeps labelled/clickable/id-carrying elements minus redundant wrappers; filter=\"clickable\" returns only tap targets (much smaller); filter=\"all\" returns every bounded node — the only mode where absence proves an element isn't in the hierarchy. Canvas-drawn (RN/Flutter/Skia) content appears in NO mode.",
		describeUI)

	// --- Interact ---
	add(s, "tap",
		"Tap a single coordinate in true device pixels. Use a center value from describe_ui. If a tap seems to do nothing, the coordinate is almost always stale/misscaled — re-run describe_ui and use a fresh center. Prefer tap_on_text when you know the element's label.",
		tap)
	add(s, "tap_on_text",
		"Find an element by its visible text or content-description and tap its center — the one-shot way to press a labelled button/row without computing coordinates yourself. Runs describe_ui internally and prefers a clickable match. Use exact match (partial=false) to avoid hitting the wrong item when labels overlap.",
		tapOnText)
	add(s, "tap_element",
		"Find an element by resource_id and tap its center — the id-addressed sibling of tap_on_text, for elements with no visible label. Runs describe_ui internally (filter=all, so even unlabeled wrapper nodes are findable) and re-resolves the element right before tapping, narrowing the window where a stale coordinate lands on an overlay (e.g. an Expo dev-menu bubble) the a11y tree never reported. Use exact match (partial=false) to avoid hitting the wrong item when ids overlap; verify_change reports whether the tap had any visible effect.",
		tapElement)
	add(s, "swipe",
		"Swipe/drag from a start point to an end point. Params: x1,y1 (start) and x2,y2 (end) in true device pixels — x and y are accepted aliases for x1 and y1. To SCROLL DOWN a list, swipe from a HIGH y to a LOW y (drag the content up); reverse to scroll up. A longer duration_ms gives a slower, controlled drag; a short one flings.",
		swipe)
	add(s, "drag",
		"Press-hold-move-release drag from (x1,y1) to (x2,y2) in true device pixels (input draganddrop, Android 11+). Unlike swipe (which flings), this holds at the start first — use it for drag handles, long-press-to-reorder lists, and drag-and-drop targets a quick swipe skips.",
		drag)
	add(s, "input_text",
		"Type text into the currently focused input field via the IME. Tap the field first so it has focus. Afterwards the soft keyboard may cover buttons lower on screen — dismiss it with press_key escape (or back) before tapping them. For native non-IME PIN pads this does nothing; use enter_pin instead.",
		inputText)
	add(s, "press_key",
		"Press a hardware/navigation key by name (enter, back, home, menu, tab, del, escape, up, down, left, right, dpad_center, app_switch, search, power, volume_up, volume_down, ...) or a raw Android keycode number. Handy to submit a form (enter), dismiss the keyboard (escape), or go back (back). A key can be silently consumed with no effect (e.g. back while a biometric prompt is up) — pass verify_change=true to get ui_changed: true/false instead of guessing.",
		pressKey)
	add(s, "input_key_combo",
		"Press several keys together as a chord (input keycombination, Android 11+). Use preset=\"select_all\" (or copy/paste/cut/undo/redo/save/find) for a named shortcut, or keys=[\"ctrl\",\"a\"] / [\"alt\",\"tab\"] to spell one out — modifier(s) first, then the action key; each is a key name (ctrl/alt/shift/meta, a-z, enter, tab, ...) or a raw keycode. For a single key use press_key instead.",
		inputKeyCombo)
	add(s, "long_press",
		"Press and hold a coordinate (true device pixels) for a duration — for context menus, drag handles, and long-press actions.",
		longPress)
	add(s, "enter_pin",
		"Enter digits on a numeric PIN pad by tapping each key with a settle delay. Use when input_text does nothing because the pad renders its own key views. Visibility is PAD-SPECIFIC — run describe_ui on the pad screen first: a native-view pad (digits listed as Buttons with text) works with the default hierarchy lookup, no extra args. Only CANVAS-DRAWN pads (React Native / Skia SDK pads, whose keys are invisible to describe_ui) need 'grid' (the pad's bounding box; digits placed on a standard 3x4 dialpad) or 'coords' (explicit per-digit x,y) — read those bounds off a screenshot.",
		enterPIN)
	add(s, "wait_for_text",
		"Poll the UI until an element with the given text/content-description appears (or times out), then return it. Use this after an async action (network load, navigation, animation) instead of a blind wait-then-screenshot — it returns as soon as the element is present, with its tappable center. Note: canvas-drawn (RN/Skia) text never enters the hierarchy, so it will time out on those — screenshot instead.",
		waitForText)
	add(s, "wait",
		"Sleep for a number of seconds (fractions ok, capped at 300), then return. For TIME-based conditions where wait_for_text's polling doesn't apply: backgrounding an app long enough to trip a native auth timer, waiting out a cooldown or rate limit, letting a long animation finish.",
		wait)

	// --- Device lock / Keystore ---
	add(s, "set_device_lock",
		"Set a secure lock screen (type: pin [default], pattern, or password). REQUIRED before AndroidKeyStore / Keystore-backed crypto flows, which fail with 'A secure lock screen is required' on a fresh emulator that has no lock. Follow with is_device_secure to confirm.",
		setDeviceLock)
	add(s, "clear_device_lock",
		"Remove the secure lock screen, supplying the current credential as old_value. Use to restore a clean state after testing a Keystore flow.",
		clearDeviceLock)
	add(s, "is_device_secure",
		"Report whether a secure lock screen is set (KeyguardManager.isDeviceSecure). Use it to verify set_device_lock worked before running a Keystore-gated flow.",
		isDeviceSecure)
	add(s, "fingerprint_touch",
		"Simulate a fingerprint-sensor touch on an EMULATOR (adb emu finger touch). With a fingerprint enrolled, this satisfies a BiometricPrompt — drive the app's REAL biometric unlock path instead of cancelling into the PIN fallback every run. finger_id must match an enrolled finger (default 1). GOTCHA: the command reports OK even when the id matches nothing — if the prompt doesn't resolve, the enrolled id differs (re-enrollments increment it): try finger_id 2..5, send a second touch after ~1s, or re-enroll deterministically at session start (Settings > Security > Fingerprint, calling this tool for each wizard touch). Emulator-only; physical devices cannot inject biometrics.",
		fingerTouch)
	add(s, "finger_remove",
		"Lift the simulated finger off the sensor (adb emu finger remove) — the complement to fingerprint_touch, for flows that watch for the finger-up event. Emulator-only.",
		fingerRemove)

	// --- Extended Controls (emulator console) ---
	// These drive the emulator's Extended Controls panel (a window of the emulator
	// process itself, invisible to describe_ui/tap) through the emulator console.
	// All are emulator-only; a physical device has no console equivalent.
	add(s, "send_sms",
		"Deliver an incoming SMS to the emulator (adb emu sms send) — the standard way to drive OTP / 2FA flows without a second phone. Pass a sender number (from) and the message text (e.g. the code). Emulator-only.",
		sendSMS)
	add(s, "phone_call",
		"Drive an emulated voice call (adb emu gsm). action=\"call\" (default) rings an incoming call from number; \"accept\"/\"cancel\"/\"busy\"/\"hold\" transition an in-progress call. Use to test call-interruption behaviour and CALL_PHONE flows. Emulator-only.",
		phoneCall)
	add(s, "set_battery",
		"Set the emulated battery level (0-100) and/or charging state (adb emu power) — test low-battery UI and charging-only logic deterministically. Provide level, charging, or both. Emulator-only. (For a fake battery in a clean SCREENSHOT status bar, use set_status_bar instead.)",
		setBattery)
	add(s, "rotate_screen",
		"Rotate the emulator to its next orientation (adb emu rotate) — the quick way to exercise landscape/portrait layout and rotation-driven state loss. Emulator-only.",
		rotateScreen)
	add(s, "avd_snapshot",
		"Manage emulator AVD snapshots (adb emu avd snapshot): action=save|load|delete a named snapshot, or list them. Save a known-good state, then load it to reset the device deterministically between runs — faster than a wipe_data cold boot. Emulator-only.",
		avdSnapshot)
	add(s, "cellular",
		"Shape the emulated cellular radio (adb emu gsm / network): data and voice registration state (unregistered/home/roaming/searching/denied/off/on), signal strength (0-4), and mobile-data throughput/latency (network_speed like \"lte\"/\"edge\" or \"<up>:<down>\" kbps; network_delay like \"umts\" or \"<min>:<max>\" ms). Test offline/roaming/weak-signal and slow-network behaviour deterministically. Every field optional; set at least one. Emulator-only.",
		cellular)
	add(s, "set_sensor",
		"Set an emulated hardware sensor value (adb emu sensor set) — drive accelerometer/gyroscope/orientation (pass x, y, z) or a single-value sensor like light/proximity/temperature/pressure/humidity (pass x only). Use to exercise shake/tilt/rotation handlers or ambient-light/proximity logic. Emulator-only.",
		setSensor)

	// --- Logs & capture ---
	add(s, "logcat",
		"Dump recent native log lines — the last N (default 400) or, with since=\"2m\"/\"90s\", everything from that long ago on the device clock (the right axis when the report is 'I just hit an error'; on a chatty emulator 400 lines can span seconds). Optionally filtered by a case-insensitive substring, a minimum priority (V/D/I/W/E/F — e.g. priority=\"E\" for errors and up), and/or tags (OR'd). This is how you find the REAL reason a native call failed when the UI only shows a generic 'X failed' alert: filter by your app tag or 'Exception'/'Caused by' and read the 'Caused by:' line — that is the root cause. Dumps and exits (does not stream); chatty spam is stripped.",
		logcat)
	add(s, "clear_logcat",
		"Empty the device's logcat ring buffer (adb logcat -c). The sharpest isolation primitive for a press→observe loop: clear, perform ONE action, then logcat — every line you read was caused by that action. Without it, a filter hit may be minutes old and an empty result may just mean the buffer rotated. (For reaching BACK in time instead, use logcat's since param.)",
		clearLogcat)
	add(s, "start_logcat_capture",
		"Begin streaming logcat into a buffer for this device (optionally clearing first). Pair with stop_logcat_capture to get everything logged DURING a flow — use this instead of the one-shot 'logcat' when you need logs across an interaction.",
		startLogcatCapture)
	add(s, "stop_logcat_capture",
		"Stop the running logcat capture and return what was collected since start, optionally filtered by a case-insensitive substring, a minimum priority (V/D/I/W/E/F), and/or tags (OR'd). Output is capped to the last 500 lines by default (override with tail) so a long capture doesn't blow the token budget — narrow with the filters first.",
		stopLogcatCapture)
	add(s, "start_screen_record",
		"Start recording the screen to an mp4 on the device (Android caps a single recording at ~180s). Pair with stop_screen_record.",
		startScreenRecord)
	add(s, "stop_screen_record",
		"Stop the screen recording, finalize the mp4, and pull it to a local path.",
		stopScreenRecord)

	// --- App lifecycle ---
	add(s, "list_packages",
		"List installed package names, optionally filtered by substring — to confirm an app is installed and get its exact package name for launch_app/stop_app.",
		listPackages)
	add(s, "install_app",
		"Install (or reinstall, -r) an APK from a local file path onto the device. Use to deploy a build you want to test.",
		installApp)
	add(s, "launch_app",
		"Launch an app by package name (starts its LAUNCHER activity) and echo the resolved component on success. Fails with a clear message (not a raw monkey dump) when the package isn't installed or has no launcher activity. Combine with stop_app to restart an app cleanly from a known state.",
		launchApp)
	add(s, "stop_app",
		"Force-stop an app by package name. Pair with launch_app to reset an app to a clean start when reproducing a bug.",
		stopApp)
	add(s, "reload_app",
		"Best-effort: trigger a Metro/JS reload on a React Native dev-client build via the classic <package>.RELOAD_APP_ACTION broadcast. Only works on debug builds of classic (non-bridgeless) RN architectures that register the receiver — on newer RN/Expo dev clients it may silently no-op with no error. If the app doesn't visibly reload, use open_dev_menu then tap_on_text(\"Reload\") instead. PREREQUISITE: the app must be able to reach Metro at all — run adb_reverse {device_port: 8081} first, or a reload lands you back on the EMBEDDED bundle and your edits still won't appear.",
		reloadApp)
	add(s, "open_dev_menu",
		"Open the React Native dev menu (KEYCODE_MENU) on the foreground app — the reliable way to reach a dev build's Reload/Debug JS Remotely/etc. options when reload_app's broadcast doesn't apply. Follow with tap_on_text or describe_ui to pick a menu item.",
		openDevMenu)
	add(s, "uninstall_app",
		"Uninstall an app by package name (adb uninstall). Use to remove a build before a clean install, or to verify first-run behavior after reinstalling. To keep the app but reset it, prefer clear_app_data.",
		uninstallApp)
	add(s, "clear_app_data",
		"Wipe an app's data and cache (pm clear) to reset it to a first-launch state — the fastest way to reproduce onboarding/permission flows from scratch.",
		clearAppData)
	add(s, "grant_permission",
		"Grant a runtime permission to an app (pm grant), e.g. android.permission.CAMERA — skips the in-app permission dialog so you can drive straight to the feature.",
		grantPermission)
	add(s, "revoke_permission",
		"Revoke a runtime permission from an app (pm revoke) — to test the denied path or re-trigger the permission-request dialog on next use. Pairs with grant_permission. Note: revoking some permissions kills the app process.",
		revokePermission)
	add(s, "open_url",
		"Open a URL or deep link via an ACTION_VIEW intent (am start) — the way to jump straight to a deep-linked screen. Optionally target a specific package.",
		openURL)
	add(s, "launch_dev_client",
		"Launch an Expo dev build straight at a Metro dev server, skipping the Dev Launcher's server-picker screen. Builds the \"<scheme>://expo-development-client/?url=http://host:port\" deep link and opens it. Pass scheme (your app.json \"scheme\"); host/port default to localhost:8081. PREREQUISITE: run adb_reverse tcp:8081 first so the device can reach Metro, otherwise the dev client falls back to its embedded bundle. For plain Expo Go (not a dev build) use open_url with the exp:// URL instead.",
		launchDevClient)
	add(s, "get_app_details",
		"Report an installed app's version name/code and its launchable activity (dumpsys package + resolve-activity) — to confirm what build is installed and find the activity to launch.",
		getAppDetails)
	add(s, "last_crash",
		"Return the most recent app crash from the system DropBox (dumpsys dropbox — JVM/React-Native and native crashes), with the full exception header and stack in one call. Optionally filter to a package. Use this instead of grepping logcat when an app just crashed: DropBox keeps the whole fatal (header + Caused by + frames) together even after it has scrolled out of the logcat ring buffer.",
		lastCrash)
	add(s, "push_file",
		"Copy a local file onto the device (adb push) — e.g. seed test data or a file to import.",
		pushFile)
	add(s, "pull_file",
		"Copy a file off the device to a local path (adb pull) — e.g. retrieve a generated file, database, or screenshot.",
		pullFile)

	// --- Device discovery ---
	add(s, "adb_reverse",
		"Forward a DEVICE TCP port to a HOST port (adb reverse) so the emulator/device can reach a server on this machine — the canonical use is tcp:8081 for Metro. CRITICAL for RN/Expo dev clients: if the app can't reach its dev server it may SILENTLY fall back to the embedded bundle and ignore every code edit you make — set this up before a dev-client session, and suspect it whenever edits seem to have no effect. remove=true undoes the forward.",
		adbReverse)
	add(s, "connect_wireless",
		"Connect to a device over Wi-Fi/TCP (adb connect), optionally pairing first (adb pair) with the 6-digit code from Android 11+ Wireless debugging. Pass host:port; for pairing also pass the pairing address + code shown on the device.",
		connectWireless)

	// --- Environment ---
	add(s, "set_dark_mode",
		"Turn the system dark theme on or off (cmd uimode night) — to test light/dark appearances.",
		setDarkMode)
	add(s, "set_location",
		"Set the emulator's mock GPS location (longitude, latitude) — for location-gated features.",
		setLocation)
	add(s, "set_status_bar",
		"Pin a clean status bar via SystemUI demo mode (enabled=true) — fixed clock, chosen signal/battery, no notification icons by default — so screenshots for docs don't leak the wall clock or a random signal state. Optionally set clock (HHMM), battery (0-100), network_type (wifi/mobile/none) with mobile_level/data_type/carrier for mobile, and notifications_visible/notification_icon. Call with enabled=false to restore the live bar.",
		setStatusBar)
	add(s, "doctor",
		"Diagnose the local Android tooling: SDK path, adb/emulator availability, known AVDs, and attached devices. Run this first when something isn't working.",
		doctor)

	// --- Build & test (Gradle) ---
	add(s, "gradle_build",
		"Build the app with Gradle (default task assembleDebug) in project_dir, and report the produced APK path(s). project_dir must contain the Gradle wrapper (gradlew). Runs on the host, not a device.",
		gradleBuild)
	add(s, "build_and_run",
		"One-shot build → install → launch: runs Gradle (default task assembleDebug) in project_dir, installs the resulting APK on the device, and launches package. Equivalent to gradle_build + install_app + launch_app but in a single call. If several APKs exist under build/outputs (multi-flavor projects, leftover androidTest APKs), the newest non-test one is installed — the artifact the build just produced.",
		buildAndRun)
	add(s, "run_unit_tests",
		"Run Gradle JVM unit tests (default task 'test') in project_dir and return the result summary, including per-suite timing and failing-test stack traces. Pass json=true for a structured JSON summary instead of the text form.",
		runUnitTests)
	add(s, "run_instrumented_tests",
		"Run Gradle instrumented (on-device) tests (default task 'connectedAndroidTest') in project_dir — requires a booted device/emulator. Returns per-suite timing and failing-test stack traces; pass json=true for a structured JSON summary instead of the text form.",
		runInstrumentedTests)
	add(s, "list_gradle_tasks",
		"List the available Gradle tasks in project_dir (gradlew tasks) — to discover build/test/install targets.",
		listGradleTasks)
}

// add is a small generic wrapper over mcp.AddTool to cut boilerplate.
func add[In any](s *mcp.Server, name, desc string, h func(context.Context, In) (*mcp.CallToolResult, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			res, err := h(ctx, in)
			return res, nil, err
		})
}
