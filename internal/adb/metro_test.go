package adb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLsofListener(t *testing.T) {
	// `lsof -nP -iTCP:8081 -sTCP:LISTEN -Fpc` field output.
	out := "p40122\ncnode\n"
	pid, cmd, ok := parseLsofListener(out)
	if !ok || pid != 40122 || cmd != "node" {
		t.Fatalf("parseLsofListener = (%d, %q, %t), want (40122, \"node\", true)", pid, cmd, ok)
	}
}

func TestParseLsofListenerFirstOfSeveral(t *testing.T) {
	// A forked dev server, or a stale one alongside a live one: several records,
	// each starting with a p-line. The first is the one a new connection reaches.
	out := "p40122\ncnode\np40123\ncnode\n"
	pid, cmd, ok := parseLsofListener(out)
	if !ok || pid != 40122 || cmd != "node" {
		t.Fatalf("parseLsofListener = (%d, %q, %t), want the first record", pid, cmd, ok)
	}
}

func TestParseLsofListenerEmpty(t *testing.T) {
	if _, _, ok := parseLsofListener(""); ok {
		t.Error("nothing listening must report ok=false, not a zero pid that looks real")
	}
}

func TestParseLsofCWD(t *testing.T) {
	out := "p40122\nfcwd\nn/Users/dev/src/MyApp\n"
	root, ok := parseLsofCWD(out)
	if !ok || root != "/Users/dev/src/MyApp" {
		t.Fatalf("parseLsofCWD = (%q, %t), want the n-line path", root, ok)
	}
	if _, ok := parseLsofCWD("p40122\nfcwd\n"); ok {
		t.Error("a record with no n-line must report ok=false")
	}
}

func TestSameTreeContainmentNeedsProjectMarker(t *testing.T) {
	// A Metro whose cwd is a bare directory above the source (~ or /) must not
	// match every checkout beneath it.
	home := t.TempDir()
	proj := filepath.Join(home, "work", "app")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if SameTree(home, proj) || SameTree(proj, home) {
		t.Error("containment under a root with no package.json must not count as the same tree")
	}
	if err := os.WriteFile(filepath.Join(home, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !SameTree(home, proj) || !SameTree(proj, home) {
		t.Error("containment under a package.json root is the same checkout")
	}
	if !SameTree(home, home) {
		t.Error("equal paths are always the same tree")
	}
}

func TestSameTree(t *testing.T) {
	base := t.TempDir()
	// The outer root is a project (package.json), so containment counts here.
	if err := os.WriteFile(filepath.Join(base, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(base, "app", "src")
	cases := []struct {
		name, a, b string
		want       bool
	}{
		{"identical", base, base, true},
		{"parent contains child", base, nested, true},
		{"child inside parent", nested, base, true},
		{"different trees", filepath.Join(base, "one"), filepath.Join(base, "two"), false},
		// The trap this exists for: a sibling checkout whose path shares a prefix
		// with the real one. Prefix-matching without the separator would call
		// "/src/MyApp" and "/src/MyApp-old" the same tree.
		{"sibling with shared prefix", filepath.Join(base, "MyApp"), filepath.Join(base, "MyApp-old"), false},
		{"empty is never the same tree", "", base, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SameTree(tc.a, tc.b); got != tc.want {
				t.Errorf("SameTree(%q, %q) = %t, want %t", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
