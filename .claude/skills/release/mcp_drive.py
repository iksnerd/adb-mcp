#!/usr/bin/env python3
"""Drive an adb-mcp binary over real stdio JSON-RPC.

Unit tests exercise the parsers; this exercises the tool as a client sees it —
schema, arg resolution, error results, and the text a model actually reads.

    python3 mcp_drive.py ./adb-mcp 'tool_name|{"json":"args"}' ...

Each step is NAME|JSON_ARGS. Steps run in order against one server process, so
session state (e.g. session_set_defaults) carries across them.

Prefix a step with '!' to assert it MUST fail — that is how you exercise error
paths (missing arg, not-found, ambiguous input) without the run reporting them
as failures:

    python3 mcp_drive.py ./adb-mcp \
        'get_file_coverage|{"file":"Foo.kt"}' \
        '!get_file_coverage|{"file":"NoSuchFile.kt"}'

Exits non-zero when any step's outcome differs from what was asserted, so it can
gate a release.
"""
import json
import queue
import subprocess
import sys
import threading
import time


def main(argv):
    if len(argv) < 2:
        sys.exit(__doc__)
    binary, steps = argv[1], argv[2:]

    proc = subprocess.Popen(
        [binary],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        bufsize=1,
    )
    out_q = queue.Queue()

    def reader():
        for line in proc.stdout:
            if line.strip():
                out_q.put(line.strip())

    threading.Thread(target=reader, daemon=True).start()

    counter = {"id": 0}

    def send(obj):
        proc.stdin.write(json.dumps(obj) + "\n")
        proc.stdin.flush()

    def request(method, params, timeout=900):
        counter["id"] += 1
        rid = counter["id"]
        send({"jsonrpc": "2.0", "id": rid, "method": method, "params": params})
        while True:
            # Gradle steps can run for minutes; the timeout is per-response.
            resp = json.loads(out_q.get(timeout=timeout))
            if resp.get("id") == rid:
                return resp

    init = request(
        "initialize",
        {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "release-verify", "version": "0.0.1"},
        },
    )
    print("server:", init["result"]["serverInfo"])
    send({"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}})
    time.sleep(0.2)

    failures = 0
    for step in steps:
        expect_error = step.startswith("!")
        name, _, args_json = step.lstrip("!").partition("|")
        args = json.loads(args_json or "{}")
        print(f"\n=== {name} {args}{' [expect error]' if expect_error else ''} ===")
        resp = request("tools/call", {"name": name, "arguments": args})
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

    proc.stdin.close()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
    err = proc.stderr.read()
    if err.strip():
        print("\n--- stderr ---\n" + err[:2000])
    print(f"\n{len(steps) - failures}/{len(steps)} steps as expected")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
