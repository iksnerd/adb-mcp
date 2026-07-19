package adb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeRun is a Runner that records every adb argv it is handed and replies with
// a canned stdout/err — the seam that lets us assert what command a builder
// produces without a device.
type fakeRun struct {
	calls [][]string
	reply string
	err   error
}

func (f *fakeRun) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return []byte(f.reply), f.err
}

func newFake(reply string) (*Client, *fakeRun) {
	f := &fakeRun{reply: reply}
	return &Client{Serial: "emulator-5554", run: f.run}, f
}

// last returns the argv of the most recent adb call.
func (f *fakeRun) last() []string {
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

func wantArgv(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv =\n  %q\nwant\n  %q", got, want)
	}
}

func TestInputBuilders(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		call func(*Client) error
		want []string
	}{
		{"tap", func(c *Client) error { return c.Tap(ctx, 10, 20) },
			[]string{"shell", "input", "tap", "10", "20"}},
		{"swipe default duration", func(c *Client) error { return c.Swipe(ctx, 1, 2, 3, 4, 0) },
			[]string{"shell", "input", "swipe", "1", "2", "3", "4", "300"}},
		{"drag default duration", func(c *Client) error { return c.Drag(ctx, 1, 2, 3, 4, 0) },
			[]string{"shell", "input", "draganddrop", "1", "2", "3", "4", "400"}},
		{"long press same point", func(c *Client) error { return c.LongPress(ctx, 5, 6, 0) },
			[]string{"shell", "input", "swipe", "5", "6", "5", "6", "600"}},
		{"key combo", func(c *Client) error { return c.KeyCombo(ctx, []int{113, 29}) },
			[]string{"shell", "input", "keycombination", "113", "29"}},
		{"press key", func(c *Client) error { return c.PressKey(ctx, 4) },
			[]string{"shell", "input", "keyevent", "4"}},
		{"dev menu is keycode 82", func(c *Client) error { return c.OpenDevMenu(ctx) },
			[]string{"shell", "input", "keyevent", "82"}},
		{"dark mode on", func(c *Client) error { return c.SetDarkMode(ctx, true) },
			[]string{"shell", "cmd", "uimode", "night", "yes"}},
		{"dark mode off", func(c *Client) error { return c.SetDarkMode(ctx, false) },
			[]string{"shell", "cmd", "uimode", "night", "no"}},
		{"grant permission", func(c *Client) error { return c.GrantPermission(ctx, "com.x", "android.permission.CAMERA") },
			[]string{"shell", "pm", "grant", "com.x", "android.permission.CAMERA"}},
		{"stay awake on", func(c *Client) error { return c.StayAwake(ctx, true) },
			[]string{"shell", "svc", "power", "stayon", "true"}},
		{"stay awake off", func(c *Client) error { return c.StayAwake(ctx, false) },
			[]string{"shell", "svc", "power", "stayon", "false"}},
		{"reverse create defaults host to device", func(c *Client) error { return c.Reverse(ctx, 8081, 0, false) },
			[]string{"reverse", "tcp:8081", "tcp:8081"}},
		{"reverse remove", func(c *Client) error { return c.Reverse(ctx, 8081, 0, true) },
			[]string{"reverse", "--remove", "tcp:8081"}},
		{"lock defaults to pin", func(c *Client) error { return c.SetDeviceLock(ctx, "", "1234", "") },
			[]string{"shell", "locksettings", "set-pin", "1234"}},
		{"lock change supplies old", func(c *Client) error { return c.SetDeviceLock(ctx, "pin", "1234", "0000") },
			[]string{"shell", "locksettings", "set-pin", "--old", "0000", "1234"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, f := newFake("")
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			wantArgv(t, f.last(), tc.want)
		})
	}
}

// TestInputTextEscaping locks in the shell-quoting: a bare string with a space
// and an embedded single quote must reach the device as one single-quoted arg.
func TestInputTextEscaping(t *testing.T) {
	c, f := newFake("")
	if err := c.InputText(context.Background(), "a b'c"); err != nil {
		t.Fatal(err)
	}
	want := []string{"shell", "input", "text", `'a b'\''c'`}
	wantArgv(t, f.last(), want)
}

