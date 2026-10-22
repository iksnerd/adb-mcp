# adb-mcp plugin

Registers the [adb-mcp](https://github.com/iksnerd/adb-mcp) MCP server with Claude Code and adds a skill for each driving guide (`getting-started`, `driving`, `rn-expo`, `pin-and-lock`, `crash-triage`, `build-and-test`). Each skill tells the agent to read the matching `android://guide/*` resource before it starts that kind of task.

## Prerequisite

`adb-mcp` must already be on your `PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/iksnerd/adb-mcp/main/install.sh | sh
```

See the [main README](https://github.com/iksnerd/adb-mcp) for full setup, including the Android SDK requirement.

## What this plugin does

Bundles a `.mcp.json` that registers the `adb` server, so installing the plugin is
equivalent to running:

```bash
claude mcp add adb -- adb-mcp
```
