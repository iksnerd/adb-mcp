package gradle

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
	// logRetention is how long a build log stays in the log directory; older
	// ones are removed when a new log is created.
	logRetention = 24 * time.Hour
)

// waitDelay bounds how long a finished wrapper may keep the output pipes open.
// On Windows gradlew.bat leaves a child JVM holding them after it exits, which
// would otherwise block cmd.Run forever. A variable so tests can shorten it.
var waitDelay = 10 * time.Second

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
	// Started is when the Gradle process was started. Reports and APKs older
	// than this were not produced by this build.
	Started time.Time
	// LogMissing is set by BuildStatus when LogPath no longer exists (pruned
	// or deleted).
	LogMissing bool
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
		if s.LogMissing {
			b.WriteString(" (no longer exists; logs are pruned after a day)")
		}
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
	cancel  context.CancelFunc
}

// finished reports whether the build has left StatusRunning. Callers hold j.mu.
func (j *job) finished() bool { return j.summary.Status != StatusRunning }

// maxFinishedJobs is how many finished jobs stay queryable; older ones are
// evicted when a new build starts. Running jobs are never evicted.
const maxFinishedJobs = 32

// evictFinished drops the oldest finished jobs from m until at most keep
// remain. Callers hold jobsMu.
func evictFinished(m map[string]*job, keep int) {
	type entry struct {
		id      string
		started time.Time
	}
	var fin []entry
	for id, j := range m {
		j.mu.Lock()
		if j.finished() {
			fin = append(fin, entry{id, j.started})
		}
		j.mu.Unlock()
	}
	if len(fin) <= keep {
		return
	}
	sort.Slice(fin, func(a, b int) bool { return fin[a].started.Before(fin[b].started) })
	for _, e := range fin[:len(fin)-keep] {
		delete(m, e.id)
	}
}

// CancelAll cancels every running background build. The server calls it on
// shutdown so no Gradle process outlives it.
func CancelAll() {
	jobsMu.Lock()
	all := make([]*job, 0, len(jobs))
	for _, j := range jobs {
		all = append(all, j)
	}
	jobsMu.Unlock()
	for _, j := range all {
		j.cancel()
	}
}

