package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Device and project resolution is convention: handlers call resolve() and
// resolveProjectDir() so the session defaults and the single-device fallback
// apply everywhere. A handler that builds its own client, or reads the raw
// argument, silently skips them. This greps the non-test sources for that.

func toolSources(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(b)
	}
	return out
}

// stripComments drops // comments so prose mentioning adb.New( is not a hit.
func stripComments(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func TestHandlersResolveThroughHelpers(t *testing.T) {
	// File -> reason, for files that own the resolution.
	owners := map[string]string{
		"helpers.go": "defines resolve(), the one place a client is built",
		"session.go": "defines resolveProjectDir() and stores the pinned project_dir",
	}
	// Reading the raw ProjectDir is fine when it is the argument to the helper.
	passedToHelper := regexp.MustCompile(`resolveProjectDir\(in\.ProjectDir\)`)
	for name, src := range toolSources(t) {
		if _, ok := owners[name]; ok {
			continue
		}
		code := stripComments(src)
		for _, banned := range []string{"adb.New(", "adb.ResolveSerial("} {
			if strings.Contains(code, banned) {
				t.Errorf("%s calls %s directly; use resolve(ctx, in.Serial)", name, banned)
			}
		}
		if strings.Contains(passedToHelper.ReplaceAllString(code, ""), ".ProjectDir") {
			t.Errorf("%s reads ProjectDir outside resolveProjectDir(in.ProjectDir)", name)
		}
	}
}

// A handler whose input type has a serial but binds it to `_` ignores the
// caller's target device. Allowlisted cases need a reason.
func TestNoHandlerDiscardsItsSerialArgument(t *testing.T) {
	allowed := map[string]string{
		"listDevices": "lists every attached device, so a serial argument has nothing to select",
	}
	sources := toolSources(t)
	var all strings.Builder
	for _, s := range sources {
		all.WriteString(stripComments(s))
	}
	code := all.String()
	blank := regexp.MustCompile(`func (\w+)\([^)]*, _ (\w+)\)`)
	for _, m := range blank.FindAllStringSubmatch(code, -1) {
		fn, typ := m[1], m[2]
		hasSerial := typ == "serialArg"
		if !hasSerial {
			decl := regexp.MustCompile(`type ` + typ + ` struct \{([^}]*)\}`).FindStringSubmatch(code)
			hasSerial = decl != nil && strings.Contains(decl[1], "serialArg")
		}
		if hasSerial {
			if _, ok := allowed[fn]; !ok {
				t.Errorf("%s takes %s with a serial but discards it", fn, typ)
			}
		}
	}
}
