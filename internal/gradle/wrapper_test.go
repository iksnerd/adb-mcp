package gradle

import (
	"runtime"
	"testing"
)

func TestWrapperName(t *testing.T) {
	want := "gradlew"
	if runtime.GOOS == "windows" {
		want = "gradlew.bat"
	}
	if got := WrapperName(); got != want {
		t.Errorf("WrapperName() = %q, want %q", got, want)
	}
}
