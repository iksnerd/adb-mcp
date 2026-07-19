package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iksnerd/adb_mcp/internal/adb"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Arguments ----

type listAVDsArgs struct{}

type bootArgs struct {
	AVD         string `json:"avd" jsonschema:"AVD name to boot (see list_avds)."`
	WaitForBoot *bool  `json:"wait_for_boot,omitempty" jsonschema:"Wait until fully booted before returning. Default true."`
	NoSnapshot  *bool  `json:"no_snapshot,omitempty" jsonschema:"Cold boot without loading a saved snapshot. Default true."`
	WipeData    *bool  `json:"wipe_data,omitempty" jsonschema:"Factory-reset the AVD on this boot (-wipe-data). Default false. Use to start from a pristine device."`
	TimeoutS    int    `json:"timeout_s,omitempty" jsonschema:"Boot timeout in seconds. Default 180."`
}

type waitBootArgs struct {
	serialArg
	TimeoutS int `json:"timeout_s,omitempty" jsonschema:"Timeout in seconds. Default 120."`
}

type connectWirelessArgs struct {
	HostPort    string `json:"host_port" jsonschema:"Device address to connect to, host:port (e.g. 192.168.1.42:5555)."`
	PairAddress string `json:"pair_address,omitempty" jsonschema:"Pairing address host:port (Android 11+ Wireless debugging), if different from host_port. Only needed when pairing."`
	PairingCode string `json:"pairing_code,omitempty" jsonschema:"6-digit pairing code shown on the device. Provide to pair before connecting."`
}

// ---- Handlers ----

func listAVDs(ctx context.Context, _ listAVDsArgs) (*mcp.CallToolResult, error) {
	avds, err := adb.ListAVDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(avds) == 0 {
		return text("No AVDs found. Create one in Android Studio's Device Manager."), nil
	}
	return text("Available AVDs:\n%s", strings.Join(avds, "\n")), nil
}

func bootEmulator(ctx context.Context, in bootArgs) (*mcp.CallToolResult, error) {
	if strings.TrimSpace(in.AVD) == "" {
		return nil, fmt.Errorf("avd is required")
	}
	timeout := 180 * time.Second
	if in.TimeoutS > 0 {
		timeout = time.Duration(in.TimeoutS) * time.Second
	}
	serial, err := adb.BootEmulator(ctx, in.AVD, boolOr(in.NoSnapshot, true), boolOr(in.WaitForBoot, true), boolOr(in.WipeData, false), timeout)
	if err != nil {
		if serial != "" {
			return nil, fmt.Errorf("emulator %s came up as %s but %w", in.AVD, serial, err)
		}
		return nil, err
	}
	return text("Booted %q as %s.", in.AVD, serial), nil
}

func listDevices(ctx context.Context, _ serialArg) (*mcp.CallToolResult, error) {
	devices, err := adb.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return text("No devices attached."), nil
	}
	return jsonResult(devices)
}

func waitForBoot(ctx context.Context, in waitBootArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(in.TimeoutS) * time.Second
	if err := c.WaitForBoot(ctx, timeout); err != nil {
		return nil, err
	}
	return text("%s is booted.", c.Serial), nil
}

func shutdownEmulator(ctx context.Context, in serialArg) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.Shutdown(ctx); err != nil {
		return nil, err
	}
	return text("Shutdown requested for %s.", c.Serial), nil
}

type fingerTouchArgs struct {
	serialArg
	FingerID int `json:"finger_id,omitempty" jsonschema:"Id of the enrolled finger to touch with (must match a finger enrolled in Settings). Default 1."`
}

func fingerTouch(ctx context.Context, in fingerTouchArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.FingerTouch(ctx, in.FingerID); err != nil {
		return nil, err
	}
	id := in.FingerID
	if id <= 0 {
		id = 1
	}
	return text("Simulated fingerprint touch (finger %d). If a BiometricPrompt was up it should resolve now — confirm with describe_ui (its top window line shows whether the prompt is gone).", id), nil
}

