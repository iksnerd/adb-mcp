package tools

import (
	"context"
	"strings"
	"testing"
)

func TestRequireNonEmpty(t *testing.T) {
	for _, v := range []string{"", " ", "\t\n"} {
		err := requireNonEmpty("package", v)
		if err == nil || !strings.Contains(err.Error(), "package") {
			t.Errorf("requireNonEmpty(package, %q) = %v, want an error naming the parameter", v, err)
		}
	}
	if err := requireNonEmpty("package", "com.example"); err != nil {
		t.Errorf("requireNonEmpty with a value = %v, want nil", err)
	}
}

// Schema `required` only checks presence, so "" reaches the handler. Each of
// these must be refused naming the parameter, before adb is touched
// (`adb uninstall ""` and `pm clear ""` used to run).
func TestHandlersRejectEmptyRequiredStrings(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		param string
		call  func() error
	}{
		{"launch_app", "package", func() error { _, e := launchApp(ctx, packageArg{}); return e }},
		{"stop_app", "package", func() error { _, e := stopApp(ctx, packageArg{}); return e }},
		{"reload_app", "package", func() error { _, e := reloadApp(ctx, packageArg{}); return e }},
		{"uninstall_app", "package", func() error { _, e := uninstallApp(ctx, packageArg{}); return e }},
		{"clear_app_data", "package", func() error { _, e := clearAppData(ctx, packageArg{}); return e }},
		{"app_state", "package", func() error { _, e := appState(ctx, appStateArgs{}); return e }},
		{"get_app_details", "package", func() error { _, e := getAppDetails(ctx, appDetailsArgs{Package: "  "}); return e }},
		{"grant_permission/package", "package", func() error { _, e := grantPermission(ctx, permissionArgs{Permission: "p"}); return e }},
		{"grant_permission/permission", "permission", func() error { _, e := grantPermission(ctx, permissionArgs{Package: "a.b"}); return e }},
		{"revoke_permission/package", "package", func() error { _, e := revokePermission(ctx, permissionArgs{Permission: "p"}); return e }},
		{"revoke_permission/permission", "permission", func() error { _, e := revokePermission(ctx, permissionArgs{Package: "a.b"}); return e }},
		{"open_url", "url", func() error { _, e := openURL(ctx, openURLArgs{}); return e }},
		{"install_app", "apk_path", func() error { _, e := installApp(ctx, installArgs{}); return e }},
		{"push_file/local_path", "local_path", func() error { _, e := pushFile(ctx, pushArgs{DevicePath: "/sdcard/x"}); return e }},
		{"push_file/device_path", "device_path", func() error { _, e := pushFile(ctx, pushArgs{LocalPath: "/tmp/x"}); return e }},
		{"pull_file/local_path", "local_path", func() error { _, e := pullFile(ctx, pullArgs{DevicePath: "/sdcard/x"}); return e }},
		{"pull_file/device_path", "device_path", func() error { _, e := pullFile(ctx, pullArgs{LocalPath: "/tmp/x"}); return e }},
		{"set_device_lock", "value", func() error { _, e := setDeviceLock(ctx, setLockArgs{}); return e }},
		{"render_stats", "package", func() error { _, e := renderStats(ctx, renderStatsArgs{}); return e }},
		{"wait_for_text", "text", func() error { _, e := waitForText(ctx, waitForTextArgs{}); return e }},
		{"stop_screen_record", "local_path", func() error { _, e := stopScreenRecord(ctx, stopRecordArgs{}); return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetSessionDefaults(t)
			calls := fakeAdb(t, "")
			err := tc.call()
			if err == nil || !strings.Contains(err.Error(), tc.param) {
				t.Fatalf("err = %v, want an error naming %q", err, tc.param)
			}
			if got := calls(); len(got) != 0 {
				t.Errorf("adb was invoked despite the empty argument: %v", got)
			}
		})
	}
}

// A whitespace serial must fall back to the session default exactly like an
// empty one, not slide into adb.ResolveSerial's single-device rule.
func TestResolveWhitespaceSerialUsesSessionDefault(t *testing.T) {
	resetSessionDefaults(t)
	fakeAdb(t, "")
	sessionDefaults.mu.Lock()
	sessionDefaults.serial = "pinned-1"
	sessionDefaults.mu.Unlock()
	for _, s := range []string{"", "  ", "\t"} {
		c, err := resolve(context.Background(), s)
		if err != nil {
			t.Fatalf("resolve(%q): %v", s, err)
		}
		if c.Serial != "pinned-1" {
			t.Errorf("resolve(%q).Serial = %q, want the session default pinned-1", s, c.Serial)
		}
	}
}
