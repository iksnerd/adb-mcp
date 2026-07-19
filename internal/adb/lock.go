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
	// Positive check first: verifying an EMPTY credential succeeds only when no
	// secure lock is set, so a success is definitive proof the device is NOT
	// secure. This closes the false positive in the get-disabled heuristic
	// below, which returns "false" (read as secure) even for a default device
	// with no lock at all — get-disabled reflects whether the lockscreen is
	// administratively disabled, not whether a credential exists.
	if out, err := c.adb(ctx, "shell", "locksettings", "verify"); err == nil &&
		strings.Contains(strings.ToLower(out), "success") {
		return false, nil // empty credential verified => no lock set
	}
	// Fallback (older images / no `verify` subcommand): get-disabled == "false"
	// means the lockscreen is not disabled, which — combined with the verify
	// probe above having failed to prove otherwise — indicates a secure device.
	out, err := c.adb(ctx, "shell", "locksettings", "get-disabled")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "false", nil
}
