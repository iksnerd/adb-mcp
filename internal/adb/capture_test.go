package adb

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Pins the registry contract the tools layer relies on: sessions are keyed by
// the concrete serial (tools.resolve always passes ResolveSerial's result), so
// a second start for the same device is refused and stop finds the session.
func TestSessionRegistryKeyedBySerial(t *testing.T) {
	r := &sessionRegistry[recordSession]{}
	mk := func() (*recordSession, error) { return &recordSession{}, nil }

	if err := r.start("emulator-5554", "screen recording", mk); err != nil {
		t.Fatal(err)
	}
	err := r.start("emulator-5554", "screen recording", mk)
	if err == nil || !strings.Contains(err.Error(), "already running for emulator-5554") {
		t.Fatalf("expected already-running error, got %v", err)
	}
	if _, ok, _ := r.take("emulator-5554"); !ok {
		t.Fatal("stop should find the session under the same serial")
	}
	if _, ok, _ := r.take("emulator-5554"); ok {
		t.Fatal("session should be gone after take")
	}
}

func TestSessionRegistryRejectsEmptyKey(t *testing.T) {
	r := &sessionRegistry[recordSession]{}
	mk := func() (*recordSession, error) { return &recordSession{}, nil }
	if err := r.start("", "screen recording", mk); err == nil {
		t.Fatal("start with an empty serial must fail")
	}
	if _, _, err := r.take(""); err == nil {
		t.Fatal("take with an empty serial must fail")
	}
}

func TestStopLogcatCaptureReportsEarlyExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	f, err := os.CreateTemp(t.TempDir(), "log-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", "echo partial; exit 3")
	cmd.Stdout = f
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := watchLogSession(cmd, f, f.Name())
	const serial = "dead-logcat"
	if err := logSessions.start(serial, "logcat capture", func() (*logSession, error) { return s, nil }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("child exit was never observed")
	}
	c := &Client{Serial: serial}
	out, err := c.StopLogcatCapture(LogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "capture ended early at") || !strings.Contains(out, "exit status 3") {
		t.Errorf("want an early-exit note naming the error, got %q", out)
	}
}

// recFake answers `stat` with a growing-then-stable size sequence and records
// every call.
type recFake struct {
	mu    sync.Mutex
	calls []string
	sizes []string
}

func (f *recFake) run(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, strings.Join(args, " "))
	if len(args) > 1 && args[1] == "stat" {
		s := f.sizes[0]
		if len(f.sizes) > 1 {
			f.sizes = f.sizes[1:]
		}
		return []byte(s + "\n"), nil
	}
	return nil, nil
}

func TestStopScreenRecordSignalsOnlyOurFileAndWaitsForStableSize(t *testing.T) {
	old := recordPollInterval
	recordPollInterval = time.Millisecond
	t.Cleanup(func() { recordPollInterval = old })

	f := &recFake{sizes: []string{"100", "5000", "9000", "9000", "9000"}}
	c := &Client{Serial: "rec-1", run: f.run}
	const dev = "/sdcard/aemcp-record-abc123.mp4"
	if err := recSessions.start(c.Serial, "screen recording", func() (*recordSession, error) {
		return &recordSession{devicePath: dev}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.StopScreenRecord(context.Background(), "/tmp/out.mp4"); err != nil {
		t.Fatal(err)
	}

	var sawKill, sawPull bool
	stats := 0
	for _, call := range f.calls {
		switch {
		case strings.Contains(call, "pkill"):
			sawKill = true
			if !strings.Contains(call, "abc123") {
				t.Errorf("pkill must target this recording's file, got %q", call)
			}
			if stats != 0 {
				t.Errorf("must signal before polling the size")
			}
		case strings.Contains(call, "stat"):
			stats++
		case strings.HasPrefix(call, "pull "+dev):
			sawPull = true
			if stats < 4 {
				t.Errorf("pulled after only %d size checks; the size had not been stable", stats)
			}
		}
	}
	if !sawKill || !sawPull {
		t.Errorf("kill=%v pull=%v; calls: %v", sawKill, sawPull, f.calls)
	}
}
