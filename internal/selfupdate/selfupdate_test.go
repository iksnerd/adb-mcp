package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestAssetName(t *testing.T) {
	if got := assetName("v0.11.0", "darwin", "arm64"); got != "adb-mcp_v0.11.0_darwin_arm64.tar.gz" {
		t.Errorf("assetName darwin = %q", got)
	}
	if got := assetName("v0.11.0", "windows", "amd64"); got != "adb-mcp_v0.11.0_windows_amd64.zip" {
		t.Errorf("assetName windows = %q", got)
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("release archive bytes")
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:]) + "  adb-mcp_v1_darwin_arm64.tar.gz\nabc123  other.zip\n"

	if err := verifyChecksum(data, good, "adb-mcp_v1_darwin_arm64.tar.gz"); err != nil {
		t.Errorf("expected checksum to verify: %v", err)
	}
	if err := verifyChecksum([]byte("tampered"), good, "adb-mcp_v1_darwin_arm64.tar.gz"); err == nil {
		t.Error("expected mismatch error for tampered data")
	}
	if err := verifyChecksum(data, good, "missing.tar.gz"); err == nil {
		t.Error("expected error for asset absent from checksums")
	}
}

func TestExtractBinaryTarGz(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	payload := []byte("fake-elf")
	_ = tw.WriteHeader(&tar.Header{Name: "adb-mcp", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(payload)
	_ = tw.Close()
	_ = gz.Close()

	got, err := extractBinary(buf.Bytes(), "adb-mcp_v1_linux_amd64.tar.gz")
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("extractBinary tar.gz = %q, %v", got, err)
	}

	if _, err := extractBinary(buf.Bytes(), "adb-mcp_v1_windows_amd64.zip"); err == nil {
		t.Error("expected zip parse of a tar.gz to fail")
	}
}

func TestExtractBinaryZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("adb-mcp.exe")
	payload := []byte("fake-pe")
	_, _ = w.Write(payload)
	_ = zw.Close()

	got, err := extractBinary(buf.Bytes(), "adb-mcp_v1_windows_amd64.zip")
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("extractBinary zip = %q, %v", got, err)
	}
}
