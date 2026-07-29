package fvm

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DownloadAndExtract verifies the exact pub.dev package archive before
// extracting regular files into destination. Symlinks and path escapes are
// rejected because a package archive is untrusted until this point.
func DownloadAndExtract(ctx context.Context, release Release, destination string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, release.ArchiveURL, nil)
	if err != nil {
		return fmt.Errorf("create FVM archive request: %w", err)
	}
	response, err := (&http.Client{Timeout: 2 * time.Minute}).Do(request)
	if err != nil {
		return fmt.Errorf("download FVM archive: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("download FVM archive: server returned %s", response.Status)
	}
	download, err := os.CreateTemp("", "mob-fvm-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create FVM archive file: %w", err)
	}
	downloadPath := download.Name()
	defer os.Remove(downloadPath)
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(download, hash), response.Body); err != nil {
		download.Close()
		return fmt.Errorf("save FVM archive: %w", err)
	}
	if err := download.Close(); err != nil {
		return err
	}
	if actual := fmt.Sprintf("%x", hash.Sum(nil)); !strings.EqualFold(actual, release.SHA256) {
		return fmt.Errorf("FVM archive checksum mismatch: expected %s, got %s", release.SHA256, actual)
	}
	archive, err := os.Open(downloadPath)
	if err != nil {
		return err
	}
	defer archive.Close()
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open FVM archive: %w", err)
	}
	defer gzipReader.Close()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read FVM archive: %w", err)
		}
		path, err := safeArchivePath(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("FVM archive contains unsupported entry %q", header.Name)
		}
	}
	if _, err := os.Stat(filepath.Join(destination, "pubspec.yaml")); err != nil {
		return fmt.Errorf("FVM archive does not contain pubspec.yaml")
	}
	return nil
}

func safeArchivePath(root, name string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(name))
	if cleaned == "." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("unsafe FVM archive path %q", name)
	}
	path := filepath.Join(root, cleaned)
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe FVM archive path %q", name)
	}
	return path, nil
}