// ---- Extended Controls (emulator console) ----

func fingerRemove(ctx context.Context, in serialArg) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.FingerRemove(ctx); err != nil {
		return nil, err
	}
	return text("Lifted the simulated finger off the sensor on %s.", c.Serial), nil
}

func hasBiometricEnrolled(ctx context.Context, in serialArg) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	enrolled, count, err := c.HasBiometricEnrolled(ctx)
	if err != nil {
		return nil, err
	}
	if enrolled {
		return text("%d fingerprint(s) enrolled — fingerprint_touch can satisfy a BiometricPrompt on this device.", count), nil
	}
	return text("No fingerprint enrolled — fingerprint_touch will sit on \"Touch the sensor\" and never resolve. Enroll one first (Settings > Security > Fingerprint, calling fingerprint_touch for each wizard prompt), or drive the PIN path instead."), nil
}

type sendSMSArgs struct {
	serialArg
	From string `json:"from" jsonschema:"Sender phone number the SMS appears to come from, e.g. \"+15551234567\"."`
	Text string `json:"text" jsonschema:"Message body (e.g. an OTP code) delivered to the device's SMS inbox."`
}

func sendSMS(ctx context.Context, in sendSMSArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.SendSMS(ctx, in.From, in.Text); err != nil {
		return nil, err
	}
	return text("Delivered an SMS from %s to %s.", in.From, c.Serial), nil
}

type phoneCallArgs struct {
	serialArg
	Number string `json:"number" jsonschema:"Phone number for the call, e.g. \"+15551234567\"."`
	Action string `json:"action,omitempty" jsonschema:"What to do: \"call\" (default — ring an incoming call), \"accept\", \"cancel\" (hang up), \"busy\", or \"hold\"."`
}

func phoneCall(ctx context.Context, in phoneCallArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	action := in.Action
	if action == "" {
		action = "call"
	}
	if err := c.GSMCall(ctx, action, in.Number); err != nil {
		return nil, err
	}
	return text("Telephony %s for %s on %s.", action, in.Number, c.Serial), nil
}

type batteryArgs struct {
	serialArg
	Level    *int  `json:"level,omitempty" jsonschema:"Battery charge level 0-100. Omit to leave the level unchanged."`
	Charging *bool `json:"charging,omitempty" jsonschema:"true = plugged into AC, false = on battery. Omit to leave the charging state unchanged."`
}

func setBattery(ctx context.Context, in batteryArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.SetBattery(ctx, in.Level, in.Charging); err != nil {
		return nil, err
	}
	parts := []string{}
	if in.Level != nil {
		parts = append(parts, fmt.Sprintf("level %d%%", *in.Level))
	}
	if in.Charging != nil {
		parts = append(parts, map[bool]string{true: "charging", false: "on battery"}[*in.Charging])
	}
	return text("Battery set (%s) on %s.", strings.Join(parts, ", "), c.Serial), nil
}

func rotateScreen(ctx context.Context, in serialArg) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.Rotate(ctx); err != nil {
		return nil, err
	}
	return text("Rotated %s to its next orientation.", c.Serial), nil
}

type snapshotArgs struct {
	serialArg
	Action string `json:"action" jsonschema:"One of: \"save\", \"load\", \"delete\", or \"list\"."`
	Name   string `json:"name,omitempty" jsonschema:"Snapshot name. Required for save/load/delete; ignored for list."`
}

func avdSnapshot(ctx context.Context, in snapshotArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	out, err := c.Snapshot(ctx, in.Action, in.Name)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return text("Snapshot %s done on %s.", in.Action, c.Serial), nil
	}
	return text("%s", strings.TrimSpace(out)), nil
}

