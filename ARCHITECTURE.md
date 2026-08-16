# Architecture

Ten packages in a strict dependency line, and one convention: **every MCP tool
file mirrors an execution file of the same name.** Find a tool and its real
logic sits one layer down under the matching filename.

```
cmd/adb-mcp/main.go    entry: build server, register tools + resources, Run(stdio)
internal/tools/        thin MCP adapters — resolve a device, call adb/gradle, format
internal/adb/          the device layer: an adb.Client whose methods are the commands
internal/gradle/       host-side Gradle: build, find APKs, parse test + coverage reports
internal/uiauto/       pure uiautomator-hierarchy model + parsing (Element, filters, find)
internal/sdk/          resolves the Android SDK (adb/emulator paths, PATH + ANDROID_HOME)
internal/concurrent/   RunAll/RunIndexed — fan out independent I/O calls, join, done
internal/guides/       the skill guides, embedded and served as MCP resources
internal/scaffold/     generates a new Android project (scaffold_android_project)
internal/selfupdate/   the `adb-mcp update` self-updater (cmd/adb-mcp only)
internal/bridgeupdate/ the `adb-mcp bridge install` installer (cmd/adb-mcp only)
```

Dependencies point inward only: `sdk`, `uiauto`, and `concurrent` are leaves;
`gradle → sdk`; `adb → sdk, uiauto, concurrent`; `tools → adb, gradle, uiauto,
scaffold`; `selfupdate → concurrent`; `bridgeupdate → adb, concurrent`.
Nothing imports `tools`, and no execution package imports the MCP SDK.
`selfupdate`/`bridgeupdate` are wired only from `cmd/adb-mcp/main.go`, not
from `tools`.

**`bridge/`** (repo root, sibling to `cmd/`/`internal/`) is a separate,
non-Go Android Gradle project — the small companion AccessibilityService APK
that `internal/bridgeupdate` downloads and `internal/adb/bridge.go` drives.
EXPERIMENTAL; see `bridge/README.md`.

## Diagram

Source: [`docs/architecture.mmd`](docs/architecture.mmd) (rendered below; GitHub
renders the Mermaid block natively).

```mermaid
flowchart TB
    client["MCP client (agent)"]

    subgraph entry["cmd/adb-mcp/main.go"]
        server["build server · Register(tools) + resources · Run(stdio)"]
    end

    subgraph tools["internal/tools — MCP adapters (thin)"]
        register["register.go<br/><i>tool catalog: add(name, desc, handler)</i>"]
        t_files["emulator · observe · interact · lock · logs · apps · environment · gradle · session"]
        t_helpers["helpers.go<br/><i>resolve → *adb.Client, text, jsonResult, boolOr</i>"]
    end

    subgraph adb["internal/adb — device layer (adb.Client)"]
        client_t["adb.go<br/><i>Client{Serial, run Runner} · New · c.adb/c.adbBytes</i>"]
        a_cmds["input · packages · screen · ui · lock · logcat · capture<br/>permissions · files · environment · devices · emulator · crash · statusbar"]
    end

    gradle["internal/gradle<br/><i>Gradle · FindAPKs · PickAPK · ParseTestResults<br/>FindCoverageReports · MergeReports · SummarizeCoverage<br/>GenerateWrapper · ToolchainReport</i>"]
    uiauto["internal/uiauto<br/><i>Element · UIFilter · ParseHierarchy · FindByText/ResourceID</i>"]
    sdk["internal/sdk<br/><i>Root · IsSDKRoot · AdbPath · EmulatorPath · CommandEnv</i>"]
    concurrent["internal/concurrent<br/><i>RunAll · RunIndexed</i>"]
    guides["internal/guides<br/>android://guide/* resources"]
    scaffold["internal/scaffold<br/><i>generates a new Android project</i>"]
    selfupdate["internal/selfupdate<br/><i>adb-mcp update</i>"]
    bridgeupdate["internal/bridgeupdate<br/><i>adb-mcp bridge install (EXPERIMENTAL)</i>"]

    client -->|stdio JSON-RPC| server
    server --> register
    server --> guides
    server -. "adb-mcp update" .-> selfupdate
    server -. "adb-mcp bridge install" .-> bridgeupdate
    register --> t_files
    t_files --> t_helpers

    t_helpers --> client_t
    t_files --> gradle
    t_files --> uiauto
    t_files --> scaffold

    a_cmds -. "c.adb(...)" .-> client_t
    a_cmds --> uiauto
    a_cmds --> concurrent
    bridgeupdate --> client_t
    bridgeupdate --> concurrent
    selfupdate --> concurrent
    client_t --> sdk
    gradle --> sdk
```

