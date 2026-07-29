package android

import (
	"context"
	"encoding/xml"
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

const OfficialRepositoryURL = "https://dl.google.com/android/repository/repository2-1.xml"

type CatalogItem struct {
	PackageID         string `json:"packageId"`
	Version           string `json:"version"`
	DisplayName       string `json:"displayName"`
	License           string `json:"license"`
	Size              int64  `json:"size"`
	Checksum          string `json:"checksum"`
	ChecksumAlgorithm string `json:"checksumAlgorithm"`
	ArchiveURL        string `json:"archiveUrl"`
}

type Catalog struct {
	Source      string        `json:"source"`
	RefreshedAt time.Time     `json:"refreshedAt"`
	Cached      bool          `json:"cached"`
	Items       []CatalogItem `json:"items"`
}

type repositoryXML struct {
	Packages []remotePackageXML `xml:"remotePackage"`
}

type remotePackageXML struct {
	Path        string       `xml:"path,attr"`
	DisplayName string       `xml:"display-name"`
	Revision    revisionXML  `xml:"revision"`
	License     licenseXML   `xml:"uses-license"`
	Archives    []archiveXML `xml:"archives>archive"`
}

type revisionXML struct {
	Major   string `xml:"major"`
	Minor   string `xml:"minor"`
	Micro   string `xml:"micro"`
	Preview string `xml:"preview"`
}

type licenseXML struct {
	Ref string `xml:"ref,attr"`
}
type archiveXML struct {
	HostOS   string      `xml:"host-os"`
	Complete completeXML `xml:"complete"`
}
type completeXML struct {
	Size     int64  `xml:"size"`
	Checksum string `xml:"checksum"`
	URL      string `xml:"url"`
}

// LoadCatalog reuses a local cache unless refresh is requested. If no cache is
// available it loads the official Android repository over HTTPS.
func LoadCatalog(ctx context.Context, cachePath string, refresh bool) (Catalog, error) {
	if !refresh {
		if data, err := os.ReadFile(cachePath); err == nil {
			catalog, parseErr := ParseCatalog(data, OfficialRepositoryURL)
			if parseErr == nil {
				if info, statErr := os.Stat(cachePath); statErr == nil {
					catalog.RefreshedAt = info.ModTime().UTC()
				}
				catalog.Cached = true
				return catalog, nil
			}
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, OfficialRepositoryURL, nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("create Android repository request: %w", err)
	}
	response, err := (&http.Client{Timeout: 45 * time.Second}).Do(request)
	if err != nil {
		return Catalog{}, fmt.Errorf("fetch Android repository: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Catalog{}, fmt.Errorf("fetch Android repository: server returned %s", response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return Catalog{}, fmt.Errorf("read Android repository: %w", err)
	}
	catalog, err := ParseCatalog(data, OfficialRepositoryURL)
	if err != nil {
		return Catalog{}, err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return Catalog{}, fmt.Errorf("create Android catalog cache: %w", err)
	}
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		return Catalog{}, fmt.Errorf("cache Android repository: %w", err)
	}
	catalog.RefreshedAt = time.Now().UTC()
	catalog.Cached = false
	return catalog, nil
}

func ParseCatalog(data []byte, source string) (Catalog, error) {
	var repository repositoryXML
	if err := xml.Unmarshal(data, &repository); err != nil {
		return Catalog{}, fmt.Errorf("parse Android repository: %w", err)
	}
	items := make([]CatalogItem, 0, len(repository.Packages))
	for _, remote := range repository.Packages {
		archive, found := selectArchive(remote.Archives)
		if !found || remote.Path == "" || archive.Complete.URL == "" {
			continue
		}
		items = append(items, CatalogItem{PackageID: remote.Path, Version: formatRevision(remote.Revision), DisplayName: remote.DisplayName, License: remote.License.Ref, Size: archive.Complete.Size, Checksum: archive.Complete.Checksum, ChecksumAlgorithm: checksumAlgorithm(archive.Complete.Checksum), ArchiveURL: archive.Complete.URL})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].PackageID == items[j].PackageID {
			return items[i].Version > items[j].Version
		}
		return items[i].PackageID < items[j].PackageID
	})
	return Catalog{Source: source, Items: items}, nil
}

func (catalog Catalog) SDKItems(api int) []CatalogItem {
	items := make([]CatalogItem, 0)
	for _, item := range catalog.Items {
		if api == 0 && (strings.HasPrefix(item.PackageID, "platforms;android-") || strings.HasPrefix(item.PackageID, "build-tools;") || strings.HasPrefix(item.PackageID, "system-images;")) {
			items = append(items, item)
			continue
		}
		if api > 0 && (item.PackageID == fmt.Sprintf("platforms;android-%d", api) || strings.HasPrefix(item.PackageID, fmt.Sprintf("build-tools;%d.", api)) || strings.HasPrefix(item.PackageID, fmt.Sprintf("system-images;android-%d;", api))) {
			items = append(items, item)
			continue
		}
		if item.PackageID == "platform-tools" || item.PackageID == "emulator" || item.PackageID == "cmdline-tools;latest" {
			items = append(items, item)
		}
	}
	return items
}

func (catalog Catalog) NDKItems() []CatalogItem {
	items := make([]CatalogItem, 0)
	for _, item := range catalog.Items {
		if strings.HasPrefix(item.PackageID, "ndk;") {
			items = append(items, item)
		}
	}
	return items
}

func (catalog Catalog) SystemImageItems(api int) []CatalogItem {
	items := make([]CatalogItem, 0)
	prefix := "system-images;"
	if api > 0 {
		prefix = fmt.Sprintf("system-images;android-%d;", api)
	}
	for _, item := range catalog.Items {
		if strings.HasPrefix(item.PackageID, prefix) {
			items = append(items, item)
		}
	}
	return items
}

func ContainsPackage(items []CatalogItem, packageID string) bool {
	for _, item := range items {
		if item.PackageID == packageID {
			return true
		}
	}
	return false
}

func (catalog Catalog) FindPackage(packageID string) (CatalogItem, bool) {
	for _, item := range catalog.Items {
		if item.PackageID == packageID {
			return item, true
		}
	}
	return CatalogItem{}, false
}

func selectArchive(archives []archiveXML) (archiveXML, bool) {
	host := runtime.GOOS
	if host == "darwin" {
		host = "macosx"
	}
	for _, archive := range archives {
		if archive.HostOS == host {
			return archive, true
		}
	}
	for _, archive := range archives {
		if archive.HostOS == "" {
			return archive, true
		}
	}
	return archiveXML{}, false
}

func formatRevision(revision revisionXML) string {
	parts := []string{revision.Major, revision.Minor, revision.Micro}
	for len(parts) > 1 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	version := strings.Join(parts, ".")
	if revision.Preview != "" {
		version += "-preview." + revision.Preview
	}
	return version
}

func checksumAlgorithm(value string) string {
	if len(value) == 40 {
		return "sha1"
	}
	if len(value) == 64 {
		return "sha256"
	}
	return "unknown"
}

func CatalogCachePath(home string) string {
	return filepath.Join(home, "cache", "catalogs", "android-repository.xml")
}
