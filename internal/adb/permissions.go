package adb

import "context"

// GrantPermission grants a runtime permission, skipping its in-app dialog.
func (c *Client) GrantPermission(ctx context.Context, pkg, permission string) error {
	_, err := c.adb(ctx, "shell", "pm", "grant", pkg, permission)
	return err
}

// RevokePermission revokes a previously granted runtime permission.
func (c *Client) RevokePermission(ctx context.Context, pkg, permission string) error {
	_, err := c.adb(ctx, "shell", "pm", "revoke", pkg, permission)
	return err
}
