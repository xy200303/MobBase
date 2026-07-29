package android

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifySHA1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	if err := verifySHA1(path, "a9993e364706816aba3e25717850c26c9cd0d89d"); err != nil {
		t.Fatalf("verify expected checksum: %v", err)
	}
	if err := verifySHA1(path, "0000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected mismatched checksum to fail")
	}
}

func TestProgressReaderReportsDownloadedBytes(t *testing.T) {
	var reports []DownloadProgress
	reader := &progressReader{Reader: strings.NewReader("mob"), total: 3, report: func(progress DownloadProgress) {
		reports = append(reports, progress)
	}}
	reader.notify()
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("read progress reader: %v", err)
	}
	if len(reports) < 2 {
		t.Fatalf("report count = %d, want at least start and completion", len(reports))
	}
	final := reports[len(reports)-1]
	if final.Downloaded != 3 || final.Total != 3 {
		t.Fatalf("final progress = %#v", final)
	}
}
