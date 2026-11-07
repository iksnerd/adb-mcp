package adb

import (
	"context"
	"reflect"
	"testing"
)

func TestResolveSerialTrimsExplicitSerial(t *testing.T) {
	got, err := ResolveSerial(context.Background(), "  emulator-5554 \t")
	if err != nil || got != "emulator-5554" {
		t.Fatalf("ResolveSerial = %q, %v; want emulator-5554", got, err)
	}
}

func TestParseDevices(t *testing.T) {
	out := "* daemon not running; starting now at tcp:5037\n" +
		"* daemon started successfully\n" +
		"List of devices attached\n" +
		"emulator-5554\tdevice\n" +
		"R5CT123\tno permissions (user in plugdev group; are your udev rules wrong?); see [http://developer.android.com/tools/device.html]\n" +
		"0123456789\tunauthorized\n" +
		"no-tab-line device\n"
	want := []Device{
		{Serial: "emulator-5554", State: "device"},
		{Serial: "R5CT123", State: "no permissions (user in plugdev group; are your udev rules wrong?); see [http://developer.android.com/tools/device.html]"},
		{Serial: "0123456789", State: "unauthorized"},
	}
	if got := parseDevices(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDevices =\n %+v\nwant\n %+v", got, want)
	}
}
