package adb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Device is one entry from `adb devices`.
type Device struct {
	Serial string `json:"serial"`
	State  string `json:"state"` // "device", "offline", "unauthorized", ...
}

// ListDevices parses `adb devices`.
func ListDevices(ctx context.Context) ([]Device, error) {
	out, err := runAdb(ctx, "", "devices")
	if err != nil {
		return nil, err
	}
	var devices []Device
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			devices = append(devices, Device{Serial: fields[0], State: fields[1]})
		}
	}
	return devices, nil
}

// ResolveSerial returns the serial to target. If serial is set it is used
// as-is. Otherwise, if exactly one device is in the "device" state that one is
// chosen; any other count yields an actionable error.
func ResolveSerial(ctx context.Context, serial string) (string, error) {
	if strings.TrimSpace(serial) != "" {
		return serial, nil
	}
	devices, err := ListDevices(ctx)
	if err != nil {
		return "", err
	}
	var ready []string
	for _, d := range devices {
		if d.State == "device" {
			ready = append(ready, d.Serial)
		}
	}
	switch len(ready) {
	case 1:
		return ready[0], nil
	case 0:
		return "", fmt.Errorf("no ready device attached; boot an emulator first (list_devices to check)")
	default:
		return "", fmt.Errorf("multiple devices attached (%s); pass the 'serial' argument to pick one", strings.Join(ready, ", "))
	}
}

// ConnectWireless connects to a device over TCP/IP. If pairingCode is set it
// first runs `adb pair` (Android 11+ wireless debugging shows a host:port and a
// 6-digit code under Developer options), then `adb connect`. hostPort is
// host:port; for pairing that is the *pairing* port, and pairPort/connect may
// differ — pass the connect address as hostPort and, when pairing is needed,
// pairAddr as the pairing host:port.
func ConnectWireless(ctx context.Context, hostPort, pairAddr, pairingCode string) (string, error) {
	var out strings.Builder
	if strings.TrimSpace(pairingCode) != "" {
		addr := pairAddr
		if addr == "" {
			addr = hostPort
		}
		res, err := runAdb(ctx, "", "pair", addr, pairingCode)
		out.WriteString(res)
		out.WriteByte('\n')
		if err != nil {
			return out.String(), err
		}
	}
	res, err := runAdb(ctx, "", "connect", hostPort)
	out.WriteString(res)
	return strings.TrimSpace(out.String()), err
}

// Reverse forwards a device-side TCP port to a host-side one
// (adb reverse tcp:<devicePort> tcp:<hostPort>), so the device can reach a
// server running on the host — e.g. tcp:8081→tcp:8081 lets an RN/Expo dev
// client reach Metro. Without it, a dev client may SILENTLY fall back to its
// embedded bundle and ignore every edit. remove=true undoes the forward.
func (c *Client) Reverse(ctx context.Context, devicePort, hostPort int, remove bool) error {
	if devicePort <= 0 {
		return fmt.Errorf("device_port must be positive, got %d", devicePort)
	}
	dev := "tcp:" + strconv.Itoa(devicePort)
	if remove {
		_, err := c.adb(ctx, "reverse", "--remove", dev)
		return err
	}
	if hostPort <= 0 {
		hostPort = devicePort
	}
	_, err := c.adb(ctx, "reverse", dev, "tcp:"+strconv.Itoa(hostPort))
	return err
}
