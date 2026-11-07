package adb

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var errFake = errors.New("boom")

func fastBootPolling(t *testing.T) {
	t.Helper()
	oldPoll, oldGrace, oldRetry := bootPollInterval, bootNameGracePolls, avdNameRetryDelay
	bootPollInterval, bootNameGracePolls, avdNameRetryDelay = 10*time.Millisecond, 3, time.Millisecond
	t.Cleanup(func() { bootPollInterval, bootNameGracePolls, avdNameRetryDelay = oldPoll, oldGrace, oldRetry })
}

// A physical device that shows up while the emulator boots is not the emulator.
func TestBootEmulatorIgnoresNewNonEmulatorDevices(t *testing.T) {
	fastBootPolling(t)
	f := newFakeSDK(t)
	f.set(t, "after_launch", "R58ABC\tdevice\nemulator-5556\tdevice\n")
	f.set(t, "avd_emulator-5556", "Pixel_10a\n")

	serial, err := BootEmulator(context.Background(), "Pixel_10a", boolPtr(false), false, false, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if serial != "emulator-5556" {
		t.Errorf("serial = %q, want emulator-5556 (not the physical device)", serial)
	}
}

// Another emulator of a different AVD appearing at the same time is skipped
// when its name is known to differ.
func TestBootEmulatorSkipsNewEmulatorOfAnotherAVD(t *testing.T) {
	fastBootPolling(t)
	f := newFakeSDK(t)
	f.set(t, "after_launch", "emulator-5556\tdevice\nemulator-5558\tdevice\n")
	f.set(t, "avd_emulator-5556", "Other_AVD\n")
	f.set(t, "avd_emulator-5558", "Pixel_10a\n")

	serial, err := BootEmulator(context.Background(), "Pixel_10a", boolPtr(false), false, false, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if serial != "emulator-5558" {
		t.Errorf("serial = %q, want emulator-5558 (AVD match)", serial)
	}
}

// An emulator whose console has not come up reports no AVD name. It must not be
// treated as "not this AVD": launching a second emulator would fight it for the
// AVD lock. The error names the device.
func TestBootEmulatorRefusesToLaunchBesideAnUnnamedEmulator(t *testing.T) {
	fastBootPolling(t)
	f := newFakeSDK(t)
	f.set(t, "devices", "emulator-5554\toffline\n") // no avd_ file: name unknown

	_, err := BootEmulator(context.Background(), "Pixel_10a", boolPtr(false), false, false, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "emulator-5554") {
		t.Fatalf("want an error naming emulator-5554, got %v", err)
	}
	if f.launched() {
		t.Error("an emulator was launched beside one whose AVD is unknown")
	}
}

// pm answering is the point where installs and launches work; sys.boot_completed
// alone is too early.
func TestWaitForBootAlsoNeedsPackageManager(t *testing.T) {
	pmCalls := 0
	c := &Client{Serial: "emulator-5554", run: func(_ context.Context, args ...string) ([]byte, error) {
		j := strings.Join(args, " ")
		switch {
		case strings.Contains(j, "sys.boot_completed"):
			return []byte("1\n"), nil
		case strings.Contains(j, "pm path android"):
			pmCalls++
			if pmCalls < 2 {
				return []byte("Error: Could not access the Package Manager"), errFake
			}
			return []byte("package:/system/framework/framework-res.apk\n"), nil
		}
		return nil, nil
	}}
	oldPoll := waitForBootPoll
	waitForBootPoll = time.Millisecond
	defer func() { waitForBootPoll = oldPoll }()

	if err := c.WaitForBoot(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if pmCalls < 2 {
		t.Errorf("returned after %d pm probes, before package manager answered", pmCalls)
	}
}

func TestWaitForBootTimesOutWhenPackageManagerNeverAnswers(t *testing.T) {
	c := &Client{Serial: "emulator-5554", run: func(_ context.Context, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "sys.boot_completed") {
			return []byte("1\n"), nil
		}
		return nil, errFake
	}}
	oldPoll := waitForBootPoll
	waitForBootPoll = time.Millisecond
	defer func() { waitForBootPoll = oldPoll }()

	if err := c.WaitForBoot(context.Background(), 50*time.Millisecond); err == nil {
		t.Fatal("want a timeout: package manager never answered")
	}
}
