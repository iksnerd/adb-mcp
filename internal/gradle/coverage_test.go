package gradle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindCoverageReport(t *testing.T) {
	dir := t.TempDir()
	reportDir := filepath.Join(dir, "app", "build", "reports", "jacoco", "jacocoTestReport")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(reportDir, "jacocoTestReport.xml")
	writeFile(t, report, `<?xml version="1.0"?><report><counter type="LINE" missed="1" covered="1"/></report>`)
	manifestDir := filepath.Join(dir, "app", "src", "main")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(manifestDir, "AndroidManifest.xml"), `<manifest/>`)

	got, err := FindCoverageReports(dir)
	if err != nil {
		t.Fatalf("FindCoverageReports: %v", err)
	}
	if len(got) != 1 || got[0] != report {
		t.Errorf("FindCoverageReports = %q, want [%q]", got, report)
	}
}

// Within one module (same path prefix before /reports/jacoco/) only the
// newest report is kept, so per-variant or stale reports can't double-count.
func TestFindCoverageReportsNewestWinsWithinAModule(t *testing.T) {
	dir := t.TempDir()
	oldDir := filepath.Join(dir, "build", "reports", "jacoco", "old")
	newDir := filepath.Join(dir, "build", "reports", "jacoco", "new")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldReport := filepath.Join(oldDir, "old.xml")
	newReport := filepath.Join(newDir, "new.xml")
	writeFile(t, oldReport, `<report/>`)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldReport, old, old); err != nil {
		t.Fatal(err)
	}
	writeFile(t, newReport, `<report/>`)

	got, err := FindCoverageReports(dir)
	if err != nil {
		t.Fatalf("FindCoverageReports: %v", err)
	}
	if len(got) != 1 || got[0] != newReport {
		t.Errorf("FindCoverageReports = %q, want just the newest %q", got, newReport)
	}
}

