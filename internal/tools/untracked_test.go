package tools

import (
	"strings"
	"testing"

	"github.com/iksnerd/adb-mcp/internal/adb"
)

// The untracked table must name real tools and give a reason, or it rots into a
// list of tools that no longer exist.
func TestUntrackedToolsNameRealToolsWithAReason(t *testing.T) {
	real := map[string]bool{}
	for _, tool := range listedTools(t) {
		real[tool.Name] = true
	}
	table := adb.UntrackedTools()
	if len(table) == 0 {
		t.Fatal("untracked table is empty")
	}
	for name, why := range table {
		if !real[name] {
			t.Errorf("%q is not a registered tool", name)
		}
		if strings.TrimSpace(why) == "" {
			t.Errorf("%q has no reason", name)
		}
	}
}
