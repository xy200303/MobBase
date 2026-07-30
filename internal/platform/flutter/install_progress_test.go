package flutter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDownloadReportsProgress(t *testing.T) {
	content := make([]byte, 3*1024*1024)
	for index := range content {
		content[index] = byte(index % 251)
	}
	sum := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		_, _ = w.Write(content)
	}))
	defer server.Close()

	var reports []DownloadProgress
	archive, err := download(context.Background(), t.TempDir(), Release{Archive: server.URL + "/flutter.zip", SHA256: hex.EncodeToString(sum[:])}, func(progress DownloadProgress) {
		reports = append(reports, progress)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) < 2 {
		t.Fatalf("expected progress reports, got %d", len(reports))
	}
	if reports[0].Total != int64(len(content)) {
		t.Fatalf("unexpected total: %+v", reports[0])
	}
	last := reports[len(reports)-1]
	if last.Downloaded != int64(len(content)) {
		t.Fatalf("unexpected final downloaded: %+v", last)
	}
	if got := filepath.Ext(archive); got != ".zip" {
		t.Fatalf("unexpected archive path: %s", archive)
	}
}

func TestDownloadWithoutReportDoesNotPanic(t *testing.T) {
	content := []byte("archive")
	sum := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		_, _ = w.Write(content)
	}))
	defer server.Close()
	if _, err := download(context.Background(), t.TempDir(), Release{Archive: server.URL + "/flutter.zip", SHA256: hex.EncodeToString(sum[:])}, nil); err != nil {
		t.Fatal(err)
	}
}
