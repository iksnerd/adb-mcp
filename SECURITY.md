# Security Policy

## Reporting a vulnerability

Please report security issues privately rather than opening a public issue.

Use GitHub's [private vulnerability reporting](https://github.com/iksnerd/adb-mcp/security/advisories/new)
to open a confidential advisory. Include:

- what the issue is and its impact,
- steps to reproduce (a minimal case is ideal), and
- any suggested fix if you have one.

You can expect an initial response within a few days. Once a fix is available it
will be released and the advisory published with credit, unless you prefer to
remain anonymous.

## Scope

adb-mcp drives Android emulators and devices over `adb` on the local machine: it
runs `adb`/`emulator`/`gradlew`, installs APKs, and reads device state. Reports
that are especially in scope:

- command-injection or argument-injection through tool inputs into the `adb`,
  `emulator`, or Gradle command lines,
- path-traversal or unintended file access via `push_file`/`pull_file`/`install_app`,
- leaking of local files or environment beyond what a tool is documented to expose.

The server trusts the MCP client it is wired to and the local Android SDK
toolchain; issues that require an already-compromised local machine are generally
out of scope.
