package tools

// Instructions is returned to the client on initialize and lands in the model's
// system prompt before any tool is called — the only channel here that is read
// without the agent choosing to read it. Guide resources are a *pull* channel
// and agents do not pull: a field session drove a device for an hour, worked
// several things out the hard way, and read zero android://guide/* resources
// until a human intervened.
//
// So this carries ONLY what is catastrophic to miss, and stays short: it is
// permanently in every session's context, competing with everything else there.
// Anything that can instead live in a tool description (read at the moment of
// use) or an error message (read at the moment of being wrong) belongs there.
const Instructions = `adb-mcp drives Android emulators and devices over adb: boot, observe (screenshot/describe_ui), act (tap/swipe/type/keys), inspect apps, read logcat, run Gradle builds and tests.

React Native / Expo dev builds — in this order, before concluding anything:
  1. adb_reverse {device_port: 8081} BEFORE launching. A dev client that cannot
     reach Metro does not error; it silently runs its EMBEDDED bundle and
     ignores every code edit.
  2. app_state {package, source_path} to confirm the process is on Metro, that
     it is the RIGHT Metro (a dev server from another checkout still holds the
     port), that its JavaScript is not stale, and that only one process is live.

Absence of a signal is not evidence until the harness is proven to deliver that
signal — an empty logcat is often a rotated buffer, and missing logs are often
an embedded bundle. describe_ui and wait_for_text see only the current
viewport; Android omits off-screen ScrollView content, so scroll before
concluding an element is absent.

Batch timing-sensitive flows with run_sequence: a per-step round trip perturbs
native timers.

Recipes: android://guide/rn-expo, android://guide/driving, android://guide/getting-started.`
