// Package bridgeupdate implements `adb-mcp bridge install`: download the
// accessibility bridge APK from the latest GitHub release, verify its
// checksum, install it on a device, and enable its AccessibilityService.
// It mirrors internal/selfupdate's fetch/verify shape, but installs to a
// device instead of replacing the running binary.
package bridgeupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/iksnerd/adb-mcp/internal/adb"
)

const (
	repo         = "iksnerd/adb-mcp"
	apkAssetName = "adb-mcp-bridge.apk"
	maxAPKBytes  = 20 << 20
)

// Run downloads the latest bridge APK, verifies its checksum, installs it on
// the resolved device, and enables its AccessibilityService. Progress goes
// to out.
func Run(ctx context.Context, serial string, out io.Writer) error {
	resolved, err := adb.ResolveSerial(ctx, serial)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "installing accessibility bridge on %s\n", resolved)

	client := &http.Client{Timeout: 60 * time.Second}
	tag, err := latestTag(ctx, client)
	if err != nil {
		return err
	}
	base := "https://github.com/" + repo + "/releases/download/" + tag + "/"

	apk, err := fetch(ctx, client, base+apkAssetName)
	if err != nil {
		return fmt.Errorf("download %s: %w", apkAssetName, err)
	}
	sums, err := fetch(ctx, client, base+"checksums.txt")
	if err != nil {
		return fmt.Errorf("download checksums.txt: %w", err)
	}
	if err := verifyChecksum(apk, string(sums), apkAssetName); err != nil {
		return err
	}
	fmt.Fprintln(out, "checksum verified")

	tmp, err := os.CreateTemp("", "adb-mcp-bridge-*.apk")
	if err != nil {
		return fmt.Errorf("stage bridge apk: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(apk); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	c := adb.New(resolved)
	if _, err := c.InstallBridge(ctx, tmp.Name()); err != nil {
		return fmt.Errorf("install bridge apk: %w", err)
	}
	fmt.Fprintf(out, "installed %s\n", tag)

	if err := c.EnableBridgeService(ctx); err != nil {
		return fmt.Errorf("enable bridge accessibility service: %w", err)
	}
	fmt.Fprintln(out, "accessibility service enabled — tap_on_text/tap_element can now use via_accessibility=true")
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAPKBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAPKBytes {
		return nil, fmt.Errorf("GET %s: response exceeds %d bytes", url, maxAPKBytes)
	}
	return data, nil
}

// verifyChecksum checks data against the `sha256sum`-format checksums file
// shipped with each release (the same file's format as internal/selfupdate).
func verifyChecksum(data []byte, checksums, asset string) error {
	want := ""
	for line := range strings.SplitSeq(checksums, "\n") {
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
		return fmt.Errorf("checksum mismatch for %s (got %s, want %s) — aborting install", asset, got, want)
	}
	return nil
}
