package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/iksnerd/adb-mcp/internal/gradle"
	"github.com/iksnerd/adb-mcp/internal/scaffold"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Arguments ----

type gradleArgs struct {
	projectDirArg
	Task string   `json:"task,omitempty" jsonschema:"Gradle task to run. Defaults to the tool's standard task."`
	Args []string `json:"args,omitempty" jsonschema:"Extra arguments passed to Gradle (e.g. --stacktrace, -Pflavor=free)."`
	JSON bool     `json:"json,omitempty" jsonschema:"For run_unit_tests/run_instrumented_tests: return the test summary as structured JSON (per-suite timing, full failure stack traces) instead of the human-readable text summary. Ignored by gradle_build and list_gradle_tasks."`
}

type gradleProjectsArgs struct {
	projectDirArg
}

// coverageTaskArgs is the shared shape of both coverage tools: which report
// task to run and how to render the result. Embedded rather than repeated so
// the task hint — the one an agent reads when the default doesn't exist — can
// only be described in one place.
type coverageTaskArgs struct {
	projectDirArg
	Task string   `json:"task,omitempty" jsonschema:"Gradle task that generates the JaCoCo XML report. Defaults to jacocoTestReport, which is NOT a task the Android Gradle plugin defines — it exists only if the project declares it. If it is missing, check list_gradle_tasks for AGP's built-in \"createDebugUnitTestCoverageReport\" (present when a build type sets enableUnitTestCoverage = true). Module-qualified names work: \":app:jacocoTestReport\"."`
	Args []string `json:"args,omitempty" jsonschema:"Extra arguments passed to Gradle (e.g. --stacktrace)."`
	JSON bool     `json:"json,omitempty" jsonschema:"Return structured JSON instead of the human-readable text summary."`
}

type coverageReportArgs struct {
	coverageTaskArgs
}

type fileCoverageArgs struct {
	coverageTaskArgs
	File string `json:"file" jsonschema:"Source file to report coverage for — a filename (e.g. Foo.kt) or package-qualified path (e.g. com/example/Foo.kt). Matched by suffix; an ambiguous bare filename returns every match."`
}

type gradleVariantsArgs struct {
	projectDirArg
	Task string `json:"task,omitempty" jsonschema:"Gradle task to scope to, e.g. \":app:tasks\" to list variants for the :app module. Defaults to the root project's tasks."`
}

type gradlePropertiesArgs struct {
	projectDirArg
	Module string `json:"module" jsonschema:"Gradle module path, e.g. :app or :feature:login."`
}

type scaffoldArgs struct {
	Destination string `json:"destination" jsonschema:"Empty or new directory to create the project in."`
	Name        string `json:"name" jsonschema:"Human-readable app name."`
	Package     string `json:"package" jsonschema:"Application id/package, e.g. com.example.app."`
}

type buildAndRunArgs struct {
	serialArg
	projectDirArg
	Package string   `json:"package" jsonschema:"Application package name to install and launch, e.g. com.example.app."`
	Task    string   `json:"task,omitempty" jsonschema:"Gradle task to run. Defaults to assembleDebug."`
	Args    []string `json:"args,omitempty" jsonschema:"Extra arguments passed to Gradle (e.g. --stacktrace, -Pflavor=free)."`
}

// ---- Handlers ----

// buildAPKs is the shared build phase of gradle_build and build_and_run: run
// the task (defaulting to assembleDebug) and locate the produced APKs, newest
// first. Keeping it in one place means the two tools cannot drift.
func buildAPKs(ctx context.Context, projectDir, task string, extra []string) (resolvedTask string, apks []string, out string, err error) {
	if task == "" {
		task = "assembleDebug"
	}
	out, err = gradle.Gradle(ctx, projectDir, append([]string{task}, extra...)...)
	if err != nil {
		return task, nil, out, err
	}
	return task, gradle.FindAPKs(projectDir), out, nil
}

func gradleBuild(ctx context.Context, in gradleArgs) (*mcp.CallToolResult, error) {
	projectDir, err := resolveProjectDir(in.ProjectDir)
	if err != nil {
		return nil, err
	}
	task, apks, out, err := buildAPKs(ctx, projectDir, in.Task, in.Args)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, tailLines(out, 40))
	}
	msg := "Build succeeded (" + task + ")."
	if len(apks) > 0 {
		msg += "\nAPK(s):\n" + strings.Join(apks, "\n")
	}
	return text("%s\n\n%s", msg, tailLines(out, 20)), nil
}

