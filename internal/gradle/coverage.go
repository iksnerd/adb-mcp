package gradle

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---- JaCoCo XML report structs ----
//
// Mirrors the JaCoCo XML report DTD (-//JACOCO//DTD Report 1.1//EN). The
// DOCTYPE references report.dtd via SYSTEM but encoding/xml never fetches
// it, so unmarshalling a real report just works.

type jacocoCounter struct {
	Type    string `xml:"type,attr"`
	Missed  int    `xml:"missed,attr"`
	Covered int    `xml:"covered,attr"`
}

type jacocoMethod struct {
	Name     string          `xml:"name,attr"`
	Line     int             `xml:"line,attr"`
	Counters []jacocoCounter `xml:"counter"`
}

type jacocoClass struct {
	Name           string         `xml:"name,attr"`
	SourceFileName string         `xml:"sourcefilename,attr"`
	Methods        []jacocoMethod `xml:"method"`
}

type jacocoLine struct {
	Nr int `xml:"nr,attr"`
	MI int `xml:"mi,attr"`
	CI int `xml:"ci,attr"`
	MB int `xml:"mb,attr"`
	CB int `xml:"cb,attr"`
}

type jacocoSourceFile struct {
	Name     string          `xml:"name,attr"`
	Lines    []jacocoLine    `xml:"line"`
	Counters []jacocoCounter `xml:"counter"`
}

type jacocoPackage struct {
	Name        string             `xml:"name,attr"`
	Classes     []jacocoClass      `xml:"class"`
	SourceFiles []jacocoSourceFile `xml:"sourcefile"`
	Counters    []jacocoCounter    `xml:"counter"`
}

// CoverageReport is a parsed JaCoCo XML report.
type CoverageReport struct {
	Packages []jacocoPackage `xml:"package"`
	Counters []jacocoCounter `xml:"counter"`
}

func counterOf(counters []jacocoCounter, typ string) (missed, covered int, ok bool) {
	for _, c := range counters {
		if c.Type == typ {
			return c.Missed, c.Covered, true
		}
	}
	return 0, 0, false
}

func pct(covered, missed int) float64 {
	total := covered + missed
	if total == 0 {
		return 0
	}
	return 100 * float64(covered) / float64(total)
}

// maxPackagesListed caps how many per-package rows CoverageSummary carries,
// keeping the worst-covered (Packages is sorted ascending by LinePercent)
// so a huge multi-module report doesn't blow up the result.
const maxPackagesListed = 50

// MaxAvailableListed caps how many "did you mean" paths a file-coverage miss
// suggests.
const MaxAvailableListed = 20

// FindCoverageReports walks projectDir for JaCoCo XML reports (produced by
// the jacoco Gradle plugin's XML report, typically under
// build/reports/jacoco/<task>/<task>.xml).
//
// A multi-module build produces one report per module, and `gradlew
// jacocoTestReport` runs all of them — so every module's report is returned
// and the caller merges them. Within a single module only the most recently
// modified report is kept: a module that also emits per-variant reports
// (jacocoTestDebugUnitTestReport alongside jacocoTestReleaseUnitTestReport)
// would otherwise double-count the same classes, and a stale report from an
// earlier run would shadow the one just generated.
func FindCoverageReports(projectDir string) ([]string, error) {
	type candidate struct {
		path string
		mod  time.Time
	}
	// Keyed by the path prefix before "/reports/jacoco/", i.e. one entry per
	// module (".../app/build", ".../core/build").
	newestPerModule := map[string]candidate{}
	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		slash := filepath.ToSlash(path)
		if !strings.HasSuffix(slash, ".xml") {
			return nil
		}
		module, _, ok := strings.Cut(slash, "/reports/jacoco/")
		if !ok {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if cur, seen := newestPerModule[module]; !seen || info.ModTime().After(cur.mod) {
			newestPerModule[module] = candidate{path: path, mod: info.ModTime()}
		}
		return nil
	})
	if len(newestPerModule) == 0 {
		return nil, fmt.Errorf("no JaCoCo XML report found under %s (expected build/reports/jacoco/**/*.xml) — does this project apply the jacoco Gradle plugin with its XML report enabled (jacocoTestReport { reports { xml.required = true } })?", projectDir)
	}
	paths := make([]string, 0, len(newestPerModule))
	for _, c := range newestPerModule {
		paths = append(paths, c.path)
	}
	sort.Strings(paths)
	return paths, nil
}

