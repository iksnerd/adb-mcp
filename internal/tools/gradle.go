package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/iksnerd/adb_mcp/internal/gradle"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Arguments ----

type gradleArgs struct {
	ProjectDir string   `json:"project_dir" jsonschema:"Path to the Android project root containing the Gradle wrapper (gradlew)."`
	Task       string   `json:"task,omitempty" jsonschema:"Gradle task to run. Defaults to the tool's standard task."`
	Args       []string `json:"args,omitempty" jsonschema:"Extra arguments passed to Gradle (e.g. --stacktrace, -Pflavor=free)."`
	JSON       bool     `json:"json,omitempty" jsonschema:"For run_unit_tests/run_instrumented_tests: return the test summary as structured JSON (per-suite timing, full failure stack traces) instead of the human-readable text summary. Ignored by gradle_build and list_gradle_tasks."`
}

type buildAndRunArgs struct {
	serialArg
	ProjectDir string   `json:"project_dir" jsonschema:"Path to the Android project root containing the Gradle wrapper (gradlew)."`
	Package    string   `json:"package" jsonschema:"Application package name to install and launch, e.g. com.example.app."`
	Task       string   `json:"task,omitempty" jsonschema:"Gradle task to run. Defaults to assembleDebug."`
	Args       []string `json:"args,omitempty" jsonschema:"Extra arguments passed to Gradle (e.g. --stacktrace, -Pflavor=free)."`
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
	task, apks, out, err := buildAPKs(ctx, in.ProjectDir, in.Task, in.Args)
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
	task, apks, out, err := buildAPKs(ctx, in.ProjectDir, in.Task, in.Args)
	if err != nil {
		return nil, fmt.Errorf("build failed: %v\n%s", err, tailLines(out, 40))
	}
	if len(apks) == 0 {
		return nil, fmt.Errorf("build succeeded (%s) but no APK was found under %s/**/build/outputs/ — check that the task produces one", task, in.ProjectDir)
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
	task := in.Task
	if task == "" {
		task = defaultTask
	}
	out, err := gradle.Gradle(ctx, in.ProjectDir, append([]string{task}, in.Args...)...)
	// Parse the JUnit XML regardless of exit code: a non-zero Gradle exit is
	// exactly when the per-test breakdown (which tests failed and why) is most
	// useful, so surface it in both the success and failure paths.
	summary, found := gradle.ParseTestResults(in.ProjectDir)
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

func listGradleTasks(ctx context.Context, in gradleArgs) (*mcp.CallToolResult, error) {
	out, err := gradle.Gradle(ctx, in.ProjectDir, "tasks")
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, tailLines(out, 40))
	}
	return text("%s", tailLines(out, 120)), nil
}

func listGradleVariants(ctx context.Context, in gradleArgs) (*mcp.CallToolResult, error) {
	variants, out, err := gradle.ListVariants(ctx, in.ProjectDir)
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

func listGradleProjects(ctx context.Context, in gradleArgs) (*mcp.CallToolResult, error) {
	paths, out, err := gradle.ListProjects(ctx, in.ProjectDir)
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

// tailLines keeps the last n non-trivial lines of possibly-huge tool output
// (Gradle logs) so results stay readable.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return fmt.Sprintf("… (%d earlier lines omitted)\n%s", len(lines)-n, strings.Join(lines[len(lines)-n:], "\n"))
}
