package main

import (
	"strings"
	"testing"
)

const frontMatter = "---\nname: driving\ndescription: Use whenever tapping.\n---\n"

func TestSyncSkillKeepsFrontMatterAndCarriesTheGuide(t *testing.T) {
	stub := frontMatter + "\nRead the `android://guide/driving` resource.\n"
	guide := "# Driving\n\nObserve, then act.\n"

	got, err := syncSkill(stub, guide)
	if err != nil {
		t.Fatal(err)
	}
	want := frontMatter + "\n# Driving\n\nObserve, then act.\n"
	if got != want {
		t.Errorf("syncSkill =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "Read the `android://guide") {
		t.Error("the stale pointer sentence survived")
	}
}

func TestSyncSkillIsIdempotent(t *testing.T) {
	guide := "# Driving\n\nObserve, then act.\n"
	once, err := syncSkill(frontMatter+"\nold\n", guide)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := syncSkill(once, guide)
	if err != nil {
		t.Fatal(err)
	}
	if once != twice {
		t.Errorf("second pass changed the file:\n%q\n%q", once, twice)
	}
}

func TestSyncSkillNormalisesTrailingNewlines(t *testing.T) {
	got, err := syncSkill(frontMatter, "# G\n\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "# G\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("want exactly one trailing newline, got %q", got)
	}
}

func TestSyncSkillRefusesBrokenFrontMatter(t *testing.T) {
	for name, skill := range map[string]string{
		"missing":  "# no front matter\n",
		"unclosed": "---\nname: driving\nstill going\n",
		"empty":    "",
	} {
		if _, err := syncSkill(skill, "# G\n"); err == nil {
			t.Errorf("%s front matter: want an error, got none", name)
		}
	}
}
