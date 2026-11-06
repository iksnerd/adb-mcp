# adb-mcp skills

Six skills that teach Claude how to drive an Android emulator or device with the
[adb-mcp](https://github.com/iksnerd/adb-mcp) server: the observe, locate, act,
re-observe loop; React Native and Expo apps; PIN and lock-screen handling; crash
triage; and Gradle builds and tests. Each skill holds a full guide: `getting-started`,
`driving`, `rn-expo`, `pin-and-lock`, `crash-triage` and `build-and-test`.

## This plugin has no server

The skills describe tools that the adb-mcp server provides. This plugin does not
start that server, run any program, download anything, or make any network request.
It is text only.

To get the tools, install the server separately and register it with Claude Code.
The [adb-mcp README](https://github.com/iksnerd/adb-mcp#getting-started) has the
steps and the Android SDK prerequisites. The server runs on your computer, so it works
in Claude Code and in Cowork sessions that run locally, not in claude.ai chat.

Without the server the skills still read as guides, but there is nothing for Claude to
call.

## Privacy

The plugin reads and sends nothing. The server's own data handling is described in
[docs/PRIVACY.md](https://github.com/iksnerd/adb-mcp/blob/main/docs/PRIVACY.md).

## License

MIT.
