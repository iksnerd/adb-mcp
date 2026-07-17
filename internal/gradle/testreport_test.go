package gradle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTestResults(t *testing.T) {
	dir := t.TempDir()
	// A passing+failing JVM unit-test report.
	unit := filepath.Join(dir, "build", "test-results", "testDebugUnitTest")
	if err := os.MkdirAll(unit, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(unit, "TEST-com.example.MathTest.xml"), `<?xml version="1.0"?>
<testsuite name="com.example.MathTest" tests="3" failures="1" errors="0" skipped="1" time="1.500">
  <testcase name="adds" classname="com.example.MathTest"/>
  <testcase name="divides" classname="com.example.MathTest">
    <failure message="expected 2 but was 3">assert
	at com.example.MathTest.divides(MathTest.java:42)</failure>
  </testcase>
  <testcase name="skipMe" classname="com.example.MathTest"><skipped/></testcase>
</testsuite>`)
	// A second suite via the <testsuites> wrapper form.
	inst := filepath.Join(dir, "build", "outputs", "androidTest-results", "connected")
	if err := os.MkdirAll(inst, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(inst, "TEST-emulator.xml"), `<?xml version="1.0"?>
<testsuites>
  <testsuite name="UiTest" tests="2" failures="0" errors="1" skipped="0" time="0.750">
    <testcase name="loads" classname="com.example.UiTest"/>
    <testcase name="crashes" classname="com.example.UiTest">
      <error message="NullPointerException">boom
	at com.example.UiTest.crashes(UiTest.java:17)</error>
    </testcase>
  </testsuite>
</testsuites>`)
	// A non-report xml that must be ignored.
	writeFile(t, filepath.Join(dir, "AndroidManifest.xml"), `<manifest/>`)

	sum, found := ParseTestResults(dir)
	if !found {
		t.Fatal("expected reports to be found")
	}
	if sum.Suites != 2 {
		t.Errorf("Suites = %d, want 2", sum.Suites)
	}
	if sum.Tests != 5 {
		t.Errorf("Tests = %d, want 5", sum.Tests)
	}
	if sum.Failures != 1 {
		t.Errorf("Failures = %d, want 1", sum.Failures)
	}
	if sum.Errors != 1 {
		t.Errorf("Errors = %d, want 1", sum.Errors)
	}
	if sum.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", sum.Skipped)
	}
	if sum.Passed != 2 { // 5 - 1 failure - 1 error - 1 skipped
		t.Errorf("Passed = %d, want 2", sum.Passed)
	}
	if len(sum.Failed) != 2 {
		t.Fatalf("Failed count = %d, want 2 (%v)", len(sum.Failed), sum.Failed)
	}
	// Failure message should be truncated to its first line and prefixed with class.method.
	wantContains := "com.example.MathTest.divides: expected 2 but was 3"
	found2 := false
	for _, f := range sum.Failed {
		if f == wantContains {
			found2 = true
		}
	}
	if !found2 {
		t.Errorf("Failed = %v, want an entry %q", sum.Failed, wantContains)
	}

	// Per-suite breakdown: the <testsuites> wrapper must keep its child suite
	// distinct rather than flattening it into an unnamed combined suite.
	if len(sum.SuiteBreakdown) != 2 {
		t.Fatalf("SuiteBreakdown count = %d, want 2 (%+v)", len(sum.SuiteBreakdown), sum.SuiteBreakdown)
	}
	byName := map[string]SuiteResult{}
	for _, sr := range sum.SuiteBreakdown {
		byName[sr.Name] = sr
	}
	if sr, ok := byName["com.example.MathTest"]; !ok || sr.TimeSec != 1.5 {
		t.Errorf("SuiteBreakdown[com.example.MathTest] = %+v, want TimeSec=1.5", sr)
	}
	if sr, ok := byName["UiTest"]; !ok || sr.TimeSec != 0.75 {
		t.Errorf("SuiteBreakdown[UiTest] = %+v, want TimeSec=0.75", sr)
	}
	if sum.TotalTimeSec != 2.25 {
		t.Errorf("TotalTimeSec = %v, want 2.25", sum.TotalTimeSec)
	}

	// Full stack traces must be captured (not just the first line).
	if len(sum.FailedDetail) != 2 {
		t.Fatalf("FailedDetail count = %d, want 2 (%+v)", len(sum.FailedDetail), sum.FailedDetail)
	}
	var mathFail *TestFailure
	for i := range sum.FailedDetail {
		if sum.FailedDetail[i].Name == "com.example.MathTest.divides" {
			mathFail = &sum.FailedDetail[i]
		}
	}
	if mathFail == nil {
		t.Fatalf("FailedDetail missing com.example.MathTest.divides (%+v)", sum.FailedDetail)
	}
	if !strings.Contains(mathFail.Stack, "at com.example.MathTest.divides(MathTest.java:42)") {
		t.Errorf("FailedDetail stack = %q, want it to contain the trace line", mathFail.Stack)
	}
}

// Regression: a <testsuites> wrapper that ALSO carries aggregate counts on its
// root element must still surface the per-testcase failing-test names from its
// child <testsuite> elements (they used to be dropped).
func TestParseTestResultsAggregatedWrapper(t *testing.T) {
	dir := t.TempDir()
	rep := filepath.Join(dir, "build", "outputs", "androidTest-results", "connected")
	if err := os.MkdirAll(rep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(rep, "TEST-aggregate.xml"), `<?xml version="1.0"?>
<testsuites tests="2" failures="1" errors="0" skipped="0">
  <testsuite name="LoginTest" tests="2" failures="1" errors="0" skipped="0">
    <testcase name="opens" classname="com.example.LoginTest"/>
    <testcase name="submits" classname="com.example.LoginTest">
      <failure message="expected OK but was ERROR">assert</failure>
    </testcase>
  </testsuite>
</testsuites>`)

	sum, found := ParseTestResults(dir)
	if !found {
		t.Fatal("expected the report to be found")
	}
	if sum.Tests != 2 || sum.Failures != 1 {
		t.Errorf("Tests=%d Failures=%d, want 2 and 1", sum.Tests, sum.Failures)
	}
	want := "com.example.LoginTest.submits: expected OK but was ERROR"
	if len(sum.Failed) != 1 || sum.Failed[0] != want {
		t.Errorf("Failed = %v, want exactly [%q]", sum.Failed, want)
	}
}

func TestParseTestResultsNone(t *testing.T) {
	if _, found := ParseTestResults(t.TempDir()); found {
		t.Error("expected found=false for a dir with no reports")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
