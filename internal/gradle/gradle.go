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

	"github.com/iksnerd/adb-mcp/internal/sdk"
)

// Gradle runs one or more Gradle tasks in projectDir using its wrapper
// (./gradlew) and returns combined stdout+stderr. Gradle runs on the host, not
// on a device, so no serial is involved.
func Gradle(ctx context.Context, projectDir string, args ...string) (string, error) {
	cmd, err := gradleCmd(ctx, projectDir, args...)
	if err != nil {
		return "", err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return RedactSecrets(string(out)), fmt.Errorf("gradle %s failed: %w", strings.Join(args, " "), err)
	}
	return RedactSecrets(string(out)), nil
}

// WrapperGradleVersion is the Gradle version `gradle wrapper` pins in a
// scaffolded project. The scaffold uses Android Gradle Plugin 8.7.3, which
// needs Gradle 8.9 or newer; an older system Gradle would otherwise generate a
// wrapper for itself and the project would not build.
const WrapperGradleVersion = "8.9"

// GenerateWrapper runs the *system* `gradle wrapper` in projectDir to create
// the wrapper scripts and gradle-wrapper.jar. Every other Gradle call in this
// package goes through the wrapper, so a freshly scaffolded project can't be
// built until this has run once — but the wrapper jar is a binary artifact
// that can't simply be templated out, so it has to come from a real Gradle.
//
// Returns a false ok (not an error) when no `gradle` is on PATH: that is a
// normal, recoverable state the caller reports as a next step, not a failure
// of the thing the user actually asked for.
func GenerateWrapper(ctx context.Context, projectDir string) (out string, ok bool, err error) {
	bin, lookErr := exec.LookPath("gradle")
	if lookErr != nil {
		return "", false, nil
	}
	cmd := exec.CommandContext(ctx, bin, "wrapper", "--gradle-version", WrapperGradleVersion)
	cmd.Dir = projectDir
	cmd.Env = sdk.CommandEnv()
	b, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return RedactSecrets(string(b)), false, fmt.Errorf("gradle wrapper failed: %w", runErr)
	}
	return RedactSecrets(string(b)), true, nil
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

// VariantFromTask derives the build variant an assemble task produces, in the
// lower-camel form ListVariants uses: assembleDebug -> debug,
// assembleFooRelease -> fooRelease. A module prefix (":app:assembleDebug") is
// ignored. It returns "" for anything that does not name one variant (bare
// assemble, build, bundle*, the AndroidTest/UnitTest tasks).
func VariantFromTask(task string) string {
	if i := strings.LastIndex(task, ":"); i >= 0 {
		task = task[i+1:]
	}
	if v := ParseVariants(task); len(v) == 1 {
		return v[0]
	}
	return ""
}

// apkVariantDir returns the directory segments between build/outputs/apk/ and
// the file, joined and lower-cased: ".../apk/free/debug/x.apk" -> "freedebug",
// which equals the lower-cased variant "freeDebug". "" if the path has no
// outputs/apk part.
func apkVariantDir(apk string) string {
	slash := filepath.ToSlash(apk)
	_, rest, ok := strings.Cut(slash, "/outputs/apk/")
	if !ok {
		return ""
	}
	dir := rest[:max(0, strings.LastIndex(rest, "/"))]
	return strings.ToLower(strings.ReplaceAll(dir, "/", ""))
}

// PickAPKForBuild chooses the APK to install after running task. When the task
// names a variant, only APKs in that variant's output directory qualify (newest
// first within it), so an UP-TO-DATE assembleDebug cannot install a newer
// release APK left by another build; if none matches, it is an error rather
// than a silent pick of another variant. Without a variant it falls back to
// PickAPK. note is non-empty when the chosen APK is older than started: a
// legitimately up-to-date build, but worth saying.
func PickAPKForBuild(apks []string, task string, started time.Time) (apk, note string, err error) {
	variant := VariantFromTask(task)
	if variant == "" {
		apk = PickAPK(apks)
	} else {
		want := strings.ToLower(variant)
		var matching []string
		for _, a := range apks {
			if apkVariantDir(a) == want {
				matching = append(matching, a)
			}
		}
		if len(matching) == 0 {
			return "", "", fmt.Errorf("no APK for variant %q found among %d APK(s) under build/outputs/apk/ — the task %s may not produce one for it", variant, len(apks), task)
		}
		apk = matching[0]
	}
	if apk != "" && !started.IsZero() {
		if info, serr := os.Stat(apk); serr == nil && info.ModTime().Before(started.Add(-mtimeSlack)) {
			note = fmt.Sprintf("%s predates this build (modified %s): Gradle treated %s as up to date, so this is the existing APK, not a rebuilt one.", apk, info.ModTime().Format(time.RFC3339), task)
		}
	}
	return apk, note, nil
}

// WrapperName is the Gradle wrapper script name for this OS.
func WrapperName() string {
	if runtime.GOOS == "windows" {
		return "gradlew.bat"
	}
	return "gradlew"
}
