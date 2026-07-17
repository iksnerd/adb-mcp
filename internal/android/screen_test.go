package android

import "testing"

func TestParseCurrentFocus(t *testing.T) {
	dump := `  mGlobalConfiguration={1.0 310mcc260mnc}
  mCurrentFocus=Window{f8ec3b7 u0 com.example.demoapp/com.example.demoapp.MainActivity}
  mFocusedApp=ActivityRecord{...}`
	if got := parseCurrentFocus(dump); got != "com.example.demoapp/com.example.demoapp.MainActivity" {
		t.Errorf("parseCurrentFocus = %q", got)
	}

	overlay := `  mCurrentFocus=Window{1a2b3c u0 com.android.systemui.biometrics.ui.BiometricPromptDialog}`
	if got := parseCurrentFocus(overlay); got != "com.android.systemui.biometrics.ui.BiometricPromptDialog" {
		t.Errorf("parseCurrentFocus overlay = %q", got)
	}

	// Fallback key used by some Android versions.
	fallback := ` mFocusedWindow=Window{9dd6ba u0 NotificationShade}`
	if got := parseCurrentFocus(fallback); got != "NotificationShade" {
		t.Errorf("parseCurrentFocus fallback = %q", got)
	}

	if got := parseCurrentFocus("mCurrentFocus=null\n"); got != "" {
		t.Errorf("parseCurrentFocus(null) = %q, want empty", got)
	}
}
