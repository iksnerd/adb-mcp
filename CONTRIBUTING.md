# Contributing to adb-mcp

Thanks for your interest in improving adb-mcp. This is the Android counterpart to
[XcodeBuildMCP](https://github.com/getsentry/XcodeBuildMCP): an MCP server that
drives Android emulators/devices over `adb`.

## Getting set up

Requirements are the same as running the server (see the [README](README.md)):

- Go 1.26+
- Android SDK with `platform-tools` (`adb`) and `emulator` on your machine
- At least one AVD, if you want to smoke-test against a live device

```bash
make check     # go vet + go test — the unit tests need NO emulator
make build     # compile ./bin/adb-mcp
make run       # run over stdio for manual JSON-RPC poking
```

`make check` is the bar every change must clear before it's ready for review.

## Architecture

The code is execution layers mirrored by a thin adapter layer — read
[ARCHITECTURE.md](ARCHITECTURE.md) for the full map and diagram:

- `internal/adb/` — the device layer: an `adb.Client` whose methods wrap
  `adb`/`emulator`. No dependency on the MCP SDK, so it stays unit-testable.
- `internal/gradle/`, `internal/uiauto/`, `internal/sdk/` — the other pure
  execution/parse layers (builds, UI-hierarchy parsing, SDK resolution).
- `internal/tools/` — thin MCP tool bindings. Each `tools/<domain>.go` mirrors an
  execution file of the same name (e.g. `adb/<domain>.go`).
- `internal/guides/` — the driving "skill" guides, embedded and served as MCP
  resources.

## Conventions

- **Keep the layers honest.** Real logic lives in the execution packages
  (testable); `internal/tools` just resolves the device, calls into them, and
  formats the result. Don't put `adb` calls or parsing in the tools layer.
- **Every device-facing tool takes an optional `serial`** (adb `-s`). Omit it and
  the server targets the single attached device; with several it returns an
  actionable "pass serial" error.
- **Unit-test any new pure logic** — parsers, coordinate math, argument parsing.
  Tests must not require a live emulator.
- **Write tool descriptions for the model that calls them**: what the tool does,
  when to reach for it, and the one gotcha that most often wastes a turn.

## Submitting changes

1. Fork and branch off `main`.
2. Make your change with tests; run `make check`.
3. Open a pull request describing the change and how you verified it (include a
   live smoke test against an emulator when the change touches device behavior).

Open a [GitHub issue](https://github.com/iksnerd/adb-mcp/issues) first if you want
to discuss a larger change before building it.