// A multi-module build writes one report per module and `gradlew
// jacocoTestReport` runs all of them — every module must come back, or the
// summary silently reports whichever module Gradle wrote last.
func TestFindCoverageReportsAcrossModules(t *testing.T) {
	dir := t.TempDir()
	var want []string
	for _, module := range []string{"app", "core"} {
		reportDir := filepath.Join(dir, module, "build", "reports", "jacoco", "test")
		if err := os.MkdirAll(reportDir, 0o755); err != nil {
			t.Fatal(err)
		}
		report := filepath.Join(reportDir, "jacocoTestReport.xml")
		writeFile(t, report, `<report/>`)
		want = append(want, report)
	}
	// Make one module's report clearly older; it must still be returned.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(want[0], old, old); err != nil {
		t.Fatal(err)
	}

	got, err := FindCoverageReports(dir)
	if err != nil {
		t.Fatalf("FindCoverageReports: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("FindCoverageReports = %q, want both module reports %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("FindCoverageReports[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// AGP's built-in createDebugUnitTestCoverageReport (enableUnitTestCoverage =
// true) writes the same JaCoCo DTD document to build/reports/coverage/... —
// nothing in that path says "jacoco", so matching only /reports/jacoco/ missed
// the more common setup entirely.
func TestFindCoverageReportsAgpLayout(t *testing.T) {
	dir := t.TempDir()
	reportDir := filepath.Join(dir, "app", "build", "reports", "coverage", "test", "debug")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(reportDir, "report.xml")
	writeFile(t, report, `<report/>`)

	got, err := FindCoverageReports(dir)
	if err != nil {
		t.Fatalf("FindCoverageReports: %v", err)
	}
	if len(got) != 1 || got[0] != report {
		t.Errorf("FindCoverageReports = %q, want [%q]", got, report)
	}
}

// A module with both an AGP report and a hand-written jacocoTestReport holds
// the same coverage twice; merging both would double-count it.
func TestFindCoverageReportsOneModuleBothLayouts(t *testing.T) {
	dir := t.TempDir()
	agp := filepath.Join(dir, "app", "build", "reports", "coverage", "test", "debug", "report.xml")
	hand := filepath.Join(dir, "app", "build", "reports", "jacoco", "jacocoTestReport", "jacocoTestReport.xml")
	for _, p := range []string{agp, hand} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, p, `<report/>`)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(hand, old, old); err != nil {
		t.Fatal(err)
	}

	got, err := FindCoverageReports(dir)
	if err != nil {
		t.Fatalf("FindCoverageReports: %v", err)
	}
	if len(got) != 1 || got[0] != agp {
		t.Errorf("FindCoverageReports = %q, want only the newest of the one module (%q)", got, agp)
	}
}

func TestFindCoverageReportNone(t *testing.T) {
	if _, err := FindCoverageReports(t.TempDir()); err == nil {
		t.Error("expected an error when no report exists")
	}
}

// The whole point of merging: totals span every module, and a package that
// exists in two modules is combined rather than duplicated.
func TestMergeReports(t *testing.T) {
	first, err := ParseCoverageReport(writeTempReport(t, syntheticReport))
	if err != nil {
		t.Fatalf("ParseCoverageReport: %v", err)
	}
	second, err := ParseCoverageReport(writeTempReport(t, `<report name="other">
  <package name="com/example/other">
    <sourcefile name="Other.kt"><counter type="LINE" missed="3" covered="7"/></sourcefile>
    <counter type="LINE" missed="3" covered="7"/>
    <counter type="CLASS" missed="0" covered="2"/>
  </package>
  <package name="com/example/bad">
    <counter type="LINE" missed="5" covered="0"/>
  </package>
  <counter type="LINE" missed="8" covered="7"/>
  <counter type="CLASS" missed="0" covered="2"/>
</report>`))
	if err != nil {
		t.Fatalf("ParseCoverageReport: %v", err)
	}

	sum := SummarizeCoverage(MergeReports([]CoverageReport{first, second}), []string{"a.xml", "b.xml"})

	// Report-wide LINE: 8 covered/12 missed + 7 covered/8 missed.
	if sum.LineCovered != 15 || sum.LineMissed != 20 {
		t.Errorf("Line = %d/%d, want 15/20 (summed across modules)", sum.LineCovered, sum.LineMissed)
	}
	if sum.ClassCovered != 3 || sum.ClassMissed != 1 {
		t.Errorf("Class = %d/%d, want 3/1", sum.ClassCovered, sum.ClassMissed)
	}
	// com/example/bad appears in both reports: merged, not duplicated.
	if sum.TotalPackages != 3 {
		t.Fatalf("TotalPackages = %d, want 3 (good, bad, other)", sum.TotalPackages)
	}
	var bad *PackageCoverage
	for i := range sum.Packages {
		if sum.Packages[i].Name == "com.example.bad" {
			bad = &sum.Packages[i]
		}
	}
	if bad == nil {
		t.Fatalf("com.example.bad missing from %+v", sum.Packages)
	}
	if bad.LineMissed != 15 || bad.LineCovered != 0 {
		t.Errorf("com.example.bad = %d/%d covered/missed, want 0/15 (10+5 merged)", bad.LineCovered, bad.LineMissed)
	}
	// A file from the second module must be reachable through the merged report.
	if matches, _ := FileCoverageFor(MergeReports([]CoverageReport{first, second}), "Other.kt"); len(matches) != 1 {
		t.Errorf("FileCoverageFor(Other.kt) = %d matches, want 1 from the merged report", len(matches))
	}
	if !strings.Contains(sum.String(), "Merged from 2 module reports") {
		t.Errorf("String() should say it merged multiple reports, got:\n%s", sum.String())
	}
}

const syntheticReport = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<!DOCTYPE report PUBLIC "-//JACOCO//DTD Report 1.1//EN" "report.dtd">
<report name="demo">
  <package name="com/example/good">
    <class name="com/example/good/Bar" sourcefilename="Bar.kt">
      <method name="baz" desc="()V" line="10">
        <counter type="INSTRUCTION" missed="0" covered="5"/>
        <counter type="LINE" missed="0" covered="2"/>
        <counter type="METHOD" missed="0" covered="1"/>
      </method>
      <method name="qux" desc="()V" line="20">
        <counter type="INSTRUCTION" missed="4" covered="0"/>
        <counter type="LINE" missed="2" covered="0"/>
        <counter type="METHOD" missed="1" covered="0"/>
      </method>
      <counter type="INSTRUCTION" missed="4" covered="5"/>
      <counter type="LINE" missed="2" covered="2"/>
      <counter type="METHOD" missed="1" covered="1"/>
      <counter type="CLASS" missed="0" covered="1"/>
    </class>
    <sourcefile name="Bar.kt">
      <line nr="10" mi="0" ci="1" mb="0" cb="0"/>
      <line nr="11" mi="0" ci="1" mb="1" cb="1"/>
      <line nr="20" mi="1" ci="0" mb="0" cb="0"/>
      <line nr="21" mi="1" ci="0" mb="0" cb="0"/>
      <counter type="INSTRUCTION" missed="4" covered="5"/>
      <counter type="LINE" missed="2" covered="2"/>
      <counter type="BRANCH" missed="1" covered="1"/>
    </sourcefile>
    <counter type="INSTRUCTION" missed="4" covered="5"/>
    <counter type="LINE" missed="2" covered="8"/>
    <counter type="BRANCH" missed="1" covered="1"/>
    <counter type="METHOD" missed="1" covered="1"/>
    <counter type="CLASS" missed="0" covered="1"/>
  </package>
  <package name="com/example/bad">
    <class name="com/example/bad/Worse" sourcefilename="Worse.kt">
      <method name="fail" desc="()V" line="5">
        <counter type="LINE" missed="10" covered="0"/>
        <counter type="METHOD" missed="1" covered="0"/>
      </method>
      <counter type="LINE" missed="10" covered="0"/>
      <counter type="METHOD" missed="1" covered="0"/>
      <counter type="CLASS" missed="1" covered="0"/>
    </class>
    <sourcefile name="Worse.kt">
      <line nr="5" mi="1" ci="0" mb="0" cb="0"/>
      <counter type="LINE" missed="10" covered="0"/>
    </sourcefile>
    <counter type="LINE" missed="10" covered="0"/>
    <counter type="METHOD" missed="1" covered="0"/>
    <counter type="CLASS" missed="1" covered="0"/>
  </package>
  <counter type="INSTRUCTION" missed="4" covered="5"/>
  <counter type="LINE" missed="12" covered="8"/>
  <counter type="BRANCH" missed="1" covered="1"/>
  <counter type="METHOD" missed="2" covered="1"/>
  <counter type="CLASS" missed="1" covered="1"/>
</report>`

func TestSummarizeCoverage(t *testing.T) {
	report, err := ParseCoverageReport(writeTempReport(t, syntheticReport))
	if err != nil {
		t.Fatalf("ParseCoverageReport: %v", err)
	}
	sum := SummarizeCoverage(report, []string{"path/to/report.xml"})

	if len(sum.ReportPaths) != 1 || sum.ReportPaths[0] != "path/to/report.xml" {
		t.Errorf("ReportPaths = %q", sum.ReportPaths)
	}
	if sum.LineCovered != 8 || sum.LineMissed != 12 {
		t.Errorf("Line = %d/%d, want 8/12", sum.LineCovered, sum.LineMissed)
	}
	if got, want := sum.LinePercent, 40.0; got != want {
		t.Errorf("LinePercent = %v, want %v", got, want)
	}
	if sum.MethodCovered != 1 || sum.MethodMissed != 2 {
		t.Errorf("Method = %d/%d, want 1/2", sum.MethodCovered, sum.MethodMissed)
	}
	if sum.ClassCovered != 1 || sum.ClassMissed != 1 {
		t.Errorf("Class = %d/%d, want 1/1", sum.ClassCovered, sum.ClassMissed)
	}
	if sum.TotalPackages != 2 {
		t.Fatalf("TotalPackages = %d, want 2", sum.TotalPackages)
	}
	if len(sum.Packages) != 2 {
		t.Fatalf("Packages count = %d, want 2", len(sum.Packages))
	}
	// Worst-covered (com.example.bad, 0%) must sort first.
	if sum.Packages[0].Name != "com.example.bad" {
		t.Errorf("Packages[0] = %q, want com.example.bad first (worst-covered)", sum.Packages[0].Name)
	}
	if sum.Packages[0].LinePercent != 0 {
		t.Errorf("Packages[0].LinePercent = %v, want 0", sum.Packages[0].LinePercent)
	}
	if sum.Packages[1].Name != "com.example.good" {
		t.Errorf("Packages[1] = %q, want com.example.good", sum.Packages[1].Name)
	}
	if got, want := sum.Packages[1].LinePercent, 80.0; got != want {
		t.Errorf("Packages[1].LinePercent = %v, want %v", got, want)
	}

	if !strings.Contains(sum.String(), "com.example.bad") {
		t.Errorf("String() = %q, want it to mention com.example.bad", sum.String())
	}
}

func TestSummarizeCoveragePackageCap(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<report name="demo">`)
	for i := range maxPackagesListed + 5 {
		fmt.Fprintf(&b, `<package name="com/example/pkg%d"><counter type="LINE" missed="1" covered="1"/></package>`, i)
	}
	b.WriteString(`<counter type="LINE" missed="5" covered="5"/></report>`)

	report, err := ParseCoverageReport(writeTempReport(t, b.String()))
	if err != nil {
		t.Fatalf("ParseCoverageReport: %v", err)
	}
	sum := SummarizeCoverage(report, []string{"r.xml"})
	if sum.TotalPackages != maxPackagesListed+5 {
		t.Errorf("TotalPackages = %d, want %d", sum.TotalPackages, maxPackagesListed+5)
	}
	if len(sum.Packages) != maxPackagesListed {
		t.Errorf("Packages count = %d, want capped at %d", len(sum.Packages), maxPackagesListed)
	}
	if !strings.Contains(sum.String(), "more package") {
		t.Error("String() should note truncated packages")
	}
}

