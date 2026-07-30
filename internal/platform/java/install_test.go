package java

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
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
	if _, err := download(context.Background(), t.TempDir(), Release{Archive: server.URL + "/jdk.zip", SHA256: hex.EncodeToString(sum[:])}, func(progress DownloadProgress) {
		reports = append(reports, progress)
	}); err != nil {
		t.Fatal(err)
	}
	if len(reports) < 2 {
		t.Fatalf("expected progress reports, got %d", len(reports))
	}
	if reports[0].Total != int64(len(content)) {
		t.Fatalf("unexpected total: %+v", reports[0])
	}
	if last := reports[len(reports)-1]; last.Downloaded != int64(len(content)) {
		t.Fatalf("unexpected final downloaded: %+v", last)
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
	if _, err := download(context.Background(), t.TempDir(), Release{Archive: server.URL + "/jdk.zip", SHA256: hex.EncodeToString(sum[:])}, nil); err != nil {
		t.Fatal(err)
	}
}
