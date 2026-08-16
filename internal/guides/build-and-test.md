# Building, testing, and measuring coverage with Gradle

These tools run **on the host**, not on a device. Only `run_instrumented_tests`
and `build_and_run` need a booted device. `project_dir` must be the directory
holding the Gradle wrapper (`gradlew`), not a module inside it.

Pin it once instead of repeating it:

```
session_set_defaults {project_dir: "/path/to/project"}
```

Every Gradle tool below then takes it from there. An explicit `project_dir` on a
single call still wins.

## Map the project before you build it

The single biggest time-waster is guessing a task name. Two calls remove the
guessing:

```
list_gradle_projects   → :app, :core, :feature:login   (empty = single-module)
list_gradle_variants   → debug, release, prodDebug, …
```

**In a multi-module build the root project usually has no variants of its own** —
the Android plugin is applied in `:app`, not at the root. `list_gradle_variants`
with no arguments will tell you it found none and to point at the module that
builds APKs. That is not an error to route around; it means scope the call:

```
list_gradle_variants {task: ":app:tasks"}
```

The same applies to `list_gradle_tasks`, which lists the *root* project's tasks
by default. Anywhere you can name a task you can qualify it with a module path:
`:app:assembleDebug`, `:feature:login:testDebugUnitTest`.

## Build

```
gradle_build                             → assembleDebug, reports the APK path(s)
gradle_build {task: "assembleRelease"}   → a specific variant
build_and_run {package: "com.example"}   → build → install → launch, one call
```

A variant name `V` from `list_gradle_variants` maps to the task `assembleV`.
`build_and_run` installs the **newest non-test** APK the build produced, so
leftover `androidTest` APKs and multi-flavor outputs don't confuse it.

Gradle is slow and the first run in a project downloads the distribution. Expect
build calls to take minutes, not seconds.

## Test

```
run_unit_tests                                  → JVM tests, no device needed
run_unit_tests {task: "testDebugUnitTest"}      → just one variant
run_instrumented_tests                          → on-device, needs a booted device
```

**`test` runs every variant's unit tests** — debug *and* release — so on an
Android project it does roughly twice the work you probably wanted. Pass
`task: "testDebugUnitTest"` (or `":app:testDebugUnitTest"`) once you know the
module.

Both return a pass/fail/skip summary, per-suite timing, and stack traces for
failures. Read the failure trace before re-running anything: the summary already
contains the assertion message and the frame, so a second run rarely adds
information. `json: true` gives the same data structured, which is easier to
filter when a suite is large.

`run_instrumented_tests` needs a device in state `device` — check `list_devices`
first if it errors, and see `android://guide/getting-started` for booting one.

## Coverage

```
get_coverage_report                      → overall %, per-package, worst first
get_file_coverage {file: "Foo.kt"}       → missed lines + per-method detail
```

Both run a Gradle task that generates a JaCoCo XML report, then parse it. They
cover **JVM unit tests only** — there is no on-device coverage analogue.

**The default task `jacocoTestReport` is not something Android gives you.** The
Android Gradle plugin defines no such task; it exists only if the project wrote
one. If it isn't found, there are two setups and you want to know which one you
are in:

- The project applies the `jacoco` plugin and declares its own report task —
  usually `jacocoTestReport`, but the name is whatever the build chose. Find it
  with `list_gradle_tasks {task: ":app:tasks"}`.
- A build type sets `enableUnitTestCoverage = true`, and AGP provides
  **`createDebugUnitTestCoverageReport`**. Nothing about this one says "jacoco",
  including its output path, but it writes the same JaCoCo XML:

```
get_coverage_report {task: ":app:createDebugUnitTestCoverageReport"}
```

If neither exists the project has no coverage set up, and the error says so
rather than leaving you to guess.

In a multi-module build every module's report is merged into one set of totals,
and the output lists the files it merged — so the headline percentage describes
the whole build, not whichever module Gradle happened to finish last.

### Using coverage to actually do something

The two tools are a funnel, not alternatives:

1. `get_coverage_report` → the package list is sorted **worst-covered first**.
   The top row is where a test is worth the most.
2. `get_file_coverage {file: "Thing.kt"}` → `missed lines: 12,17` and
   `partial lines: 9` are the exact lines with no test behind them. Partial
   means a branch went one way only — usually an untested `else`.
3. Write the test, re-run `get_coverage_report`, confirm the number moved.

`file` matches by suffix, so `Foo.kt` works when it's unambiguous and
`com/example/Foo.kt` disambiguates when it isn't. A name that matches several
files returns all of them; a name that matches none lists what coverage data
does exist, which is faster than guessing again.

## Scaffolding a project

`scaffold_android_project` writes a minimal Kotlin app into a **new empty
directory** (it refuses a non-empty one). It does not generate the wrapper — run
`gradle wrapper` in the result before any tool that shells out to `gradlew`.

## When a build fails

The error returns the tail of Gradle's output, which is where the real message
is. Before changing anything:

- Confirm the task exists (`list_gradle_tasks`) and that you scoped it to the
  right module. "Task 'x' not found in root project" almost always means it
  belongs to a module.
- `gradle_project_properties {module: ":app"}` dumps a module's effective
  configuration — namespace, SDK levels, build dir — when the failure looks like
  a config mismatch rather than a code error. Secret-shaped values are redacted.
- A JVM-target mismatch ("Inconsistent JVM-target compatibility") is a project
  config problem, not something a tool argument fixes: the build needs
  `compileOptions`/`kotlinOptions` pinned to the same version.
