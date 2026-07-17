package adb

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/iksnerd/adb_mcp/internal/sdk"
)

// ListAVDs returns the names of available AVDs via `emulator -list-avds`.
func ListAVDs(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, sdk.EmulatorPath(), "-list-avds")
	cmd.Env = sdk.CommandEnv()
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
	cmd := exec.Command(sdk.EmulatorPath(), args...)
	cmd.Env = sdk.CommandEnv()
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
	if err := New(serial).WaitForBoot(ctx, remaining); err != nil {
		return serial, err
	}
	return serial, nil
}

// WaitForBoot blocks until the device reports sys.boot_completed=1 or timeout.
func (c *Client) WaitForBoot(ctx context.Context, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := c.adb(ctx, "shell", "getprop", "sys.boot_completed")
		if err == nil && strings.TrimSpace(out) == "1" {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("device %s did not finish booting within %s", c.Serial, timeout)
}

// Shutdown asks the emulator to power off.
func (c *Client) Shutdown(ctx context.Context) error {
	_, err := c.adb(ctx, "emu", "kill")
	return err
}

// emu runs one emulator-console command (adb emu <args...>) — the bridge to the
// emulator's Extended Controls (fingerprint, battery, telephony, sensors, …),
// which live in the emulator process's own window and are invisible to
// adb/uiautomator. The console only exists on emulators, and it reports
// failures as a "KO:" stdout line rather than a non-zero exit, so both the
// wrong-target and the KO cases are turned into errors here.
func (c *Client) emu(ctx context.Context, what string, args ...string) (string, error) {
	if !strings.HasPrefix(c.Serial, "emulator-") {
		return "", fmt.Errorf("%s only works on an emulator (serial %q is not emulator-*): the emulator console has no physical-device equivalent", what, c.Serial)
	}
	out, err := c.adb(ctx, append([]string{"emu"}, args...)...)
	if err != nil {
		return out, err
	}
	if strings.Contains(out, "KO:") {
		return out, fmt.Errorf("emulator rejected %s: %s", what, strings.TrimSpace(out))
	}
	return out, nil
}

// FingerTouch simulates a fingerprint sensor touch on an emulator
// (adb emu finger touch <id>). With a fingerprint enrolled this satisfies a
// BiometricPrompt, letting a flow exercise the real biometric path instead of
// cancelling into the PIN fallback. Emulator-only: physical devices have no
// way to inject biometrics.
func (c *Client) FingerTouch(ctx context.Context, fingerID int) error {
	if fingerID <= 0 {
		fingerID = 1
	}
	_, err := c.emu(ctx, "fingerprint simulation", "finger", "touch", strconv.Itoa(fingerID))
	return err
}

// FingerRemove lifts the simulated finger off the sensor (adb emu finger
// remove) — the complement to FingerTouch for flows that watch for the
// finger-up event.
func (c *Client) FingerRemove(ctx context.Context) error {
	_, err := c.emu(ctx, "fingerprint removal", "finger", "remove")
	return err
}

// SendSMS injects an incoming SMS from number with the given body
// (adb emu sms send <number> <text>) — drives OTP/2FA flows without a second
// device.
func (c *Client) SendSMS(ctx context.Context, number, text string) error {
	if strings.TrimSpace(number) == "" || strings.TrimSpace(text) == "" {
		return fmt.Errorf("both a sender number and message text are required")
	}
	_, err := c.emu(ctx, "sms send", "sms", "send", number, text)
	return err
}

// gsmActions are the telephony verbs the emulator console accepts.
var gsmActions = map[string]bool{"call": true, "accept": true, "cancel": true, "busy": true, "hold": true}

// GSMCall drives an emulated voice call: "call" rings an incoming call from
// number, and "accept"/"cancel"/"busy"/"hold" transition it
// (adb emu gsm <action> <number>).
func (c *Client) GSMCall(ctx context.Context, action, number string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	if !gsmActions[action] {
		return fmt.Errorf("unknown call action %q (use call, accept, cancel, busy, or hold)", action)
	}
	if strings.TrimSpace(number) == "" {
		return fmt.Errorf("a phone number is required")
	}
	_, err := c.emu(ctx, "gsm "+action, "gsm", action, number)
	return err
}

// SetBattery sets the emulated battery: level is 0-100 (nil leaves it), and
// charging toggles the AC line (nil leaves it) — via adb emu power capacity /
// power ac. At least one must be set.
func (c *Client) SetBattery(ctx context.Context, level *int, charging *bool) error {
	if level == nil && charging == nil {
		return fmt.Errorf("set a battery level and/or charging state")
	}
	if level != nil {
		if *level < 0 || *level > 100 {
			return fmt.Errorf("battery level must be 0-100, got %d", *level)
		}
		if _, err := c.emu(ctx, "power capacity", "power", "capacity", strconv.Itoa(*level)); err != nil {
			return err
		}
	}
	if charging != nil {
		state := "off"
		if *charging {
			state = "on"
		}
		if _, err := c.emu(ctx, "power ac", "power", "ac", state); err != nil {
			return err
		}
	}
	return nil
}

// Rotate rotates the emulator to its next orientation (adb emu rotate).
func (c *Client) Rotate(ctx context.Context) error {
	_, err := c.emu(ctx, "rotate", "rotate")
	return err
}

// gsmRegStates are the registration states adb emu gsm data|voice accept.
var gsmRegStates = map[string]bool{
	"unregistered": true, "home": true, "roaming": true,
	"searching": true, "denied": true, "off": true, "on": true,
}

// Cellular drives the emulated radio through the emulator console: data/voice
// registration state (adb emu gsm data|voice <state>), signal strength
// (adb emu gsm signal-profile <0-4>), and mobile-data throughput/latency
// (adb emu network speed|delay <value>). Every field is optional; at least one
// must be set. speed/delay pass through to the console (which accepts named
// profiles like "lte"/"edge" or raw "<up>:<down>" kbps / "<min>:<max>" ms), so
// a bad value surfaces as the console's own KO error.
func (c *Client) Cellular(ctx context.Context, data, voice string, signal *int, speed, delay string) error {
	data = strings.ToLower(strings.TrimSpace(data))
	voice = strings.ToLower(strings.TrimSpace(voice))
	speed = strings.TrimSpace(speed)
	delay = strings.TrimSpace(delay)
	if data == "" && voice == "" && signal == nil && speed == "" && delay == "" {
		return fmt.Errorf("set at least one of data, voice, signal, network_speed, or network_delay")
	}
	if data != "" {
		if !gsmRegStates[data] {
			return fmt.Errorf("unknown data state %q (use unregistered, home, roaming, searching, denied, off, or on)", data)
		}
		if _, err := c.emu(ctx, "gsm data", "gsm", "data", data); err != nil {
			return err
		}
	}
	if voice != "" {
		if !gsmRegStates[voice] {
			return fmt.Errorf("unknown voice state %q (use unregistered, home, roaming, searching, denied, off, or on)", voice)
		}
		if _, err := c.emu(ctx, "gsm voice", "gsm", "voice", voice); err != nil {
			return err
		}
	}
	if signal != nil {
		if *signal < 0 || *signal > 4 {
			return fmt.Errorf("signal must be 0-4 (0 = none, 4 = great), got %d", *signal)
		}
		if _, err := c.emu(ctx, "gsm signal-profile", "gsm", "signal-profile", strconv.Itoa(*signal)); err != nil {
			return err
		}
	}
	if speed != "" {
		if _, err := c.emu(ctx, "network speed", "network", "speed", speed); err != nil {
			return err
		}
	}
	if delay != "" {
		if _, err := c.emu(ctx, "network delay", "network", "delay", delay); err != nil {
			return err
		}
	}
	return nil
}

// SetSensor sets an emulated hardware sensor's value(s) via the emulator console
// (adb emu sensor set <name> <a>[:<b>[:<c>]]). Multi-axis sensors
// (acceleration, gyroscope, magnetic-field, orientation) take three values;
// single-value sensors (light, proximity, temperature, pressure, humidity) take
// one. The sensor name and value count pass through to the console, so an
// unknown name or wrong arity surfaces as its KO error.
func (c *Client) SetSensor(ctx context.Context, name string, values []float64) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("a sensor name is required (e.g. acceleration, light, proximity)")
	}
	if len(values) == 0 || len(values) > 3 {
		return fmt.Errorf("provide 1 to 3 sensor values, got %d", len(values))
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	_, err := c.emu(ctx, "sensor set", "sensor", "set", name, strings.Join(parts, ":"))
	return err
}

// snapshotActions are the avd-snapshot verbs (list takes no name).
var snapshotActions = map[string]bool{"save": true, "load": true, "delete": true, "list": true}

// Snapshot manages emulator AVD snapshots (adb emu avd snapshot <action>
// [name]): save/load/delete a named snapshot, or list them — the deterministic
// way to reset a device to a known state between runs.
func (c *Client) Snapshot(ctx context.Context, action, name string) (string, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if !snapshotActions[action] {
		return "", fmt.Errorf("unknown snapshot action %q (use save, load, delete, or list)", action)
	}
	if action == "list" {
		return c.emu(ctx, "avd snapshot list", "avd", "snapshot", "list")
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("a snapshot name is required for %s", action)
	}
	return c.emu(ctx, "avd snapshot "+action, "avd", "snapshot", action, name)
}
