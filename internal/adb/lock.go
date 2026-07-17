package adb

import (
	"context"
	"fmt"
	"strings"
)

// SetDeviceLock sets a secure lock screen. lockType is pin, pattern, or
// password. When a lock already exists, pass old (the current credential) so it
// can be changed in one call — locksettings refuses to overwrite otherwise.
func (c *Client) SetDeviceLock(ctx context.Context, lockType, value, old string) error {
	var sub string
	switch strings.ToLower(lockType) {
	case "", "pin":
		sub = "set-pin"
	case "pattern":
		sub = "set-pattern"
	case "password":
		sub = "set-password"
	default:
		return fmt.Errorf("unknown lock type %q (use pin, pattern, or password)", lockType)
	}
	args := []string{"shell", "locksettings", sub}
	if strings.TrimSpace(old) != "" {
		args = append(args, "--old", old)
	}
	args = append(args, value)
	_, err := c.adb(ctx, args...)
	return err
}

// ClearDeviceLock removes the lock screen, supplying the current credential.
func (c *Client) ClearDeviceLock(ctx context.Context, old string) error {
	_, err := c.adb(ctx, "shell", "locksettings", "clear", "--old", old)
	return err
}

// IsDeviceSecure reports whether a secure lock screen is set
// (KeyguardManager.isDeviceSecure), which Keystore-backed crypto flows require.
func (c *Client) IsDeviceSecure(ctx context.Context) (bool, error) {
	out, err := c.adb(ctx, "shell", "locksettings", "get-disabled")
	if err != nil {
		return false, err
	}
	// get-disabled == "false" means the lock is NOT disabled, i.e. device is secure.
	return strings.TrimSpace(out) == "false", nil
}