func buildAndRun(ctx context.Context, in buildAndRunArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	projectDir, err := resolveProjectDir(in.ProjectDir)
	if err != nil {
		return nil, err
	}
	task, apks, out, err := buildAPKs(ctx, projectDir, in.Task, in.Args)
	if err != nil {
		return nil, fmt.Errorf("build failed: %v\n%s", err, tailLines(out, 40))
	}
	if len(apks) == 0 {
		return nil, fmt.Errorf("build succeeded (%s) but no APK was found under %s/**/build/outputs/ — check that the task produces one", task, projectDir)
	}
	apk := gradle.PickAPK(apks)
	if _, err := c.InstallApp(ctx, apk); err != nil {
		return nil, fmt.Errorf("build succeeded (%s) but install of %s failed: %v", task, apk, err)
	}
	component, err := c.LaunchApp(ctx, in.Package)
	if err != nil {
		return nil, fmt.Errorf("build+install succeeded but launch of %s failed: %v", in.Package, err)
	}
	msg := fmt.Sprintf("Built (%s), installed %s, and launched %s", task, apk, in.Package)
	if component != "" {
		msg += fmt.Sprintf(" (%s)", component)
	}
	msg += "."
	if len(apks) > 1 {
		msg += fmt.Sprintf("\nNote: %d APKs were found; installed the newest non-test one: %s\nAll (newest first): %s", len(apks), apk, strings.Join(apks, ", "))
	}
	return text("%s", msg), nil
}

func runUnitTests(ctx context.Context, in gradleArgs) (*mcp.CallToolResult, error) {
	return runGradleReporting(ctx, in, "test")
}

func runInstrumentedTests(ctx context.Context, in gradleArgs) (*mcp.CallToolResult, error) {
	return runGradleReporting(ctx, in, "connectedAndroidTest")
}

func runGradleReporting(ctx context.Context, in gradleArgs, defaultTask string) (*mcp.CallToolResult, error) {
	projectDir, err := resolveProjectDir(in.ProjectDir)
	if err != nil {
		return nil, err
	}
	task := in.Task
	if task == "" {
		task = defaultTask
	}
	out, err := gradle.Gradle(ctx, projectDir, append([]string{task}, in.Args...)...)
	// Parse the JUnit XML regardless of exit code: a non-zero Gradle exit is
	// exactly when the per-test breakdown (which tests failed and why) is most
	// useful, so surface it in both the success and failure paths.
	summary, found := gradle.ParseTestResults(projectDir)
	if err != nil {
		msg := fmt.Sprintf("%v", err)
		if found {
			msg += "\n\n" + summary.String()
		}
		return nil, fmt.Errorf("%s\n\n%s", msg, tailLines(out, 60))
	}
	if found {
		if in.JSON {
			return jsonResult(summary)
		}
		return text("Tests passed (%s).\n\n%s\n\n%s", task, summary.String(), tailLines(out, 20)), nil
	}
	return text("Tests passed (%s).\n\n%s", task, tailLines(out, 30)), nil
}

// runCoverageTask runs the JaCoCo report-generating Gradle task (default
// jacocoTestReport) and returns the parsed XML report. Shared by
// getCoverageReport and getFileCoverage so both run-then-find-then-parse
// identically. A multi-module build emits one report per module; all of them
// are parsed and merged, so the result covers the whole build rather than
// whichever module Gradle happened to write last.
func runCoverageTask(ctx context.Context, projectDir, task string, extra []string) (report gradle.CoverageReport, reportPaths []string, err error) {
	if task == "" {
		task = "jacocoTestReport"
	}
	out, err := gradle.Gradle(ctx, projectDir, append([]string{task}, extra...)...)
	if err != nil {
		return report, nil, fmt.Errorf("%v\n%s", err, tailLines(out, 40))
	}
	reportPaths, err = gradle.FindCoverageReports(projectDir)
	if err != nil {
		return report, nil, fmt.Errorf("%v\n%s", err, tailLines(out, 40))
	}
	reports := make([]gradle.CoverageReport, 0, len(reportPaths))
	for _, p := range reportPaths {
		r, perr := gradle.ParseCoverageReport(p)
		if perr != nil {
			return report, nil, perr
		}
		reports = append(reports, r)
	}
	return gradle.MergeReports(reports), reportPaths, nil
}

func getCoverageReport(ctx context.Context, in coverageReportArgs) (*mcp.CallToolResult, error) {
	projectDir, err := resolveProjectDir(in.ProjectDir)
	if err != nil {
		return nil, err
	}
	report, reportPaths, err := runCoverageTask(ctx, projectDir, in.Task, in.Args)
	if err != nil {
		return nil, err
	}
	summary := gradle.SummarizeCoverage(report, reportPaths)
	if in.JSON {
		return jsonResult(summary)
	}
	return text("%s", summary.String()), nil
}