func TestFileCoverageFor(t *testing.T) {
	report, err := ParseCoverageReport(writeTempReport(t, syntheticReport))
	if err != nil {
		t.Fatalf("ParseCoverageReport: %v", err)
	}

	// Bare filename match.
	matches, _ := FileCoverageFor(report, "Bar.kt")
	if len(matches) != 1 {
		t.Fatalf("FileCoverageFor(Bar.kt) = %d matches, want 1", len(matches))
	}
	fc := matches[0]
	if fc.Package != "com.example.good" {
		t.Errorf("Package = %q, want com.example.good", fc.Package)
	}
	if fc.LineCovered != 2 || fc.LineMissed != 2 {
		t.Errorf("Line = %d/%d, want 2/2", fc.LineCovered, fc.LineMissed)
	}
	if fc.MissedLines != "20-21" {
		t.Errorf("MissedLines = %q, want 20-21", fc.MissedLines)
	}
	if fc.PartialLines != "11" {
		t.Errorf("PartialLines = %q, want 11", fc.PartialLines)
	}
	if len(fc.Methods) != 2 {
		t.Fatalf("Methods count = %d, want 2", len(fc.Methods))
	}
	if fc.Methods[0].Name != "baz" || !fc.Methods[0].Covered {
		t.Errorf("Methods[0] = %+v, want baz covered", fc.Methods[0])
	}
	if fc.Methods[1].Name != "qux" || fc.Methods[1].Covered {
		t.Errorf("Methods[1] = %+v, want qux not covered", fc.Methods[1])
	}

	// Suffix (package-qualified) match.
	matches, _ = FileCoverageFor(report, "com/example/good/Bar.kt")
	if len(matches) != 1 {
		t.Errorf("FileCoverageFor(qualified path) = %d matches, want 1", len(matches))
	}

	// No match: available lists every package-qualified path.
	matches, available := FileCoverageFor(report, "Nope.kt")
	if len(matches) != 0 {
		t.Errorf("FileCoverageFor(Nope.kt) = %d matches, want 0", len(matches))
	}
	if len(available) != 2 {
		t.Errorf("available = %v, want 2 entries", available)
	}
}

