package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestApplyFailsWhenAPatternMatchesNothing(t *testing.T) {
	r := rewrite{path: "x", subs: []sub{{regexp.MustCompile(`\d+ tools`), "9 tools"}}}
	if got, err := r.apply("80 tools"); err != nil || got != "9 tools" {
		t.Fatalf("apply on a match = %q, %v", got, err)
	}
	if _, err := r.apply("the tool count was reworded"); err == nil {
		t.Fatal("a pattern that matches nothing must be an error, not a silent pass")
	}
	r.optional = true
	if _, err := r.apply("the tool count was reworded"); err != nil {
		t.Fatalf("an optional file may lack the text: %v", err)
	}
}

func TestEveryRealRewriteRuleMatchesItsFile(t *testing.T) {
	root := filepath.Join("..", "..")
	c := counts{tools: 80, guides: 6, areas: 10, version: "0.0.1"}
	for _, f := range rewrites(c) {
		body, err := os.ReadFile(filepath.Join(root, f.path))
		if err != nil {
			if os.IsNotExist(err) && f.optional {
				continue
			}
			t.Fatal(err)
		}
		if _, err := f.apply(string(body)); err != nil {
			t.Errorf("%s: %v", f.path, err)
		}
	}
}

func TestRegisteredToolPatternReadsDigitsAndLooseCountCatchesTheRest(t *testing.T) {
	src := "\tadd(s, \"tap2\",\n\tadd(s, \"swipe\",\n"
	if got := len(reRegisteredTool.FindAllString(src, -1)); got != 2 {
		t.Errorf("strict count = %d, want 2 (a digit in a name is allowed)", got)
	}
	odd := src + "\tadd(s, name,\n\tadd(s, \"Upper\",\n"
	if strict, loose := len(reRegisteredTool.FindAllString(odd, -1)), len(reAddCall.FindAllString(odd, -1)); loose <= strict {
		t.Errorf("loose %d must exceed strict %d when a name is not a plain literal", loose, strict)
	}
}

func TestDeriveRejectsAToolItCannotRead(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("internal/tools/register.go", "\tadd(s, \"tap\",\n\tadd(s, name,\n")
	write("internal/guides/guides.go", "\t\turi: \"android://guide/a\"\n")
	write("docs/TOOLS.md", "### A\n")
	write("VERSION", "1.0.0\n")
	if _, err := derive(root); err == nil || !strings.Contains(err.Error(), "add(s,") {
		t.Fatalf("derive = %v, want the add(s, mismatch error", err)
	}
}

func TestUndocumentedNeedsATableRow(t *testing.T) {
	register := []byte("\tadd(s, \"tap\",\n\tadd(s, \"swipe\",\n\tadd(s, \"drag\",\n")
	doc := []byte("Use `swipe` for scrolling.\n| Tool | Purpose |\n| `tap` / `drag` | x |\n")
	got := undocumentedIn(register, doc)
	if len(got) != 1 || got[0] != "swipe" {
		t.Errorf("undocumentedIn = %v, want [swipe]: prose does not document a tool", got)
	}
}

func TestSkillTreesIsEmptyWhenThePluginTreesMoved(t *testing.T) {
	root := t.TempDir()
	if got := skillTrees(root); len(got) != 0 {
		t.Errorf("skillTrees on an empty root = %v", got)
	}
	if err := os.MkdirAll(filepath.Join(root, pluginSkillDirs[0]), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := skillTrees(root); len(got) != 1 {
		t.Errorf("skillTrees = %v, want one", got)
	}
}

func TestCompareRegistryName(t *testing.T) {
	docker := "FROM x\nLABEL io.modelcontextprotocol.server.name=\"io.github.a/b\"\n"
	if err := compareRegistryName(docker, []byte(`{"name":"io.github.a/b"}`)); err != nil {
		t.Errorf("equal names: %v", err)
	}
	if err := compareRegistryName(docker, []byte(`{"name":"io.github.a/c"}`)); err == nil {
		t.Error("different names must fail")
	}
	if err := compareRegistryName("FROM x\n", []byte(`{"name":"io.github.a/b"}`)); err == nil {
		t.Error("a missing label must fail")
	}
}

func TestRealRegistryNamesAgree(t *testing.T) {
	if err := registryNameMismatch(filepath.Join("..", "..")); err != nil {
		t.Error(err)
	}
}
