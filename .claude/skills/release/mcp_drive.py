#!/usr/bin/env python3
"""Drive an adb-mcp binary over real stdio JSON-RPC.

Unit tests exercise the parsers; this exercises the tool as a client sees it —
schema, arg resolution, error results, and the text a model actually reads.

    python3 mcp_drive.py [OPTIONS] ./adb-mcp 'tool_name|{"json":"args"}[|SECONDS]' ...

Each step is NAME|JSON_ARGS, optionally followed by |SECONDS to give that step
its own timeout (a Gradle build needs minutes, a list call does not). Steps run
in order against one server process, so session state (e.g. session_set_defaults)
carries across them.

Prefix a step with '!' to assert it MUST fail — that is how you exercise error
paths (missing arg, not-found, ambiguous input) without the run reporting them
as failures:

    python3 mcp_drive.py ./adb-mcp \\
        'get_file_coverage|{"file":"Foo.kt"}' \\
        '!get_file_coverage|{"file":"NoSuchFile.kt"}'

Options:
  --timeout N     seconds to wait for each response (default 120). A step that
                  times out is named, the server is killed, later steps are
                  reported as not run, and the run still ends with its summary.
  --list-tools    first ask for tools/list and check what it advertises: every
                  tool has a title and a destructiveHint (unless read-only).
  --stats PATH    with --list-tools, also compare the tool, read_only, additive,
                  destructive and open_world counts with this docs/stats.json.

Exits non-zero when any step's outcome differs from what was asserted, so it can
gate a release.
"""
import json
import queue
import subprocess
import sys
import threading
import time

DEFAULT_TIMEOUT = 120
STATS_KEYS = ("tools", "read_only", "additive", "destructive", "open_world")


class StepTimeout(Exception):
    pass


def summarize_tools(tools):
    """Count what tools/list advertises.

    A tool that is not read-only leaves readOnlyHint out (false is the MCP
    default) and carries an explicit destructiveHint, so destructive means
    "not read-only and destructiveHint true".
    """

    def ann(t):
        return t.get("annotations") or {}

    not_read_only = [t for t in tools if ann(t).get("readOnlyHint") is not True]
    return {
        "tools": len(tools),
        "untitled": sorted(
            t["name"] for t in tools if not (t.get("title") or ann(t).get("title"))
        ),
        "unhinted": sorted(
            t["name"] for t in not_read_only if "destructiveHint" not in ann(t)
        ),
        "read_only": len(tools) - len(not_read_only),
        "additive": sum(1 for t in not_read_only if ann(t).get("destructiveHint") is False),
        "destructive": sum(1 for t in not_read_only if ann(t).get("destructiveHint") is True),
        "open_world": sum(1 for t in tools if ann(t).get("openWorldHint") is True),
    }


def parse_args(argv):
    opts = {"timeout": DEFAULT_TIMEOUT, "list_tools": False, "stats": None}
    rest = list(argv[1:])
    while rest and rest[0].startswith("--"):
        flag = rest.pop(0)
        if flag == "--timeout" and rest:
            opts["timeout"] = float(rest.pop(0))
        elif flag == "--list-tools":
            opts["list_tools"] = True
        elif flag == "--stats" and rest:
            opts["stats"] = rest.pop(0)
        else:
            sys.exit(__doc__)
    if not rest:
        sys.exit(__doc__)
    opts["binary"], opts["steps"] = rest[0], rest[1:]
    return opts


def split_step(step):
    """'!name|{json}|30' -> (expect_error, name, args, seconds or None)."""
    expect_error = step.startswith("!")
    name, _, rest = step.lstrip("!").partition("|")
    head, sep, tail = rest.rpartition("|")
    if sep and tail.strip().isdigit():
        rest, seconds = head, int(tail)
    else:
        seconds = None
    return expect_error, name, json.loads(rest or "{}"), seconds