type cellularArgs struct {
	serialArg
	Data         string `json:"data,omitempty" jsonschema:"Mobile-data registration state: one of unregistered, home, roaming, searching, denied, off, on. Omit to leave unchanged."`
	Voice        string `json:"voice,omitempty" jsonschema:"Voice registration state: one of unregistered, home, roaming, searching, denied, off, on. Omit to leave unchanged."`
	Signal       *int   `json:"signal,omitempty" jsonschema:"Signal strength 0-4 (0 = no bars, 4 = full). Omit to leave unchanged."`
	NetworkSpeed string `json:"network_speed,omitempty" jsonschema:"Data throughput: a named profile (gsm, gprs, edge, umts, hsdpa, lte, evdo, full) or raw \"<up>:<down>\" in kbps. Omit to leave unchanged."`
	NetworkDelay string `json:"network_delay,omitempty" jsonschema:"Latency: a named profile (none, gprs, edge, umts) or raw \"<min>:<max>\" in ms. Omit to leave unchanged."`
}

func cellular(ctx context.Context, in cellularArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.Cellular(ctx, in.Data, in.Voice, in.Signal, in.NetworkSpeed, in.NetworkDelay); err != nil {
		return nil, err
	}
	parts := []string{}
	if in.Data != "" {
		parts = append(parts, "data "+in.Data)
	}
	if in.Voice != "" {
		parts = append(parts, "voice "+in.Voice)
	}
	if in.Signal != nil {
		parts = append(parts, fmt.Sprintf("signal %d", *in.Signal))
	}
	if in.NetworkSpeed != "" {
		parts = append(parts, "speed "+in.NetworkSpeed)
	}
	if in.NetworkDelay != "" {
		parts = append(parts, "delay "+in.NetworkDelay)
	}
	return text("Cellular set (%s) on %s.", strings.Join(parts, ", "), c.Serial), nil
}

type setSensorArgs struct {
	serialArg
	Sensor string   `json:"sensor" jsonschema:"Sensor name, e.g. acceleration, gyroscope, magnetic-field, orientation (3 values) or light, proximity, temperature, pressure, humidity (1 value)."`
	X      float64  `json:"x" jsonschema:"First value (the only value for single-axis sensors like light/proximity)."`
	Y      *float64 `json:"y,omitempty" jsonschema:"Second value for multi-axis sensors. Omit for single-value sensors."`
	Z      *float64 `json:"z,omitempty" jsonschema:"Third value for multi-axis sensors. Omit for single-value sensors."`
}

func setSensor(ctx context.Context, in setSensorArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	values := []float64{in.X}
	if in.Y != nil {
		values = append(values, *in.Y)
	}
	if in.Z != nil {
		values = append(values, *in.Z)
	}
	if err := c.SetSensor(ctx, in.Sensor, values); err != nil {
		return nil, err
	}
	return text("Set sensor %s on %s.", in.Sensor, c.Serial), nil
}

func connectWireless(ctx context.Context, in connectWirelessArgs) (*mcp.CallToolResult, error) {
	out, err := adb.ConnectWireless(ctx, in.HostPort, in.PairAddress, in.PairingCode)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, out)
	}
	return text("%s", out), nil
}

type adbReverseArgs struct {
	serialArg
	DevicePort int   `json:"device_port" jsonschema:"TCP port on the DEVICE to forward, e.g. 8081 for Metro."`
	HostPort   int   `json:"host_port,omitempty" jsonschema:"TCP port on the HOST to forward to. Defaults to device_port."`
	Remove     *bool `json:"remove,omitempty" jsonschema:"Remove the forward for device_port instead of creating it."`
}

func adbReverse(ctx context.Context, in adbReverseArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	remove := boolOr(in.Remove, false)
	if err := c.Reverse(ctx, in.DevicePort, in.HostPort, remove); err != nil {
		return nil, err
	}
	if remove {
		return text("Removed reverse forward for tcp:%d on %s.", in.DevicePort, c.Serial), nil
	}
	host := in.HostPort
	if host <= 0 {
		host = in.DevicePort
	}
	return text("Device port tcp:%d now reaches host tcp:%d on %s. An already-running app may need a restart (or reload_app) to pick up the connection.", in.DevicePort, host, c.Serial), nil
}
