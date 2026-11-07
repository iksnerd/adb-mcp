package adb

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// MetroServer identifies the HOST-side dev server a device process is connected
// to. Knowing that the app is "on Metro" is not enough: a `expo run:android`
// left running from a previous branch keeps holding port 8081, so a dev client
// connects to it happily and reports a healthy `bundle_source: metro` while the
// JavaScript actually executing belongs to a different checkout. That reads as
// reassuring in exactly the wrong direction — worse than the embedded-bundle
// case, which at least looks wrong the moment you check it. The project root
// below (the listening process's working directory) is the one line that
// settles it.
type MetroServer struct {
	Port        int    `json:"port"`
	URL         string `json:"url"`
	PID         int    `json:"pid,omitempty"`
	Command     string `json:"command,omitempty"`
	ProjectRoot string `json:"project_root,omitempty"`
}

// HostMetroServer resolves which local process is listening on port and where
// it is rooted, via lsof. It is best-effort by design — no lsof (Windows), a
// server on another machine, or a permission-restricted process all yield
// ok=false, and the caller degrades to the port alone rather than failing.
func HostMetroServer(ctx context.Context, port int) (MetroServer, bool) {
	m := MetroServer{Port: port, URL: fmt.Sprintf("http://localhost:%d", port)}
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		return m, false
	}
	out, err := exec.CommandContext(ctx, lsof, "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fpc").Output()
	if err != nil {
		return m, false
	}
	pid, command, ok := parseLsofListener(string(out))
	if !ok {
		return m, false
	}
	m.PID, m.Command = pid, command
	if cwdOut, err := exec.CommandContext(ctx, lsof, "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output(); err == nil {
		if root, ok := parseLsofCWD(string(cwdOut)); ok {
			m.ProjectRoot = root
		}
	}
	return m, true
}

// parseLsofListener reads `lsof -Fpc` field output: one "p<pid>" line starting
// each process record, followed by "c<command>". Several processes can hold the
// same listening port (a forked server, or a stale one alongside a live one);
// the first record is reported. That is only a heuristic: lsof lists records in
// its own order and nothing guarantees the first is the process a new
// connection reaches, so callers should treat the result as "a listener", not
// "the listener".
func parseLsofListener(out string) (pid int, command string, ok bool) {
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			if pid != 0 {
				return pid, command, true // next record starts; we already have one
			}
			n, err := strconv.Atoi(line[1:])
			if err != nil {
				return 0, "", false
			}
			pid = n
		case 'c':
			if pid != 0 && command == "" {
				command = line[1:]
			}
		}
	}
	return pid, command, pid != 0
}

// parseLsofCWD reads `lsof -d cwd -Fn` field output, returning the "n<path>"
// line — the process's working directory, which for a dev server started with
// `npx expo start` / `npm start` is the project root.
func parseLsofCWD(out string) (string, bool) {
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "n") && len(line) > 1 {
			return line[1:], true
		}
	}
	return "", false
}

// SameTree reports whether two host paths belong to the same checkout: equal,
// or one containing the other when the outer one is a project root (has a
// package.json). Used to decide whether the Metro a device is
// talking to is serving the source_path the caller is reasoning about. Paths
// that cannot be resolved compare false, which surfaces as "couldn't confirm"
// rather than as a false all-clear.
func SameTree(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	pa, err1 := resolvePath(a)
	pb, err2 := resolvePath(b)
	if err1 != nil || err2 != nil {
		return false
	}
	if pa == pb {
		return true
	}
	// Containment counts only when the outer path is itself a project root: a
	// Metro started from ~ or / would otherwise match every checkout.
	if strings.HasPrefix(pa, pb+string(filepath.Separator)) {
		return hasProjectMarker(pb)
	}
	if strings.HasPrefix(pb, pa+string(filepath.Separator)) {
		return hasProjectMarker(pa)
	}
	return false
}

// hasProjectMarker reports whether dir is a JS project root (has package.json).
func hasProjectMarker(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "package.json"))
	return err == nil
}

// resolvePath makes a path absolute and symlink-free so two paths naming the
// same directory compare equal. Symlinks are resolved on the deepest ancestor
// that actually exists, then the remainder is re-appended: EvalSymlinks fails
// outright on a path whose tail is missing, and resolving one side while the
// other stays unresolved is how a macOS /var vs /private/var pair ends up
// looking like two different checkouts.
func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rest := ""
	for cur := abs; ; {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(resolved, rest), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil // nothing along the path exists; the literal path is the best answer
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}
