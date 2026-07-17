package adb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// StatusBarOptions configures StatusBarDemo. Zero-value fields leave the
// corresponding status bar element unchanged (clock/battery) or fall back to
// the clean default (network/notifications) documented per field below.
type StatusBarOptions struct {
	// Clock is "HHMM" (e.g. "1200"); empty leaves it unchanged.
	Clock string
	// Battery is 0-100, or nil to leave it unchanged.
	Battery *int
	// NetworkType selects the network icon: "" or "wifi" (default, full
	// signal), "mobile" (signal/data-type/carrier below apply), or "none"
	// (hide both wifi and mobile icons).
	NetworkType string
	// MobileLevel is 0-4 signal bars, used only when NetworkType is "mobile".
	// Defaults to 4 (full signal) when nil.
	MobileLevel *int
	// DataType is the data-type icon shown next to the mobile signal, e.g.
	// "lte", "4g", "5g", "3g", "edge", "1x", "h", "h+", "roam". Used only
	// when NetworkType is "mobile".
	DataType string
	// Carrier is the operator name shown in the status bar. Used only when
	// NetworkType is "mobile".
	Carrier string
	// NotificationsVisible shows/hides notification icons. Defaults to false
	// (hidden, for clean screenshots) when nil.
	NotificationsVisible *bool
	// NotificationIcon is a best-effort AOSP SystemUI icon resource name shown
	// in the first notification slot. Support for this is an obscure,
	// version-dependent SystemUI internal (DemoStatusIcons) and may silently
	// no-op on some SDK images, unlike the other fields here.
	NotificationIcon string
}

// validate checks every field up front so StatusBarDemo can reject a bad
// request before it sends any broadcast — otherwise an invalid later field
// (e.g. network_type) would leave the bar partway configured (demo entered,
// clock/battery already set).
func (opts StatusBarOptions) validate() error {
	if opts.Clock != "" && (len(opts.Clock) != 4 || !isAllDigits(opts.Clock)) {
		return fmt.Errorf("clock must be 4 digits HHMM (e.g. \"0930\"), got %q", opts.Clock)
	}
	if opts.Battery != nil && (*opts.Battery < 0 || *opts.Battery > 100) {
		return fmt.Errorf("battery must be 0-100, got %d", *opts.Battery)
	}
	if opts.MobileLevel != nil && (*opts.MobileLevel < 0 || *opts.MobileLevel > 4) {
		return fmt.Errorf("mobile_level must be 0-4, got %d", *opts.MobileLevel)
	}
	switch opts.NetworkType {
	case "", "wifi", "mobile", "none":
	default:
		return fmt.Errorf("network_type must be \"wifi\", \"mobile\", or \"none\", got %q", opts.NetworkType)
	}
	return nil
}

// StatusBarDemo drives SystemUI "demo mode", which pins a clean, deterministic
// status bar — fixed clock, chosen signal/battery, no notification icons by
// default — so screenshots for docs don't leak the wall clock or a random
// signal state. enter=false exits demo mode and restores the live status bar
// (opts is ignored in that case).
//
// Requires the demo-mode broadcast to be allowed, which this enables first
// (settings global sysui_demo_allowed=1).
func (c *Client) StatusBarDemo(ctx context.Context, enter bool, opts StatusBarOptions) error {
	if enter {
		if err := opts.validate(); err != nil {
			return err
		}
	}
	if _, err := c.adb(ctx, "shell", "settings", "put", "global", "sysui_demo_allowed", "1"); err != nil {
		return err
	}
	if !enter {
		return c.demoBroadcast(ctx, "exit")
	}
	if err := c.demoBroadcast(ctx, "enter"); err != nil {
		return err
	}
	if opts.Clock != "" {
		if err := c.demoBroadcast(ctx, "clock", "hhmm", opts.Clock); err != nil {
			return err
		}
	}
	if opts.Battery != nil {
		if err := c.demoBroadcast(ctx, "battery", "level", strconv.Itoa(*opts.Battery), "plugged", "false"); err != nil {
			return err
		}
	}
	if err := c.networkBroadcast(ctx, opts); err != nil {
		return err
	}
	visible := "false"
	if opts.NotificationsVisible != nil && *opts.NotificationsVisible {
		visible = "true"
	}
	kv := []string{"visible", visible}
	if opts.NotificationIcon != "" {
		kv = append(kv, "icon1", opts.NotificationIcon)
	}
	return c.demoBroadcast(ctx, "notifications", kv...)
}

// networkBroadcast sends the "network" demo command for the selected
// NetworkType. Default ("" or "wifi") is full wifi signal; "mobile" applies
// signal level/data-type/carrier; "none" hides both network icons.
func (c *Client) networkBroadcast(ctx context.Context, opts StatusBarOptions) error {
	switch opts.NetworkType {
	case "", "wifi":
		return c.demoBroadcast(ctx, "network", "wifi", "show", "level", "4")
	case "mobile":
		level := 4
		if opts.MobileLevel != nil {
			level = *opts.MobileLevel
		}
		kv := []string{"mobile", "show", "level", strconv.Itoa(level)}
		if opts.DataType != "" {
			kv = append(kv, "datatype", opts.DataType)
		}
		if opts.Carrier != "" {
			kv = append(kv, "carriername", opts.Carrier)
		}
		return c.demoBroadcast(ctx, "network", kv...)
	case "none":
		return c.demoBroadcast(ctx, "network", "wifi", "hide", "mobile", "hide")
	default:
		return fmt.Errorf("network_type must be \"wifi\", \"mobile\", or \"none\", got %q", opts.NetworkType)
	}
}

// demoBroadcast sends one SystemUI demo-mode command. Extra args are key/value
// pairs appended as `-e key value` (e.g. command=clock, hhmm=1200).
func (c *Client) demoBroadcast(ctx context.Context, command string, kv ...string) error {
	args := []string{"shell", "am", "broadcast", "-a", "com.android.systemui.demo", "-e", "command", command}
	for i := 0; i+1 < len(kv); i += 2 {
		args = append(args, "-e", kv[i], kv[i+1])
	}
	_, err := c.adb(ctx, args...)
	return err
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	return strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) == -1
}