// TestSetLocationLonLatOrder guards the easy-to-flip argument order: adb's
// `emu geo fix` takes longitude first, then latitude.
func TestSetLocationLonLatOrder(t *testing.T) {
	c, f := newFake("")
	if err := c.SetLocation(context.Background(), 13.4, 52.5); err != nil { // Berlin
		t.Fatal(err)
	}
	wantArgv(t, f.last(), []string{"emu", "geo", "fix", "13.4", "52.5"})
}

func TestKeyComboNeedsTwoKeys(t *testing.T) {
	c, _ := newFake("")
	if err := c.KeyCombo(context.Background(), []int{29}); err == nil {
		t.Error("expected an error for a single-key combo")
	}
}

// TestFingerTouch covers the three interesting outcomes: rejects a physical
// device, defaults finger id to 1, and turns a console KO: reply into an error.
func TestFingerTouch(t *testing.T) {
	ctx := context.Background()

	phys := &Client{Serial: "1a2b3c4d", run: (&fakeRun{reply: "OK"}).run}
	if err := phys.FingerTouch(ctx, 1); err == nil {
		t.Error("expected fingerprint on a non-emulator serial to be rejected")
	}

	c, f := newFake("OK")
	if err := c.FingerTouch(ctx, 0); err != nil {
		t.Fatalf("finger touch: %v", err)
	}
	wantArgv(t, f.last(), []string{"emu", "finger", "touch", "1"}) // 0 → default 1

	ko, _ := newFake("KO: no enrolled finger")
	if err := ko.FingerTouch(ctx, 2); err == nil {
		t.Error("expected a KO: console reply to surface as an error")
	}
}

// TestSetBatteryPaths pins the emulator-vs-physical split: an emulator uses the
// console (emu power), a physical serial forces values via dumpsys battery, and
// reset restores automatic reporting on either.
func TestSetBatteryPaths(t *testing.T) {
	ctx := context.Background()
	lvl := 20
	charging := false

	// Physical device → dumpsys battery set.
	phys := func() (*Client, *fakeRun) {
		f := &fakeRun{reply: ""}
		return &Client{Serial: "1a2b3c4d", run: f.run}, f
	}
	c, f := phys()
	if err := c.SetBattery(ctx, &lvl, nil, false); err != nil {
		t.Fatal(err)
	}
	wantArgv(t, f.last(), []string{"shell", "dumpsys", "battery", "set", "level", "20"})

	c, f = phys()
	if err := c.SetBattery(ctx, nil, &charging, false); err != nil {
		t.Fatal(err)
	}
	wantArgv(t, f.last(), []string{"shell", "dumpsys", "battery", "set", "ac", "0"})

	// reset → dumpsys battery reset (works on emulator serial too).
	c, f = newFake("")
	if err := c.SetBattery(ctx, nil, nil, true); err != nil {
		t.Fatal(err)
	}
	wantArgv(t, f.last(), []string{"shell", "dumpsys", "battery", "reset"})
}

