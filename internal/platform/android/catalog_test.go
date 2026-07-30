package android

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseCatalogSelectsHostArchiveAndSDKItems(t *testing.T) {
	data := []byte(`<?xml version="1.0"?>
<sdk-repository>
  <remotePackage path="platform-tools">
    <revision><major>35</major><minor>0</minor><micro>2</micro></revision>
    <display-name>Platform Tools</display-name><uses-license ref="android-sdk-license"/>
    <archives><archive><complete><size>12</size><checksum>0123456789012345678901234567890123456789</checksum><url>linux.zip</url></complete><host-os>linux</host-os></archive><archive><complete><size>13</size><checksum>abcdefabcdefabcdefabcdefabcdefabcdefabcd</checksum><url>windows.zip</url></complete><host-os>windows</host-os></archive></archives>
  </remotePackage>
  <remotePackage path="ndk;27.2.12479018"><revision><major>27</major><minor>2</minor><micro>12479018</micro></revision><archives><archive><complete><size>1</size><checksum>0123456789012345678901234567890123456789</checksum><url>ndk.zip</url></complete></archive></archives></remotePackage>
</sdk-repository>`)
	catalog, err := ParseCatalog(data, "test")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(catalog.Items) != 2 {
		t.Fatalf("items = %#v", catalog.Items)
	}
	items := catalog.SDKItems(0)
	if len(items) != 1 || items[0].PackageID != "platform-tools" || items[0].ChecksumAlgorithm != "sha1" {
		t.Fatalf("SDK items = %#v", items)
	}
	ndks := catalog.NDKItems()
	if len(ndks) != 1 || !ContainsPackage(ndks, "ndk;27.2.12479018") {
		t.Fatalf("NDK items = %#v", ndks)
	}
}

func TestSystemImageItemsFiltersAPI(t *testing.T) {
	catalog := Catalog{Items: []CatalogItem{{PackageID: "system-images;android-35;google_apis;x86_64"}, {PackageID: "system-images;android-34;google_apis;x86_64"}, {PackageID: "platform-tools"}}}
	items := catalog.SystemImageItems(35)
	if len(items) != 1 || items[0].PackageID != "system-images;android-35;google_apis;x86_64" {
		t.Fatalf("items: %#v", items)
	}
}

func TestMergeCatalogsIncludesSystemImageRepository(t *testing.T) {
	refreshed := time.Date(2026, 7, 30, 4, 0, 0, 0, time.UTC)
	merged := mergeCatalogs(
		Catalog{Source: OfficialRepositoryURL, Cached: true, Items: []CatalogItem{{PackageID: "platform-tools"}}},
		Catalog{Source: OfficialSystemImageURL, RefreshedAt: refreshed, Cached: false, Items: []CatalogItem{{PackageID: "system-images;android-35;google_apis;x86_64"}}},
	)
	if got := merged.SystemImageItems(0); len(got) != 1 || got[0].PackageID != "system-images;android-35;google_apis;x86_64" {
		t.Fatalf("system image items = %#v", got)
	}
	if merged.Cached || !merged.RefreshedAt.Equal(refreshed) {
		t.Fatalf("metadata = %#v", merged)
	}
	if merged.Source != OfficialRepositoryURL+"; "+OfficialSystemImageURL {
		t.Fatalf("source = %q", merged.Source)
	}
}

func TestSystemImageCatalogCachePath(t *testing.T) {
	path := systemImageCatalogCachePath(filepath.Join("cache", "catalogs", "android-repository.xml"))
	if path != filepath.Join("cache", "catalogs", "android-system-images.xml") {
		t.Fatalf("cache path = %q", path)
	}
}
