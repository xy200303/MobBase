package flutter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

type Release struct {
	Version string `json:"version"`
	Archive string `json:"archive"`
	SHA256  string `json:"sha256"`
	Current bool   `json:"current"`
}
type Catalog struct {
	Source      string    `json:"source"`
	RefreshedAt time.Time `json:"refreshedAt"`
	Cached      bool      `json:"cached"`
	Releases    []Release `json:"releases"`
}
type feed struct {
	Current  map[string]string `json:"current_release"`
	Releases []struct {
		Hash    string `json:"hash"`
		Channel string `json:"channel"`
		Version string `json:"version"`
		Archive string `json:"archive"`
		SHA256  string `json:"sha256"`
	} `json:"releases"`
}

func OfficialReleaseURL() (string, error) {
	host := runtime.GOOS
	if host == "darwin" {
		host = "macos"
	}
	if host != "windows" && host != "linux" && host != "macos" {
		return "", fmt.Errorf("Flutter releases are not supported on %s", runtime.GOOS)
	}
	return "https://storage.googleapis.com/flutter_infra_release/releases/releases_" + host + ".json", nil
}
func CachePath(home string) string {
	return filepath.Join(home, "cache", "catalogs", "flutter-releases-"+runtime.GOOS+".json")
}
func LoadCatalog(ctx context.Context, cachePath string, refresh bool) (Catalog, error) {
	source, err := OfficialReleaseURL()
	if err != nil {
		return Catalog{}, err
	}
	if !refresh {
		if data, err := os.ReadFile(cachePath); err == nil {
			catalog, parseErr := ParseCatalog(data, source)
			if parseErr == nil {
				if info, statErr := os.Stat(cachePath); statErr == nil {
					catalog.RefreshedAt = info.ModTime().UTC()
				}
				catalog.Cached = true
				return catalog, nil
			}
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return Catalog{}, err
	}
	response, err := (&http.Client{Timeout: 45 * time.Second}).Do(request)
	if err != nil {
		return Catalog{}, fmt.Errorf("fetch Flutter releases: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Catalog{}, fmt.Errorf("fetch Flutter releases: server returned %s", response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return Catalog{}, err
	}
	catalog, err := ParseCatalog(data, source)
	if err != nil {
		return Catalog{}, err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return Catalog{}, err
	}
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		return Catalog{}, err
	}
	catalog.RefreshedAt = time.Now().UTC()
	return catalog, nil
}
func ParseCatalog(data []byte, source string) (Catalog, error) {
	var parsed feed
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Catalog{}, fmt.Errorf("parse Flutter releases: %w", err)
	}
	current := parsed.Current["stable"]
	releases := make([]Release, 0)
	for _, release := range parsed.Releases {
		if release.Channel == "stable" && release.Version != "" && release.Archive != "" && release.SHA256 != "" {
			releases = append(releases, Release{Version: release.Version, Archive: release.Archive, SHA256: release.SHA256, Current: release.Hash == current})
		}
	}
	sort.Slice(releases, func(i, j int) bool { return releases[i].Version > releases[j].Version })
	return Catalog{Source: source, Releases: releases}, nil
}
