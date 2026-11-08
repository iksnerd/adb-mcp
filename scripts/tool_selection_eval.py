#!/usr/bin/env python3
"""Does the model still pick the right tool? Run a set of prompts against a server build.

    tool_selection_eval.py --server bin/adb-mcp --label current --out current.json
    tool_selection_eval.py --compare current.json trimmed.json

For each case in scripts/tool-selection-cases.json, starts `claude -p` with only this MCP server
and records the order of the `mcp__*` tools the model tried. Nothing runs: with no --allowedTools,
the client denies every call, and the model's attempt is the measurement. A case passes when the
first tool tried is one the case expects (`first`), or an expected tool is among the first three
(`hit`). Run it on two builds (before and after a description change) and compare.

Costs real model calls (about a cent per run on a small model). A small model is the stricter test:
it leans on the tool text more than a large one. Not part of the pre-commit hook.
Standard library only; needs the `claude` CLI signed in.
"""

import argparse
import concurrent.futures
import json
import os
import shutil
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_CASES = os.path.join(HERE, "tool-selection-cases.json")

# A bare "Swipe up to scroll the list." can read to a small model as no request at all, so every
# prompt is framed as a task for the tools. Not a hint about which tool to use.
FRAME = "Use the Android device tools you have to do this task, then stop. Task: "

# Calls that look around before acting. Trying one first is sensible, not a wrong pick, so the score
# skips them unless the case itself expects one.
PREAMBLE = {"list_devices", "session_show_defaults", "list_avds"}


def run_case(claude: str, server: str, prompt: str, model: str, budget: float) -> list[str]:
    """Return the mcp tools the model tried, in order, without the server prefix."""
    with tempfile.TemporaryDirectory() as tmp:
        cfg = os.path.join(tmp, "mcp.json")
        with open(cfg, "w", encoding="utf-8") as fh:
            json.dump({"mcpServers": {"s": {"command": os.path.abspath(server)}}}, fh)
        proc = subprocess.run(
            [claude, "-p", prompt, "--model", model, "--mcp-config", cfg, "--strict-mcp-config",
             "--setting-sources", "project", "--disable-slash-commands", "--no-session-persistence",
             "--output-format", "stream-json", "--verbose", "--max-budget-usd", str(budget)],
            cwd=tmp, stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=240,
        )
    tools = []
    for line in proc.stdout.splitlines():
        try:
            msg = json.loads(line)
        except ValueError:
            continue
        if msg.get("type") != "assistant":
            continue
        for block in msg["message"].get("content", []):
            name = block.get("name", "") if block.get("type") == "tool_use" else ""
            if name.startswith("mcp__"):
                tools.append(name.split("__", 2)[2])
    return tools


def score(case: dict, runs: list[list[str]]) -> dict:
    expect = set(case["expect"])
    runs = [[t for t in r if t not in PREAMBLE or t in expect] for r in runs]
    first = sum(1 for r in runs if r and r[0] in expect)
    hit = sum(1 for r in runs if expect & set(r[:3]))
    return {"prompt": case["prompt"], "expect": case["expect"], "runs": runs,
            "first": first, "hit": hit, "n": len(runs)}


def evaluate(args) -> int:
    claude = shutil.which("claude")
    if not claude:
        print("tool_selection_eval: the claude CLI is not on PATH", file=sys.stderr)
        return 2
    if not os.path.exists(args.server):
        print(f"tool_selection_eval: no server at {args.server}", file=sys.stderr)
        return 2
    with open(args.cases, encoding="utf-8") as fh:
        cases = json.load(fh)
    jobs = [(i, c) for i, c in enumerate(cases) for _ in range(args.reps)]
    runs: dict[int, list[list[str]]] = {i: [] for i in range(len(cases))}
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as pool:
        futs = {pool.submit(run_case, claude, args.server, FRAME + c["prompt"], args.model, args.budget): i
                for i, c in jobs}
        for fut in concurrent.futures.as_completed(futs):
            try:
                runs[futs[fut]].append(fut.result())
            except (subprocess.TimeoutExpired, OSError) as err:
                print(f"tool_selection_eval: a run failed: {err}", file=sys.stderr)
                runs[futs[fut]].append([])
    results = [score(c, runs[i]) for i, c in enumerate(cases)]
    total = sum(r["n"] for r in results)
    out = {"label": args.label, "model": args.model, "reps": args.reps,
           "first": sum(r["first"] for r in results), "hit": sum(r["hit"] for r in results),
           "total": total, "cases": results}
    print(f"{args.label}: first tool right {out['first']}/{total}, expected tool in first three {out['hit']}/{total}")
    for r in results:
        if r["first"] < r["n"]:
            tried = " | ".join(",".join(x) or "-" for x in r["runs"])
            print(f"  {r['first']}/{r['n']}  want {'/'.join(r['expect'])}  tried {tried}  <- {r['prompt'][:60]}")
    if args.out:
        with open(args.out, "w", encoding="utf-8") as fh:
            json.dump(out, fh, indent=2)
        print(f"wrote {args.out}")
    return 0


def compare(a_path: str, b_path: str) -> int:
    a, b = (json.load(open(p, encoding="utf-8")) for p in (a_path, b_path))
    print(f"{a['label']}: first {a['first']}/{a['total']}, hit {a['hit']}/{a['total']}")
    print(f"{b['label']}: first {b['first']}/{b['total']}, hit {b['hit']}/{b['total']}")
    for ca, cb in zip(a["cases"], b["cases"]):
        if ca["prompt"] == cb["prompt"] and (ca["first"], ca["hit"]) != (cb["first"], cb["hit"]):
            print(f"  changed: first {ca['first']}->{cb['first']}, hit {ca['hit']}->{cb['hit']}  {ca['prompt'][:60]}")
    return 0 if b["first"] >= a["first"] and b["hit"] >= a["hit"] else 1


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--server", help="path to the server binary")
    ap.add_argument("--label", default="run")
    ap.add_argument("--out", help="write the results as JSON")
    ap.add_argument("--cases", default=DEFAULT_CASES)
    ap.add_argument("--model", default="haiku")
    ap.add_argument("--reps", type=int, default=2, help="runs per case; models are not deterministic")
    ap.add_argument("--jobs", type=int, default=4)
    ap.add_argument("--budget", type=float, default=0.5, help="max dollars per run")
    ap.add_argument("--compare", nargs=2, metavar=("BEFORE", "AFTER"),
                    help="compare two result files; exit 1 if AFTER is worse")
    args = ap.parse_args(argv)
    if args.compare:
        return compare(*args.compare)
    if not args.server:
        ap.error("--server is required")
    return evaluate(args)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
