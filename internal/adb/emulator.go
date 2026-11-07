package adb

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/iksnerd/adb-mcp/internal/sdk"
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
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			avds = append(avds, s)
		}
	}
	return avds, nil
}

// Bounds for boot discovery, vars so tests can shrink them.
var (
	bootPollInterval   = time.Second
	bootNameGracePolls = 5                      // polls a new emulator may stay unnamed before it is accepted
	avdNameRetryDelay  = 500 * time.Millisecond // between AVDName tries for one emulator
	avdNameTries       = 4
	waitForBootPoll    = 2 * time.Second
)

// BootEmulator launches an AVD detached (so it outlives the request) and, when
// waitForBoot is true, waits until it reports sys.boot_completed. It returns the
// serial of the newly booted device.
func BootEmulator(ctx context.Context, avd string, noSnapshot *bool, waitForBoot, wipeData bool, timeout time.Duration) (string, error) {
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

	// An emulator of this AVD may already be up. It is in the "before" snapshot,
	// so the discovery loop below could never see it as new and would wait out
	// the whole timeout, then report failure for a device that works. Two
	// emulators cannot share an AVD's state either, so launching another is wrong.
	attached := attachedEmulators(ctx, devs)
	running, ok := findRunningAVD(attached, avd)
	if !ok {
		// An emulator still starting does not answer `avd name` yet. Its AVD is
		// unknown, not different: launching beside it could fight it for the
		// AVD lock. Say which device and let the caller wait or pick it.
		for _, e := range attached {
			if e.AVD == "" {
				return "", fmt.Errorf("emulator %s (%s) is attached but would not say which AVD it runs, so booting %q could start a second emulator on the same AVD: wait for it to finish starting and retry, or use its serial directly", e.Serial, e.State, avd)
			}
		}
	}
	if ok {
		if (noSnapshot != nil && *noSnapshot) || wipeData {
			return "", fmt.Errorf("emulator %q is already running as %s, so no_snapshot / wipe_data cannot apply to it: stop it first (shutdown_emulator) and boot again, or leave both off to use it as it is", avd, running.Serial)
		}
		if !waitForBoot {
			return running.Serial, nil
		}
		if err := New(running.Serial).WaitForBoot(ctx, timeout); err != nil {
			return running.Serial, err
		}
		return running.Serial, nil
	}

	args := []string{"-avd", avd}
	if noSnapshot == nil || *noSnapshot { // unset: a cold boot, the default
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

	// Discover the new serial by polling for a new emulator-* device that runs
	// avd. Physical devices that appear meanwhile are not ours. A new emulator
	// in any adb state counts (it shows as offline while starting; WaitForBoot
	// covers that), but one whose AVD name is known and different is another
	// emulator. One whose name is still unknown gets bootNameGracePolls polls
	// to answer before it is accepted on trust.
	deadline := time.Now().Add(timeout)
	var serial string
	unnamed := map[string]int{}
	for time.Now().Before(deadline) && serial == "" {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		devs, err := ListDevices(ctx)
		if err == nil {
			for _, d := range devs {
				if before[d.Serial] || !strings.HasPrefix(d.Serial, "emulator-") {
					continue
				}
				name, nerr := New(d.Serial).AVDName(ctx)
				if nerr == nil && name != avd {
					before[d.Serial] = true // another AVD: never ours
					continue
				}
				if nerr != nil {
					unnamed[d.Serial]++
					if unnamed[d.Serial] < bootNameGracePolls {
						continue
					}
				}
				serial = d.Serial
				break
			}
		}
		if serial == "" {
			time.Sleep(bootPollInterval)
		}
	}
	if serial == "" {
		// Say what IS attached. boot_emulator only reports a NEW device, so a
		// caller that sees a bare failure may boot a second emulator or give up
		// on one that works.
		now, _ := ListDevices(ctx)
		return "", fmt.Errorf("emulator %q did not appear within %s as a new device (attached now: %s); if one of those is the AVD you wanted, use its serial", avd, timeout, describeAttached(attachedEmulators(ctx, now)))
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

// attachedEmulator is one emulator-* device and the AVD it reports running.
type attachedEmulator struct {
	Serial, State, AVD string // AVD is empty when the emulator would not say
}

// attachedEmulators asks every emulator-* device which AVD it runs (the console
// answers even while adb still reports the device as offline). Physical devices
// have no AVD and are skipped.
func attachedEmulators(ctx context.Context, devs []Device) []attachedEmulator {
	var out []attachedEmulator
	for _, d := range devs {
		if !strings.HasPrefix(d.Serial, "emulator-") {
			continue
		}
		name := New(d.Serial).avdNameRetry(ctx)
		out = append(out, attachedEmulator{Serial: d.Serial, State: d.State, AVD: name})
	}
	return out
}

// findRunningAVD returns the attached emulator that runs avd, if any.
func findRunningAVD(emus []attachedEmulator, avd string) (attachedEmulator, bool) {
	for _, e := range emus {
		if e.AVD == avd {
			return e, true
		}
	}
	return attachedEmulator{}, false
}

// describeAttached renders "emulator-5554 (device, Pixel_10a), …" for an error.
func describeAttached(emus []attachedEmulator) string {
	if len(emus) == 0 {
		return "no emulators"
	}
	parts := make([]string, len(emus))
	for i, e := range emus {
		avd := e.AVD
		if avd == "" {
			avd = "AVD unknown"
		}
		parts[i] = fmt.Sprintf("%s (%s, %s)", e.Serial, e.State, avd)
	}
	return strings.Join(parts, ", ")
}

// avdNameRetry asks for the AVD name a few times: a starting emulator's console
// is not up for the first moments. Empty when it never answers.
func (c *Client) avdNameRetry(ctx context.Context) string {
	for i := 0; i < avdNameTries; i++ {
		if name, err := c.AVDName(ctx); err == nil {
			return name
		}
		if i < avdNameTries-1 {
			select {
			case <-ctx.Done():
				return ""
			case <-time.After(avdNameRetryDelay):
			}
		}
	}
	return ""
}

// AVDName returns the AVD this emulator is running (adb emu avd name).
func (c *Client) AVDName(ctx context.Context) (string, error) {
	out, err := c.emu(ctx, "avd name", "avd", "name")
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(out, "\n") {
		if s := strings.TrimSpace(line); s != "" && s != "OK" {
			return s, nil
		}
	}
	return "", fmt.Errorf("emulator %s reported no AVD name", c.Serial)
}

// WaitForBoot blocks until the device reports sys.boot_completed=1 and the
// package manager answers (`pm path android`), or timeout. boot_completed is
// set before the package manager is ready, and installs/launches fail in that gap.
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
			if pm, perr := c.adb(ctx, "shell", "pm", "path", "android"); perr == nil && strings.Contains(pm, "package:") {
				return nil
			}
		}
		time.Sleep(waitForBootPoll)
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
	// The console is line oriented: a newline inside any argument would end the
	// command early and run the rest as a second console command.
	for _, a := range args {
		if i := strings.IndexFunc(a, unicode.IsControl); i >= 0 {
			return "", fmt.Errorf("%s: argument contains a control character (%q at byte %d); the emulator console would read it as a separate command", what, a[i], i)
		}
	}
	out, err := c.adb(ctx, append([]string{"emu"}, args...)...)
	if err != nil {
		return out, err
	}
	if consoleRejected(out) {
		return out, fmt.Errorf("emulator rejected %s: %s", what, strings.TrimSpace(out))
	}
	return out, nil
}

// consoleRejected reports whether the console answered KO. The marker counts
// only at the start of a line: payload text (a snapshot list entry) may
// contain "KO:" anywhere else.
func consoleRejected(out string) bool {
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(strings.TrimRight(line, "\r"), "KO:") {
			return true
		}
	}
	return false
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

// HasBiometricEnrolled reports whether any fingerprint is enrolled, and the
// enrolled count, from `dumpsys fingerprint`. This is the reliable branch point
// before a biometric flow: with nothing enrolled, fingerprint_touch can never
// satisfy a BiometricPrompt (it will sit on "Touch the sensor"), so a flow
// should enroll first or take the PIN path. Works on emulator and physical
// devices alike (it reads the framework, not the emulator console).
//
// Note: the dump exposes only a per-user enrolled COUNT, never the enrolled
// finger id — and a wrong `emu finger touch <id>` trips a HAL lockout after a
// few tries — so there is deliberately no "which id is enrolled" probe here;
// enroll deterministically instead of guessing ids at auth time.
func (c *Client) HasBiometricEnrolled(ctx context.Context) (enrolled bool, count int, err error) {
	out, err := c.adb(ctx, "shell", "dumpsys", "fingerprint")
	if err != nil {
		return false, 0, err
	}
	count, ok := parseEnrolledFingerprints(out)
	if !ok {
		return false, 0, fmt.Errorf("could not read the enrolled-fingerprint count from `dumpsys fingerprint` (unrecognised format on this image)")
	}
	return count > 0, count, nil
}

// fpCountRe pulls the enrolled count out of the modern JSON dump, whose per-user
// object is: {"id":<userId>,"count":<enrolledTemplates>,"accept":...}. Verified
// live: an empty AVD reports "count":0, and it becomes "count":1 after enrolling
// one fingerprint.
var fpCountRe = regexp.MustCompile(`"count":\s*(\d+)`)

// parseEnrolledFingerprints returns the number of enrolled fingerprints from
// `dumpsys fingerprint`, summing across users. It handles the modern JSON form
// ("prints":[{"id":0,"count":N,...}]) and falls back to the legacy text form
// (a "... count: N" line). ok is false when neither shape is present. Pure —
// unit-tested with captured dumps.
func parseEnrolledFingerprints(out string) (count int, ok bool) {
	// Modern JSON: sum the count field of every entry in the prints array.
	if i := strings.Index(out, `"prints":[`); i >= 0 {
		seg := out[i:]
		if j := strings.Index(seg, "]"); j >= 0 {
			seg = seg[:j]
		}
		total, found := 0, false
		for _, m := range fpCountRe.FindAllStringSubmatch(seg, -1) {
			n, _ := strconv.Atoi(m[1])
			total += n
			found = true
		}
		if found {
			return total, true
		}
	}
	// Legacy text form: any "count: N" (case-insensitive) on a per-user line.
	total, found := 0, false
	for line := range strings.SplitSeq(out, "\n") {
		low := strings.ToLower(line)
		if !strings.Contains(low, "count") {
			continue
		}
		if m := fpLegacyCountRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			total += n
			found = true
		}
	}
	if found {
		return total, true
	}
	return 0, false
}