func TestFileCoverageForAmbiguous(t *testing.T) {
	xmlDoc := `<report name="demo">
  <package name="com/example/a">
    <sourcefile name="Same.kt"><counter type="LINE" missed="1" covered="0"/></sourcefile>
  </package>
  <package name="com/example/b">
    <sourcefile name="Same.kt"><counter type="LINE" missed="0" covered="1"/></sourcefile>
  </package>
</report>`
	report, err := ParseCoverageReport(writeTempReport(t, xmlDoc))
	if err != nil {
		t.Fatalf("ParseCoverageReport: %v", err)
	}
	matches, _ := FileCoverageFor(report, "Same.kt")
	if len(matches) != 2 {
		t.Fatalf("FileCoverageFor(Same.kt) = %d matches, want 2 (ambiguous across packages)", len(matches))
	}
}

func TestCollapseRanges(t *testing.T) {
	cases := []struct {
		in   []int
		want string
	}{
		{nil, ""},
		{[]int{5}, "5"},
		{[]int{1, 2, 3, 4}, "1-4"},
		{[]int{1, 2, 3, 10, 20, 21, 22}, "1-3,10,20-22"},
		{[]int{9, 7, 8}, "7-9"}, // unsorted input
	}
	for _, c := range cases {
		if got := collapseRanges(c.in); got != c.want {
			t.Errorf("collapseRanges(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func writeTempReport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.xml")
	writeFile(t, path, content)
	return path
}
