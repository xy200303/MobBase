package fvm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

func testArchive(t *testing.T) ([]byte, string) {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("name: fvm\n")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "pubspec.yaml", Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes(), fmt.Sprintf("%x", sha256.Sum256(buffer.Bytes()))
}

func TestDownloadAndExtractReportsProgress(t *testing.T) {
	archive, sum := testArchive(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	var reports []DownloadProgress
	destination := filepath.Join(t.TempDir(), "fvm")
	if err := DownloadAndExtract(context.Background(), Release{ArchiveURL: server.URL, SHA256: sum}, destination, func(progress DownloadProgress) {
		reports = append(reports, progress)
	}); err != nil {
		t.Fatal(err)
	}
	if len(reports) < 2 {
		t.Fatalf("expected progress reports, got %d", len(reports))
	}
	if reports[0].Total != int64(len(archive)) {
		t.Fatalf("unexpected total: %+v", reports[0])
	}
	if last := reports[len(reports)-1]; last.Downloaded != int64(len(archive)) {
		t.Fatalf("unexpected final downloaded: %+v", last)
	}
}

func TestDownloadAndExtractWithoutReportDoesNotPanic(t *testing.T) {
	archive, sum := testArchive(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	if err := DownloadAndExtract(context.Background(), Release{ArchiveURL: server.URL, SHA256: sum}, filepath.Join(t.TempDir(), "fvm"), nil); err != nil {
		t.Fatal(err)
	}
}