var fpLegacyCountRe = regexp.MustCompile(`(?i)count:\s*(\d+)`)

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

// isEmulator reports whether the client targets an emulator (serial emulator-*),
// which alone has the `adb emu` console.
func (c *Client) isEmulator() bool { return strings.HasPrefix(c.Serial, "emulator-") }

// SetBattery sets the battery: level is 0-100 (nil leaves it) and charging
// toggles the AC line (nil leaves it). On an emulator it uses the console
// (`adb emu power`); on a PHYSICAL device it forces the values through the
// framework (`dumpsys battery set`), which persist until reset (below) or a
// reboot. reset restores automatic battery reporting (`dumpsys battery reset`)
// on either — call it to undo a forced state. At least one of level/charging
// must be set unless reset is true.
func (c *Client) SetBattery(ctx context.Context, level *int, charging *bool, reset bool) error {
	if reset {
		return c.resetBattery(ctx)
	}
	if level == nil && charging == nil {
		return fmt.Errorf("set a battery level and/or charging state (or pass reset)")
	}
	if level != nil && (*level < 0 || *level > 100) {
		return fmt.Errorf("battery level must be 0-100, got %d", *level)
	}
	// Record before the first mutating call: a set that fails half way (the
	// level applied, the charging state not) has still changed the device.
	NoteOverride(c.Serial, OverrideBattery)
	return c.applyBattery(ctx, level, charging)
}

