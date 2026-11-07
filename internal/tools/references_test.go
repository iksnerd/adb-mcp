package tools

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Prose that tells an agent to call a tool must name a real tool and real
// parameters, or the agent follows advice the schema rejects. Checked for the
// initialize instructions and the undo hints in adb/overrides.go.

func schemaProps(t *testing.T) map[string][]string {
	t.Helper()
	m := map[string][]string{}
	for _, tool := range listedTools(t) {
		m[tool.Name] = toolProps(tool)
	}
	return m
}

func TestInstructionsNameRealToolsAndParams(t *testing.T) {
	props := schemaProps(t)
	// "tool_name {key: value, key2}" call shapes.
	call := regexp.MustCompile(`\b([a-z]+(?:_[a-z]+)*) \{([^}]*)\}`)
	found := 0
	for _, m := range call.FindAllStringSubmatch(Instructions, -1) {
		found++
		p, ok := props[m[1]]
		if !ok {
			t.Errorf("instructions call %q, which is not a registered tool", m[1])
			continue
		}
		for _, kv := range strings.Split(m[2], ",") {
			key := strings.TrimSpace(strings.SplitN(kv, ":", 2)[0])
			if !slices.Contains(p, key) {
				t.Errorf("instructions pass %s.%s, which is not in its schema %v", m[1], key, p)
			}
		}
	}
	if found == 0 {
		t.Error("found no tool call shapes in the instructions; the pattern needs updating")
	}
}

func TestUndoHintsNameRealToolsAndParams(t *testing.T) {
	src, err := os.ReadFile("../adb/overrides.go")
	if err != nil {
		t.Fatal(err)
	}
	props := schemaProps(t)
	block := regexp.MustCompile(`(?s)var undoHint = map\[OverrideKind\]string\{(.*?)\n\}`).FindStringSubmatch(string(src))
	if block == nil {
		t.Fatal("undoHint map not found in adb/overrides.go")
	}
	hint := regexp.MustCompile(`"([a-z_]+)(?: ([^"(]*))?`)
	found := 0
	for _, line := range strings.Split(block[1], "\n") {
		i := strings.Index(line, ": \"")
		if i < 0 {
			continue
		}
		m := hint.FindStringSubmatch(line[i+2:])
		if m == nil {
			continue
		}
		found++
		p, ok := props[m[1]]
		if !ok {
			t.Errorf("undo hint names %q, which is not a registered tool", m[1])
			continue
		}
		for _, tok := range strings.Fields(m[2]) {
			if k, _, isKV := strings.Cut(tok, "="); isKV && !slices.Contains(p, k) {
				t.Errorf("undo hint %s uses %q, which is not in its schema %v", m[1], k, p)
			}
		}
	}
	if found == 0 {
		t.Error("found no undo hints; the pattern needs updating")
	}
}
