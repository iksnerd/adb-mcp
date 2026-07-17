# Finding out WHY a native call failed

A JS-level or UI-level "X failed" alert almost always hides the real cause. The
truth is in the native log or the crash record. Reach for these in order.

## 1. If the app crashed: `last_crash` first

`last_crash` returns the most recent crash from the system DropBox — full
header and stack (JVM/RN *and* native), kept intact even after the lines have
rotated out of the logcat ring buffer. Optionally filter to your package. For
an actual crash this is one call instead of log spelunking.

## 2. For a failure that didn't crash: isolate, then read

The core mistake is reading a buffer full of history and treating "no match"
as "didn't happen" (or an old match as fresh). Isolate first:

- **Know when it happened?** `logcat {since: "2m"}` — only lines from the last
  2 minutes (device clock). The right axis when a user says "I just hit it".
- **About to reproduce it?** `clear_logcat` → perform ONE action → `logcat`.
  Everything you read was caused by that action.
- **Reproducing a longer flow?** `start_logcat_capture {clear: true}` → drive
  the flow → `stop_logcat_capture` (bounded to the last 500 lines by default;
  narrow with filters, raise `tail` only if truly needed).

## 3. Filter by the right axis

- `tags: ["YourAppTag", "AndroidRuntime"]` — anchored to logcat's tag field;
  far more precise than substring (a package-name substring drags in
  `ImeTracker`/`PackageConfigPersister` noise).
- `priority: "E"` — errors and worse (`W` for warnings and worse).
- `filter: "Caused by"` — substring across the whole line, best for exception
  hunting.

Read from the bottom up. The **`Caused by:`** line is the root error — that is
the one that matters, not the top-level wrapper exception.

An empty result is only meaningful if you isolated (steps above): on a chatty
emulator, 400 lines can span seconds, and a crash may have already rotated out
— that's what `last_crash` and the capture flow are for.

## Things the root cause often reveals

- An obfuscated/renamed class breaking JNA/JNI struct layout at runtime.
- `No StrongBox available` → the device fell back from hardware-backed to
  software keys (relevant to Keystore attestation flows).
- A missing permission, a `ClassNotFoundException`, or a native `.so` that failed
  to load.

## App lifecycle while triaging

- `list_packages` with a filter confirms the app is installed and gives its exact
  package name.
- `stop_app` then `launch_app` restarts it cleanly to reproduce from a known
  state.
- `install_app` (re)installs an APK build you want to test.
- For RN/Expo dev clients: absent logs may mean the app is running its
  **embedded bundle**, not your edited code — set up `adb_reverse
  {device_port: 8081}` and reload before trusting any absence of evidence.
