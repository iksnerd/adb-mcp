package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAdb installs a stand-in adb binary under a temp ANDROID_HOME so handlers
// run end to end without a device. body is a POSIX sh fragment that sees the
// adb arguments as "$@" and may exit early; every invocation is first appended
// to the returned log (one line per call). The fake always lists one ready
// device for `adb devices`, and otherwise runs body, then succeeds silently.
func fakeAdb(t *testing.T, body string) (calls func() []string) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "platform-tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(home, "calls.log")
	script := "#!/bin/sh\necho \"$*\" >> '" + log + "'\n" +
		"case \"$*\" in *devices*) printf 'List of devices attached\\nemulator-5554\\tdevice\\n'; exit 0;; esac\n" +
		body + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "adb"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_HOME", home)
	t.Setenv("ANDROID_SDK_ROOT", "")
	return func() []string {
		b, err := os.ReadFile(log)
		if err != nil || strings.TrimSpace(string(b)) == "" {
			return nil
		}
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}
}
