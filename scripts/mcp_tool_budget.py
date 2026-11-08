#!/usr/bin/env python3
"""How big is an MCP server's tool list, and has it grown?

    mcp_tool_budget.py -- CMD [ARG...]                         print the size table
    mcp_tool_budget.py --baseline FILE -- CMD [ARG...]         also fail when it grew
    mcp_tool_budget.py --baseline FILE --update -- CMD ...     record today's size as the baseline

Starts CMD as a stdio MCP server, asks it for `tools/list` (following `nextCursor`), and measures
each tool entry as compact JSON: name, description, schemas and annotations, which is what a client
that loads every tool puts in the model's context on every turn. Tokens are an estimate (bytes / 4);
the bytes are exact.

The baseline is a small JSON file, {"total_bytes": N, "tools": {name: bytes}}. A check fails when
the total, or any one tool that is in the baseline, is bigger than the baseline says. A new tool
grows the total, so it fails too: growth is a choice, made by running --update and committing the
file. Removing or shrinking a tool never fails.

Exit status: 0 within budget, 1 over budget, 2 the server could not be measured.
Standard library only.
"""

import argparse
import json
import subprocess
import sys
import threading

PROTOCOL = "2025-06-18"
TIMEOUT = 60  # seconds for the whole measurement; a server that never answers is killed


class MeasureError(Exception):
    pass


def entry_bytes(tool: dict) -> int:
    return len(json.dumps(tool, separators=(",", ":"), ensure_ascii=False).encode("utf-8"))


def measure(tools: list[dict]) -> dict:
    sizes = {t["name"]: entry_bytes(t) for t in tools}
    desc = {t["name"]: len(t.get("description", "")) for t in tools}
    return {"total_bytes": sum(sizes.values()), "tools": sizes, "description_chars": desc}


def compare(baseline: dict, current: dict) -> list[str]:
    """Return the ways `current` is over `baseline`; an empty list is within budget."""
    problems = []
    if current["total_bytes"] > baseline["total_bytes"]:
        problems.append(
            f"total grew from {baseline['total_bytes']} to {current['total_bytes']} bytes "
            f"(+{current['total_bytes'] - baseline['total_bytes']})"
        )
    old = baseline["tools"]
    for name, size in sorted(current["tools"].items()):
        if name not in old:
            problems.append(f"new tool {name}: {size} bytes")
        elif size > old[name]:
            problems.append(f"{name} grew from {old[name]} to {size} bytes (+{size - old[name]})")
    return problems


def list_tools(cmd: list[str], timeout: float = TIMEOUT) -> list[dict]:
    try:
        proc = subprocess.Popen(
            cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True
        )
    except OSError as err:
        raise MeasureError(f"cannot start {cmd[0]}: {err}") from err
    # Killing the process ends the blocked read with end-of-file, which reply() reports.
    timer = threading.Timer(timeout, proc.kill)
    timer.start()
    try:
        def send(msg: dict) -> None:
            proc.stdin.write(json.dumps(msg) + "\n")
            proc.stdin.flush()

        def reply(want: int) -> dict:
            while True:
                line = proc.stdout.readline()
                if not line:
                    raise MeasureError(f"{cmd[0]} closed its output before answering request {want} (or it was killed after the timeout)")
                try:
                    msg = json.loads(line)
                except ValueError:
                    continue
                if msg.get("id") == want:
                    if "error" in msg:
                        raise MeasureError(f"request {want} failed: {msg['error']}")
                    return msg["result"]

        send({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
            "protocolVersion": PROTOCOL, "capabilities": {},
            "clientInfo": {"name": "mcp_tool_budget", "version": "1"}}})
        reply(1)
        send({"jsonrpc": "2.0", "method": "notifications/initialized"})
        tools, cursor, rid = [], None, 2
        while True:
            params = {"cursor": cursor} if cursor else {}
            send({"jsonrpc": "2.0", "id": rid, "method": "tools/list", "params": params})
            result = reply(rid)
            tools += result.get("tools", [])
            cursor = result.get("nextCursor")
            rid += 1
            if not cursor:
                break
        if not tools:
            raise MeasureError("the server listed no tools; refusing to record or pass an empty list")
        return tools
    finally:
        timer.cancel()
        proc.kill()
        proc.wait()


def table(current: dict, top: int) -> str:
    sizes = current["tools"]
    total = current["total_bytes"]
    ranked = sorted(sizes.items(), key=lambda kv: (-kv[1], kv[0]))
    ordered = sorted(sizes.values())
    lines = [
        f"{len(sizes)} tools, {total} bytes, about {total // 4} tokens (bytes / 4, an estimate)",
        f"median {ordered[len(ordered) // 2]} bytes, largest {ranked[0][1]}",
        f"largest {min(top, len(ranked))}:",
    ]
    for name, size in ranked[:top]:
        lines.append(f"  {size:6}  {name}  (description {current['description_chars'][name]} chars)")
    return "\n".join(lines)


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--baseline", help="baseline JSON file; the check fails when the list grew")
    ap.add_argument("--update", action="store_true", help="write the baseline instead of checking it")
    ap.add_argument("--timeout", type=float, default=TIMEOUT, help="seconds before a silent server is killed")
    ap.add_argument("--top", type=int, default=8, help="how many of the largest tools to print")
    ap.add_argument("cmd", nargs=argparse.REMAINDER, help="-- then the server command")
    args = ap.parse_args(argv)
    cmd = args.cmd[1:] if args.cmd[:1] == ["--"] else args.cmd
    if not cmd:
        ap.error("give the server command after --")
    if args.update and not args.baseline:
        ap.error("--update needs --baseline FILE")
    try:
        current = measure(list_tools(cmd, args.timeout))
    except MeasureError as err:
        print(f"mcp_tool_budget: {err}", file=sys.stderr)
        return 2
    print(table(current, args.top))
    if not args.baseline:
        return 0
    if args.update:
        with open(args.baseline, "w", encoding="utf-8") as fh:
            json.dump({"total_bytes": current["total_bytes"], "tools": dict(sorted(current["tools"].items()))},
                      fh, indent=2)
            fh.write("\n")
        print(f"wrote {args.baseline}")
        return 0
    try:
        with open(args.baseline, encoding="utf-8") as fh:
            baseline = json.load(fh)
    except (OSError, ValueError) as err:
        print(f"mcp_tool_budget: cannot read baseline {args.baseline}: {err}", file=sys.stderr)
        return 2
    problems = compare(baseline, current)
    if problems:
        print("\nover the tool-size budget:", file=sys.stderr)
        for p in problems:
            print(f"  {p}", file=sys.stderr)
        print(f"if the growth is on purpose, rerun with --update and commit {args.baseline}", file=sys.stderr)
        return 1
    print(f"within the budget in {args.baseline}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
