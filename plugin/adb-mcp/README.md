# adb-mcp plugin

Lets Claude drive an Android emulator or a USB or wireless device through
[adb-mcp](https://github.com/iksnerd/adb-mcp): take screenshots, read the UI
hierarchy, tap, swipe and type, install and launch apps, read logcat, and run
Gradle builds and tests. It adds a skill for each driving guide (`getting-started`,
`driving`, `rn-expo`, `pin-and-lock`, `crash-triage`, `build-and-test`); each skill
tells Claude to read the matching `android://guide/*` resource before it starts that
kind of task.

## Install

```bash
claude plugin marketplace add iksnerd/adb-mcp
claude plugin install adb-mcp@adb-mcp
```

You need `node` on your `PATH` and the Android SDK (`adb`) on your machine. See the
[main README](https://github.com/iksnerd/adb-mcp) for the SDK setup. The server runs
on your computer, so it works in Claude Code and in Cowork sessions that run locally,
not in claude.ai chat.

## What the plugin runs, downloads and sends

The plugin starts one local MCP server named `adb` with
`node ${CLAUDE_PLUGIN_ROOT}/launcher/launch.js ${CLAUDE_PLUGIN_DATA}`. The server itself is a Go binary
that is too large to ship inside a plugin, so the launcher fetches it:

1. On first start, `launcher/launch.js` downloads the build for your OS and CPU
   (macOS, Linux or Windows; amd64 or arm64) from this project's GitHub release,
   `https://github.com/iksnerd/adb-mcp/releases/download/v<version>/`. It is the
   only network request the plugin makes.
2. It checks the SHA-256 of the download against the value pinned in
   `launcher/release.json`, which is committed in this plugin and recorded by the
   release workflow. A file that does not match is discarded and never run.
3. It keeps the verified binary in the plugin's data directory
   (`${CLAUDE_PLUGIN_DATA}`), checks it again on every start, and starts it with
   your standard input and output passed straight through.

The server runs `adb` and Gradle on your machine. It sends nothing to any service:
screenshots, UI dumps, logcat output and file contents go only to Claude, as tool
results in your session. See [docs/PRIVACY.md](../../docs/PRIVACY.md) for the full
statement. The source of the launcher is the one file `launcher/launch.js`, and the
source of the server is this repository.

## Updating

A new plugin version pins a new server version. The launcher downloads it the next
time it starts and leaves the older one in the data directory until you uninstall.
