package main

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleTable = `var hints = map[string]hint{
	// --- group ---
	"list_avds":         observe(),
	"tap":               change(),
	"rotate_screen":     change().idempotent(),
	"open_url":          change().network(),
	"uninstall_app":     destroy().idempotent(),
	"gradle_build":      destroy().network(), // task= is arbitrary
	"list_gradle_tasks": observe().network(),
}
`

func TestCountHints(t *testing.T) {
	got := countHints(sampleTable)
	want := hintCounts{readOnly: 2, additive: 3, destructive: 2, openWorld: 3}
	if got != want {
		t.Errorf("countHints = %+v, want %+v", got, want)
	}
}

func TestRenderStatsIsExactAndEndsWithANewline(t *testing.T) {
	got := renderStats(counts{tools: 80, guides: 6, version: "0.24.3"},
		hintCounts{readOnly: 22, additive: 46, destructive: 12, openWorld: 12})
	want := `{
  "version": "0.24.3",
  "tools": 80,
  "guides": 6,
  "read_only": 22,
  "additive": 46,
  "destructive": 12,
  "open_world": 12
}
`
	if got != want {
		t.Errorf("renderStats =\n%s\nwant\n%s", got, want)
	}
}

// The scan is a regex over Go source. If the table's shape changes, a silent
// undercount would publish a wrong number to the website, so the real table has
// to account for every registered tool.
func TestCountHintsAccountsForEveryRegisteredTool(t *testing.T) {
	root := filepath.Join("..", "..")
	register, err := os.ReadFile(filepath.Join(root, "internal/tools/register.go"))
	if err != nil {
		t.Fatal(err)
	}
	table, err := os.ReadFile(filepath.Join(root, "internal/tools/annotations.go"))
	if err != nil {
		t.Fatal(err)
	}
	tools := len(reRegisteredTool.FindAllSubmatch(register, -1))
	h := countHints(string(table))
	if got := h.readOnly + h.additive + h.destructive; got != tools {
		t.Errorf("hint rows = %d (%+v), registered tools = %d", got, h, tools)
	}
	if h.destructive == 0 || h.readOnly == 0 {
		t.Errorf("a class counted zero: %+v", h)
	}
}