// CancelBuild stops a running background build. Cancelling a finished build
// is a no-op. It is not wired to an MCP tool.
func CancelBuild(id string) error {
	jobsMu.Lock()
	j, ok := jobs[id]
	jobsMu.Unlock()
	if !ok {
		return fmt.Errorf("unknown build job %q (jobs live only as long as this server process, or were evicted)", id)
	}
	j.cancel()
	return nil
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
	j := &job{started: time.Now(), summary: BuildSummary{ID: id, Task: task, Status: StatusRunning, LogPath: logFile.Name()}, cancel: cancel}
	jobsMu.Lock()
	evictFinished(jobs, maxFinishedJobs)
	jobs[id] = j
	jobsMu.Unlock()
	go func() {
		defer cancel()
		final := execute(cmd, logFile, projectDir, BuildSummary{ID: id, Task: task, LogPath: logFile.Name()})
		j.mu.Lock()
		j.summary = final
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
		return BuildSummary{}, fmt.Errorf("unknown build job %q (jobs live only as long as this server process, or were evicted)", id)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	s := j.summary
	if !j.finished() {
		s.Duration = time.Since(j.started)
	}
	if s.LogPath != "" {
		if _, err := os.Stat(s.LogPath); err != nil {
			s.LogMissing = true
		}
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
	dir, err := logDir()
	if err != nil {
		return nil, nil, err
	}
	pruneLogs(dir, logRetention)
	logFile, err := os.CreateTemp(dir, "build-*.log")
	if err != nil {
		return nil, nil, err
	}
	return cmd, logFile, nil
}

// logDir returns the private directory build logs live in, creating it if
// needed. The directory sits in the shared temp dir, so another local user
// could have pre-created it (or planted a symlink) to read or redirect logs:
// refuse a symlink or a directory owned by someone else, and force mode 0700.
func logDir() (string, error) {
	dir := filepath.Join(os.TempDir(), "adb-mcp-gradle")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to write build logs: %s is a symlink", dir)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("refusing to write build logs: %s is not a directory", dir)
	}
	if !ownedByCurrentUser(info) {
		return "", fmt.Errorf("refusing to write build logs: %s is owned by another user", dir)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// pruneLogs removes build logs in dir older than maxAge. Best effort.
func pruneLogs(dir string, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "build-") || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// execute runs cmd with its output redacted into logFile, then summarizes it.
func execute(cmd *exec.Cmd, logFile *os.File, projectDir string, s BuildSummary) BuildSummary {
	defer logFile.Close()
	w := &redactingWriter{dst: logFile}
	cmd.Stdout, cmd.Stderr = w, w
	start := time.Now()
	s.Started = start
	runErr := cmd.Run()
	// The wrapper itself exited cleanly but a child kept the pipes open past
	// waitDelay: the build is done, only the pipes were abandoned.
	if errors.Is(runErr, exec.ErrWaitDelay) {
		runErr = nil
	}
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

// scanErrors counts the failures in a build log and returns the first few:
// compiler-style error lines (Kotlin "e: …", javac "…: error: …" or a bare
// "error: …") and, only when there are none, Gradle's own reason for the
// failure, the line under "* What went wrong:" (task not found, dependency
// resolution, and so on, which have no compiler line). When compiler lines
// exist that summary line only restates them, so it is not counted on top.
func scanErrors(out string) (count int, first []string) {
	var compiler []string
	var reason string
	afterWhatWentWrong := false
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "* What went wrong:":
			afterWhatWentWrong = true
		case afterWhatWentWrong && line != "":
			if reason == "" {
				reason = line
			}
			afterWhatWentWrong = false
		case strings.HasPrefix(line, "e: ") || strings.HasPrefix(line, "error:") || strings.Contains(line, ": error:"):
			compiler = append(compiler, line)
		}
	}
	if len(compiler) == 0 && reason != "" {
		compiler = []string{reason}
	}
	return len(compiler), compiler[:min(len(compiler), maxFirstErrors)]
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
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(gradlew); err == nil && info.Mode().Perm()&0o111 == 0 {
			return nil, fmt.Errorf("%s is not executable — run: chmod +x %s", gradlew, gradlew)
		}
	}
	cmd := exec.CommandContext(ctx, gradlew, args...)
	cmd.Dir = projectDir
	cmd.Env = sdk.CommandEnv()
	cmd.WaitDelay = waitDelay
	return cmd, nil
}

// redactingWriter redacts secrets line by line before they reach dst, so the
// log file on disk never holds one. A PEM private key spans many lines, so the
// writer also tracks whether it is inside one and drops the body.
type redactingWriter struct {
	dst   io.Writer
	buf   bytes.Buffer
	inPEM bool
}

func (w *redactingWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			return len(p), nil
		}
		line := string(w.buf.Next(i + 1))
		if err := w.emit(line); err != nil {
			return 0, err
		}
	}
}

// emit writes one (possibly final, newline-less) line, redacted.
func (w *redactingWriter) emit(line string) error {
	if w.inPEM {
		if pemEndRe.MatchString(line) {
			w.inPEM = false
		}
		return nil
	}
	if loc := pemBeginRe.FindStringIndex(line); loc != nil && !pemEndRe.MatchString(line[loc[1]:]) {
		w.inPEM = true
		nl := ""
		if strings.HasSuffix(line, "\n") {
			nl = "\n"
		}
		_, err := io.WriteString(w.dst, RedactSecrets(line[:loc[0]])+pemRedacted+nl)
		return err
	}
	_, err := io.WriteString(w.dst, RedactSecrets(line))
	return err
}

func (w *redactingWriter) flush() {
	if w.buf.Len() > 0 {
		_ = w.emit(w.buf.String())
		w.buf.Reset()
	}
}
