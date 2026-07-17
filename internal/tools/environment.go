package tools

import (
	"context"

	"github.com/iksnerd/adb_mcp/internal/adb"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Arguments ----

type darkModeArgs struct {
	serialArg
	Enabled bool `json:"enabled" jsonschema:"true = dark theme on, false = off."`
}

type locationArgs struct {
	serialArg
	Longitude float64 `json:"longitude" jsonschema:"Longitude of the mock GPS fix."`
	Latitude  float64 `json:"latitude" jsonschema:"Latitude of the mock GPS fix."`
}

type statusBarArgs struct {
	serialArg
	Enabled              bool   `json:"enabled" jsonschema:"true = enter demo mode (clean, fixed status bar); false = exit and restore the live status bar."`
	Clock                string `json:"clock,omitempty" jsonschema:"Fixed clock as 4 digits HHMM, e.g. \"1200\". Only when enabled=true; omit to leave unchanged."`
	Battery              *int   `json:"battery,omitempty" jsonschema:"Fixed battery level 0-100 (shown unplugged). Only when enabled=true; omit to leave unchanged."`
	NetworkType          string `json:"network_type,omitempty" jsonschema:"Network icon: \"wifi\" (default, full signal), \"mobile\", or \"none\" (hide network icons). Only when enabled=true."`
	MobileLevel          *int   `json:"mobile_level,omitempty" jsonschema:"Mobile signal bars 0-4. Only used when network_type=mobile. Default 4."`
	DataType             string `json:"data_type,omitempty" jsonschema:"Mobile data-type icon shown next to the signal, e.g. lte, 4g, 5g, 3g, edge, 1x, h, h+, roam. Only used when network_type=mobile."`
	Carrier              string `json:"carrier,omitempty" jsonschema:"Carrier/operator name shown in the status bar. Only used when network_type=mobile."`
	NotificationsVisible *bool  `json:"notifications_visible,omitempty" jsonschema:"Show notification icons in the status bar. Default false (hidden, for clean screenshots). Only when enabled=true."`
	NotificationIcon     string `json:"notification_icon,omitempty" jsonschema:"Best-effort: an AOSP SystemUI icon resource name to show in the first notification slot. Support varies by SystemUI/Android version. Only when enabled=true."`
}

type doctorArgs struct{}

// ---- Handlers ----

func setDarkMode(ctx context.Context, in darkModeArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.SetDarkMode(ctx, in.Enabled); err != nil {
		return nil, err
	}
	return text("Dark mode: %v", in.Enabled), nil
}

func setLocation(ctx context.Context, in locationArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.SetLocation(ctx, in.Longitude, in.Latitude); err != nil {
		return nil, err
	}
	return text("Location set to (lon %v, lat %v).", in.Longitude, in.Latitude), nil
}

func setStatusBar(ctx context.Context, in statusBarArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	opts := adb.StatusBarOptions{
		Clock:                in.Clock,
		Battery:              in.Battery,
		NetworkType:          in.NetworkType,
		MobileLevel:          in.MobileLevel,
		DataType:             in.DataType,
		Carrier:              in.Carrier,
		NotificationsVisible: in.NotificationsVisible,
		NotificationIcon:     in.NotificationIcon,
	}
	if err := c.StatusBarDemo(ctx, in.Enabled, opts); err != nil {
		return nil, err
	}
	if in.Enabled {
		return text("Status bar demo mode on for %s (clean bar for screenshots). Call again with enabled=false to restore.", c.Serial), nil
	}
	return text("Status bar demo mode off for %s (live bar restored).", c.Serial), nil
}

// ServerVersion is set by main at startup so doctor can report which build is
// actually serving — the first question when a documented tool or param seems
// to be missing is "is this install current?", and this answers it in-band.
var ServerVersion = "unknown"

func doctor(ctx context.Context, _ doctorArgs) (*mcp.CallToolResult, error) {
	return text("adb-mcp server version: %s (latest: https://github.com/iksnerd/adb_mcp/releases — update with `adb-mcp update`; a restarted MCP client picks up the new binary)\n\n%s", ServerVersion, adb.Doctor(ctx)), nil
}
