package android

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xy200303/MobBase/internal/state"
)

func TestDiscoverInspectsRegisteredSDK(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"platforms/android-35", "build-tools/35.0.0", "ndk/27.2.12479018", "system-images/android-35/google_apis/x86_64", "platform-tools"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatalf("create SDK component: %v", err)
		}
	}
	config := state.Default()
	config.Android.SDKs = []state.AndroidSDK{{Name: "shared", Path: root, Ownership: state.OwnershipImported}}
	config.Android.CurrentSDK = "shared"
	sdks, err := Discover(config)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, sdk := range sdks {
		if sdk.Name != "shared" {
			continue
		}
		if !sdk.Current || sdk.Components.Platforms[0] != "android-35" || !sdk.Components.PlatformTools || sdk.Components.SystemImages[0] != "system-images;android-35;google_apis;x86_64" {
			t.Fatalf("unexpected SDK inspection: %#v", sdk)
		}
		return
	}
	t.Fatalf("registered SDK was not discovered: %#v", sdks)
}

func TestValidateSDKRootRejectsUnrelatedDirectory(t *testing.T) {
	if _, err := ValidateSDKRoot(t.TempDir()); err == nil {
		t.Fatal("expected an empty directory to be rejected")
	}
}
