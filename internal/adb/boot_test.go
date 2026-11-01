package adb

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeSDK installs stub adb and emulator scripts under a temp ANDROID_HOME so
// BootEmulator runs its real code against scripted devices. The state dir holds:
//
//	devices         lines attached from the start ("emulator-5554<TAB>device")
//	after_launch    lines that appear once the emulator binary has been run
//	avd_<serial>    the AVD name that serial reports for `emu avd name`
//	launched        written by the stub emulator; absent means it never ran
type fakeSDK struct{ state string }

func newFakeSDK(t *testing.T) fakeSDK {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell stubs for adb and emulator")
	}
	root, state := t.TempDir(), t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("platform-tools/adb", `#!/bin/sh
S="$FAKE_STATE"
if [ "$1" = "-s" ]; then SER="$2"; shift 2; fi
case "$1" in
  devices)
    echo "List of devices attached"
    [ -f "$S/devices" ] && cat "$S/devices"
    [ -f "$S/launched" ] && [ -f "$S/after_launch" ] && cat "$S/after_launch"
    exit 0 ;;
  emu)
    if [ "$2" = "avd" ] && [ "$3" = "name" ]; then
      [ -f "$S/avd_$SER" ] && cat "$S/avd_$SER"
      echo OK
    fi
    exit 0 ;;
  shell) echo 1; exit 0 ;;
esac
exit 0
`)
	write("emulator/emulator", `#!/bin/sh
echo "$@" > "$FAKE_STATE/launched"
exit 0
`)
	t.Setenv("ANDROID_HOME", root)
	t.Setenv("FAKE_STATE", state)
	return fakeSDK{state: state}
}

func (f fakeSDK) set(t *testing.T, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.state, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f fakeSDK) launched() bool {
	_, err := os.Stat(filepath.Join(f.state, "launched"))
	return err == nil
}

// The reported failure: the AVD was already up as emulator-5554 when boot_emulator
// ran, so it was in the "before" snapshot and could never count as new. The tool
// waited out its whole timeout and reported failure for a usable device.
func TestBootEmulatorReusesARunningEmulatorOfThatAVD(t *testing.T) {
	f := newFakeSDK(t)
	f.set(t, "devices", "emulator-5554\tdevice\n")
	f.set(t, "avd_emulator-5554", "Pixel_10a\n")

	start := time.Now()
	serial, err := BootEmulator(context.Background(), "Pixel_10a", false, true, false, 20*time.Second)
	if err != nil {
		t.Fatalf("a running emulator of that AVD is a success, got: %v", err)
	}
	if serial != "emulator-5554" {
		t.Errorf("serial = %q, want emulator-5554", serial)
	}
	if f.launched() {
		t.Error("a second emulator was launched beside the running one")
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("took %s: it should return at once, not wait out the timeout", took)
	}
}

func TestBootEmulatorStartsAnotherAVDBesideARunningOne(t *testing.T) {
	f := newFakeSDK(t)
	f.set(t, "devices", "emulator-5554\tdevice\n")
	f.set(t, "avd_emulator-5554", "Other_AVD\n")
	f.set(t, "after_launch", "emulator-5556\tdevice\n")
	f.set(t, "avd_emulator-5556", "Pixel_10a\n")

	serial, err := BootEmulator(context.Background(), "Pixel_10a", false, true, false, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if serial != "emulator-5556" {
		t.Errorf("serial = %q, want the new emulator-5556", serial)
	}
	if !f.launched() {
		t.Error("the emulator was never launched")
	}
}

// no_snapshot and wipe_data ask for a different boot. Quietly returning the
// running emulator would ignore that request, so it is refused with the serial
// and the way out.
func TestBootEmulatorRefusesAFreshBootOfARunningAVD(t *testing.T) {
	for name, args := range map[string][2]bool{"no_snapshot": {true, false}, "wipe_data": {false, true}} {
		t.Run(name, func(t *testing.T) {
			f := newFakeSDK(t)
			f.set(t, "devices", "emulator-5554\tdevice\n")
			f.set(t, "avd_emulator-5554", "Pixel_10a\n")

			_, err := BootEmulator(context.Background(), "Pixel_10a", args[0], true, args[1], 5*time.Second)
			if err == nil {
				t.Fatal("want an error: the AVD is already running")
			}
			for _, want := range []string{"already running", "emulator-5554", "shutdown_emulator"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should mention %q", err, want)
				}
			}
			if f.launched() {
				t.Error("an emulator was launched anyway")
			}
		})
	}
}

// When nothing new appears, the caller needs to know what IS attached, or it
// boots a second emulator or gives up on a device that works.
func TestBootEmulatorTimeoutNamesWhatIsAttached(t *testing.T) {
	f := newFakeSDK(t)
	f.set(t, "devices", "emulator-5554\tdevice\n")
	f.set(t, "avd_emulator-5554", "Other_AVD\n")

	_, err := BootEmulator(context.Background(), "Pixel_10a", false, true, false, 2*time.Second)
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if !strings.Contains(err.Error(), "emulator-5554 (device, Other_AVD)") {
		t.Errorf("error should list what is attached, got: %v", err)
	}
}