def main(argv):
    sys.stdout.reconfigure(line_buffering=True)  # a hung step must not hide earlier output
    opts = parse_args(argv)
    steps = opts["steps"]

    proc = subprocess.Popen(
        [opts["binary"]],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        bufsize=1,
    )
    out_q = queue.Queue()
    err_lines = []

    def read_stdout():
        for line in proc.stdout:
            if line.strip():
                out_q.put(line.strip())

    def read_stderr():  # drained so a chatty server cannot block on a full pipe
        for line in proc.stderr:
            err_lines.append(line)

    threading.Thread(target=read_stdout, daemon=True).start()
    threading.Thread(target=read_stderr, daemon=True).start()

    counter = {"id": 0}

    def send(obj):
        proc.stdin.write(json.dumps(obj) + "\n")
        proc.stdin.flush()

    def request(method, params, timeout):
        counter["id"] += 1
        rid = counter["id"]
        send({"jsonrpc": "2.0", "id": rid, "method": method, "params": params})
        while True:
            try:
                resp = json.loads(out_q.get(timeout=timeout))
            except queue.Empty:
                raise StepTimeout(f"no response in {timeout:g}s") from None
            if resp.get("id") == rid:
                return resp

    def finish(failures, total, killed=False):
        if killed:
            proc.kill()
        else:
            proc.stdin.close()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
        time.sleep(0.1)
        err = "".join(err_lines)
        if err.strip():
            print("\n--- stderr ---\n" + err[:2000])
        print(f"\n{total - failures}/{total} steps as expected")
        return 1 if failures else 0

    try:
        init = request(
            "initialize",
            {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "release-verify", "version": "0.0.1"},
            },
            opts["timeout"],
        )
    except StepTimeout as e:
        print(f"FAIL: initialize timed out: {e}")
        return finish(1, max(len(steps), 1), killed=True)
    print("server:", init["result"]["serverInfo"])
    send({"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}})
    time.sleep(0.2)

    failures = 0
    total = len(steps)

    if opts["list_tools"]:
        total += 1
        print("\n=== tools/list ===")
        try:
            resp = request("tools/list", {}, opts["timeout"])
        except StepTimeout as e:
            print(f"  FAIL: tools/list timed out: {e}")
            return finish(total, total, killed=True)
        summary = summarize_tools(resp["result"]["tools"])
        print(json.dumps(summary, indent=2))
        problems = []
        if summary["untitled"]:
            problems.append("untitled tools: " + ", ".join(summary["untitled"]))
        if summary["unhinted"]:
            problems.append("no destructiveHint: " + ", ".join(summary["unhinted"]))
        if opts["stats"]:
            with open(opts["stats"]) as f:
                stats = json.load(f)
            for key in STATS_KEYS:
                if stats.get(key) != summary[key]:
                    problems.append(
                        f"{key}: tools/list says {summary[key]}, {opts['stats']} says {stats.get(key)}"
                    )
        for p in problems:
            print("  FAIL:", p)
        failures += 1 if problems else 0

    for i, step in enumerate(steps):
        expect_error, name, args, seconds = split_step(step)
        print(f"\n=== {name} {args}{' [expect error]' if expect_error else ''} ===")
        try:
            resp = request("tools/call", {"name": name, "arguments": args}, seconds or opts["timeout"])
        except StepTimeout as e:
            failures += 1 + (len(steps) - i - 1)
            print(f"  FAIL: {name} timed out: {e}")
            for later in steps[i + 1:]:
                print(f"  not run: {later.lstrip('!').partition('|')[0]}")
            return finish(failures, total, killed=True)
        result = resp.get("result", {})
        for item in result.get("content", []):
            print(item.get("text", item))
        if "error" in resp:
            print("  FAIL: protocol error", resp["error"])
            failures += 1
            continue
        # An error result is the assertion for '!' steps — read the message and
        # confirm it tells a model what to do next, not just that it failed.
        got_error = bool(result.get("isError"))
        if got_error != expect_error:
            print("  FAIL: expected an error" if expect_error else "  FAIL: unexpected error")
            failures += 1
        elif got_error:
            print("  [expected error]")

    return finish(failures, total)


if __name__ == "__main__":
    sys.exit(main(sys.argv))
