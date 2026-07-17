// Package selfupdate implements `adb-mcp update`: fetch the latest GitHub
// release, verify its checksum, and atomically replace the running binary.
// It mirrors what install.sh does, so an installed binary can update itself
// without Go, curl scripts, or a package manager.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const repo = "iksnerd/adb_mcp"

// maxArchiveBytes bounds release downloads (and the extracted binary) so a
// compromised or corrupted release can't OOM the updater. Real archives are
// a few MB.
const maxArchiveBytes = 200 << 20

// Run checks the latest release and, when it is newer than currentVersion,
// downloads, verifies, and installs it over the running executable. Progress
// goes to out.
func Run(ctx context.Context, currentVersion string, out io.Writer) error {
	client := &http.Client{Timeout: 120 * time.Second}

	tag, err := latestTag(ctx, client)
	if err != nil {
		return err
	}
	if strings.TrimPrefix(tag, "v") == strings.TrimPrefix(currentVersion, "v") {
		fmt.Fprintf(out, "adb-mcp %s is already the latest release.\n", currentVersion)
		return nil
	}
	fmt.Fprintf(out, "updating adb-mcp %s -> %s\n", currentVersion, tag)

	asset := assetName(tag, runtime.GOOS, runtime.GOARCH)
	base := "https://github.com/" + repo + "/releases/download/" + tag + "/"

	archive, err := fetch(ctx, client, base+asset)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}
	sums, err := fetch(ctx, client, base+"checksums.txt")
	if err != nil {
		return fmt.Errorf("download checksums.txt: %w", err)
	}
	if err := verifyChecksum(archive, string(sums), asset); err != nil {
		return err
	}
	fmt.Fprintln(out, "checksum verified")

	bin, err := extractBinary(archive, asset)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running binary: %w", err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return fmt.Errorf("resolve running binary path: %w", err)
	}
	if err := replaceBinary(exe, bin); err != nil {
		return err
	}
	fmt.Fprintf(out, "installed %s at %s\n", tag, exe)
	return nil
}

func latestTag(ctx context.Context, client *http.Client) (string, error) {
	body, err := fetch(ctx, client, "https://api.github.com/repos/"+repo+"/releases/latest")
	if err != nil {
		return "", fmt.Errorf("query latest release: %w", err)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil || rel.TagName == "" {
		return "", fmt.Errorf("unexpected release metadata from GitHub")
	}
	return rel.TagName, nil
}

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxArchiveBytes {
		return nil, fmt.Errorf("GET %s: response exceeds %d bytes", url, maxArchiveBytes)
	}
	return data, nil
}

// assetName mirrors the naming scheme of .github/workflows/release.yml.
func assetName(tag, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "adb-mcp_" + tag + "_" + goos + "_" + goarch + ext
}

// verifyChecksum checks data against the `sha256sum`-format checksums file
// shipped with each release.
func verifyChecksum(data []byte, checksums, asset string) error {
	want := ""
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("%s not found in checksums.txt", asset)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s (got %s, want %s) — aborting update", asset, got, want)
	}
	return nil
}

// extractBinary pulls the adb-mcp binary out of a release archive
// (.tar.gz everywhere, .zip on Windows).
func extractBinary(archive []byte, asset string) ([]byte, error) {
	want := "adb-mcp"
	if strings.HasSuffix(asset, ".zip") {
		want = "adb-mcp.exe"
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, fmt.Errorf("open release zip: %w", err)
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) != want {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return readAllLimited(rc)
		}
		return nil, fmt.Errorf("%s not found in %s", want, asset)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open release archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s not found in %s", want, asset)
		}
		if err != nil {
			return nil, fmt.Errorf("read release archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == want {
			return readAllLimited(tr)
		}
	}
}

func readAllLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxArchiveBytes {
		return nil, fmt.Errorf("extracted binary exceeds %d bytes", maxArchiveBytes)
	}
	return data, nil
}

// replaceBinary swaps the file at exe for the new binary as atomically as the
// platform allows: write next to it, then rename into place (the running
// process keeps executing its old inode). The old binary is first moved aside
// rather than overwritten, which also works on Windows where a running
// executable cannot be replaced in place.
func replaceBinary(exe string, bin []byte) error {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".adb-mcp-update-*")
	if err != nil {
		return fmt.Errorf("stage new binary (is %s writable?): %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after the successful rename

	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}

	old := exe + ".old"
	os.Remove(old) // a leftover from a previous update; ignore errors
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("move current binary aside: %w", err)
	}
	if err := os.Rename(tmpName, exe); err != nil {
		// Try to put the old binary back so the install isn't left broken.
		if rbErr := os.Rename(old, exe); rbErr != nil {
			return fmt.Errorf("install new binary: %w (and restoring the old one failed: %v — reinstall manually)", err, rbErr)
		}
		return fmt.Errorf("install new binary: %w (old binary restored)", err)
	}
	// Best-effort: Windows can refuse to delete the still-running old binary;
	// the next update's os.Remove(old) will clean it up.
	os.Remove(old)
	return nil
}
