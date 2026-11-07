package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resetSessionDefaults clears package-level session state before/after a test
// so tests don't leak defaults into each other (tests in this package run in
// one process, same as the real server within one stdio connection).
func resetSessionDefaults(t *testing.T) {
	t.Helper()
	clear := func() {
		sessionDefaults.mu.Lock()
		sessionDefaults.projectDir = ""
		sessionDefaults.serial = ""
		sessionDefaults.mu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

// gradleProject makes a temp dir that looks like a Gradle project root (it has
// the wrapper) and returns its absolute path.
func gradleProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "gradlew"
	if runtime.GOOS == "windows" {
		name = "gradlew.bat"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResolveProjectDirExplicitWins(t *testing.T) {
	resetSessionDefaults(t)
	def := gradleProject(t)
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: def}); err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	got, err := resolveProjectDir("/explicit/proj")
	if err != nil {
		t.Fatalf("resolveProjectDir: %v", err)
	}
	if got != "/explicit/proj" {
		t.Errorf("resolveProjectDir = %q, want explicit value to win over the session default", got)
	}
}

func TestResolveProjectDirFallsBackToSessionDefault(t *testing.T) {
	resetSessionDefaults(t)
	def := gradleProject(t)
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: def}); err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	got, err := resolveProjectDir("")
	if err != nil {
		t.Fatalf("resolveProjectDir: %v", err)
	}
	if got != def {
		t.Errorf("resolveProjectDir = %q, want the session default", got)
	}
}

func TestResolveProjectDirErrorsWithNoDefault(t *testing.T) {
	resetSessionDefaults(t)
	if _, err := resolveProjectDir(""); err == nil {
		t.Error("resolveProjectDir(\"\") with no session default set: expected an error, got nil")
	}
}

func TestSessionSetDefaultsPartialUpdatePreservesOtherField(t *testing.T) {
	resetSessionDefaults(t)
	fakeAdb(t, "")
	proj, proj2 := gradleProject(t), gradleProject(t)
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: proj, Serial: "emulator-5554"}); err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	// Setting only project_dir must leave the previously-set serial untouched.
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: proj2}); err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	dir, serial := getSessionDefaults()
	if dir != proj2 {
		t.Errorf("projectDir = %q, want %q", dir, proj2)
	}
	if serial != "emulator-5554" {
		t.Errorf("serial = %q, want emulator-5554 to survive the partial update", serial)
	}
}

func TestSessionClearDefaults(t *testing.T) {
	resetSessionDefaults(t)
	fakeAdb(t, "")
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: gradleProject(t), Serial: "emulator-5554"}); err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	if _, err := sessionClearDefaults(context.Background(), sessionClearDefaultsArgs{}); err != nil {
		t.Fatalf("sessionClearDefaults: %v", err)
	}
	dir, serial := getSessionDefaults()
	if dir != "" || serial != "" {
		t.Errorf("after clear: projectDir=%q serial=%q, want both empty", dir, serial)
	}
}

func TestSessionShowDefaultsReportsUnsetState(t *testing.T) {
	resetSessionDefaults(t)
	res, err := sessionShowDefaults(context.Background(), sessionShowDefaultsArgs{})
	if err != nil {
		t.Fatalf("sessionShowDefaults: %v", err)
	}
	got := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(got, "No session defaults set") {
		t.Errorf("sessionShowDefaults with nothing set = %q, want the unset message", got)
	}
}

func TestFormatSessionDefaults(t *testing.T) {
	if got := formatSessionDefaults("", ""); !strings.Contains(got, "No session defaults set") {
		t.Errorf("formatSessionDefaults(\"\",\"\") = %q, want the unset message", got)
	}
	got := formatSessionDefaults("/proj", "emulator-5554")
	if !strings.Contains(got, "/proj") || !strings.Contains(got, "emulator-5554") {
		t.Errorf("formatSessionDefaults = %q, want both values present", got)
	}
}

func TestSessionSetDefaultsRejectsBadProjectDir(t *testing.T) {
	notGradle := t.TempDir()
	for name, dir := range map[string]string{
		"relative":       "some/relative/dir",
		"dot":            ".",
		"missing":        filepath.Join(t.TempDir(), "nope"),
		"no gradlew":     notGradle,
		"tilde unexpand": "~/proj",
	} {
		t.Run(name, func(t *testing.T) {
			resetSessionDefaults(t)
			fakeAdb(t, "")
			if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: dir, Serial: "emulator-5554"}); err == nil {
				t.Fatalf("project_dir %q: expected an error", dir)
			}
			// Nothing is stored on failure, the serial included.
			if d, s := getSessionDefaults(); d != "" || s != "" {
				t.Errorf("a rejected call stored dir=%q serial=%q", d, s)
			}
		})
	}
}

func TestSessionSetDefaultsUnattachedSerialIsStoredWithNote(t *testing.T) {
	resetSessionDefaults(t)
	fakeAdb(t, "")
	res, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{Serial: "typo-123"})
	if err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	if _, s := getSessionDefaults(); s != "typo-123" {
		t.Errorf("serial = %q, want it stored even though not attached", s)
	}
	got := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(got, "not currently attached") {
		t.Errorf("result %q does not say the serial is not attached", got)
	}
}

func TestSessionSetDefaultsAttachedSerialHasNoNote(t *testing.T) {
	resetSessionDefaults(t)
	fakeAdb(t, "")
	res, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{Serial: "emulator-5554"})
	if err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	if got := res.Content[0].(*mcp.TextContent).Text; strings.Contains(got, "not currently attached") {
		t.Errorf("attached serial got a warning: %q", got)
	}
}

func TestSessionSetDefaultsAdbErrorDoesNotFail(t *testing.T) {
	resetSessionDefaults(t)
	home := t.TempDir() // no platform-tools/adb here, and PATH has none we rely on
	t.Setenv("ANDROID_HOME", home)
	t.Setenv("PATH", home)
	res, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{Serial: "emulator-5554"})
	if err != nil {
		t.Fatalf("an adb failure must not fail the call: %v", err)
	}
	if _, s := getSessionDefaults(); s != "emulator-5554" {
		t.Errorf("serial = %q, want stored", s)
	}
	if got := res.Content[0].(*mcp.TextContent).Text; strings.Contains(got, "not currently attached") {
		t.Errorf("no device list was obtained, so no claim about attachment: %q", got)
	}
}
