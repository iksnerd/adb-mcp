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

type logSession struct {
	cmd  *exec.Cmd
	file *os.File
	path string
}

var (
	logMu       sync.Mutex
	logSessions = map[string]*logSession{}
)

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
	logMu.Lock()
	defer logMu.Unlock()
	if _, ok := logSessions[c.Serial]; ok {
		return fmt.Errorf("a logcat capture is already running for %s; stop it first", c.Serial)
	}
	if clear {
		_, _ = c.adb(ctx, "logcat", "-c")
	}
	f, err := os.CreateTemp("", "aemcp-logcat-*.txt")
	if err != nil {
		return err
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
		return fmt.Errorf("start logcat capture: %w", err)
	}
	logSessions[c.Serial] = &logSession{cmd: cmd, file: f, path: f.Name()}
	return nil
}

// StopLogcatCapture stops the running capture for the device and returns the
// collected lines, filtered per f (see LogFilter) with chatty spam removed.
func (c *Client) StopLogcatCapture(f LogFilter) (string, error) {
	if err := f.validate(); err != nil {
		return "", err
	}
	logMu.Lock()
	s, ok := logSessions[c.Serial]
	if ok {
		delete(logSessions, c.Serial)
	}
	logMu.Unlock()
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
	logMu.Lock()
	for serial, s := range logSessions {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
		s.file.Close()
		os.Remove(s.path)
		delete(logSessions, serial)
	}
	logMu.Unlock()

	recMu.Lock()
	for serial, s := range recSessions {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
		delete(recSessions, serial)
	}
	recMu.Unlock()
}

type recordSession struct {
	cmd        *exec.Cmd
	devicePath string
}

var (
	recMu       sync.Mutex
	recSessions = map[string]*recordSession{}
)

// StartScreenRecord starts screenrecord on the device (max ~180s per Android's
// limit). StopScreenRecord finalizes and pulls the mp4 to localPath.
func (c *Client) StartScreenRecord(ctx context.Context) error {
	recMu.Lock()
	defer recMu.Unlock()
	if _, ok := recSessions[c.Serial]; ok {
		return fmt.Errorf("a screen recording is already running for %s; stop it first", c.Serial)
	}
	devicePath := "/sdcard/aemcp-record.mp4"
	cmd := exec.Command(sdk.AdbPath(), c.deviceArgs("shell", "screenrecord", devicePath)...)
	cmd.Env = sdk.CommandEnv()
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start screenrecord: %w", err)
	}
	recSessions[c.Serial] = &recordSession{cmd: cmd, devicePath: devicePath}
	return nil
}

// StopScreenRecord stops the recording (SIGINT so the mp4 is finalized) and
// pulls it to localPath.
func (c *Client) StopScreenRecord(ctx context.Context, localPath string) (string, error) {
	recMu.Lock()
	s, ok := recSessions[c.Serial]
	if ok {
		delete(recSessions, c.Serial)
	}
	recMu.Unlock()
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
