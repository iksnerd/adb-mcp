# Native PIN pads & device lock (Keystore flows)

## Native / custom PIN pads

A custom native pad (e.g. an SDK's own PIN entry) renders its own key views —
there is no IME, so `input_text` does nothing. Use `enter_pin`. It resolves each
digit's tap point in priority order: explicit `coords` → bounds-derived `grid`
→ the digit's element in the UI hierarchy, then taps each with a settle delay.

**Visibility is pad-specific — check before assuming.** Whether a pad appears
in `describe_ui` depends on how it is drawn, not on it being a PIN pad:

- A pad built from **real native views** (e.g. a Kotlin view with `Button`
  digits) is *fully visible*: digits arrive as `Button` elements with
  `text: "1"…"0"`, auxiliary keys by `content_desc` (e.g. `"Cancel"`). Such
  pads often expose **no view ids** — match by text/content-desc, and plain
  `enter_pin` with no grid/coords just works.
- A pad **drawn on a canvas** (RN / Flutter / Skia) is *invisible* — see below.

Run `describe_ui` once on the pad screen: if the digits are listed, use the
hierarchy path; only reach for grid/coords when they are not.

### If the pad is invisible to describe_ui (RN / Skia pads)

Some SDK pads draw their keys on a **canvas**, so they are absent from the
uiautomator hierarchy and the default hierarchy lookup fails with
*"digit N not found …"*. You have two overrides:

- **`grid`** — pass the pad's bounding box `{x1,y1,x2,y2}` (read it off a
  `screenshot`). `enter_pin` lays the digits out on a standard 3×4 dialpad
  inside that box: `1 2 3 / 4 5 6 / 7 8 9 / _ 0 _`. This is the easy path for a
  regular grid pad.
- **`coords`** — pass explicit per-digit points as `"1:540,1600;2:640,1600;…"`.
  Use this when the pad is not a regular grid, or the digit order is scrambled
  (some secure pads shuffle keys each session — re-read them from the screenshot
  each time).

Coordinates are **true device pixels**. The `screenshot` is downscaled, so
multiply what you read by the downscale factor, or grab a 1:1 image with
`max_dim: 0` first.

For the **system lock-screen** PIN the keys usually *are* in the hierarchy, so
plain `enter_pin` with no grid/coords works once the pad is visible.

## Device lock — required for Keystore-backed crypto

Crypto SDKs frequently require a **secure lock screen**: the AndroidKeyStore
gates key use behind `KeyguardManager.isDeviceSecure()`. A fresh emulator has no
lock, so those flows fail with *"A secure lock screen is required."*

Fix it before running the flow:

1. `set_device_lock` with `value: "1234"` (type defaults to `pin`;
   `pattern` and `password` also work).
2. `is_device_secure` → should now report `true`.
3. Run your flow.
4. `clear_device_lock` with `old_value: "1234"` to remove it afterwards.

`is_device_secure` maps to `locksettings get-disabled` — `false` there means the
lock is **not** disabled, i.e. the device **is** secure.

### Apps that check the lock at startup — restart after setting it

Some apps (integrity/anti-tamper SDKs) verify `isDeviceSecure()` **once at
bootstrap** and refuse to run on an unlocked device ("Unsupported device" /
"A secure lock screen is required"). If you set the lock *while the app is
already open*, it won't notice. The sequence that works:

1. `set_device_lock {value:"1234"}` → 2. `is_device_secure` returns `true` →
3. **`stop_app` then `launch_app`** so the startup check re-runs.

(Setting a PIN also *locks the screen*; if a relaunch seems to "still fail",
the screen may just be locked — take a `screenshot` to check, unlock if needed,
then relaunch.)

## Biometrics on the emulator — fingerprint_touch

Apps whose primary unlock is biometric (BiometricPrompt auto-fires on top of
the PIN pad) can be driven end-to-end on an emulator — don't settle for
cancelling into the PIN fallback every run:

1. **Enroll once per AVD:** a fingerprint requires a secure lock first
   (`set_device_lock`). Then Settings → Security → Fingerprint: walk the
   wizard, and every time it asks for a touch, call `fingerprint_touch` —
   repeat until enrollment completes.
2. **Unlock during tests:** when the BiometricPrompt is up (the `top window:`
   line in `describe_ui` shows a systemui biometric window), call
   `fingerprint_touch {finger_id: 1}` — the prompt resolves via the real
   biometric path.

`fingerprint_touch` is emulator-only (`adb emu finger touch`); physical
devices cannot inject biometrics. While the prompt is up, the app's own
hierarchy is occluded — `describe_ui` sees systemui's tree, and a `back`
key press may be silently consumed (use `verify_change: true`).

On emulators specifically: use a **Google Play ("gphone") system image**, not a
plain AOSP/Google-APIs image — a `generic` build fingerprint trips many SDKs'
emulator detection, whereas the Play image's `sdk_gphone…` fingerprint passes.
With a Play image + a secure lock, these flows run on the emulator without a
real device.
