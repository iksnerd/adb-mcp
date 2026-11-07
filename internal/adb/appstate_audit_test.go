package adb

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

const auditPkg = "com.example"

// stateRunner answers GetAppState's probes from a table keyed on the adb
// subcommand; missing keys answer empty/nil.
type probeReply struct {
	out string
	err error
}

func stateRunner(replies map[string]probeReply) Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		key := args[1]
		if key == "dumpsys" && len(args) > 2 {
			key = "dumpsys " + args[2]
		}
		r := replies[key]
		return []byte(r.out), r.err
	}
}

func installedDump() string {
	return "Package [" + auditPkg + "]\n  firstInstallTime=2026-01-01\n  lastUpdateTime=2026-01-01\n"
}

func notesJoined(s AppState) string { return strings.Join(s.Notes, " | ") }

func TestGetAppStateDumpsysErrorIsNotNotInstalled(t *testing.T) {
	c := &Client{Serial: "e", run: stateRunner(map[string]probeReply{
		"dumpsys package": {err: errors.New("device offline")},
	})}
	s, err := c.GetAppState(context.Background(), auditPkg)
	if err != nil {
		t.Fatal(err)
	}
	n := notesJoined(s)
	if !strings.Contains(n, "install state unknown") || !strings.Contains(n, "device offline") {
		t.Errorf("notes = %q, want the probe error and an explicit unknown", n)
	}
	if strings.Contains(n, "package not installed") {
		t.Errorf("notes = %q must not assert not-installed on a probe error", n)
	}
	if s.BundleSource != "unknown" {
		t.Errorf("BundleSource = %q, want unknown", s.BundleSource)
	}
}

func TestGetAppStatePidofErrorIsNotNotRunning(t *testing.T) {
	c := &Client{Serial: "e", run: stateRunner(map[string]probeReply{
		"dumpsys package": {out: installedDump()},
		"pidof":           {err: errors.New("device offline")},
	})}
	s, err := c.GetAppState(context.Background(), auditPkg)
	if err != nil {
		t.Fatal(err)
	}
	n := notesJoined(s)
	if s.Running || s.BundleSource != "unknown" || !strings.Contains(n, "running state unknown") {
		t.Errorf("Running=%v BundleSource=%q notes=%q, want unknown running state", s.Running, s.BundleSource, n)
	}
	if strings.Contains(n, "not running") {
		t.Errorf("notes = %q must not assert not-running on a probe error", n)
	}
}

// pidof exits 1 with no output when nothing matches; that is a real "not
// running", unlike a transport error.
func TestGetAppStatePidofExit1IsNotRunning(t *testing.T) {
	exit1 := exec.Command("sh", "-c", "exit 1").Run()
	if exit1 == nil {
		t.Skip("cannot build an exit-status-1 error here")
	}
	c := &Client{Serial: "e", run: stateRunner(map[string]probeReply{
		"dumpsys package": {out: installedDump()},
		"pidof":           {err: exit1},
	})}
	s, err := c.GetAppState(context.Background(), auditPkg)
	if err != nil {
		t.Fatal(err)
	}
	if s.Running || s.BundleSource != "n/a" || !strings.Contains(notesJoined(s), "not running") {
		t.Errorf("Running=%v BundleSource=%q notes=%q, want a definite not-running", s.Running, s.BundleSource, notesJoined(s))
	}
}

func TestGetAppStateMultiplePIDsLeaveBundleUndetermined(t *testing.T) {
	c := &Client{Serial: "e", run: stateRunner(map[string]probeReply{
		"dumpsys package":  {out: installedDump()},
		"pidof":            {out: "111 222"},
		"dumpsys activity": {out: "topResumedActivity=ActivityRecord{a u0 " + auditPkg + "/.Main t1}"},
		"logcat":           {out: "1700000000.000 I HMRClient: connected\n"},
	})}
	s, err := c.GetAppStateWithSource(context.Background(), auditPkg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.BundleSource != "unknown" || s.StaleVerdict != "undetermined" {
		t.Errorf("BundleSource=%q StaleVerdict=%q, want unknown/undetermined with two pids", s.BundleSource, s.StaleVerdict)
	}
	if !strings.Contains(notesJoined(s), "2 live processes") {
		t.Errorf("notes = %q, want the multi-process note", notesJoined(s))
	}
}

func TestGetAppStateRejectsGarbageUptime(t *testing.T) {
	for in, want := range map[string]string{"01-02:03:04": "01-02:03:04", "05:06": "05:06", "ps: bad pid '1'": "", "Usage: ps": ""} {
		c := &Client{Serial: "e", run: stateRunner(map[string]probeReply{
			"dumpsys package": {out: installedDump()},
			"pidof":           {out: "111"},
			"ps":              {out: in},
		})}
		s, err := c.GetAppState(context.Background(), auditPkg)
		if err != nil {
			t.Fatal(err)
		}
		if s.ProcessUptime != want {
			t.Errorf("ps output %q -> ProcessUptime %q, want %q", in, s.ProcessUptime, want)
		}
	}
}

func TestIsPackageInstalledHonoursInstalledFalse(t *testing.T) {
	removed := "Package [com.example] (abc):\n  User 0: ceDataInode=0 installed=false hidden=false\n"
	if isPackageInstalled(removed) {
		t.Error("a record whose only user entry has installed=false is not installed")
	}
	present := "Package [com.example] (abc):\n  User 0: ceDataInode=5 installed=true hidden=false\n"
	if !isPackageInstalled(present) {
		t.Error("installed=true must stay installed")
	}
	if !isPackageInstalled("Package [com.example] (abc):\n  versionName=1\n") {
		t.Error("a record with no installed= field keeps the old behaviour")
	}
}

func TestRegisteredURLSchemesForms(t *testing.T) {
	dump := `Package [com.example] (abc):
      Action: "android.intent.action.VIEW"
      Scheme: "myapp"
      Scheme: 'quoted'
      scheme=lower
      Schemes: plain
      Scheme: "myapp"
      Scheme: ""
`
	c, _ := newFake(dump)
	got, err := c.RegisteredURLSchemes(context.Background(), "com.example")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"myapp", "quoted", "lower", "plain"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("schemes = %v, want %v", got, want)
	}
}
