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

func Install(ctx context.Context, destination, cacheDirectory string, release Release) error {
	if len(release.SHA256) != 64 || !strings.HasSuffix(strings.ToLower(release.Archive), ".zip") {
		return fmt.Errorf("Flutter release %s is not a supported ZIP archive for this host", release.Version)
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("Flutter destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	archive, err := download(ctx, cacheDirectory, release)
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
func download(ctx context.Context, cache string, release Release) (string, error) {
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
	if _, err := io.Copy(temporary, response.Body); err != nil {
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
