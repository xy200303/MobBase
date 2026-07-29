package app

import (
	"testing"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
)

func TestAndroidRequirementPackagesAndSelection(t *testing.T) {
	requirements := project.AndroidRequirements{CompileSDK: 35, BuildTools: "35.0.0", NDKVersion: "27.2.12479018"}
	packages := requiredAndroidPackages(requirements, true)
	if len(packages) != 4 || packages[0] != "platforms;android-35" || packages[3] != "platform-tools" {
		t.Fatalf("unexpected packages: %#v", packages)
	}
	sdk := android.SDK{Components: android.Components{Platforms: []string{"android-35"}, BuildTools: []string{"35.0.0"}, NDK: []string{"27.2.12479018"}, PlatformTools: true}}
	if !sdkHasAndroidRequirements(sdk, requirements, true) {
		t.Fatal("expected complete SDK to match")
	}
	sdk.Components.PlatformTools = false
	if sdkHasAndroidRequirements(sdk, requirements, true) {
		t.Fatal("expected SDK without ADB to fail run requirements")
	}
}

func TestDefaultSystemImagePackage(t *testing.T) {
	catalog := android.Catalog{Items: []android.CatalogItem{
		{PackageID: "system-images;android-35;default;x86_64"},
		{PackageID: "system-images;android-35;google_apis;x86_64"},
	}}
	image, found := defaultSystemImagePackage(catalog, 35)
	if !found || image != "system-images;android-35;google_apis;x86_64" {
		t.Fatalf("image = %q, found = %t", image, found)
	}
}

func TestUniquePackages(t *testing.T) {
	packages := uniquePackages([]string{"platform-tools", "emulator", "platform-tools", ""})
	if len(packages) != 2 || packages[0] != "platform-tools" || packages[1] != "emulator" {
		t.Fatalf("unexpected packages: %#v", packages)
	}
}
