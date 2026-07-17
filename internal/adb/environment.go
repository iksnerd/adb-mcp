package adb

import (
	"context"
	"strconv"
)

// SetDarkMode toggles the system dark theme (cmd uimode night yes|no).
func (c *Client) SetDarkMode(ctx context.Context, on bool) error {
	mode := "no"
	if on {
		mode = "yes"
	}
	_, err := c.adb(ctx, "shell", "cmd", "uimode", "night", mode)
	return err
}

// SetLocation sets the emulator's GPS fix (longitude, latitude), via the console
// `geo fix` command. Note adb's `emu geo fix` takes longitude first.
func (c *Client) SetLocation(ctx context.Context, lon, lat float64) error {
	_, err := c.adb(ctx, "emu", "geo", "fix",
		strconv.FormatFloat(lon, 'f', -1, 64), strconv.FormatFloat(lat, 'f', -1, 64))
	return err
}