// resetBattery restores automatic battery reporting and clears the override only
// once every step has been confirmed. On an emulator the values were set through
// the console (adb emu power), which `dumpsys battery reset` may not undo, so
// the console is also sent back to its automatic state (full, on AC).
func (c *Client) resetBattery(ctx context.Context) error {
	if _, err := c.adb(ctx, "shell", "dumpsys", "battery", "reset"); err != nil {
		return err
	}
	if c.isEmulator() {
		if _, err := c.emu(ctx, "power capacity", "power", "capacity", "100"); err != nil {
			return err
		}
		if _, err := c.emu(ctx, "power ac", "power", "ac", "on"); err != nil {
			return err
		}
	}
	ClearOverride(c.Serial, OverrideBattery)
	return nil
}

func (c *Client) applyBattery(ctx context.Context, level *int, charging *bool) error {
	if c.isEmulator() {
		if level != nil {
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

	// Physical device: force values via the framework. They stick until reset.
	if level != nil {
		if _, err := c.adb(ctx, "shell", "dumpsys", "battery", "set", "level", strconv.Itoa(*level)); err != nil {
			return err
		}
	}
	if charging != nil {
		v := "0"
		if *charging {
			v = "1"
		}
		if _, err := c.adb(ctx, "shell", "dumpsys", "battery", "set", "ac", v); err != nil {
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
