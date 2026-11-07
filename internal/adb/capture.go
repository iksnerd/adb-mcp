package adb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/iksnerd/adb-mcp/internal/sdk"
)

// Capture sessions are stateful: start spawns a background process that keeps
// running across tool calls, stop tears it down and returns what it collected.
// Keyed by resolved device serial: the only entry points are tools.resolve (via
// ResolveSerial, which never returns ""), so the key is always concrete.

// sessionRegistry is a serial-keyed table of in-flight background sessions
// (a logcat capture or a screen recording), guarded by one mutex. start holds
// the lock across mk so "does a session already exist" and "register the new
// one" stay atomic; take atomically removes a session so its (possibly slow)
// teardown runs outside the lock.
type sessionRegistry[T any] struct {
	mu       sync.Mutex
	sessions map[string]*T
}

// errEmptySerial rejects the zero key: a session registered under "" could be
// found by (and stopped for) any caller that forgot to resolve a device.
var errEmptySerial = errors.New("capture sessions need a concrete device serial")

func (r *sessionRegistry[T]) start(serial, kind string, mk func() (*T, error)) error {
	if serial == "" {
		return errEmptySerial
	}
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

func (r *sessionRegistry[T]) take(serial string) (*T, bool, error) {
	if serial == "" {
		return nil, false, errEmptySerial
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[serial]
	if ok {
		delete(r.sessions, serial)
	}
	return s, ok, nil
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

	// done closes when the logcat child exits, whoever caused it; exitErr and
	// exitAt are valid after that. A child that dies on its own (device
	// unplugged, adb server restarted) leaves a partial capture that Stop must
	// not present as complete.
	done    chan struct{}
	exitErr error
	exitAt  time.Time
}

// watchLogSession wraps a started logcat child and records when it exits.
func watchLogSession(cmd *exec.Cmd, f *os.File, path string) *logSession {
	s := &logSession{cmd: cmd, file: f, path: path, done: make(chan struct{})}
	go func() {
		s.exitErr = cmd.Wait()
		s.exitAt = time.Now()
		close(s.done)
	}()
	return s
}

// killAndWait ends the child (a no-op if it already exited) and waits for the
// watcher to reap it.
func (s *logSession) killAndWait() {
	_ = s.cmd.Process.Kill()
	<-s.done
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
		return watchLogSession(cmd, f, f.Name()), nil
	})
}

// StopLogcatCapture stops the running capture for the device and returns the
// collected lines, filtered per f (see LogFilter) with chatty spam removed.
func (c *Client) StopLogcatCapture(f LogFilter) (string, error) {
	if err := f.validate(); err != nil {
		return "", err
	}
	s, ok, err := logSessions.take(c.Serial)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no logcat capture running for %s", c.Serial)
	}
	var diedEarly bool
	select {
	case <-s.done:
		diedEarly = true
	default:
	}
	s.killAndWait()
	s.file.Close()
	data, err := os.ReadFile(s.path)
	os.Remove(s.path)
	if err != nil {
		return "", err
	}
	out := strings.TrimRight(f.redact(f.apply(string(data))), "\n")
	if diedEarly {
		note := fmt.Sprintf("capture ended early at %s: %v (partial capture follows)", s.exitAt.Format(time.RFC3339), s.exitErr)
		if out == "" {
			return note, nil
		}
		return note + "\n" + out, nil
	}
	return out, nil
}

// StopAllCaptures tears down every running logcat/screen-record session,
// killing the detached adb client processes and removing the logcat temp files.
// Call it on server shutdown so an interrupted session does not leak an adb
// process or a /tmp file. (On-device artifacts from an interrupted screen
// recording are left as-is; a clean stop_screen_record removes them.)
func StopAllCaptures() {
	logSessions.stopAll(func(s *logSession) {
		s.killAndWait()
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
		// Per-session path: a second server (or session) must not clobber it, and
		// Stop can signal exactly this screenrecord by its path.
		devicePath := "/sdcard/aemcp-record-" + randToken() + ".mp4"
		cmd := exec.Command(sdk.AdbPath(), c.deviceArgs("shell", "screenrecord", devicePath)...)
		cmd.Env = sdk.CommandEnv()
		detach(cmd)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("start screenrecord: %w", err)
		}
		return &recordSession{cmd: cmd, devicePath: devicePath}, nil
	})
}

// recordPollInterval and recordSettleTimeout bound the wait for screenrecord to
// finish writing the mp4 after SIGINT.
var (
	recordPollInterval  = 300 * time.Millisecond
	recordSettleTimeout = 15 * time.Second
)

// StopScreenRecord stops the recording (SIGINT so the mp4 is finalized) and
// pulls it to localPath.
func (c *Client) StopScreenRecord(ctx context.Context, localPath string) (string, error) {
	s, ok, err := recSessions.take(c.Serial)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no screen recording running for %s", c.Serial)
	}
	// Interrupt only OUR screenrecord ON THE DEVICE so it flushes the moov atom.
	// The pattern is the unique device path, bracketed so pkill's own shell
	// (whose command line also carries it) does not match itself.
	pattern := "[" + s.devicePath[:1] + "]" + s.devicePath[1:]
	_, _ = c.adb(ctx, "shell", "pkill", "-INT", "-f", pattern)
	c.waitFileStable(ctx, s.devicePath)
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}
	if _, err := c.PullFile(ctx, s.devicePath, localPath); err != nil {
		return "", err
	}
	_, _ = c.adb(ctx, "shell", "rm", "-f", s.devicePath)
	return localPath, nil
}

// waitFileStable polls the device file's size until two consecutive non-zero
// reads agree (screenrecord has finished finalizing), bounded by
// recordSettleTimeout. On timeout it returns anyway; the pull then surfaces
// whatever is there.
func (c *Client) waitFileStable(ctx context.Context, devicePath string) {
	deadline := time.Now().Add(recordSettleTimeout)
	prev := ""
	for time.Now().Before(deadline) {
		out, err := c.adb(ctx, "shell", "stat", "-c", "%s", devicePath)
		size := strings.TrimSpace(out)
		if err == nil && size != "" && size != "0" && size == prev {
			return
		}
		if err == nil {
			prev = size
		} else {
			prev = ""
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(recordPollInterval):
		}
	}
}
