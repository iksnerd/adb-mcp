package gradle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/iksnerd/adb_mcp/internal/sdk"
)

// Gradle runs one or more Gradle tasks in projectDir using its wrapper
// (./gradlew) and returns combined stdout+stderr. Gradle runs on the host, not
// on a device, so no serial is involved.
func Gradle(ctx context.Context, projectDir string, args ...string) (string, error) {
	if strings.TrimSpace(projectDir) == "" {
		return "", fmt.Errorf("project_dir is required (path to the Android project root containing gradlew)")
	}
	gradlew := filepath.Join(projectDir, wrapperName())
	if !sdk.FileExists(gradlew) {
		return "", fmt.Errorf("%s not found in %s — point project_dir at the module/root that has the Gradle wrapper", wrapperName(), projectDir)
	}
	cmd := exec.CommandContext(ctx, gradlew, args...)
	cmd.Dir = projectDir
	cmd.Env = sdk.CommandEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("gradle %s failed: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// FindAPKs returns any .apk files under projectDir's build outputs, newest
// modification time first — so right after a build, the first entry is the
// artifact that build just produced, not a stale one that happens to sort
// earlier. Directories that can never contain build outputs (node_modules,
// .git, .gradle and other dot-dirs) are pruned so the walk stays cheap in
// large (React Native) projects.
func FindAPKs(projectDir string) []string {
	type apkFile struct {
		path string
		mod  time.Time
	}
	var apks []apkFile
	_ = filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if name := d.Name(); path != projectDir &&
				(name == "node_modules" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".apk") && strings.Contains(filepath.ToSlash(path), "/build/outputs/") {
			var mod time.Time
			if info, ierr := d.Info(); ierr == nil {
				mod = info.ModTime()
			}
			apks = append(apks, apkFile{path: path, mod: mod})
		}
		return nil
	})
	sort.SliceStable(apks, func(i, j int) bool {
		if !apks[i].mod.Equal(apks[j].mod) {
			return apks[i].mod.After(apks[j].mod)
		}
		return apks[i].path < apks[j].path
	})
	out := make([]string, len(apks))
	for i, a := range apks {
		out[i] = a.path
	}
	return out
}

// PickAPK chooses which APK from a FindAPKs result to install: the first
// (newest) one that is not an androidTest instrumentation APK, falling back
// to the first entry when only test APKs exist.
func PickAPK(apks []string) string {
	if len(apks) == 0 {
		return ""
	}
	for _, a := range apks {
		if !strings.Contains(filepath.ToSlash(a), "/androidTest/") {
			return a
		}
	}
	return apks[0]
}

func wrapperName() string {
	if runtime.GOOS == "windows" {
		return "gradlew.bat"
	}
	return "gradlew"
}
