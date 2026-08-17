# adb-mcp plugin

Registers the [adb-mcp](https://github.com/iksnerd/adb-mcp) MCP server with Claude Code.

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
