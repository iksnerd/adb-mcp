package gradle

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNonExecutableWrapperNamesFileAndChmod(t *testing.T) {
	dir := fakeProject(t, `echo hi`)
	if err := os.Chmod(filepath.Join(dir, "gradlew"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := RunBuild(context.Background(), dir, "", nil)
	if s.Status != StatusFailed || !strings.Contains(s.Err, "chmod +x") || !strings.Contains(s.Err, filepath.Join(dir, "gradlew")) {
		t.Errorf("Status=%q Err=%q, want a message naming the file and chmod +x", s.Status, s.Err)
	}
}

func TestWaitDelayFreesPipeHeldByChild(t *testing.T) {
	old := waitDelay
	waitDelay = 300 * time.Millisecond
	defer func() { waitDelay = old }()
	// The backgrounded sleep inherits the output pipes and outlives gradlew,
	// as a gradlew.bat child JVM does.
	dir := fakeProject(t, `sleep 5 &
echo "BUILD SUCCESSFUL"`)
	start := time.Now()
	s := RunBuild(context.Background(), dir, "", nil)
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("RunBuild blocked for %v on an inherited pipe", d)
	}
	if s.Status != StatusPassed {
		t.Errorf("Status = %q (%s), want passed: gradlew itself exited 0", s.Status, s.Err)
	}
}

func TestCancelAllCancelsEveryRunningJob(t *testing.T) {
	dir := fakeProject(t, `exec sleep 30`)
	var ids []string
	for range 2 {
		j, err := StartBuild(dir, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, j.ID)
	}
	CancelAll()
	deadline := time.Now().Add(10 * time.Second)
	for _, id := range ids {
		for {
			s, _ := BuildStatus(id)
			if s.Status == StatusFailed {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("job %s never reported failed after CancelAll", id)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestLogDirIsPrivateAndRefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix modes")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	logDir := filepath.Join(tmp, "adb-mcp-gradle")
	if err := os.Mkdir(logDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(logDir, 0o777); err != nil {
		t.Fatal(err)
	}
	dir := fakeProject(t, `echo ok`)
	s := RunBuild(context.Background(), dir, "", nil)
	if s.Status != StatusPassed {
		t.Fatalf("Status=%q Err=%q", s.Status, s.Err)
	}
	if info, _ := os.Stat(logDir); info.Mode().Perm() != 0o700 {
		t.Errorf("log dir mode = %v, want 0700", info.Mode().Perm())
	}

	// Replace it with a symlink: must be refused.
	if err := os.RemoveAll(logDir); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.Symlink(target, logDir); err != nil {
		t.Fatal(err)
	}
	s = RunBuild(context.Background(), dir, "", nil)
	if s.Status != StatusFailed || !strings.Contains(s.Err, "symlink") {
		t.Errorf("Status=%q Err=%q, want a refusal naming the symlink", s.Status, s.Err)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("logs written through the symlink: %v", entries)
	}
}

func TestOldBuildLogsPruned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix modes")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	logDir := filepath.Join(tmp, "adb-mcp-gradle")
	if err := os.Mkdir(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(logDir, "build-old.log")
	recent := filepath.Join(logDir, "build-recent.log")
	other := filepath.Join(logDir, "keep.txt")
	for _, p := range []string{old, recent, other} {
		writeFile(t, p, "x")
	}
	setMtime(t, old, time.Now().Add(-48*time.Hour))
	setMtime(t, other, time.Now().Add(-48*time.Hour))
	RunBuild(context.Background(), fakeProject(t, `echo ok`), "", nil)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old build log should be pruned")
	}
	for _, p := range []string{recent, other} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s should be kept: %v", p, err)
		}
	}
}

func TestBuildStatusSaysWhenLogIsGone(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gone.log")
	jobsMu.Lock()
	jobs["loggone"] = &job{summary: BuildSummary{ID: "loggone", Status: StatusPassed, Task: "assembleDebug", LogPath: logPath}}
	jobsMu.Unlock()
	s, err := BuildStatus("loggone")
	if err != nil {
		t.Fatal(err)
	}
	if !s.LogMissing || !strings.Contains(s.String(), "no longer exists") {
		t.Errorf("LogMissing=%v, String=%q; want the missing log reported", s.LogMissing, s.String())
	}
}

func TestScanErrorsFixtures(t *testing.T) {
	cases := []struct {
		name, out string
		count     int
		first0    string
	}{
		{"javac bare error", "error: cannot find symbol\n  symbol: class Foo\n", 1, "error: cannot find symbol"},
		{"javac file error", "A.java:3: error: ';' expected\n", 1, "A.java:3: error: ';' expected"},
		{"kotlin", "e: /src/A.kt:3:5 Unresolved reference: foo\n", 1, "e: /src/A.kt:3:5 Unresolved reference: foo"},
		{"summary not double counted", "e: /src/A.kt:3:5 Unresolved reference: foo\ne: /src/B.kt:1:1 Type mismatch\n\n* What went wrong:\nExecution failed for task ':app:compileDebugKotlin'.\n> Compilation error. See log for more details\n", 2, "e: /src/A.kt:3:5 Unresolved reference: foo"},
		{"summary alone counted", "* What went wrong:\nTask 'x' not found in root project 'p'.\n", 1, "Task 'x' not found in root project 'p'."},
		{"prose mentioning errors ignored", "0 errors reported, no error: here\n", 0, ""},
		{"clean", "BUILD SUCCESSFUL\n", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, first := scanErrors(tc.out)
			if n != tc.count {
				t.Fatalf("count = %d (%v), want %d", n, first, tc.count)
			}
			if tc.count > 0 && first[0] != tc.first0 {
				t.Errorf("first = %q, want %q", first[0], tc.first0)
			}
		})
	}
}

func TestRedactingWriterRedactsPEMAcrossLines(t *testing.T) {
	var dst bytes.Buffer
	w := &redactingWriter{dst: &dst}
	// Split the writes mid-line to prove state survives chunking.
	for _, chunk := range []string{
		"start\n-----BEGIN PRIVATE", " KEY-----\nhunter2abc\nMIIEv", "s3cret\n-----END PRIVATE KEY-----\nBUILD SUCCESSFUL\n",
	} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	w.flush()
	got := dst.String()
	for _, leak := range []string{"hunter2", "s3cret", "MIIEv"} {
		if strings.Contains(got, leak) {
			t.Errorf("PEM body leaked %q: %q", leak, got)
		}
	}
	if !strings.Contains(got, "start\n") || !strings.Contains(got, "BUILD SUCCESSFUL\n") {
		t.Errorf("ordinary output around the key was lost: %q", got)
	}
}

func TestRedactingWriterUnterminatedPEMDropsRest(t *testing.T) {
	var dst bytes.Buffer
	w := &redactingWriter{dst: &dst}
	_, _ = w.Write([]byte("-----BEGIN PRIVATE KEY-----\nhunter2abc\nmore"))
	w.flush()
	if strings.Contains(dst.String(), "hunter2") || strings.Contains(dst.String(), "more") {
		t.Errorf("unterminated key body leaked: %q", dst.String())
	}
}
