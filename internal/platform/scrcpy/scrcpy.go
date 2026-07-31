// Package scrcpy provides Mob's internal Android physical-device preview runtime.
package scrcpy

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/xy200303/MobBase/internal/system"
)

const officialLatestReleaseURL = "https://api.github.com/repos/Genymobile/scrcpy/releases/latest"

type Release struct {
	Version string `json:"version"`
	Archive string `json:"archive"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

type Progress struct {
	Downloaded int64
	Total      int64
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Hash string `json:"digest"`
	Size int64  `json:"size"`
}

func LatestRelease(ctx context.Context) (Release, error) {
	if runtime.GOOS != "windows" {
		return Release{}, fmt.Errorf("Mob-managed scrcpy installation is currently supported on Windows only")
	}
	if runtime.GOARCH != "amd64" {
		return Release{}, fmt.Errorf("Mob-managed scrcpy installation currently requires Windows x64")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, officialLatestReleaseURL, nil)
	if err != nil {
		return Release{}, fmt.Errorf("create scrcpy release request: %w", err)
	}
	request.Header.Set("User-Agent", "MobBase")
	response, err := (&http.Client{Timeout: 45 * time.Second}).Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("fetch official scrcpy release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Release{}, fmt.Errorf("fetch official scrcpy release: server returned %s", response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return Release{}, fmt.Errorf("read official scrcpy release: %w", err)
	}
	return parseLatestRelease(data)
}

func parseLatestRelease(data []byte) (Release, error) {
	var payload githubRelease
	if err := json.Unmarshal(data, &payload); err != nil {
		return Release{}, fmt.Errorf("parse official scrcpy release: %w", err)
	}
	if payload.TagName == "" {
		return Release{}, fmt.Errorf("official scrcpy release does not include a version")
	}
	assetName := "scrcpy-win64-" + payload.TagName + ".zip"
	for _, asset := range payload.Assets {
		if asset.Name != assetName {
			continue
		}
		hash := strings.TrimPrefix(strings.ToLower(asset.Hash), "sha256:")
		if len(hash) != 64 || asset.Size <= 0 || !strings.HasPrefix(asset.URL, "https://github.com/Genymobile/scrcpy/releases/download/") {
			return Release{}, fmt.Errorf("official scrcpy Windows asset metadata is incomplete")
		}
		return Release{Version: payload.TagName, Archive: asset.URL, SHA256: hash, Size: asset.Size}, nil
	}
	return Release{}, fmt.Errorf("official scrcpy release %s has no supported Windows x64 archive", payload.TagName)
}

func RuntimeRoot(home string) string {
	return filepath.Join(home, "runtime", "scrcpy")
}

func RuntimeExecutable(home string) (string, bool) {
	path := filepath.Join(RuntimeRoot(home), "scrcpy.exe")
	info, err := os.Stat(path)
	return path, err == nil && !info.IsDir()
}

// RuntimeServer returns the official server shipped alongside the managed
// scrcpy client. It is used for Mob's loopback video protocol, never exposed
// as a user-managed Android SDK component.
func RuntimeServer(home string) (string, bool) {
	path := filepath.Join(RuntimeRoot(home), "scrcpy-server")
	info, err := os.Stat(path)
	return path, err == nil && !info.IsDir()
}

var versionLine = regexp.MustCompile(`(?m)^scrcpy\s+([^\s]+)`)

// RuntimeVersion obtains the server-compatible version from the bundled
// native client. scrcpy rejects a server invocation with a mismatched client
// version, so the version must never be guessed by the caller.
func RuntimeVersion(ctx context.Context, executable string) (string, error) {
	command := exec.CommandContext(ctx, executable, "--version")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read scrcpy runtime version: %w", err)
	}
	match := versionLine.FindStringSubmatch(string(output))
	if len(match) != 2 || strings.TrimSpace(match[1]) == "" {
		return "", fmt.Errorf("read scrcpy runtime version: unexpected output")
	}
	return strings.TrimSpace(match[1]), nil
}

func Install(ctx context.Context, destination, cache string, release Release, report func(Progress)) error {
	if runtime.GOOS != "windows" || len(release.SHA256) != 64 || !strings.HasSuffix(strings.ToLower(release.Archive), ".zip") {
		return fmt.Errorf("scrcpy release is not a supported Windows archive")
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("scrcpy destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	archive, err := download(ctx, cache, release, report)
	if err != nil {
		return err
	}
	prefix, err := zipPrefix(archive)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(destination), "scrcpy-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := system.ExtractZipPrefix(archive, temporary, prefix); err != nil {
		return fmt.Errorf("extract scrcpy: %w", err)
	}
	if executable := filepath.Join(temporary, "scrcpy.exe"); !regularFile(executable) {
		return fmt.Errorf("official scrcpy archive does not contain scrcpy.exe")
	}
	return os.Rename(temporary, destination)
}

func download(ctx context.Context, cache string, release Release, report func(Progress)) (string, error) {
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	archive := filepath.Join(cache, release.SHA256+".zip")
	if verifySHA256(archive, release.SHA256) == nil {
		return archive, nil
	}
	if err := os.Remove(archive); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove invalid cached scrcpy archive: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, release.Archive, nil)
	if err != nil {
		return "", err
	}
	response, err := (&http.Client{Timeout: 30 * time.Minute}).Do(request)
	if err != nil {
		return "", fmt.Errorf("download official scrcpy archive: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("download official scrcpy archive: server returned %s", response.Status)
	}
	temporary, err := os.CreateTemp(cache, "scrcpy-download-*.zip")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	total := response.ContentLength
	if total <= 0 {
		total = release.Size
	}
	reader := &progressReader{Reader: response.Body, total: total, report: report}
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
		if verifySHA256(archive, release.SHA256) == nil {
			return archive, nil
		}
		return "", err
	}
	return archive, nil
}

type progressReader struct {
	io.Reader
	downloaded int64
	total      int64
	report     func(Progress)
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
		r.report(Progress{Downloaded: r.downloaded, Total: r.total})
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
		return fmt.Errorf("scrcpy archive checksum does not match the official release metadata")
	}
	return nil
}

func zipPrefix(path string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	for _, file := range reader.File {
		name := strings.TrimLeft(strings.ReplaceAll(file.Name, "\\", "/"), "/")
		if prefix, _, found := strings.Cut(name, "/"); found && prefix != "" {
			return prefix, nil
		}
	}
	return "", fmt.Errorf("scrcpy archive has no root directory")
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
