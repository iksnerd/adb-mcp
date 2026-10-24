# adb-mcp plugin

Registers the [adb-mcp](https://github.com/iksnerd/adb-mcp) MCP server with Claude Code and adds a skill for each driving guide (`getting-started`, `driving`, `rn-expo`, `pin-and-lock`, `crash-triage`, `build-and-test`). Each skill tells the agent to read the matching `android://guide/*` resource before it starts that kind of task.

## Prerequisites

- `node` on your `PATH`. The plugin starts the server through a small launcher.
- The Android SDK. See the [main README](https://github.com/iksnerd/adb-mcp) for setup.

You do not need to install `adb-mcp` separately: the plugin carries the server.

## What this plugin does

Bundles a `.mcp.json` that registers the `adb` server and runs it as
`node ${CLAUDE_PLUGIN_ROOT}/bin/launch.js`. The launcher picks the binary in
`bin/<os>-<arch>/` for your machine (macOS, Linux and Windows, amd64 and arm64),
checks its SHA-256 against `bin/checksums.txt`, and starts it with your
stdin and stdout unchanged. It refuses to start a binary that does not match.

The binaries are committed in each release's commit and built from the same
source as the release archives (`make plugin-bins`); the release gate fails if
they are missing, altered, or built for another version.