// ParseCoverageReport reads and unmarshals one JaCoCo XML report file.
func ParseCoverageReport(path string) (CoverageReport, error) {
	var r CoverageReport
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := xml.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("could not parse JaCoCo report %s: %w", path, err)
	}
	return r, nil
}

// MergeReports combines one report per module into a single report covering
// the whole build, so the summary and per-file lookup see every module rather
// than whichever one happened to be written last. Report-wide counters are
// summed; packages are merged by name (two modules can share a package),
// summing their counters and pooling their classes and source files.
func MergeReports(reports []CoverageReport) CoverageReport {
	if len(reports) == 1 {
		return reports[0]
	}
	var merged CoverageReport
	byName := map[string]int{} // package name -> index into merged.Packages
	for _, r := range reports {
		merged.Counters = addCounters(merged.Counters, r.Counters)
		for _, p := range r.Packages {
			i, seen := byName[p.Name]
			if !seen {
				byName[p.Name] = len(merged.Packages)
				merged.Packages = append(merged.Packages, p)
				continue
			}
			dst := &merged.Packages[i]
			dst.Counters = addCounters(dst.Counters, p.Counters)
			dst.Classes = append(dst.Classes, p.Classes...)
			dst.SourceFiles = append(dst.SourceFiles, p.SourceFiles...)
		}
	}
	return merged
}

// addCounters sums src into dst by counter type, appending types dst lacks.
func addCounters(dst, src []jacocoCounter) []jacocoCounter {
	for _, s := range src {
		found := false
		for i := range dst {
			if dst[i].Type == s.Type {
				dst[i].Missed += s.Missed
				dst[i].Covered += s.Covered
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, s)
		}
	}
	return dst
}

// ---- Summary result types ----

// CoverageSummary is the report-wide coverage result of get_coverage_report.
type CoverageSummary struct {
	ReportPaths   []string          `json:"report_paths"` // one per module, merged into the totals below
	LinePercent   float64           `json:"line_percent"`
	LineCovered   int               `json:"line_covered"`
	LineMissed    int               `json:"line_missed"`
	BranchPercent float64           `json:"branch_percent,omitempty"`
	BranchCovered int               `json:"branch_covered,omitempty"`
	BranchMissed  int               `json:"branch_missed,omitempty"`
	MethodPercent float64           `json:"method_percent"`
	MethodCovered int               `json:"method_covered"`
	MethodMissed  int               `json:"method_missed"`
	ClassPercent  float64           `json:"class_percent"`
	ClassCovered  int               `json:"class_covered"`
	ClassMissed   int               `json:"class_missed"`
	TotalPackages int               `json:"total_packages"`
	Packages      []PackageCoverage `json:"packages"`
}

// PackageCoverage is one <package>'s aggregated coverage.
type PackageCoverage struct {
	Name          string  `json:"name"`
	LinePercent   float64 `json:"line_percent"`
	LineCovered   int     `json:"line_covered"`
	LineMissed    int     `json:"line_missed"`
	BranchPercent float64 `json:"branch_percent,omitempty"`
	ClassCount    int     `json:"class_count"`
}

