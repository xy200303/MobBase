package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/state"
)

func TestCatalogRefreshOptions(t *testing.T) {
	refresh, err := parseRefresh([]string{"--refresh"}, "mob catalog")
	if err != nil || !refresh {
		t.Fatalf("refresh: %t %v", refresh, err)
	}
	if _, err := parseRefresh([]string{"--api", "35"}, "mob catalog"); err == nil {
		t.Fatal("expected invalid catalog option")
	}
}

func TestWriteSystemImageTableUsesCompactInstallFields(t *testing.T) {
	var output bytes.Buffer
	writeSystemImageTable(&output, []android.CatalogItem{{PackageID: "system-images;android-35;google_apis_playstore_ps16k;x86_64", Version: "5"}}, "sdkmanager", "managed")
	text := output.String()
	for _, expected := range []string{"source: sdkmanager; SDK: managed", "API", "google_apis_playstore_ps16k", "x86_64", "system-images;android-<API>;<IMAGE>;<ABI>"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("table does not contain %q: %q", expected, text)
		}
	}
	if strings.Contains(text, "Google Play Intel") {
		t.Fatalf("table should not repeat the long display name: %q", text)
	}
}

func TestFilterSystemImagesAndSDKPreference(t *testing.T) {
	items := filterSystemImages([]android.CatalogItem{
		{PackageID: "system-images;android-34;google_apis;x86_64"},
		{PackageID: "system-images;android-35;google_apis;x86_64"},
	}, 35)
	if len(items) != 1 || items[0].PackageID != "system-images;android-35;google_apis;x86_64" {
		t.Fatalf("filtered images = %#v", items)
	}
	ordered := preferredSDKs([]android.SDK{
		{Name: "external", Ownership: state.OwnershipImported},
		{Name: "managed", Ownership: state.OwnershipManaged},
		{Name: "selected", Ownership: state.OwnershipImported, Current: true},
	})
	if len(ordered) != 3 || ordered[0].Name != "selected" || ordered[1].Name != "managed" || ordered[2].Name != "external" {
		t.Fatalf("SDK preference = %#v", ordered)
	}
}
