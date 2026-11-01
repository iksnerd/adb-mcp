#!/usr/bin/env python3
"""Tests for mcp_drive.py. Run: python3 .claude/skills/release/test_mcp_drive.py

They drive the real script against a small fake MCP server (a Python script
speaking JSON-RPC over stdio), so the timeout and tools/list paths run for real.
"""
import json
import os
import stat
import subprocess
import sys
import tempfile
import time
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
DRIVER = os.path.join(HERE, "mcp_drive.py")
sys.path.insert(0, HERE)
import mcp_drive  # noqa: E402

FAKE_SERVER = r'''#!/usr/bin/env python3
import json, sys, time
TOOLS = json.loads(open(sys.argv[0] + ".tools").read())
for line in sys.stdin:
    msg = json.loads(line)
    method, rid = msg.get("method"), msg.get("id")
    if rid is None:
        continue
    if method == "initialize":
        result = {"serverInfo": {"name": "fake", "version": "0"}}
    elif method == "tools/list":
        result = {"tools": TOOLS}
    else:
        name = msg["params"]["name"]
        if name == "slow":
            time.sleep(60)
        elif name == "wait2":
            time.sleep(2)
        result = {"content": [{"type": "text", "text": "ok " + name}]}
    print(json.dumps({"jsonrpc": "2.0", "id": rid, "result": result}), flush=True)
'''

TOOLS = [
    {"name": "list_devices", "title": "List devices",
     "annotations": {"readOnlyHint": True, "openWorldHint": False}},
    {"name": "tap", "title": "Tap",
     "annotations": {"destructiveHint": False, "openWorldHint": False}},
    {"name": "uninstall_app", "title": "Uninstall app",
     "annotations": {"destructiveHint": True, "openWorldHint": False}},
    {"name": "build", "title": "Build",
     "annotations": {"destructiveHint": False, "openWorldHint": True}},
]
STATS = {"tools": 4, "read_only": 1, "additive": 2, "destructive": 1, "open_world": 1}


class Case(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.binary = os.path.join(self.dir, "fake-server")
        with open(self.binary, "w") as f:
            f.write(FAKE_SERVER)
        os.chmod(self.binary, os.stat(self.binary).st_mode | stat.S_IXUSR)
        self.write_tools(TOOLS)

    def write_tools(self, tools):
        with open(self.binary + ".tools", "w") as f:
            json.dump(tools, f)

    def write_stats(self, stats):
        path = os.path.join(self.dir, "stats.json")
        with open(path, "w") as f:
            json.dump(stats, f)
        return path

    def run_driver(self, *args):
        started = time.time()
        p = subprocess.run(
            [sys.executable, DRIVER, *args],
            capture_output=True, text=True, timeout=30,
        )
        return p.returncode, p.stdout + p.stderr, time.time() - started


class SummarizeTools(unittest.TestCase):
    def test_counts_omit_false_read_only_hint(self):
        s = mcp_drive.summarize_tools(TOOLS)
        self.assertEqual(
            {k: s[k] for k in STATS},
            STATS,
        )
        self.assertEqual(s["untitled"], [])
        self.assertEqual(s["unhinted"], [])

    def test_untitled_and_unhinted_are_named(self):
        s = mcp_drive.summarize_tools([{"name": "x", "annotations": {}}])
        self.assertEqual(s["untitled"], ["x"])
        self.assertEqual(s["unhinted"], ["x"])


class Timeout(Case):
    def test_a_stuck_step_is_named_and_the_run_still_ends(self):
        rc, out, took = self.run_driver("--timeout", "1", self.binary, "slow|{}")
        self.assertEqual(rc, 1)
        self.assertIn("timed out", out)
        self.assertIn("slow", out)
        self.assertIn("0/1 steps as expected", out)
        self.assertLess(took, 20)

    def test_a_step_can_raise_its_own_timeout(self):
        # wait2 takes 2s: it only passes if its own 10s beats the 1s default.
        rc, out, _ = self.run_driver("--timeout", "1", self.binary, "wait2|{}|10")
        self.assertEqual(rc, 0, out)
        self.assertIn("ok wait2", out)

    def test_without_its_own_timeout_the_same_step_times_out(self):
        rc, out, _ = self.run_driver("--timeout", "1", self.binary, "wait2|{}")
        self.assertEqual(rc, 1)
        self.assertIn("timed out", out)

    def test_later_steps_are_reported_not_run(self):
        rc, out, _ = self.run_driver(
            "--timeout", "1", self.binary, "slow|{}", "quick|{}")
        self.assertEqual(rc, 1)
        self.assertIn("not run", out)
        self.assertIn("0/2 steps as expected", out)


class ListTools(Case):
    def test_matching_stats_pass(self):
        stats = self.write_stats(STATS)
        rc, out, _ = self.run_driver("--list-tools", "--stats", stats, self.binary)
        self.assertEqual(rc, 0, out)
        self.assertIn('"tools": 4', out)

    def test_a_count_that_differs_from_stats_fails_and_names_it(self):
        stats = self.write_stats({**STATS, "destructive": 12})
        rc, out, _ = self.run_driver("--list-tools", "--stats", stats, self.binary)
        self.assertEqual(rc, 1)
        self.assertIn("destructive", out)
        self.assertIn("12", out)

    def test_an_untitled_tool_fails(self):
        self.write_tools(TOOLS + [{"name": "bare", "annotations": {"destructiveHint": False}}])
        rc, out, _ = self.run_driver("--list-tools", self.binary)
        self.assertEqual(rc, 1)
        self.assertIn("bare", out)


if __name__ == "__main__":
    unittest.main()
