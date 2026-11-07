package guides

import (
	"os"
	"testing"
)

// uri, name, .md file and embedded body are wired by hand in guides.go; a
// copy-pasted `body: driving` under another name would ship the wrong guide.
func TestGuideWiringAgrees(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range all {
		if g.uri != "android://guide/"+g.name {
			t.Errorf("guide %q has uri %q, want android://guide/%s", g.name, g.uri, g.name)
		}
		if seen[g.name] {
			t.Errorf("guide name %q is registered twice", g.name)
		}
		seen[g.name] = true
		want, err := os.ReadFile(g.name + ".md")
		if err != nil {
			t.Errorf("guide %q has no %s.md: %v", g.name, g.name, err)
			continue
		}
		if g.body != string(want) {
			t.Errorf("guide %q body is not the embedded %s.md", g.name, g.name)
		}
	}
}
