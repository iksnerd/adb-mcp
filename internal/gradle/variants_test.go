package gradle

import (
	"reflect"
	"testing"
)

// sampleTasksOutput mimics the relevant slice of `gradlew tasks` for a
// two-flavor (free/paid) project: aggregate assemble, real variants, and the
// test-only assemble tasks that must be excluded.
const sampleTasksOutput = `
Build tasks
-----------
assemble - Assemble main outputs for all the variants.
assembleAndroidTest - Assembles all the Test applications.
assembleFreeDebug - Assembles main output for variant freeDebug
assembleFreeRelease - Assembles main output for variant freeRelease
assemblePaidDebug - Assembles main output for variant paidDebug
assemblePaidDebugAndroidTest - Assembles the android (on-device) tests for the paidDebug build.
assembleFreeDebugUnitTest - Assembles the tests for freeDebug.
bundleFreeDebug - Assembles bundle for variant freeDebug

Install tasks
-------------
installFreeDebug - Installs the DebugFree build.
`

func TestParseVariants(t *testing.T) {
	got := ParseVariants(sampleTasksOutput)
	want := []string{"freeDebug", "freeRelease", "paidDebug"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseVariants =\n  %q\nwant\n  %q", got, want)
	}
}

// TestParseVariantsSingle covers the common no-flavor project (just debug/release)
// and confirms the bare `assemble` aggregate is never emitted as a variant.
func TestParseVariantsSingle(t *testing.T) {
	out := "assemble - Assemble main outputs.\nassembleDebug - x\nassembleRelease - y\n"
	got := ParseVariants(out)
	want := []string{"debug", "release"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseVariants = %q, want %q", got, want)
	}
}

func TestParseVariantsNone(t *testing.T) {
	if got := ParseVariants("no assemble tasks here\ntest - run tests\n"); len(got) != 0 {
		t.Errorf("expected no variants, got %q", got)
	}
}
