package adb

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/iksnerd/adb_mcp/internal/sdk"
)

// Capture sessions are stateful: start spawns a background process that keeps
// running across tool calls, stop tears it down and returns what it collected.
// Keyed by resolved device serial.

// sessionRegistry is a serial-keyed table of in-flight background sessions
// (a logcat capture or a screen recording), guarded by one mutex. start holds
// the lock across mk so "does a session already exist" and "register the new
// one" stay atomic; take atomically removes a session so its (possibly slow)
// teardown runs outside the lock.
type sessionRegistry[T any] struct {
	mu       sync.Mutex
	sessions map[string]*T
}

func (r *sessionRegistry[T]) start(serial, kind string, mk func() (*T, error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[serial]; ok {
		return fmt.Errorf("a %s is already running for %s; stop it first", kind, serial)
	}
	s, err := mk()
	if err != nil {
		return err
	}
	if r.sessions == nil {
		r.sessions = map[string]*T{}
	}
	r.sessions[serial] = s
	return nil
}

func (r *sessionRegistry[T]) take(serial string) (*T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[serial]
	if ok {
		delete(r.sessions, serial)
	}
	return s, ok
}

func (r *sessionRegistry[T]) stopAll(stop func(*T)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for serial, s := range r.sessions {
		stop(s)
		delete(r.sessions, serial)
	}
}

type logSession struct {
	cmd  *exec.Cmd
	file *os.File
	path string
}

var logSessions = &sessionRegistry[logSession]{}

// deviceArgs prefixes args with `-s <serial>` when the client targets a
// specific device, matching runAdbBytes for the background streaming processes
// that build their own exec.Cmd rather than going through the runner.
func (c *Client) deviceArgs(args ...string) []string {
	if c.Serial == "" {
		return args
	}
	return append([]string{"-s", c.Serial}, args...)
}

// StartLogcatCapture begins streaming logcat for the device into a temp file, so
// a later StopLogcatCapture returns everything logged during a flow (unlike the
// one-shot Logcat dump). Optionally clears the buffer first.
func (c *Client) StartLogcatCapture(ctx context.Context, clear bool) error {
	if clear {
		_, _ = c.adb(ctx, "logcat", "-c")
	}
	return logSessions.start(c.Serial, "logcat capture", func() (*logSession, error) {
		f, err := os.CreateTemp("", "aemcp-logcat-*.txt")
		if err != nil {
			return nil, err
		}
		// threadtime (not "time") so StopLogcatCapture's LogFilter can parse each
		// line's priority/tag — logLineRe expects the "… PID TID PRIO TAG:" shape.
		cmd := exec.Command(sdk.AdbPath(), c.deviceArgs("logcat", "-v", "threadtime")...)
		cmd.Env = sdk.CommandEnv()
		cmd.Stdout = f
		cmd.Stderr = f
		detach(cmd)
		if err := cmd.Start(); err != nil {
			f.Close()
			os.Remove(f.Name())
			return nil, fmt.Errorf("start logcat capture: %w", err)
		}
		return &logSession{cmd: cmd, file: f, path: f.Name()}, nil
	})
}

// StopLogcatCapture stops the running capture for the device and returns the
// collected lines, filtered per f (see LogFilter) with chatty spam removed.
func (c *Client) StopLogcatCapture(f LogFilter) (string, error) {
	if err := f.validate(); err != nil {
		return "", err
	}
	s, ok := logSessions.take(c.Serial)
	if !ok {
		return "", fmt.Errorf("no logcat capture running for %s", c.Serial)
	}
	_ = s.cmd.Process.Kill()
	_, _ = s.cmd.Process.Wait()
	s.file.Close()
	data, err := os.ReadFile(s.path)
	os.Remove(s.path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(f.apply(string(data)), "\n"), nil
}

// StopAllCaptures tears down every running logcat/screen-record session,
// killing the detached adb client processes and removing the logcat temp files.
// Call it on server shutdown so an interrupted session does not leak an adb
// process or a /tmp file. (On-device artifacts from an interrupted screen
// recording are left as-is; a clean stop_screen_record removes them.)
func StopAllCaptures() {
	logSessions.stopAll(func(s *logSession) {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
		s.file.Close()
		os.Remove(s.path)
	})
	recSessions.stopAll(func(s *recordSession) {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	})
}

type recordSession struct {
	cmd        *exec.Cmd
	devicePath string
}

var recSessions = &sessionRegistry[recordSession]{}

// StartScreenRecord starts screenrecord on the device (max ~180s per Android's
// limit). StopScreenRecord finalizes and pulls the mp4 to localPath.
func (c *Client) StartScreenRecord(ctx context.Context) error {
	return recSessions.start(c.Serial, "screen recording", func() (*recordSession, error) {
		devicePath := "/sdcard/aemcp-record.mp4"
		cmd := exec.Command(sdk.AdbPath(), c.deviceArgs("shell", "screenrecord", devicePath)...)
		cmd.Env = sdk.CommandEnv()
		detach(cmd)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("start screenrecord: %w", err)
		}
		return &recordSession{cmd: cmd, devicePath: devicePath}, nil
	})
}

// StopScreenRecord stops the recording (SIGINT so the mp4 is finalized) and
// pulls it to localPath.
func (c *Client) StopScreenRecord(ctx context.Context, localPath string) (string, error) {
	s, ok := recSessions.take(c.Serial)
	if !ok {
		return "", fmt.Errorf("no screen recording running for %s", c.Serial)
	}
	// Interrupt screenrecord ON THE DEVICE so it flushes the moov atom, then
	// wait for the file to finalize before pulling.
	_, _ = c.adb(ctx, "shell", "pkill", "-INT", "screenrecord")
	time.Sleep(1500 * time.Millisecond)
	_ = s.cmd.Process.Kill()
	_, _ = s.cmd.Process.Wait()
	if _, err := c.PullFile(ctx, s.devicePath, localPath); err != nil {
		return "", err
	}
	_, _ = c.adb(ctx, "shell", "rm", "-f", s.devicePath)
	return localPath, nil
}
