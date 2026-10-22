package adb

import "testing"

func TestParseBadging(t *testing.T) {
	out := `package: name='com.example.app' versionCode='42' versionName='1.4.2' platformBuildVersionName='15' platformBuildVersionCode='35' compileSdkVersion='35'
sdkVersion:'24'
targetSdkVersion:'35'
uses-permission: name='android.permission.INTERNET'
application-label:'Demo'
launchable-activity: name='com.example.app.MainActivity'  label='Demo' icon=''
`
	got, err := ParseBadging(out)
	if err != nil {
		t.Fatal(err)
	}
	want := APKInfo{Package: "com.example.app", VersionCode: "42", VersionName: "1.4.2", LauncherActivity: "com.example.app/com.example.app.MainActivity"}
	if got != want {
		t.Errorf("ParseBadging = %+v, want %+v", got, want)
	}
}

func TestParseBadgingNoLauncherActivity(t *testing.T) {
	got, err := ParseBadging("package: name='com.lib.only' versionCode='1' versionName='0.1'\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.Package != "com.lib.only" || got.LauncherActivity != "" {
		t.Errorf("ParseBadging = %+v", got)
	}
}

func TestParseBadgingRejectsNonBadgingOutput(t *testing.T) {
	if _, err := ParseBadging(""); err == nil {
		t.Error("expected an error for empty output (the symptom of aapt2 printing nothing)")
	}
}
