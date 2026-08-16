package tools

import (
	"context"
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

func TestResolveProjectDirExplicitWins(t *testing.T) {
	resetSessionDefaults(t)
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: "/default/proj"}); err != nil {
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
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: "/default/proj"}); err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	got, err := resolveProjectDir("")
	if err != nil {
		t.Fatalf("resolveProjectDir: %v", err)
	}
	if got != "/default/proj" {
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
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: "/proj", Serial: "emulator-5554"}); err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	// Setting only project_dir must leave the previously-set serial untouched.
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: "/proj2"}); err != nil {
		t.Fatalf("sessionSetDefaults: %v", err)
	}
	dir, serial := getSessionDefaults()
	if dir != "/proj2" {
		t.Errorf("projectDir = %q, want /proj2", dir)
	}
	if serial != "emulator-5554" {
		t.Errorf("serial = %q, want emulator-5554 to survive the partial update", serial)
	}
}

func TestSessionClearDefaults(t *testing.T) {
	resetSessionDefaults(t)
	if _, err := sessionSetDefaults(context.Background(), sessionSetDefaultsArgs{ProjectDir: "/proj", Serial: "emulator-5554"}); err != nil {
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
