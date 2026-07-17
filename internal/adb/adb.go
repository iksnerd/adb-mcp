package adb

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/iksnerd/adb_mcp/internal/sdk"
)

// Runner executes an adb invocation already targeted at one device and returns
// raw stdout (stderr folded into err on failure). It is the single seam a
// Client injects, so every command builder below can be unit-tested with a fake
// Runner — no real adb, no device. New wires it to the real adb binary.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// Client issues adb commands against one resolved device serial. The tools
// layer builds one per request with New; tests build one with a fake Runner.
type Client struct {
	Serial string
	run    Runner
}

// New returns a Client that shells out to the real adb binary for serial.
func New(serial string) *Client {
	return &Client{
		Serial: serial,
		run: func(ctx context.Context, args ...string) ([]byte, error) {
			return runAdbBytes(ctx, serial, args...)
		},
	}
}

// adb runs an adb command for the client's device and returns combined trimmed
// stdout.
func (c *Client) adb(ctx context.Context, args ...string) (string, error) {
	out, err := c.run(ctx, args...)
	return strings.TrimRight(string(out), "\r\n"), err
}

// adbBytes is like adb but returns raw stdout bytes (used for screencap, where
// trimming or text conversion would corrupt the PNG).
func (c *Client) adbBytes(ctx context.Context, args ...string) ([]byte, error) {
	return c.run(ctx, args...)
}

// runAdb runs an adb command (optionally targeting serial) and returns combined
// trimmed stdout. It backs New's real Runner and the hostless helpers
// (ListDevices, ResolveSerial, ConnectWireless, Doctor) that have no serial.
func runAdb(ctx context.Context, serial string, args ...string) (string, error) {
	out, err := runAdbBytes(ctx, serial, args...)
	return strings.TrimRight(string(out), "\r\n"), err
}

// runAdbBytes is like runAdb but returns raw stdout bytes.
func runAdbBytes(ctx context.Context, serial string, args ...string) ([]byte, error) {
	full := args
	if serial != "" {
		full = append([]string{"-s", serial}, args...)
	}
	cmd := exec.CommandContext(ctx, sdk.AdbPath(), full...)
	cmd.Env = sdk.CommandEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg != "" {
			return stdout.Bytes(), fmt.Errorf("adb %s: %s", strings.Join(full, " "), msg)
		}
		return stdout.Bytes(), fmt.Errorf("adb %s: %w", strings.Join(full, " "), err)
	}
	return stdout.Bytes(), nil
}
