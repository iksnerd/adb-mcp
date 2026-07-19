package adb

import (
	"bytes"
	"testing"
)

// sfDump is a real `dumpsys SurfaceFlinger --display-id` capture from a
// Pixel_10_Pro_Fold emulator (two physical displays).
const sfDump = `Display 4619827259835644672 (HWC display 0): port=0 pnpId=GGL screenPartStatus=UNSUPPORTED displayName="EMU_display_0"
Display 4619827551948147201 (HWC display 1): port=1 pnpId=GGL screenPartStatus=UNSUPPORTED displayName="EMU_display_1"`

func TestParseDisplays(t *testing.T) {
	ds := parseDisplays(sfDump)
	if len(ds) != 2 {
		t.Fatalf("parseDisplays returned %d displays, want 2", len(ds))
	}
	if ds[0].PhysicalID != "4619827259835644672" || ds[0].Index != 0 || ds[0].Name != "EMU_display_0" {
		t.Errorf("display 0 = %+v", ds[0])
	}
	if ds[1].PhysicalID != "4619827551948147201" || ds[1].Index != 1 {
		t.Errorf("display 1 = %+v", ds[1])
	}
	if got := parseDisplays("no displays here"); got != nil {
		t.Errorf("parseDisplays(garbage) = %v, want nil", got)
	}
}

func TestResolveDisplay(t *testing.T) {
	ds := parseDisplays(sfDump)
	cases := []struct {
		sel  string
		want string // "" means expect an error
	}{
		{"inner", "4619827259835644672"},
		{"primary", "4619827259835644672"},
		{"cover", "4619827551948147201"},
		{"outer", "4619827551948147201"},
		{"0", "4619827259835644672"},
		{"1", "4619827551948147201"},
		{"4619827551948147201", "4619827551948147201"}, // raw physical id passes through
		{"99", ""},
		{"nonsense", ""},
	}
	for _, tc := range cases {
		got, err := resolveDisplay(tc.sel, ds)
		if tc.want == "" {
			if err == nil {
				t.Errorf("resolveDisplay(%q) = %q, want error", tc.sel, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("resolveDisplay(%q) = %q, %v; want %q", tc.sel, got, err, tc.want)
		}
	}
}

// TestPNGFromScreencap pins the foldable fix: a multi-display warning prepended
// to the PNG is stripped so the signature leads; a clean PNG is untouched; and a
// buffer with no signature (a real failure) is returned as-is.
func TestPNGFromScreencap(t *testing.T) {
	png := append(append([]byte{}, pngMagic...), []byte("...pixels...")...)

	warned := append([]byte("[Warning] Multiple displays were found, but no display id was specified! ...\n"), png...)
	if got := pngFromScreencap(warned); !bytes.Equal(got, png) {
		t.Errorf("warning prefix not stripped: got %q", got[:min(16, len(got))])
	}
	if got := pngFromScreencap(png); !bytes.Equal(got, png) {
		t.Error("clean PNG should pass through unchanged")
	}
	fail := []byte("Failed to take the screenshot")
	if got := pngFromScreencap(fail); !bytes.Equal(got, fail) {
		t.Error("a no-signature failure buffer should be returned unchanged")
	}
}

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
