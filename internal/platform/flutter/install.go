package flutter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xy200303/MobBase/internal/system"
)

const archiveBaseURL = "https://storage.googleapis.com/flutter_infra_release/releases/"

type DownloadProgress struct {
	Downloaded int64
	Total      int64
}

func Install(ctx context.Context, destination, cacheDirectory string, release Release, report func(DownloadProgress)) error {
	if len(release.SHA256) != 64 || !strings.HasSuffix(strings.ToLower(release.Archive), ".zip") {
		return fmt.Errorf("Flutter release %s is not a supported ZIP archive for this host", release.Version)
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("Flutter destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	archive, err := download(ctx, cacheDirectory, release, report)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(destination), "flutter-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := system.ExtractZipPrefix(archive, temporary, "flutter"); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(temporary, "bin")); err != nil {
		return fmt.Errorf("Flutter archive does not contain bin directory")
	}
	return os.Rename(temporary, destination)
}
func download(ctx context.Context, cache string, release Release, report func(DownloadProgress)) (string, error) {
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	archive := filepath.Join(cache, release.SHA256+".zip")
	if verifySHA256(archive, release.SHA256) == nil {
		return archive, nil
	}
	base, _ := url.Parse(archiveBaseURL)
	relative, err := url.Parse(release.Archive)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.ResolveReference(relative).String(), nil)
	if err != nil {
		return "", err
	}
	response, err := (&http.Client{Timeout: 30 * time.Minute}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("download Flutter archive: server returned %s", response.Status)
	}
	temporary, err := os.CreateTemp(cache, "flutter-download-*.zip")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	reader := &progressReader{Reader: response.Body, total: response.ContentLength, report: report}
	reader.notify()
	if _, err := io.Copy(temporary, reader); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := verifySHA256(temporaryPath, release.SHA256); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, archive); err != nil {
		return "", err
	}
	return archive, nil
}

type progressReader struct {
	io.Reader
	downloaded int64
	total      int64
	report     func(DownloadProgress)
}

func (r *progressReader) Read(buffer []byte) (int, error) {
	count, err := r.Reader.Read(buffer)
	r.downloaded += int64(count)
	if count > 0 {
		r.notify()
	}
	return count, err
}

func (r *progressReader) notify() {
	if r.report != nil {
		r.report(DownloadProgress{Downloaded: r.downloaded, Total: r.total})
	}
}

func verifySHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected) {
		return fmt.Errorf("Flutter archive checksum does not match official catalog")
	}
	return nil
}
