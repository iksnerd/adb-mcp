package gradle

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iksnerd/adb-mcp/internal/sdk"
)

// Build states reported by BuildSummary.Status.
const (
	StatusRunning = "running"
	StatusPassed  = "passed"
	StatusFailed  = "failed"
)

const (
	defaultBuildTask = "assembleDebug"
	maxFirstErrors   = 5
	// backgroundBuildLimit bounds a detached build so a hung Gradle cannot keep
	// a process and a job entry alive forever.
	backgroundBuildLimit = 60 * time.Minute
)

// BuildSummary is the compact result of a build: enough for an agent to decide
// what to do next without reading the log, which lives in full (secrets
// redacted) at LogPath.
type BuildSummary struct {
	ID          string
	Task        string
	Status      string
	Duration    time.Duration
	ErrorCount  int
	FirstErrors []string
	APKs        []string
	LogPath     string
	// Err is why the build could not run or exited non-zero.
	Err string
}

func (s BuildSummary) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Build %s (%s), %s", s.Status, s.Task, s.Duration.Round(time.Second))
	if s.ID != "" {
		fmt.Fprintf(&b, "\nJob: %s", s.ID)
	}
	if s.Status == StatusFailed {
		fmt.Fprintf(&b, "\nErrors: %d", s.ErrorCount)
		for _, e := range s.FirstErrors {
			fmt.Fprintf(&b, "\n  %s", e)
		}
		if s.Err != "" {
			fmt.Fprintf(&b, "\n%s", s.Err)
		}
	}
	if len(s.APKs) > 0 {
		fmt.Fprintf(&b, "\nAPK(s):\n%s", strings.Join(s.APKs, "\n"))
	}
	if s.LogPath != "" {
		fmt.Fprintf(&b, "\nFull log: %s", s.LogPath)
	}
	return b.String()
}

// RunBuild runs a Gradle task to completion. The full output goes to a log
// file; the returned summary stays small.
func RunBuild(ctx context.Context, projectDir, task string, extra []string) BuildSummary {
	task = taskOrDefault(task)
	s := BuildSummary{Task: task}
	cmd, logFile, err := prepare(ctx, projectDir, task, extra)
	if err != nil {
		s.Status, s.Err = StatusFailed, err.Error()
		return s
	}
	s.LogPath = logFile.Name()
	return execute(cmd, logFile, projectDir, s)
}

// Job is a started background build.
type Job struct {
	ID      string
	LogPath string
}

type job struct {
	mu      sync.Mutex
	summary BuildSummary
	started time.Time
	done    bool
}

var (
	jobsMu sync.Mutex
	jobs   = map[string]*job{}
)

// StartBuild starts a Gradle task detached from the caller and returns at
// once. Poll it with BuildStatus; read progress from the log path.
func StartBuild(projectDir, task string, extra []string) (Job, error) {
	task = taskOrDefault(task)
	ctx, cancel := context.WithTimeout(context.Background(), backgroundBuildLimit)
	cmd, logFile, err := prepare(ctx, projectDir, task, extra)
	if err != nil {
		cancel()
		return Job{}, err
	}
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		cancel()
		logFile.Close()
		return Job{}, err
	}
	id := hex.EncodeToString(raw[:])
	j := &job{started: time.Now(), summary: BuildSummary{ID: id, Task: task, Status: StatusRunning, LogPath: logFile.Name()}}
	jobsMu.Lock()
	jobs[id] = j
	jobsMu.Unlock()
	go func() {
		defer cancel()
		final := execute(cmd, logFile, projectDir, BuildSummary{ID: id, Task: task, LogPath: logFile.Name()})
		j.mu.Lock()
		j.summary, j.done = final, true
		j.mu.Unlock()
	}()
	return Job{ID: id, LogPath: logFile.Name()}, nil
}

// BuildStatus reports a background build: running (with elapsed time), or its
// final summary once done.
func BuildStatus(id string) (BuildSummary, error) {
	jobsMu.Lock()
	j, ok := jobs[id]
	jobsMu.Unlock()
	if !ok {
		return BuildSummary{}, fmt.Errorf("unknown build job %q (jobs live only as long as this server process)", id)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	s := j.summary
	if !j.done {
		s.Duration = time.Since(j.started)
	}
	return s, nil
}

func taskOrDefault(task string) string {
	if task == "" {
		return defaultBuildTask
	}
	return task
}

// prepare builds the wrapper command and opens a fresh log file for its output.
func prepare(ctx context.Context, projectDir, task string, extra []string) (*exec.Cmd, *os.File, error) {
	cmd, err := gradleCmd(ctx, projectDir, append([]string{task}, extra...)...)
	if err != nil {
		return nil, nil, err
	}
	dir := filepath.Join(os.TempDir(), "adb-mcp-gradle")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	logFile, err := os.CreateTemp(dir, "build-*.log")
	if err != nil {
		return nil, nil, err
	}
	return cmd, logFile, nil
}

// execute runs cmd with its output redacted into logFile, then summarizes it.
func execute(cmd *exec.Cmd, logFile *os.File, projectDir string, s BuildSummary) BuildSummary {
	defer logFile.Close()
	w := &redactingWriter{dst: logFile}
	cmd.Stdout, cmd.Stderr = w, w
	start := time.Now()
	runErr := cmd.Run()
	w.flush()
	s.Duration = time.Since(start)
	if runErr != nil {
		s.Status, s.Err = StatusFailed, fmt.Sprintf("gradle %s failed: %v", s.Task, runErr)
	} else {
		s.Status = StatusPassed
		s.APKs = FindAPKs(projectDir)
	}
	if out, err := os.ReadFile(logFile.Name()); err == nil {
		s.ErrorCount, s.FirstErrors = scanErrors(string(out))
	}
	return s
}

// scanErrors counts compiler-style error lines (Kotlin "e: …", javac
// "…: error: …") and returns the first few.
func scanErrors(out string) (count int, first []string) {
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "e: ") || strings.Contains(line, ": error:") {
			count++
			if len(first) < maxFirstErrors {
				first = append(first, line)
			}
		}
	}
	return count, first
}

// gradleCmd resolves the wrapper in projectDir and returns the command that
// runs args; every Gradle entry point goes through it.
func gradleCmd(ctx context.Context, projectDir string, args ...string) (*exec.Cmd, error) {
	if strings.TrimSpace(projectDir) == "" {
		return nil, fmt.Errorf("project_dir is required (path to the Android project root containing gradlew)")
	}
	gradlew := filepath.Join(projectDir, wrapperName())
	if !sdk.FileExists(gradlew) {
		return nil, fmt.Errorf("%s not found in %s — point project_dir at the module/root that has the Gradle wrapper", wrapperName(), projectDir)
	}
	cmd := exec.CommandContext(ctx, gradlew, args...)
	cmd.Dir = projectDir
	cmd.Env = sdk.CommandEnv()
	return cmd, nil
}

// redactingWriter redacts secrets line by line before they reach dst, so the
// log file on disk never holds one.
type redactingWriter struct {
	dst io.Writer
	buf bytes.Buffer
}

func (w *redactingWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			return len(p), nil
		}
		line := string(w.buf.Next(i + 1))
		if _, err := io.WriteString(w.dst, RedactSecrets(line)); err != nil {
			return 0, err
		}
	}
}

func (w *redactingWriter) flush() {
	if w.buf.Len() > 0 {
		_, _ = io.WriteString(w.dst, RedactSecrets(w.buf.String()))
		w.buf.Reset()
	}
}
