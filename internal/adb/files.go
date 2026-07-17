package adb

import (
	"context"
	"fmt"
	"os"
)

// PushFile copies a local file onto the device.
func (c *Client) PushFile(ctx context.Context, localPath, devicePath string) (string, error) {
	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("local file not found: %s", localPath)
	}
	return c.adb(ctx, "push", localPath, devicePath)
}

// PullFile copies a file off the device to a local path.
func (c *Client) PullFile(ctx context.Context, devicePath, localPath string) (string, error) {
	return c.adb(ctx, "pull", devicePath, localPath)
}
