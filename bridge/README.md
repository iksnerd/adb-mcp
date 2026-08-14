# adb-mcp accessibility bridge (EXPERIMENTAL)

A small companion Android app — not part of the Go module, built and shipped
separately — that lets `tap_on_text`/`tap_element` dispatch a **real**
accessibility click (`AccessibilityNodeInfo.performAction(ACTION_CLICK)`)
instead of a raw coordinate `input tap`. Some native views (Compose/RN
`NativeTabs` bars, some overlays) only respond to the former; `input tap` is
a touch-injection event that never reaches them at all.

It's a single plain `AccessibilityService` — the same mechanism TalkBack and
accessibility-based automation tools use, not a UiAutomator/instrumentation
server. It does nothing until it receives a broadcast; the "service" runs
only inside the accessibility sandbox and only acts on
`com.iksnerd.adbmcp.bridge.ACTION_CLICK`.

**Experimental.** The mechanism is verified live (see the adb-mcp
CHANGELOG), but the specific "coordinate tap no-ops, accessibility click
succeeds" differential from the original field report hasn't been
reproduced against that reporter's actual app.

## Install (one time per device)

```
adb-mcp bridge install [--serial=<serial>]
```

Downloads the latest release's `adb-mcp-bridge.apk`, verifies its checksum,
`adb install -r`s it, and enables its `AccessibilityService` via `adb shell
settings put secure enabled_accessibility_services` — appending to, not
replacing, any other already-enabled services.

Then pass `via_accessibility=true` to `tap_on_text`/`tap_element`.

## What it does on a click broadcast

Receives `resource_id`/`text`/`partial` extras, walks
`rootInActiveWindow`, matches by `viewIdResourceName` or
`text`/`contentDescription`, prefers a clickable match — else walks up to
the nearest clickable ancestor — calls `performAction(ACTION_CLICK)`, and
logs one JSON result line (`Log.i("AdbMcpBridge", ...)`) that
`adb.Client.AccessibilityClick` (`internal/adb/bridge.go`) reads back via
`adb logcat`.

## Building locally

```
cd bridge
./gradlew :app:assembleRelease
adb install -r app/build/outputs/apk/release/app-release.apk
```

Signed with the repo-committed `release.keystore` — not a secret, just a
fixed signature so CI-built APKs upgrade in place across releases. This is
a side-loaded diagnostic tool, not a Play Store app; there is no identity
to protect.
