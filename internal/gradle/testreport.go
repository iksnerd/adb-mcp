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
)

// TestSummary is the aggregated result of a Gradle test run, parsed from the
// JUnit XML that both JVM unit tests (build/test-results/**) and instrumented
// tests (build/outputs/androidTest-results/**) emit.
type TestSummary struct {
	Suites         int           `json:"suites"`
	Tests          int           `json:"tests"`
	Failures       int           `json:"failures"`
	Errors         int           `json:"errors"`
	Skipped        int           `json:"skipped"`
	Passed         int           `json:"passed"`
	TotalTimeSec   float64       `json:"total_time_sec,omitempty"`
	Failed         []string      `json:"failed,omitempty"`        // "Class.method: reason", capped
	SuiteBreakdown []SuiteResult `json:"suites_detail,omitempty"` // per-suite counts and timing
	FailedDetail   []TestFailure `json:"failed_detail,omitempty"` // full message + stack trace, capped
}

// SuiteResult is the per-<testsuite> breakdown of a test run.
type SuiteResult struct {
	Name     string  `json:"name"`
	Tests    int     `json:"tests"`
	Failures int     `json:"failures"`
	Errors   int     `json:"errors"`
	Skipped  int     `json:"skipped"`
	TimeSec  float64 `json:"time_sec"`
}

// TestFailure is one failing/erroring test case with its full detail (unlike
// the capped first-line entries in TestSummary.Failed).
type TestFailure struct {
	Name    string `json:"name"` // "Class.method"
	Message string `json:"message,omitempty"`
	Stack   string `json:"stack,omitempty"` // full trace, capped at maxStackChars
}

// junitSuite mirrors the <testsuite> element of a JUnit XML report.
type junitSuite struct {
	Name     string          `xml:"name,attr"`
	Time     string          `xml:"time,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Failure   *junitFailure `xml:"failure"`
	Error     *junitFailure `xml:"error"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"` // full stack trace
}

// maxFailedListed caps how many failing-test lines/details a summary
// carries, so a wholesale failure doesn't produce an unreadable wall of text.
const maxFailedListed = 20

// maxStackChars caps how long a single failure's stack trace can be in
// FailedDetail, so one huge trace doesn't blow up JSON output.
const maxStackChars = 4000

// ParseTestResults walks projectDir for JUnit XML reports and aggregates them.
// found reports whether any report files were located at all (so callers can
// distinguish "0 tests" from "no report written").
func ParseTestResults(projectDir string) (sum TestSummary, found bool) {
	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !isTestReport(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if suites, ok := parseJUnitSuites(data); ok {
			found = true
			for _, s := range suites {
				sum.merge(s)
			}
		}
		return nil
	})
	sum.Passed = sum.Tests - sum.Failures - sum.Errors - sum.Skipped
	if sum.Passed < 0 {
		sum.Passed = 0
	}
	sort.Strings(sum.Failed)
	if len(sum.Failed) > maxFailedListed {
		extra := len(sum.Failed) - maxFailedListed
		sum.Failed = append(sum.Failed[:maxFailedListed:maxFailedListed],
			fmt.Sprintf("… and %d more", extra))
	}
	sort.Slice(sum.FailedDetail, func(i, j int) bool { return sum.FailedDetail[i].Name < sum.FailedDetail[j].Name })
	if len(sum.FailedDetail) > maxFailedListed {
		sum.FailedDetail = sum.FailedDetail[:maxFailedListed:maxFailedListed]
	}
	return sum, found
}

// isTestReport matches Gradle's JUnit XML output: a TEST-*.xml (or any .xml)
// file under a test-results or androidTest-results directory.
func isTestReport(path string) bool {
	slash := filepath.ToSlash(path)
	if !strings.HasSuffix(slash, ".xml") {
		return false
	}
	return strings.Contains(slash, "/test-results/") ||
		strings.Contains(slash, "/androidTest-results/") ||
		strings.Contains(slash, "/androidTest-results-connected/")
}

// parseJUnitSuites parses one report, which may be a single <testsuite> or a
// <testsuites> wrapper containing several — each child <testsuite> is kept
// distinct (not combined) so per-suite name/timing/case detail survives.
func parseJUnitSuites(data []byte) ([]junitSuite, bool) {
	// Try the <testsuites> wrapper first. Such a wrapper can carry aggregate
	// counts on its root (tests="N" ...) while the per-<testcase> detail lives on
	// the child <testsuite> elements. Parsing it as a single suite would read the
	// root's counts but see no cases, dropping every failing-test name. This
	// unmarshal only matches when there are genuine child <testsuite> elements,
	// so a lone <testsuite> document falls through to the single-suite path.
	var multi struct {
		Suites []junitSuite `xml:"testsuite"`
	}
	if err := xml.Unmarshal(data, &multi); err == nil && len(multi.Suites) > 0 {
		return multi.Suites, true
	}
	var single junitSuite
	if err := xml.Unmarshal(data, &single); err == nil && (single.Tests > 0 || len(single.Cases) > 0) {
		return []junitSuite{single}, true
	}
	return nil, false
}

func (sum *TestSummary) merge(s junitSuite) {
	sum.Suites++
	sum.Tests += s.Tests
	sum.Failures += s.Failures
	sum.Errors += s.Errors
	sum.Skipped += s.Skipped
	timeSec, _ := strconv.ParseFloat(strings.TrimSpace(s.Time), 64)
	sum.TotalTimeSec += timeSec
	sum.SuiteBreakdown = append(sum.SuiteBreakdown, SuiteResult{
		Name:     s.Name,
		Tests:    s.Tests,
		Failures: s.Failures,
		Errors:   s.Errors,
		Skipped:  s.Skipped,
		TimeSec:  timeSec,
	})
	for _, c := range s.Cases {
		fail := c.Failure
		if fail == nil {
			fail = c.Error
		}
		if fail == nil {
			continue
		}
		name := c.Name
		if c.Classname != "" {
			name = c.Classname + "." + c.Name
		}
		msg := strings.TrimSpace(firstLine(fail.Message))
		if msg != "" {
			sum.Failed = append(sum.Failed, name+": "+msg)
		} else {
			sum.Failed = append(sum.Failed, name)
		}
		sum.FailedDetail = append(sum.FailedDetail, TestFailure{
			Name:    name,
			Message: strings.TrimSpace(fail.Message),
			Stack:   truncateStack(strings.TrimSpace(fail.Body)),
		})
	}
}

// truncateStack caps a stack trace at maxStackChars so one huge trace can't
// blow up JSON output.
func truncateStack(s string) string {
	if len(s) <= maxStackChars {
		return s
	}
	return fmt.Sprintf("%s\n… (%d more chars)", s[:maxStackChars], len(s)-maxStackChars)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// String renders a one-line-plus-detail human summary for a tool result.
func (sum TestSummary) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d tests, %d passed, %d failed, %d errors, %d skipped (%d suite(s))",
		sum.Tests, sum.Passed, sum.Failures, sum.Errors, sum.Skipped, sum.Suites)
	for _, f := range sum.Failed {
		b.WriteString("\n  ✗ " + f)
	}
	return b.String()
}
