# React Native / Expo dev builds

This is the most expensive surface this server touches. Almost every hour lost
in field reports was lost here, and never to a hard failure — always to a
success-shaped one: the app launched, the taps landed, the logs read, and the
code being exercised was not the code on disk.

The ordering below is the part people get wrong. Follow it before you conclude
anything about the app's behaviour.

## The recipe

```
adb_reverse   {device_port: 8081}          # BEFORE launching. Not after.
launch_dev_client {scheme: "myapp"}        # scheme from app.json, NOT the package name
app_state     {package: "...",             # confirm metro, non-stale, single process
               source_path: "/path/to/checkout"}
reload_app    {package: "..."}             # after each edit; fall back to
                                           # open_dev_menu + tap_on_text("Reload")
```

`adb_reverse` first, because a dev client that cannot reach Metro does not
error — it silently loads its **embedded** bundle and ignores every edit you
make. `launch_dev_client` builds the
`<scheme>://expo-development-client/?url=http://localhost:8081` deep link and
skips the Dev Launcher's server-picker screen; the scheme is your `app.json`
`"scheme"` value, and passing the package name instead is the common slip (the
tool lists the schemes the package really registers when the launch fails).

Then `app_state`, every time, before believing anything on screen.

## `app_state` is the one call that makes the rest trustworthy

It answers four questions that otherwise take an hour of inference:

- **Which bundle?** `bundle_source: metro | embedded`, with the
  `bundle_evidence` it keyed on and the `bundle_signals` that backed it
  (`logcat` markers, a `live_socket` to the dev-server port, or both).
- **Which Metro?** See below.
- **How stale?** Pass `source_path` and read `stale_verdict` —
  `stale` / `current` / `undetermined`, always with a reason.
- **How many processes?** Two live pids for one package (a lingering old build
  next to a fresh install) means your taps and your log reads are hitting
  different apps. It says so.

## `metro` does not mean the RIGHT metro

A dev server left running from a previous checkout keeps holding port 8081. A
dev client connects to it perfectly happily, and every signal reads healthy:

```json
{"bundle_source": "metro",
 "bundle_evidence": "process has an established TCP connection to Metro port 8081"}
```

Meanwhile the JavaScript executing is the other branch's. **This is worse than
the embedded case** — embedded at least looks wrong the moment you check it,
whereas "metro" looks right.

So `app_state` also reports *which* dev server, from the host side:

```json
{"metro": {"port": 8081, "url": "http://localhost:8081",
           "pid": 40122, "command": "node",
           "project_root": "/Users/you/src/OtherBranch"}}
```

If `project_root` is not the checkout you are reasoning about, kill that server
and start Metro from the right tree. Passing `source_path` makes the tool do
that comparison for you and return `stale_verdict: "stale"` naming both paths.

## Staleness after a git operation

Metro's file watcher misses **git-driven** file replacement — `git stash push
<file>`, `git checkout -- <file>`, a branch switch. The socket stays live, the
verdict stays `metro`, and the bundle keeps serving the pre-checkout code. Two
field reports produced confidently wrong conclusions this way: instrumentation
that "never ran", and a revert that "didn't take".

`app_state {source_path}` compares the newest mtime under that path against the
latest epoch-timed Metro/HMR marker in the app's own logcat. When it cannot
compare — typically a freshly launched process that has not logged an HMR
marker yet — it returns `undetermined` **with the reason**, never silence. Treat
`undetermined` as "not yet checked", not as "fine": `clear_logcat`,
`reload_app`, then call `app_state` again.

## Traps with no other home

- **`expo run:android` skips prebuild when `android/` already exists.** After a
  branch switch, native modules autolink but config-plugin **manifest** changes
  do not apply — permissions added by a plugin (`expo-contacts`), a
  `blockedPermissions` entry, an intent filter. The symptom is a runtime
  permission request that fails for no visible reason, in an app whose JS
  clearly asks for it. Re-run prebuild (or delete `android/`) rather than
  debugging the JS.
- **`force-stop` + `monkey` lands on the Dev Launcher list**, not your app. Use
  `launch_dev_client` to get back in.
- **`reload_app` can silently no-op** on newer bridgeless Expo dev clients: the
  `<pkg>.RELOAD_APP_ACTION` broadcast receiver is only registered by classic
  debug builds. The fallback is `open_dev_menu` then `tap_on_text("Reload")`.
- **A port other than 8081** must be changed in both `adb_reverse` and
  `launch_dev_client` — they move together.
- **`describe_ui` and `wait_for_text` see only the viewport.** Android omits a
  `ScrollView`'s off-screen children from the accessibility tree, at every
  filter including `all`. Scroll (or `wait_for_text {scroll: true}`) before
  concluding an element is absent, and use `render_stats` — not `describe_ui` —
  to count how many views a list actually mounted.

## The rule underneath all of it

**Absence of a signal is not evidence until the harness is proven to deliver
that signal.** An empty `logcat` was a rotated buffer. Missing logs were an
embedded bundle. A tool that answers successfully with an empty payload is
worse than one that errors. `app_state`, `clear_logcat` /
`start_logcat_capture`, and `describe_ui {filter: "all"}` exist so that each of
those is one call to check instead of an hour to infer.