func getFileCoverage(ctx context.Context, in fileCoverageArgs) (*mcp.CallToolResult, error) {
	projectDir, err := resolveProjectDir(in.ProjectDir)
	if err != nil {
		return nil, err
	}
	report, _, err := runCoverageTask(ctx, projectDir, in.Task, in.Args)
	if err != nil {
		return nil, err
	}
	matches, available := gradle.FileCoverageFor(report, in.File)
	if len(matches) == 0 {
		sample := available
		suffix := ""
		if len(sample) > gradle.MaxAvailableListed {
			sample = sample[:gradle.MaxAvailableListed]
			suffix = fmt.Sprintf("\n… and %d more", len(available)-gradle.MaxAvailableListed)
		}
		return nil, fmt.Errorf("no coverage data for %q. Available files:\n%s%s", in.File, strings.Join(sample, "\n"), suffix)
	}
	if in.JSON {
		return jsonResult(matches)
	}
	var b strings.Builder
	for i, m := range matches {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(m.String())
	}
	return text("%s", b.String()), nil
}

func listGradleTasks(ctx context.Context, in gradleArgs) (*mcp.CallToolResult, error) {
	projectDir, err := resolveProjectDir(in.ProjectDir)
	if err != nil {
		return nil, err
	}
	task := in.Task
	if task == "" {
		task = "tasks"
	}
	out, err := gradle.Gradle(ctx, projectDir, task)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, tailLines(out, 40))
	}
	return text("%s", tailLines(out, 120)), nil
}

func listGradleVariants(ctx context.Context, in gradleVariantsArgs) (*mcp.CallToolResult, error) {
	projectDir, err := resolveProjectDir(in.ProjectDir)
	if err != nil {
		return nil, err
	}
	variants, out, err := gradle.ListVariants(ctx, projectDir, in.Task)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, tailLines(out, 40))
	}
	if len(variants) == 0 {
		return text("No build variants found via `gradlew tasks` — point project_dir at an Android application/library module (the one whose build.gradle applies the android plugin).\n\n%s", tailLines(out, 40)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d build variant(s) — build with assemble<Variant>, install with install<Variant>:\n", len(variants))
	for _, v := range variants {
		fmt.Fprintf(&b, "  %s\n", v)
	}
	return text("%s", strings.TrimRight(b.String(), "\n")), nil
}

func listGradleProjects(ctx context.Context, in gradleProjectsArgs) (*mcp.CallToolResult, error) {
	projectDir, err := resolveProjectDir(in.ProjectDir)
	if err != nil {
		return nil, err
	}
	paths, out, err := gradle.ListProjects(ctx, projectDir)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, tailLines(out, 40))
	}
	if len(paths) == 0 {
		return text("No sub-projects — this looks like a single-module build (only the root project). Its own tasks/variants are what you build.\n\n%s", tailLines(out, 40)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d module(s) — address a task at one with '<path>:<task>', e.g. %s:assembleDebug:\n", len(paths), paths[0])
	for _, p := range paths {
		fmt.Fprintf(&b, "  %s\n", p)
	}
	return text("%s", strings.TrimRight(b.String(), "\n")), nil
}

func gradleProjectProperties(ctx context.Context, in gradlePropertiesArgs) (*mcp.CallToolResult, error) {
	projectDir, err := resolveProjectDir(in.ProjectDir)
	if err != nil {
		return nil, err
	}
	out, err := gradle.ListProjectProperties(ctx, projectDir, in.Module)
	if err != nil {
		return nil, fmt.Errorf("could not read properties for %s: %v\n%s", in.Module, err, tailLines(out, 40))
	}
	return text("%s", tailLines(out, 200)), nil
}

func scaffoldProject(ctx context.Context, in scaffoldArgs) (*mcp.CallToolResult, error) {
	files, err := scaffold.Create(scaffold.Options{Destination: in.Destination, Name: in.Name, Package: in.Package})
	if err != nil {
		return nil, err
	}
	// Every Gradle tool here drives ./gradlew, so a project without a wrapper
	// is inert. Generate it now when a system Gradle can, rather than handing
	// back a project whose very next tool call is guaranteed to fail.
	out, ok, werr := gradle.GenerateWrapper(ctx, in.Destination)
	switch {
	case ok:
		return text("Created Android project in %s (%d files) and generated the Gradle wrapper. Ready to build: gradle_build with project_dir=%s (task defaults to assembleDebug).", in.Destination, len(files), in.Destination), nil
	case werr != nil:
		return text("Created Android project in %s (%d files), but generating the Gradle wrapper failed — run `gradle wrapper` there yourself before gradle_build:\n\n%s", in.Destination, len(files), tailLines(out, 15)), nil
	default:
		return text("Created Android project in %s (%d files). No `gradle` on PATH to generate the wrapper — install Gradle and run `gradle wrapper` there (every Gradle tool here drives ./gradlew), then use gradle_build with task=assembleDebug.", in.Destination, len(files)), nil
	}
}

// tailLines keeps the last n non-trivial lines of possibly-huge tool output
// (Gradle logs) so results stay readable.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return fmt.Sprintf("… (%d earlier lines omitted)\n%s", len(lines)-n, strings.Join(lines[len(lines)-n:], "\n"))
}
