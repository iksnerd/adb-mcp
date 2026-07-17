package android

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ListAVDs returns the names of available AVDs via `emulator -list-avds`.
func ListAVDs(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, emulatorPath(), "-list-avds")
	cmd.Env = commandEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("emulator -list-avds: %s", strings.TrimSpace(stderr.String()))
	}
	var avds []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			avds = append(avds, s)
		}
	}
	return avds, nil
}

// BootEmulator launches an AVD detached (so it outlives the request) and, when
// waitForBoot is true, waits until it reports sys.boot_completed. It returns the
// serial of the newly booted device.
func BootEmulator(ctx context.Context, avd string, noSnapshot, waitForBoot, wipeData bool, timeout time.Duration) (string, error) {
	// Snapshot the devices present before boot so we can identify the new one
	// afterwards. If this listing fails we must not proceed: an empty snapshot
	// would make the discovery loop below treat an already-attached, unrelated
	// device as "newly booted" and return the wrong serial.
	before := map[string]bool{}
	devs, err := ListDevices(ctx)
	if err != nil {
		return "", fmt.Errorf("list devices before boot: %w", err)
	}
	for _, d := range devs {
		before[d.Serial] = true
	}

	args := []string{"-avd", avd}
	if noSnapshot {
		args = append(args, "-no-snapshot-load")
	}
	if wipeData {
		args = append(args, "-wipe-data") // factory reset on this boot
	}
	// Detached: not bound to ctx, its own stdio, so it survives the tool call.
	cmd := exec.Command(emulatorPath(), args...)
	cmd.Env = commandEnv()
	cmd.Stdout = nil
	cmd.Stderr = nil
	if devnull, err := os.Open(os.DevNull); err == nil {
		cmd.Stdin = devnull
	}
	detach(cmd) // own session, so it outlives this process
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start emulator %q: %w", avd, err)
	}
	_ = cmd.Process.Release()

	// Discover the new serial by polling for a device not present before boot.
	deadline := time.Now().Add(timeout)
	var serial string
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		devs, err := ListDevices(ctx)
		if err == nil {
			for _, d := range devs {
				if !before[d.Serial] {
					serial = d.Serial
					break
				}
			}
		}
		if serial != "" {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if serial == "" {
		return "", fmt.Errorf("emulator %q did not appear within %s", avd, timeout)
	}
	if !waitForBoot {
		return serial, nil
	}
	// Spend only the time left in the overall budget. A non-positive remainder
	// means discovery already ate the whole timeout — don't hand WaitForBoot a
	// value <= 0, which it would silently treat as its own 120s default.
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return serial, fmt.Errorf("emulator %q appeared as %s but did not finish booting within %s", avd, serial, timeout)
	}
	if err := WaitForBoot(ctx, serial, remaining); err != nil {
		return serial, err
	}
	return serial, nil
}

// WaitForBoot blocks until the device reports sys.boot_completed=1 or timeout.
func WaitForBoot(ctx context.Context, serial string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := runAdb(ctx, serial, "shell", "getprop", "sys.boot_completed")
		if err == nil && strings.TrimSpace(out) == "1" {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("device %s did not finish booting within %s", serial, timeout)
}

// Shutdown asks the emulator to power off.
func Shutdown(ctx context.Context, serial string) error {
	_, err := runAdb(ctx, serial, "emu", "kill")
	return err
}

// FingerTouch simulates a fingerprint sensor touch on an emulator
// (adb emu finger touch <id>). With a fingerprint enrolled this satisfies a
// BiometricPrompt, letting a flow exercise the real biometric path instead of
// cancelling into the PIN fallback. Emulator-only: physical devices have no
// way to inject biometrics.
func FingerTouch(ctx context.Context, serial string, fingerID int) error {
	if fingerID <= 0 {
		fingerID = 1
	}
	if !strings.HasPrefix(serial, "emulator-") {
		return fmt.Errorf("fingerprint simulation only works on emulators (serial %q is not emulator-*): physical devices cannot inject biometrics", serial)
	}
	out, err := runAdb(ctx, serial, "emu", "finger", "touch", strconv.Itoa(fingerID))
	if err != nil {
		return err
	}
	// `adb emu` reports failures in stdout with a KO: prefix rather than a
	// non-zero exit, so surface those as errors.
	if strings.Contains(out, "KO:") {
		return fmt.Errorf("emulator rejected finger touch: %s", strings.TrimSpace(out))
	}
	return nil
}
