// Package fvm provides the official pub.dev catalog used to install Mob's
// isolated FVM launcher. It intentionally does not inspect project .fvmrc.
package fvm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const OfficialCatalogURL = "https://pub.dev/api/packages/fvm"

type Release struct {
	Version    string `json:"version"`
	ArchiveURL string `json:"archiveUrl"`
	SHA256     string `json:"sha256"`
	DartSDK    string `json:"dartSdk"`
	Current    bool   `json:"current"`
}

type Catalog struct {
	Source      string    `json:"source"`
	RefreshedAt time.Time `json:"refreshedAt"`
	Cached      bool      `json:"cached"`
	Releases    []Release `json:"releases"`
}

type packageResponse struct {
	Latest   packageVersion   `json:"latest"`
	Versions []packageVersion `json:"versions"`
}

type packageVersion struct {
	Version       string `json:"version"`
	ArchiveURL    string `json:"archive_url"`
	ArchiveSHA256 string `json:"archive_sha256"`
	Pubspec       struct {
		Environment struct {
			SDK string `json:"sdk"`
		} `json:"environment"`
	} `json:"pubspec"`
}

func LoadCatalog(ctx context.Context, cachePath string, refresh bool) (Catalog, error) {
	if !refresh {
		if data, err := os.ReadFile(cachePath); err == nil {
			catalog, parseErr := ParseCatalog(data)
			if parseErr == nil {
				if info, statErr := os.Stat(cachePath); statErr == nil {
					catalog.RefreshedAt = info.ModTime().UTC()
				}
				catalog.Cached = true
				return catalog, nil
			}
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, OfficialCatalogURL, nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("create FVM catalog request: %w", err)
	}
	response, err := (&http.Client{Timeout: 45 * time.Second}).Do(request)
	if err != nil {
		return Catalog{}, fmt.Errorf("fetch FVM catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Catalog{}, fmt.Errorf("fetch FVM catalog: server returned %s", response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return Catalog{}, fmt.Errorf("read FVM catalog: %w", err)
	}
	catalog, err := ParseCatalog(data)
	if err != nil {
		return Catalog{}, err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return Catalog{}, fmt.Errorf("create FVM catalog cache: %w", err)
	}
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		return Catalog{}, fmt.Errorf("cache FVM catalog: %w", err)
	}
	catalog.RefreshedAt = time.Now().UTC()
	return catalog, nil
}

func ParseCatalog(data []byte) (Catalog, error) {
	var response packageResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return Catalog{}, fmt.Errorf("parse FVM catalog: %w", err)
	}
	current := response.Latest.Version
	releases := make([]Release, 0, len(response.Versions))
	for _, item := range response.Versions {
		if item.Version == "" || item.ArchiveURL == "" || len(item.ArchiveSHA256) != 64 {
			continue
		}
		releases = append(releases, Release{Version: item.Version, ArchiveURL: item.ArchiveURL, SHA256: strings.ToLower(item.ArchiveSHA256), DartSDK: item.Pubspec.Environment.SDK, Current: item.Version == current})
	}
	if len(releases) == 0 {
		return Catalog{}, fmt.Errorf("FVM catalog has no installable releases")
	}
	sort.SliceStable(releases, func(i, j int) bool {
		if releases[i].Current != releases[j].Current {
			return releases[i].Current
		}
		return releases[i].Version > releases[j].Version
	})
	return Catalog{Source: OfficialCatalogURL, Releases: releases}, nil
}

func CachePath(home string) string {
	return filepath.Join(home, "cache", "catalogs", "fvm.json")
}
