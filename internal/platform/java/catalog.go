package java

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
	"strings"
	"time"
)

const officialAPI = "https://api.adoptium.net/v3/assets/latest"

type Release struct {
	Major   int    `json:"major"`
	Version string `json:"version"`
	Archive string `json:"archive"`
	SHA256  string `json:"sha256"`
	Source  string `json:"source"`
	Host    string `json:"host"`
}

type Catalog struct {
	Source      string    `json:"source"`
	RefreshedAt time.Time `json:"refreshedAt"`
	Cached      bool      `json:"cached"`
	Releases    []Release `json:"releases"`
}

type asset struct {
	Version struct {
		Semver string `json:"semver"`
	} `json:"version"`
	Binary struct {
		Package struct {
			Link     string `json:"link"`
			Checksum string `json:"checksum"`
		} `json:"package"`
	} `json:"binary"`
}

func CachePath(home string) string {
	return filepath.Join(home, "cache", "catalogs", "temurin-jdk.json")
}

func LoadCatalog(ctx context.Context, cachePath string, refresh bool) (Catalog, error) {
	if !refresh {
		if data, err := os.ReadFile(cachePath); err == nil {
			var cached Catalog
			if json.Unmarshal(data, &cached) == nil && len(cached.Releases) > 0 {
				cached.Cached = true
				if info, statErr := os.Stat(cachePath); statErr == nil {
					cached.RefreshedAt = info.ModTime().UTC()
				}
				return cached, nil
			}
		}
	}
	host, err := adoptiumHost()
	if err != nil {
		return Catalog{}, err
	}
	releases := make([]Release, 0, 4)
	for _, major := range []int{8, 11, 17, 21} {
		release, err := fetchLatest(ctx, major, host)
		if err != nil {
			return Catalog{}, err
		}
		releases = append(releases, release)
	}
	sort.Slice(releases, func(i, j int) bool { return releases[i].Major > releases[j].Major })
	catalog := Catalog{Source: officialAPI, RefreshedAt: time.Now().UTC(), Releases: releases}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return Catalog{}, err
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		return Catalog{}, err
	}
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func fetchLatest(ctx context.Context, major int, host string) (Release, error) {
	url := fmt.Sprintf("%s/%d/hotspot?architecture=%s&heap_size=normal&image_type=jdk&jvm_impl=hotspot&os=%s&vendor=eclipse", officialAPI, major, adoptiumArchitecture(), host)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	response, err := (&http.Client{Timeout: 45 * time.Second}).Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("fetch Temurin JDK %d: %w", major, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Release{}, fmt.Errorf("fetch Temurin JDK %d: server returned %s", major, response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return Release{}, err
	}
	return ParseLatest(data, major, host)
}

func ParseLatest(data []byte, major int, host string) (Release, error) {
	var assets []asset
	if err := json.Unmarshal(data, &assets); err != nil {
		return Release{}, fmt.Errorf("parse Temurin response: %w", err)
	}
	if len(assets) == 0 || assets[0].Version.Semver == "" || assets[0].Binary.Package.Link == "" || len(assets[0].Binary.Package.Checksum) != 64 {
		return Release{}, fmt.Errorf("Temurin response does not contain a verified JDK package")
	}
	return Release{Major: major, Version: assets[0].Version.Semver, Archive: assets[0].Binary.Package.Link, SHA256: strings.ToLower(assets[0].Binary.Package.Checksum), Source: officialAPI, Host: host}, nil
}

func adoptiumHost() (string, error) {
	switch runtime.GOOS {
	case "windows", "linux":
		return runtime.GOOS, nil
	case "darwin":
		return "mac", nil
	default:
		return "", fmt.Errorf("Temurin JDK releases are not supported on %s", runtime.GOOS)
	}
}

func adoptiumArchitecture() string {
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return "x64"
}
