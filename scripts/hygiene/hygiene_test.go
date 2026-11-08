// Package hygiene holds a repo-wide guard: shipped text must not cite the
// private feedback channels these notes came from.
//
// Field reports reach this project through private rooms. The CHANGELOG, the
// BACKLOG, release notes and code comments say "a field report" and keep the
// lesson; the room name and message id stay in the room. A written rule did not
// stop them leaking into the public history once, so this test does.
package hygiene

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// markers are the shapes that identify a private channel: a message id such as
// #019f75a8, and the room names themselves.
var markers = regexp.MustCompile(
	`#01[0-9a-f]{6}\b|android-mcp-papercuts|android-emulator-mcp-feedback|adb-mcp-feedback|council-hub`)

// textExts are the files that ship as readable text. Images and SVGs are left
// out: a hex colour such as #01a2b3 looks like a message id.
var textExts = map[string]bool{
	".md": true, ".go": true, ".json": true, ".yml": true, ".yaml": true,
	".js": true, ".sh": true, ".py": true, ".kts": true, ".kt": true,
	".mmd": true, ".txt": true, ".toml": true, ".gitleaksignore": true,
}

// skipDirs hold build output, local-only notes or other people's code.
var skipDirs = map[string]bool{
	".git": true, ".idea": true, ".playwright-mcp": true, "node_modules": true,
	"bin": true, "build": true, ".gradle": true, "personal": true,
}

func TestShippedTextNamesNoPrivateChannel(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	self, _ := filepath.Rel(root, filepath.Join(root, "scripts", "hygiene", "hygiene_test.go"))
	scanned := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == self || !textExts[filepath.Ext(path)] && d.Name() != ".gitleaksignore" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for i, line := range strings.Split(string(data), "\n") {
			if m := markers.FindString(line); m != "" {
				t.Errorf("%s:%d names a private channel (%q); write \"a field report\" instead", rel, i+1, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A walk that finds nothing would pass for the wrong reason.
	if scanned < 50 {
		t.Fatalf("scanned only %d files from %s; the walk is not reaching the repo", scanned, root)
	}
}