// SummarizeCoverage reduces a parsed JaCoCo report to its report-wide and
// per-package coverage, worst-covered package first.
func SummarizeCoverage(r CoverageReport, reportPaths []string) CoverageSummary {
	sum := CoverageSummary{ReportPaths: reportPaths}
	if m, c, ok := counterOf(r.Counters, "LINE"); ok {
		sum.LineMissed, sum.LineCovered = m, c
	}
	sum.LinePercent = pct(sum.LineCovered, sum.LineMissed)
	if m, c, ok := counterOf(r.Counters, "BRANCH"); ok {
		sum.BranchMissed, sum.BranchCovered = m, c
		sum.BranchPercent = pct(c, m)
	}
	if m, c, ok := counterOf(r.Counters, "METHOD"); ok {
		sum.MethodMissed, sum.MethodCovered = m, c
		sum.MethodPercent = pct(c, m)
	}
	if m, c, ok := counterOf(r.Counters, "CLASS"); ok {
		sum.ClassMissed, sum.ClassCovered = m, c
		sum.ClassPercent = pct(c, m)
	}

	for _, p := range r.Packages {
		pkg := PackageCoverage{Name: strings.ReplaceAll(p.Name, "/", "."), ClassCount: len(p.Classes)}
		if m, c, ok := counterOf(p.Counters, "LINE"); ok {
			pkg.LineMissed, pkg.LineCovered = m, c
		}
		pkg.LinePercent = pct(pkg.LineCovered, pkg.LineMissed)
		if m, c, ok := counterOf(p.Counters, "BRANCH"); ok {
			pkg.BranchPercent = pct(c, m)
		}
		sum.Packages = append(sum.Packages, pkg)
	}
	sort.Slice(sum.Packages, func(i, j int) bool {
		if sum.Packages[i].LinePercent != sum.Packages[j].LinePercent {
			return sum.Packages[i].LinePercent < sum.Packages[j].LinePercent
		}
		return sum.Packages[i].Name < sum.Packages[j].Name
	})
	sum.TotalPackages = len(sum.Packages)
	if len(sum.Packages) > maxPackagesListed {
		sum.Packages = sum.Packages[:maxPackagesListed]
	}
	return sum
}

// String renders a one-line-plus-detail human summary for a tool result.
func (sum CoverageSummary) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Line coverage: %.1f%% (%d/%d)", sum.LinePercent, sum.LineCovered, sum.LineCovered+sum.LineMissed)
	if sum.BranchCovered+sum.BranchMissed > 0 {
		fmt.Fprintf(&b, " · Branch: %.1f%% (%d/%d)", sum.BranchPercent, sum.BranchCovered, sum.BranchCovered+sum.BranchMissed)
	}
	fmt.Fprintf(&b, " · Method: %.1f%% (%d/%d) · Class: %.1f%% (%d/%d)",
		sum.MethodPercent, sum.MethodCovered, sum.MethodCovered+sum.MethodMissed,
		sum.ClassPercent, sum.ClassCovered, sum.ClassCovered+sum.ClassMissed)
	if len(sum.ReportPaths) == 1 {
		fmt.Fprintf(&b, "\nReport: %s", sum.ReportPaths[0])
	} else if len(sum.ReportPaths) > 1 {
		fmt.Fprintf(&b, "\nMerged from %d module reports:", len(sum.ReportPaths))
		for _, p := range sum.ReportPaths {
			fmt.Fprintf(&b, "\n  %s", p)
		}
	}
	if len(sum.Packages) > 0 {
		b.WriteString("\n\nPackages, worst-covered first:")
		for _, p := range sum.Packages {
			fmt.Fprintf(&b, "\n  %5.1f%%  %s  (%d/%d lines, %d class(es))", p.LinePercent, p.Name, p.LineCovered, p.LineCovered+p.LineMissed, p.ClassCount)
		}
		if sum.TotalPackages > len(sum.Packages) {
			fmt.Fprintf(&b, "\n  … and %d more package(s)", sum.TotalPackages-len(sum.Packages))
		}
	}
	return b.String()
}

// ---- Per-file coverage ----

// FileCoverage is one source file's coverage, the result of get_file_coverage.
type FileCoverage struct {
	Package       string           `json:"package"`
	SourceFile    string           `json:"source_file"`
	LinePercent   float64          `json:"line_percent"`
	LineCovered   int              `json:"line_covered"`
	LineMissed    int              `json:"line_missed"`
	BranchPercent float64          `json:"branch_percent,omitempty"`
	BranchCovered int              `json:"branch_covered,omitempty"`
	BranchMissed  int              `json:"branch_missed,omitempty"`
	MissedLines   string           `json:"missed_lines,omitempty"`  // collapsed ranges, e.g. "12-15,22,30-33"
	PartialLines  string           `json:"partial_lines,omitempty"` // some but not all branches covered
	Methods       []MethodCoverage `json:"methods,omitempty"`
}

// MethodCoverage is one method's line coverage — the per-function analogue
// of XcodeBuildMCP's get_file_coverage.
type MethodCoverage struct {
	Class       string  `json:"class"`
	Name        string  `json:"name"`
	Line        int     `json:"line"`
	Covered     bool    `json:"covered"`
	LinePercent float64 `json:"line_percent"`
}