## The layers

**`internal/sdk` — SDK resolution.** Where `adb` and `emulator` live and the
environment they need. The leaf both the device and build layers share, so
neither re-derives SDK paths. `CommandEnv` prepends the SDK tool dirs to `PATH`
and exports `ANDROID_HOME` when the caller hasn't set one — the Android Gradle
plugin does its own SDK lookup and knows nothing about `Root`'s per-platform
fallback, so without that every Gradle tool fails with "SDK location not
found". `IsSDKRoot` is the shared answer to "is this really an SDK", so
`doctor`'s warning and that export can't disagree.

**`internal/uiauto` — the UI model.** `Element`/`Bounds`/`Point`/`UIFilter` and
the pure functions over them: parse a uiautomator XML dump, filter it, find an
element by text or resource id. No I/O, no adb — trivially unit-testable, and
the type vocabulary the device layer returns.

**`internal/concurrent` — the fan-out primitive.** `RunAll(fns ...func())` and
`RunIndexed(n, fn)` run independent I/O calls (adb shell-outs, HTTP fetches)
concurrently and join before returning — the shared alternative to hand-rolling
a `sync.WaitGroup` at every call site that has probes not depending on each
other's output (`Doctor`'s adb/AVD/device checks, `GetAppStateWithSource`'s
activity/uptime/logcat reads, `DescribeUI`'s settle+focus reads,
`selfupdate`/`bridgeupdate`'s asset+checksum fetches). No knowledge of adb or
HTTP — callers own their own result via closure capture.

**`internal/adb` — the device layer.** An `adb.Client` holds a device serial and
a `Runner` (the one seam that shells out). Every device command is a **method**
(`c.Tap`, `c.InstallApp`, `c.DescribeUI`, …) that builds an argv and calls
`c.adb`/`c.adbBytes`. `New(serial)` wires the real adb binary; a test wires a
fake `Runner` and asserts the exact argv — so command builders are unit-tested
with **no device** (see `client_test.go`). Hostless helpers that have no serial
(`ListDevices`, `BootEmulator`, `ConnectWireless`, `Doctor`) stay package funcs.

**`internal/gradle` — host-side builds.** `Gradle` runs the wrapper; `FindAPKs`
locates outputs (newest-first, `node_modules`/dot-dirs pruned); `PickAPK` skips
androidTest APKs; `ParseTestResults` reads the JUnit XML. `FindCoverageReports`
locates the JaCoCo XML — newest *per module*, since a multi-module build emits
one report each and only merging them describes the whole build — and
`MergeReports`/`SummarizeCoverage`/`FileCoverageFor` reduce them to the
report-wide, per-package and per-file views. `GenerateWrapper` shells out to a
*system* `gradle` (everything else here drives `./gradlew`) so a scaffolded
project is buildable, and `ToolchainReport` probes the host JDK and Gradle for
`doctor` — that check lives here rather than in `adb` because the device layer
has no business knowing what a JDK is. Depends only on `sdk`.

**`internal/tools` — MCP adapters.** Each handler is deliberately thin: `resolve`
the target device into an `*adb.Client`, call one method (or a gradle/uiauto
function), format the result. `register.go` is *only* the tool catalog
(`add(name, description, handler)`); it holds no handler bodies.

**`internal/scaffold` — project generation.** `scaffold_android_project`'s
implementation: writes a minimal Kotlin Android project into a new empty
directory. No dependency on `adb`/`gradle`/`sdk` — it never touches a device
or build tool, just template files.

**`internal/selfupdate` — the `adb-mcp update` CLI subcommand.** Fetches the
latest GitHub release, verifies its checksum, swaps the running binary. Wired
only from `cmd/adb-mcp/main.go`, not from `internal/tools` — it's a CLI path,
not an MCP tool.

