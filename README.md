<div align="center">
<img src="assets/android-head_flat.svg" width="72" alt="Android robot logo">

# adb_mcp

**An [MCP](https://modelcontextprotocol.io) server that drives Android emulators and devices over `adb`**

[![CI](https://github.com/iksnerd/adb_mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/iksnerd/adb_mcp/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![MCP](https://img.shields.io/badge/MCP-stdio-3DDC84)](https://modelcontextprotocol.io)

</div>

---

`adb_mcp` lets an AI agent drive an Android emulator or device over `adb` —
boot an AVD, screenshot, read the UI hierarchy, tap/swipe/type, set a device
lock, read `logcat`, and manage app lifecycle.

It is the Android counterpart to [XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP):
where XcodeBuildMCP gives an agent first-class control of iOS simulators, this
gives the same for Android emulators. Built on the official
[Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) and communicates over stdio.

> Android is a trademark of Google LLC. `adb_mcp` is an independent, unofficial
> tool built for Android and is not affiliated with, sponsored, or endorsed by Google.
> The Android robot above is reproduced/modified from work created and shared by
> Google and used according to terms described in the
> [Creative Commons 3.0 Attribution License](https://creativecommons.org/licenses/by/3.0/).

## Why

Driving Android by hand means a long runbook of raw `adb` commands, and it is
easy to get wrong (stale tap coordinates, CRLF-corrupted screenshots, forgetting
`exec-out`, guessing coordinates off a downscaled image). This server bakes
that knowledge into its tools, so the agent doesn't have to relearn it:

- Screenshots use `exec-out screencap` (no CRLF corruption) and are auto-downscaled
  so the image reader accepts them.
- `describe_ui` returns each element's **center in true device pixels** (so taps
  land where you mean them to, no guessing off the image) and retries the
  transient "could not get idle state" failure on its own.

The workflow itself is bundled as readable **resources** (see below), so the
agent can consult the "skill" the same way it would read a skill file.

## Getting started

### 1. Prerequisites

- Android SDK with `platform-tools` (`adb`) and `emulator`. The server finds it
  via `$ANDROID_HOME` / `$ANDROID_SDK_ROOT`, else the platform default
  (`~/Library/Android/sdk` on macOS).
- At least one AVD (create one in Android Studio's Device Manager).

Go is **not** required — releases ship prebuilt binaries; it's only needed to
[build from source](#from-source-go-126).

### 2. Install

On macOS/Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/iksnerd/adb_mcp/main/install.sh | sh
```

The script ([install.sh](install.sh)) picks the right archive for your
OS/architecture, verifies its SHA-256 against the release's `checksums.txt`,
and installs to `~/.local/bin` (override with `BIN_DIR=...`; pin a version
with `VERSION=v0.11.0`).

On Windows, download the `windows_amd64` or `windows_arm64` zip from the
[releases page](https://github.com/iksnerd/adb_mcp/releases/latest) and put
`adb-mcp.exe` somewhere on your `PATH`.

Once installed, stay current with the built-in updater — it fetches the latest
release, verifies its checksum, and swaps the binary in place:

```bash
adb-mcp update
```

The registration below launches the server by the bare name `adb-mcp`, so it
must be on your `$PATH` (`which adb-mcp` should resolve — the installer warns
if `~/.local/bin` isn't on it). Otherwise point the client at the absolute
path to the binary instead.

### 3. Register with your MCP client

**Claude Code:**

```bash
claude mcp add adb -- adb-mcp
```

(When working inside this repo itself, the bundled `.mcp.json` is picked up
automatically — no registration needed.)

**Cursor / VS Code** — one-click install (assumes `adb-mcp` is on your `PATH`
from step 2):

[<img src="https://cursor.com/deeplink/mcp-install-dark.svg" alt="Install in Cursor" height="20">](https://cursor.com/en/install-mcp?name=adb&config=eyJjb21tYW5kIjoiYWRiLW1jcCJ9)
[<img src="https://img.shields.io/badge/VS_Code-Install_Server-0098FF?style=flat-square" alt="Install in VS Code" height="20">](https://insiders.vscode.dev/redirect?url=vscode%3Amcp%2Finstall%3F%257B%2522name%2522%253A%2522adb%2522%252C%2522command%2522%253A%2522adb-mcp%2522%257D)

**Any other client** (Windsurf, Codex, …): run `adb-mcp` over stdio.
The usual config shape:

```json
{
  "mcpServers": {
    "adb": { "command": "adb-mcp" }
  }
}
```

That's it — ask your agent to "boot an emulator and take a screenshot" to
confirm everything is wired up.

### From source (Go 1.26+)

```bash
make install                 # builds ./bin/adb-mcp and copies it to ~/.local/bin
# or:
go build -o bin/adb-mcp ./cmd/adb-mcp
# or straight from the module (note the /cmd/adb-mcp suffix):
go install github.com/iksnerd/adb_mcp/cmd/adb-mcp@latest
```

## Tools

55 tools across eight areas. Every device-facing tool takes an optional
`serial` (adb `-s`) — omit it with one device attached, or pass one from
`list_devices` with several. Full reference: [docs/TOOLS.md](docs/TOOLS.md).

- **Emulator / device** — boot, list, wait-for-boot, shut down, connect over Wi-Fi, `adb_reverse` port forwarding (Metro!)
- **Observe** — `screenshot` to see, `describe_ui` for true-pixel element centers — with the focused **top window** (spot a biometric prompt occluding your app), `filter`/`query`/`compact` modes, and a hidden-node count so absence is trustworthy
- **Interact** — tap, `tap_on_text`/`tap_element` (id-addressed), swipe, drag, long-press, type, key combos, PIN pads, `wait`; opt-in `verify_change` tells you whether a tap/key actually changed the UI
- **Lock / Keystore / Biometrics** — set/clear a secure lock screen, check lock state, `fingerprint_touch` to satisfy a BiometricPrompt on the emulator
- **App lifecycle** — install/uninstall, launch/stop, `reload_app`/`open_dev_menu`, clear data, permissions, deep links, push/pull files, `last_crash`
- **Logs & capture** — one-shot or streaming `logcat` (substring/priority/tag filters, `since` time window), `clear_logcat`, `last_crash`, screen recording
- **Environment & diagnostics** — dark mode, mock location, clean status bar, `doctor`
- **Gradle build & test** — `assembleDebug`, unit tests, instrumented tests, task discovery, one-shot `build_and_run`

The driving know-how itself ships as four MCP **resources** (`android://guide/*`)
the client can list and read — see [docs/TOOLS.md](docs/TOOLS.md) for the URIs,
or jump straight to `android://guide/driving` for the core loop below.

## The core loop

**observe → locate → act → re-observe.** `screenshot` to see, `describe_ui` to
get true-pixel centers, `tap`/`tap_on_text`/`swipe`/`input_text` to act, then
`screenshot` again to confirm. Read `android://guide/driving` for the full loop
and the gotchas that waste turns.

## Development

```bash
make check     # go vet + go test (unit tests need no emulator)
make run       # run over stdio for manual JSON-RPC poking
```

Layout:

```
cmd/adb-mcp/main.go        entry: subcommands (update/version) or serve MCP over stdio
internal/tools/            thin MCP tool bindings
internal/adb/              the device layer: an adb.Client whose methods are the commands
internal/gradle/           host-side Gradle: build, find APKs, parse test reports
internal/uiauto/           pure uiautomator-hierarchy model + parsing (unit-tested)
internal/sdk/              resolves the Android SDK (adb/emulator paths, PATH env)
internal/guides/           the skill guides, embedded and served as MCP resources
internal/selfupdate/       the `adb-mcp update` release fetch/verify/swap
```

Dependencies point inward only (`tools → adb, gradle, uiauto → sdk`), and each
`internal/tools/<domain>.go` adapter **mirrors** an execution file of the same
name (device commands are `adb.Client` methods). Full map, the package graph,
and the rules for adding a tool: [ARCHITECTURE.md](ARCHITECTURE.md).

## Documentation

- [docs/TOOLS.md](docs/TOOLS.md) — full tool-by-tool reference and the guide resources
- [ARCHITECTURE.md](ARCHITECTURE.md) — the mirror convention, package layout, and how to add a tool
- [docs/CHANGELOG.md](docs/CHANGELOG.md) — shipped work, newest first
- [docs/BACKLOG.md](docs/BACKLOG.md) — open ideas and XcodeBuildMCP parity gaps
- [TODO.md](TODO.md) — current roadmap hub

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup and conventions. Found a
security issue? See [SECURITY.md](SECURITY.md) instead of opening a public
issue. Licensed under [MIT](LICENSE).