// FileCoverageFor finds every <sourcefile> whose package-qualified path
// matches file — exactly, by path suffix (so "Foo.kt" or "com/example/Foo.kt"
// both work), or by bare filename. Multiple matches mean the name is
// ambiguous across packages; available lists every package-qualified path in
// the report, for a "did you mean" error when there's no match at all.
func FileCoverageFor(r CoverageReport, file string) (matches []FileCoverage, available []string) {
	target := strings.TrimPrefix(filepath.ToSlash(file), "/")
	for _, p := range r.Packages {
		for _, sf := range p.SourceFiles {
			full := sf.Name
			if p.Name != "" {
				full = p.Name + "/" + sf.Name
			}
			available = append(available, full)
			if full == target || sf.Name == target || strings.HasSuffix(full, "/"+target) {
				matches = append(matches, buildFileCoverage(p, sf))
			}
		}
	}
	return matches, available
}

func buildFileCoverage(p jacocoPackage, sf jacocoSourceFile) FileCoverage {
	fc := FileCoverage{Package: strings.ReplaceAll(p.Name, "/", "."), SourceFile: sf.Name}
	if m, c, ok := counterOf(sf.Counters, "LINE"); ok {
		fc.LineMissed, fc.LineCovered = m, c
	}
	fc.LinePercent = pct(fc.LineCovered, fc.LineMissed)
	if m, c, ok := counterOf(sf.Counters, "BRANCH"); ok {
		fc.BranchMissed, fc.BranchCovered = m, c
		fc.BranchPercent = pct(c, m)
	}

	var missed, partial []int
	for _, ln := range sf.Lines {
		switch {
		case ln.CI == 0 && ln.MI > 0:
			missed = append(missed, ln.Nr)
		case ln.CB > 0 && ln.MB > 0:
			partial = append(partial, ln.Nr)
		}
	}
	fc.MissedLines = collapseRanges(missed)
	fc.PartialLines = collapseRanges(partial)

	for _, cls := range p.Classes {
		if cls.SourceFileName != sf.Name {
			continue
		}
		for _, m := range cls.Methods {
			mc := MethodCoverage{Class: strings.ReplaceAll(cls.Name, "/", "."), Name: m.Name, Line: m.Line}
			if mi, ci, ok := counterOf(m.Counters, "LINE"); ok {
				mc.LinePercent = pct(ci, mi)
				mc.Covered = ci > 0
			}
			fc.Methods = append(fc.Methods, mc)
		}
	}
	sort.Slice(fc.Methods, func(i, j int) bool { return fc.Methods[i].Line < fc.Methods[j].Line })
	return fc
}

// String renders a one-file human summary for a tool result.
func (fc FileCoverage) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s/%s: %.1f%% lines (%d/%d)", fc.Package, fc.SourceFile, fc.LinePercent, fc.LineCovered, fc.LineCovered+fc.LineMissed)
	if fc.BranchCovered+fc.BranchMissed > 0 {
		fmt.Fprintf(&b, ", %.1f%% branches (%d/%d)", fc.BranchPercent, fc.BranchCovered, fc.BranchCovered+fc.BranchMissed)
	}
	if fc.MissedLines != "" {
		fmt.Fprintf(&b, "\n  missed lines: %s", fc.MissedLines)
	}
	if fc.PartialLines != "" {
		fmt.Fprintf(&b, "\n  partial lines: %s", fc.PartialLines)
	}
	for _, m := range fc.Methods {
		mark := "✗"
		if m.Covered {
			mark = "✓"
		}
		fmt.Fprintf(&b, "\n  %s %s.%s (line %d): %.1f%%", mark, m.Class, m.Name, m.Line, m.LinePercent)
	}
	return b.String()
}

// collapseRanges renders a set of line numbers as compact ranges, e.g.
// [12,13,14,15,22,30,31,32,33] -> "12-15,22,30-33".
func collapseRanges(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	sorted := append([]int(nil), nums...)
	sort.Ints(sorted)
	var parts []string
	start, prev := sorted[0], sorted[0]
	flush := func(end int) {
		if start == end {
			parts = append(parts, strconv.Itoa(start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", start, end))
		}
	}
	for _, n := range sorted[1:] {
		if n == prev+1 {
			prev = n
			continue
		}
		flush(prev)
		start, prev = n, n
	}
	flush(prev)
	return strings.Join(parts, ",")
}
