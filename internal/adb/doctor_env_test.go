package adb

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRejectedEnvNoteNamesTheBadVariable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("default SDK path differs on Windows")
	}
	home := t.TempDir()
	def := filepath.Join(home, "Library", "Android", "sdk")
	if runtime.GOOS == "linux" {
		def = filepath.Join(home, "Android", "Sdk")
	}
	if err := os.MkdirAll(filepath.Join(def, "platform-tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("ANDROID_SDK_ROOT", "")
	bad := filepath.Join(t.TempDir(), "typo")
	t.Setenv("ANDROID_HOME", bad)

	want := "ANDROID_HOME=" + bad + " is not an SDK; using " + def
	if got := rejectedEnvNote(); !strings.Contains(got, want) {
		t.Errorf("note = %q, want it to contain %q", got, want)
	}

	t.Setenv("ANDROID_HOME", def)
	if got := rejectedEnvNote(); got != "" {
		t.Errorf("valid variable should produce no note, got %q", got)
	}
}
