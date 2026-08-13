package gradle

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// sampleTasksOutput mimics the relevant slice of `gradlew tasks` for a
// two-flavor (free/paid) project: aggregate assemble, real variants, and the
// test-only assemble tasks that must be excluded.
const sampleTasksOutput = `
Build tasks
-----------
assemble - Assemble main outputs for all the variants.
assembleAndroidTest - Assembles all the Test applications.
assembleFreeDebug - Assembles main output for variant freeDebug
assembleFreeRelease - Assembles main output for variant freeRelease
assemblePaidDebug - Assembles main output for variant paidDebug
assemblePaidDebugAndroidTest - Assembles the android (on-device) tests for the paidDebug build.
assembleFreeDebugUnitTest - Assembles the tests for freeDebug.
bundleFreeDebug - Assembles bundle for variant freeDebug

Install tasks
-------------
installFreeDebug - Installs the DebugFree build.
`

func TestParseVariants(t *testing.T) {
	got := ParseVariants(sampleTasksOutput)
	want := []string{"freeDebug", "freeRelease", "paidDebug"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseVariants =\n  %q\nwant\n  %q", got, want)
	}
}

// TestParseVariantsSingle covers the common no-flavor project (just debug/release)
// and confirms the bare `assemble` aggregate is never emitted as a variant.
func TestParseVariantsSingle(t *testing.T) {
	out := "assemble - Assemble main outputs.\nassembleDebug - x\nassembleRelease - y\n"
	got := ParseVariants(out)
	want := []string{"debug", "release"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseVariants = %q, want %q", got, want)
	}
}

func TestParseVariantsNone(t *testing.T) {
	if got := ParseVariants("no assemble tasks here\ntest - run tests\n"); len(got) != 0 {
		t.Errorf("expected no variants, got %q", got)
	}
}

// writeFakeGradlew installs an executable "gradlew" in dir that echoes the
// task it was invoked with (as an assemble task, so ParseVariants has
// something to find) — enough to prove which task ListVariants actually ran,
// without needing a real Gradle project.
func writeFakeGradlew(t *testing.T, dir string) {
	t.Helper()
	script := "#!/bin/sh\necho \"ran: $1\"\necho \"assemble${1}Debug - fake\"\n"
	path := filepath.Join(dir, "gradlew")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestListVariantsDefaultsToRootTasks covers the field report that
// list_gradle_variants had no way to scope to a submodule: with no task
// given it must run the root project's own "tasks", not a hardcoded module.
func TestListVariantsDefaultsToRootTasks(t *testing.T) {
	dir := t.TempDir()
	writeFakeGradlew(t, dir)
	_, out, err := ListVariants(context.Background(), dir, "")
	if err != nil {
		t.Fatalf("ListVariants: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "ran: tasks") {
		t.Errorf("expected the default task to be \"tasks\", got output %q", out)
	}
}

// TestListVariantsAcceptsModuleQualifiedTask proves a module-qualified task
// (e.g. ":app:tasks", the workaround the field report asked for) actually
// reaches gradlew, rather than being silently ignored.
func TestListVariantsAcceptsModuleQualifiedTask(t *testing.T) {
	dir := t.TempDir()
	writeFakeGradlew(t, dir)
	_, out, err := ListVariants(context.Background(), dir, ":app:tasks")
	if err != nil {
		t.Fatalf("ListVariants: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "ran: :app:tasks") {
		t.Errorf("expected the module-qualified task to reach gradlew, got output %q", out)
	}
}
