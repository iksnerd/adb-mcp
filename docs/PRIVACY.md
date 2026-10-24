# Privacy

adb-mcp runs on your computer. It does not collect, store or send your data to the
project, to Anthropic through any channel of its own, or to any other service.

## What the server does with data

The server is a local process that Claude starts and talks to over standard input
and output. When Claude calls a tool, the server runs `adb`, `gradle` or another
local command and returns the result to Claude in the same session. That result can
include what is on the device screen, the UI hierarchy, logcat output, files you
pull from the device, and build output. It goes to Claude as part of your
conversation, under your Claude plan's terms, and nowhere else.

The server keeps no database and no account. Files it writes (screenshots, pulled
files, build logs) go where you or Claude point it, on your machine. Credentials in
Gradle output are redacted before they are returned.

## Network access

The running server makes no network requests. Two things outside it do:

- **The plugin launcher** downloads the server binary from this project's GitHub
  release on first start and verifies it against a pinned SHA-256
  (`plugin/adb-mcp/launcher/`).
- **Two commands you run yourself**, `adb-mcp update` and `adb-mcp bridge install`,
  download a release from GitHub and verify its checksum.

GitHub sees the request for the release file, as it would for any download.

## Questions

Open an issue at <https://github.com/iksnerd/adb-mcp/issues>.