**`internal/bridgeupdate` — the `adb-mcp bridge install` CLI subcommand
(EXPERIMENTAL).** Same fetch/verify shape as `selfupdate`, but installs the
`bridge/` APK on a resolved device (`adb.Client.InstallBridge`) and enables
its `AccessibilityService` (`adb.Client.EnableBridgeService`) instead of
replacing the running binary. Also CLI-only, not wired from `internal/tools`
— installing an app and flipping a system accessibility setting is a real
side effect that shouldn't happen silently from an agent tool call.

## The mirror

Each domain has a file in `internal/tools` and a matching execution file. To
change a tool, you touch two same-named files — one for the wire/argument shape,
one for the behavior.

| Domain | execution | `internal/tools/` (MCP adapter) |
|---|---|---|
| adb client core | `adb/adb.go` | `helpers.go` (`resolve → *adb.Client`) |
| device enumerate / resolve / connect | `adb/devices.go` | `emulator.go` |
| emulator lifecycle | `adb/emulator.go` | `emulator.go` |
| screen capture | `adb/screen.go`, `adb/image.go` | `observe.go` |
| runtime UI observe | `adb/ui.go` + `uiauto/uiauto.go` | `observe.go` |
| input / gestures / PIN | `adb/input.go`, `adb/keyevent.go` | `interact.go` |
| device lock | `adb/lock.go` | `lock.go` |
| logs & recording | `adb/logcat.go`, `adb/capture.go` | `logs.go` |
| app lifecycle | `adb/packages.go` | `apps.go` |
| permissions | `adb/permissions.go` | `apps.go` |
| file transfer | `adb/files.go` | `apps.go` |
| environment (dark / geo / doctor) | `adb/environment.go`, `adb/doctor.go` (device side) + `gradle/toolchain.go` (host JDK/Gradle) | `environment.go` (composes both) |
| gradle build & test | `gradle/gradle.go`, `gradle/testreport.go` | `gradle.go` |
| JaCoCo coverage reports | `gradle/coverage.go` | `gradle.go` |
| project scaffolding | `scaffold/scaffold.go` | `gradle.go` |
| session defaults (`project_dir`/`serial`) | — (no execution layer; process-local state) | `session.go` (`resolveProjectDir`, `resolve`) |
| SDK paths / shared helpers | `sdk/sdk.go` | `helpers.go` |
| self-update (CLI only, not an MCP tool) | `selfupdate/selfupdate.go` | — (`cmd/adb-mcp/main.go`) |
| accessibility bridge (EXPERIMENTAL) | `adb/bridge.go` | `interact.go` (`via_accessibility` on `tap_on_text`/`tap_element`) |
| bridge install (CLI only, not an MCP tool) | `bridgeupdate/bridgeupdate.go` | — (`cmd/adb-mcp/main.go`) |

## Conventions

- **Device commands are `adb.Client` methods; pure logic is a plain function.**
  Anything that shells out becomes a method on `*Client` so it is injectable and
  testable; parsing/geometry lives in `uiauto` (or a pure func) with its own test.
- **Handlers own their argument structs.** Each `tools/<domain>.go` declares the
  `…Args` structs (with `jsonschema` tags) for the handlers in that file.
- **Truly shared adapter helpers live in `helpers.go`:** `serialArg`, `text`,
  `jsonResult`, `resolve`, `boolOr`. Domain-specific helpers stay with their
  domain (`parseCoords` in `interact.go`, `tailLines` in `gradle.go`).
- **Dependencies point inward only.** No execution package imports `internal/tools`
  or the MCP SDK. Logic that needs testing belongs below `internal/tools`; only
  wire-format glue belongs in it.

## Adding a tool

1. Implement the behavior where it belongs: a device command → a method on
   `adb.Client` in the matching `internal/adb/<domain>.go`; pure parsing →
   `internal/uiauto`; a build step → `internal/gradle`. Unit-test it there (a
   command builder with a fake `Runner`; pure logic directly).
2. Add the argument struct and a thin handler to the matching `internal/tools/<domain>.go`.
3. Register it in `internal/tools/register.go` with a model-facing description.
