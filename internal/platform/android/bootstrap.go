package android

import (
	"context"
	"crypto/sha1"
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

const officialArchiveBaseURL = "https://dl.google.com/android/repository/"

type DownloadProgress struct {
	Downloaded int64
	Total      int64
}

// BootstrapCommandLineTools installs only the official command-line tools into
// an empty Mob-managed SDK root. It never replaces an existing installation.
func BootstrapCommandLineTools(ctx context.Context, root, cacheDirectory string, item CatalogItem, proxyURL string, report func(DownloadProgress)) error {
	if item.PackageID != "cmdline-tools;latest" {
		return fmt.Errorf("invalid command-line tools catalog item %q", item.PackageID)
	}
	if item.ChecksumAlgorithm != "sha1" || item.Checksum == "" {
		return fmt.Errorf("official command-line tools catalog item has no supported checksum")
	}
	if _, found := SDKManager(root); found {
		return nil
	}
	destination := filepath.Join(root, "cmdline-tools", "latest")
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("command-line tools destination already exists but sdkmanager is unavailable: %s", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect command-line tools destination: %w", err)
	}
	archivePath, err := downloadArchive(ctx, cacheDirectory, item, proxyURL, report)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create command-line tools directory: %w", err)
	}
	temporary, err := os.MkdirTemp(filepath.Dir(destination), "latest-install-*")
	if err != nil {
		return fmt.Errorf("create temporary command-line tools directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := system.ExtractZipPrefix(archivePath, temporary, "cmdline-tools"); err != nil {
		return fmt.Errorf("extract command-line tools: %w", err)
	}
	if info, err := os.Stat(filepath.Join(temporary, "bin", sdkManagerExecutable())); err != nil || info.IsDir() {
		return fmt.Errorf("command-line tools archive does not contain sdkmanager")
	}
	if err := os.Rename(temporary, destination); err != nil {
		return fmt.Errorf("publish command-line tools: %w", err)
	}
	return nil
}

func downloadArchive(ctx context.Context, cacheDirectory string, item CatalogItem, proxyURL string, report func(DownloadProgress)) (string, error) {
	if err := os.MkdirAll(cacheDirectory, 0o755); err != nil {
		return "", fmt.Errorf("create download cache: %w", err)
	}
	archivePath := filepath.Join(cacheDirectory, item.Checksum+".zip")
	if err := verifySHA1(archivePath, item.Checksum); err == nil {
		return archivePath, nil
	}
	parsedBase, err := url.Parse(officialArchiveBaseURL)
	if err != nil {
		return "", err
	}
	archiveURL, err := url.Parse(item.ArchiveURL)
	if err != nil {
		return "", fmt.Errorf("parse archive URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedBase.ResolveReference(archiveURL).String(), nil)
	if err != nil {
		return "", fmt.Errorf("create command-line tools download request: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return "", fmt.Errorf("parse Android download proxy: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	response, err := (&http.Client{Timeout: 30 * time.Minute, Transport: transport}).Do(request)
	if err != nil {
		return "", fmt.Errorf("download command-line tools: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("download command-line tools: server returned %s", response.Status)
	}
	temporary, err := os.CreateTemp(cacheDirectory, "download-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temporary download: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	total := response.ContentLength
	if total <= 0 {
		total = item.Size
	}
	reader := &progressReader{Reader: response.Body, total: total, report: report}
	reader.notify()
	if _, err := io.Copy(temporary, reader); err != nil {
		temporary.Close()
		return "", fmt.Errorf("save command-line tools download: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close command-line tools download: %w", err)
	}
	if err := verifySHA1(temporaryPath, item.Checksum); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, archivePath); err != nil {
		if verifyErr := verifySHA1(archivePath, item.Checksum); verifyErr == nil {
			return archivePath, nil
		}
		return "", fmt.Errorf("publish command-line tools download: %w", err)
	}
	return archivePath, nil
}

type progressReader struct {
	io.Reader
	downloaded int64
	total      int64
	report     func(DownloadProgress)
}

func (r *progressReader) Read(buffer []byte) (int, error) {
	count, err := r.Reader.Read(buffer)
	if count > 0 {
		r.downloaded += int64(count)
		r.notify()
	}
	return count, err
}

func (r *progressReader) notify() {
	if r.report != nil {
		r.report(DownloadProgress{Downloaded: r.downloaded, Total: r.total})
	}
}

func verifySHA1(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha1.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash archive: %w", err)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected) {
		return fmt.Errorf("archive checksum does not match official catalog")
	}
	return nil
}
