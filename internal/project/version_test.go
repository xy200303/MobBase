package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseVersion(t *testing.T) {
	android := t.TempDir()
	if err := os.WriteFile(filepath.Join(android, "build.gradle.kts"), []byte(`android { defaultConfig { versionName = "1.2.3" } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	version, err := ReleaseVersion(&Info{Root: android, Kind: KindAndroid})
	if err != nil || version != "1.2.3" {
		t.Fatalf("Android version: %q %v", version, err)
	}
	flutter := t.TempDir()
	if err := os.WriteFile(filepath.Join(flutter, "pubspec.yaml"), []byte("name: sample\nversion: 2.0.0+3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	version, err = ReleaseVersion(&Info{Root: flutter, Kind: KindFlutter})
	if err != nil || version != "2.0.0+3" {
		t.Fatalf("Flutter version: %q %v", version, err)
	}
}
