package adb

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// nightModeRe matches the `cmd uimode night` report, e.g. "Night mode: auto".
// The format is unverified, so anything else counts as an unknown mode.
var nightModeRe = regexp.MustCompile(`(?i)^night mode:\s*(yes|no|auto)\s*$`)

// readNightMode returns "yes", "no" or "auto", or "" when it cannot be read.
// Device output only ever reaches a later command through this filter.
func (c *Client) readNightMode(ctx context.Context) string {
	out, err := c.adb(ctx, "shell", "cmd", "uimode", "night")
	if err != nil {
		return ""
	}
	if m := nightModeRe.FindStringSubmatch(strings.TrimSpace(out)); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// SetDarkMode toggles the system dark theme (cmd uimode night yes|no).
//
// The mode in force before the first change is read and kept with the
// override, so setting it back (or shutdown) restores the user's own mode,
// including "auto". When it cannot be read the override is still recorded but
// never restored automatically.
func (c *Client) SetDarkMode(ctx context.Context, on bool) error {
	mode := "no"
	if on {
		mode = "yes"
	}
	prior, recorded := overrideState(c.Serial, OverrideDarkMode)
	if !recorded {
		prior = c.readNightMode(ctx)
	}
	if _, err := c.adb(ctx, "shell", "cmd", "uimode", "night", mode); err != nil {
		return err
	}
	if prior == mode {
		ClearOverride(c.Serial, OverrideDarkMode)
		return nil
	}
	noteOverridePrior(c.Serial, OverrideDarkMode, prior)
	return nil
}

// SetLocation sets the emulator's GPS fix (longitude, latitude), via the console
// `geo fix` command. Note adb's `emu geo fix` takes longitude first.
func (c *Client) SetLocation(ctx context.Context, lon, lat float64) error {
	_, err := c.adb(ctx, "emu", "geo", "fix",
		strconv.FormatFloat(lon, 'f', -1, 64), strconv.FormatFloat(lat, 'f', -1, 64))
	return err
}
