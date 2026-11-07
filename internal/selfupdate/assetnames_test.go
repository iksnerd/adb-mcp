package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The release asset names are spelled out in the producer
// (.github/workflows/release.yml) and in several consumers: assetName here,
// install.sh, the plugin launcher and bridgeupdate. Nothing tied them together,
// so a rename on one side would fail only for users, after the release. This
// test reads the producer's name template and matrix and makes every consumer
// agree with it.

type target struct{ goos, goarch string }

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func must(t *testing.T, re, src string) []string {
	t.Helper()
	m := regexp.MustCompile(re).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("pattern %q found nothing: the file's shape changed, update this test with it", re)
	}
	return m
}

func releaseMatrix(t *testing.T, workflow string) []target {
	t.Helper()
	list := must(t, `for p in ((?:[a-z]+/[a-z0-9]+ ?)+); do`, workflow)[1]
	var out []target
	for _, p := range strings.Fields(list) {
		goos, goarch, _ := strings.Cut(p, "/")
		out = append(out, target{goos, goarch})
	}
	if len(out) == 0 {
		t.Fatal("release.yml lists no build targets")
	}
	return out
}

func expand(tmpl string, vars map[string]string) string {
	for k, v := range vars {
		tmpl = strings.ReplaceAll(tmpl, "${"+k+"}", v)
	}
	return tmpl
}

func TestReleaseAssetNamesAgreeAcrossProducerAndConsumers(t *testing.T) {
	workflow := repoFile(t, ".github/workflows/release.yml")
	install := repoFile(t, "install.sh")
	matrix := releaseMatrix(t, workflow)

	base := must(t, `base="(adb-mcp_[^"]+)"`, workflow)[1]
	gzTmpl := must(t, `gzip -9 -n -c "\$out" > "dist/([^"]+\.gz)"`, workflow)[1]
	archiveTmpl := must(t, `(?m)^\s*archive="([^"]+)"`, install)[1]

	const tag = "v9.8.7"
	for _, tg := range matrix {
		vars := map[string]string{"VERSION": tag, "os": tg.goos, "arch": tg.goarch, "tag": tag}
		wantBase := expand(base, vars)

		// Archive: .zip on Windows, .tar.gz elsewhere.
		wantArchive := wantBase + ".tar.gz"
		if tg.goos == "windows" {
			wantArchive = wantBase + ".zip"
		}
		if got := assetName(tag, tg.goos, tg.goarch); got != wantArchive {
			t.Errorf("assetName(%s, %s/%s) = %s, release.yml builds %s", tag, tg.goos, tg.goarch, got, wantArchive)
		}
		if tg.goos != "windows" {
			if got := expand(archiveTmpl, vars); got != wantArchive {
				t.Errorf("install.sh asks for %s on %s/%s, release.yml builds %s", got, tg.goos, tg.goarch, wantArchive)
			}
		}

		// Bare gzip build the plugin launcher downloads: ${out#adb-mcp} is ".exe" on Windows.
		suffix := ""
		if tg.goos == "windows" {
			suffix = ".exe"
		}
		wantGz := strings.ReplaceAll(expand(gzTmpl, vars), "${base}", wantBase)
		wantGz = strings.ReplaceAll(wantGz, "${out#adb-mcp}", suffix)
		launcherAsset(t, tg, tag, wantGz)
	}

	// install.sh must name every OS the matrix builds a tarball for.
	for _, tg := range matrix {
		if tg.goos == "windows" {
			continue
		}
		if !strings.Contains(install, tg.goos+")") && !strings.Contains(install, tg.goos+"|") {
			t.Errorf("install.sh has no case for %s, which release.yml builds", tg.goos)
		}
	}

	// The bridge APK is published under one fixed name.
	apk := must(t, `apkAssetName\s*=\s*"([^"]+)"`, repoFile(t, "internal/bridgeupdate/bridgeupdate.go"))[1]
	if want := must(t, `cp app/build/outputs/apk/release/app-release\.apk \.\./dist/(\S+)`, workflow)[1]; apk != want {
		t.Errorf("bridgeupdate fetches %s, release.yml publishes %s", apk, want)
	}
}

// launcherAsset asks the real launcher for the URL it would download and checks
// its file name against what the workflow builds.
func launcherAsset(t *testing.T, tg target, tag, want string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the launcher half of this test needs it")
	}
	platform := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "win32"}[tg.goos]
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[tg.goarch]
	if platform == "" || arch == "" {
		t.Errorf("the launcher has no mapping for %s/%s, which release.yml builds", tg.goos, tg.goarch)
		return
	}
	script := `const {resolveTarget}=require(process.argv[1]);process.stdout.write(resolveTarget(process.argv[2],process.argv[3],process.argv[4]).url)`
	launcher, _ := filepath.Abs(filepath.Join("..", "..", "plugin/adb-mcp/launcher/launch.js"))
	out, err := exec.Command(node, "-e", script, launcher, platform, arch, strings.TrimPrefix(tag, "v")).Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	if got := filepath.Base(string(out)); got != want {
		t.Errorf("launcher downloads %s for %s/%s, release.yml builds %s", got, tg.goos, tg.goarch, want)
	}
}

// sha256sum writes "*" before the name in binary mode; both parsers must read it.
func TestVerifyChecksumAcceptsTheBinaryModeMarker(t *testing.T) {
	data := []byte("payload")
	sum := "239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5"
	for _, line := range []string{
		sum + "  asset.tar.gz\n",
		sum + " *asset.tar.gz\n",
		"0000  other.tar.gz\n" + sum + " *asset.tar.gz\n",
	} {
		if err := verifyChecksum(data, line, "asset.tar.gz"); err != nil {
			t.Errorf("verifyChecksum(%q) = %v", line, err)
		}
	}
}