// TestInstallAppFailureScan pins the regression fix: adb that prints
// "Failure [...]" with a zero exit still yields an error.
func TestInstallAppFailureScan(t *testing.T) {
	ctx := context.Background()
	apk := filepath.Join(t.TempDir(), "app.apk")
	if err := os.WriteFile(apk, []byte("not-a-real-apk"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, f := newFake("Success")
	if _, err := ok.InstallApp(ctx, apk); err != nil {
		t.Fatalf("clean install: %v", err)
	}
	wantArgv(t, f.last(), []string{"install", "-r", apk})

	bad, _ := newFake("Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE]")
	if _, err := bad.InstallApp(ctx, apk); err == nil {
		t.Error("expected exit-0 Failure output to be treated as an install failure")
	}

	missing, _ := newFake("Success")
	if _, err := missing.InstallApp(ctx, filepath.Join(t.TempDir(), "nope.apk")); err == nil {
		t.Error("expected a missing apk path to error before shelling out")
	}
}

// TestConsoleControls covers the emulator-console (Extended Controls) builders:
// each must produce the right `adb emu ...` argv.
func TestConsoleControls(t *testing.T) {
	ctx := context.Background()
	lvl := 15
	charging := true
	cases := []struct {
		name string
		call func(*Client) error
		want []string
	}{
		{"send sms", func(c *Client) error { return c.SendSMS(ctx, "+15551234567", "code 123") },
			[]string{"emu", "sms", "send", "+15551234567", "code 123"}},
		{"phone call rings", func(c *Client) error { return c.GSMCall(ctx, "call", "+15550000") },
			[]string{"emu", "gsm", "call", "+15550000"}},
		{"phone accept", func(c *Client) error { return c.GSMCall(ctx, "accept", "+15550000") },
			[]string{"emu", "gsm", "accept", "+15550000"}},
		{"battery level", func(c *Client) error { return c.SetBattery(ctx, &lvl, nil, false) },
			[]string{"emu", "power", "capacity", "15"}},
		{"battery charging", func(c *Client) error { return c.SetBattery(ctx, nil, &charging, false) },
			[]string{"emu", "power", "ac", "on"}},
		{"rotate", func(c *Client) error { return c.Rotate(ctx) },
			[]string{"emu", "rotate"}},
		{"finger remove", func(c *Client) error { return c.FingerRemove(ctx) },
			[]string{"emu", "finger", "remove"}},
		{"snapshot save", func(c *Client) error { _, err := c.Snapshot(ctx, "save", "clean"); return err },
			[]string{"emu", "avd", "snapshot", "save", "clean"}},
		{"snapshot list ignores name", func(c *Client) error { _, err := c.Snapshot(ctx, "list", ""); return err },
			[]string{"emu", "avd", "snapshot", "list"}},
		{"cellular data state", func(c *Client) error { return c.Cellular(ctx, "roaming", "", nil, "", "") },
			[]string{"emu", "gsm", "data", "roaming"}},
		{"cellular signal profile", func(c *Client) error { sig := 2; return c.Cellular(ctx, "", "", &sig, "", "") },
			[]string{"emu", "gsm", "signal-profile", "2"}},
		{"cellular network speed", func(c *Client) error { return c.Cellular(ctx, "", "", nil, "lte", "") },
			[]string{"emu", "network", "speed", "lte"}},
		{"cellular network delay", func(c *Client) error { return c.Cellular(ctx, "", "", nil, "", "200:400") },
			[]string{"emu", "network", "delay", "200:400"}},
		{"sensor three axes colon-joined", func(c *Client) error { return c.SetSensor(ctx, "acceleration", []float64{0, 9.8, 0}) },
			[]string{"emu", "sensor", "set", "acceleration", "0:9.8:0"}},
		{"sensor single value", func(c *Client) error { return c.SetSensor(ctx, "light", []float64{100}) },
			[]string{"emu", "sensor", "set", "light", "100"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, f := newFake("OK")
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			wantArgv(t, f.last(), tc.want)
		})
	}
}

// TestConsoleValidation covers the guards: emulator-only, bad enums, missing
// args, and a console KO: reply.
func TestConsoleValidation(t *testing.T) {
	ctx := context.Background()

	// A physical device has no console.
	phys := &Client{Serial: "1a2b3c4d", run: (&fakeRun{reply: "OK"}).run}
	if err := phys.SendSMS(ctx, "+1555", "hi"); err == nil {
		t.Error("expected send_sms on a non-emulator to be rejected")
	}

	c, _ := newFake("OK")
	if err := c.GSMCall(ctx, "explode", "+1555"); err == nil {
		t.Error("expected an unknown call action to be rejected")
	}
	if err := c.SetBattery(ctx, nil, nil, false); err == nil {
		t.Error("expected set_battery with no fields to be rejected")
	}
	bad := 150
	if err := c.SetBattery(ctx, &bad, nil, false); err == nil {
		t.Error("expected an out-of-range battery level to be rejected")
	}
	if _, err := c.Snapshot(ctx, "save", ""); err == nil {
		t.Error("expected a nameless save to be rejected")
	}
	if err := c.Cellular(ctx, "", "", nil, "", ""); err == nil {
		t.Error("expected cellular with no fields to be rejected")
	}
	if err := c.Cellular(ctx, "flying", "", nil, "", ""); err == nil {
		t.Error("expected an unknown data state to be rejected")
	}
	badSig := 7
	if err := c.Cellular(ctx, "", "", &badSig, "", ""); err == nil {
		t.Error("expected an out-of-range signal to be rejected")
	}
	if err := c.SetSensor(ctx, "", []float64{1}); err == nil {
		t.Error("expected an empty sensor name to be rejected")
	}
	if err := c.SetSensor(ctx, "acceleration", nil); err == nil {
		t.Error("expected zero sensor values to be rejected")
	}
	if err := c.SetSensor(ctx, "acceleration", []float64{1, 2, 3, 4}); err == nil {
		t.Error("expected more than three sensor values to be rejected")
	}

	ko, _ := newFake("KO: no finger enrolled")
	if err := ko.FingerRemove(ctx); err == nil {
		t.Error("expected a KO: console reply to surface as an error")
	}
}

// TestIsDeviceSecure covers the verify-first logic: an empty-credential verify
// that succeeds proves NO lock is set (not secure), while a device that rejects
// it falls through to the get-disabled heuristic.
func TestIsDeviceSecure(t *testing.T) {
	ctx := context.Background()

	// Empty credential verifies => no secure lock.
	noLock, _ := newFake("Lock credential verified successfully")
	if secure, err := noLock.IsDeviceSecure(ctx); err != nil || secure {
		t.Errorf("empty-verify success: secure=%v err=%v, want false/nil", secure, err)
	}

	// Locked device: `verify` rejects the empty credential (non-zero exit), so we
	// fall through to get-disabled, which reports the lockscreen is not disabled
	// => secure. A runner keyed on the subcommand models the two calls.
	locked := &Client{Serial: "emulator-5554", run: func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[1] == "verify" {
			return []byte("Old password '' didn't match"), errors.New("exit status 1")
		}
		return []byte("false"), nil // get-disabled: not disabled
	}}
	if secure, err := locked.IsDeviceSecure(ctx); err != nil || !secure {
		t.Errorf("locked device: secure=%v err=%v, want true/nil", secure, err)
	}
}

// TestExpoDevClientURL pins the deep-link format and its defaults; the scheme
// is required and a trailing "://" on it is tolerated.
func TestExpoDevClientURL(t *testing.T) {
	got, err := ExpoDevClientURL("myapp", "", 0)
	if err != nil {
		t.Fatalf("default url: %v", err)
	}
	want := "myapp://expo-development-client/?url=http%3A%2F%2Flocalhost%3A8081"
	if got != want {
		t.Errorf("url = %q, want %q", got, want)
	}

	got, err = ExpoDevClientURL("myapp://", "192.168.1.10", 19000)
	if err != nil {
		t.Fatalf("explicit host/port: %v", err)
	}
	want = "myapp://expo-development-client/?url=http%3A%2F%2F192.168.1.10%3A19000"
	if got != want {
		t.Errorf("url = %q, want %q", got, want)
	}

	if _, err := ExpoDevClientURL("", "", 0); err == nil {
		t.Error("expected an empty scheme to be rejected")
	}
}

// TestLaunchApp checks both the component parse and the no-launchable-activity
// path (monkey prints that and exits, and we turn it into a clear error).
func TestLaunchApp(t *testing.T) {
	ctx := context.Background()

	ok, _ := newFake("bla bla cmp=com.example/.MainActivity bla")
	component, err := ok.LaunchApp(ctx, "com.example")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if component != "com.example/.MainActivity" {
		t.Errorf("component = %q, want com.example/.MainActivity", component)
	}

	noAct, _ := newFake("** No activities found to run, monkey aborted.")
	if _, err := noAct.LaunchApp(ctx, "com.example"); err == nil || !strings.Contains(err.Error(), "no launchable activity") {
		t.Errorf("expected a no-launchable-activity error, got %v", err)
	}
}
