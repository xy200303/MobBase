package app

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/state"
)

func TestParseReleaseAndFindArtifact(t *testing.T) {
	options, err := parseRelease([]string{"--artifact", "apk", "--output", "dist"})
	if err != nil || options.Artifact != "apk" || options.Output != "dist" {
		t.Fatalf("options: %#v %v", options, err)
	}
	root := t.TempDir()
	artifact := filepath.Join(root, "app", "build", "outputs", "apk", "release", "app-release.apk")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("release"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := findReleaseArtifact(root, "apk")
	if err != nil || found != artifact {
		t.Fatalf("artifact: %q %v", found, err)
	}
	digest, err := sha256File(artifact)
	if err != nil || len(digest) != 64 {
		t.Fatalf("digest: %q %v", digest, err)
	}
}

func TestHasReleaseSigningConfig(t *testing.T) {
	root := t.TempDir()
	buildFile := filepath.Join(root, "app", "build.gradle.kts")
	if err := os.MkdirAll(filepath.Dir(buildFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(buildFile, []byte("buildTypes { release { signingConfig = signingConfigs.getByName(\"release\") } }"), 0o644); err != nil {
		t.Fatal(err)
	}
	configured, err := hasReleaseSigningConfig(root)
	if err != nil || !configured {
		t.Fatalf("configured = %t, err = %v", configured, err)
	}
}

func TestReleaseCheckDataBlocksUnsignedNativeProject(t *testing.T) {
	mobHome := t.TempDir()
	projectRoot := t.TempDir()
	sdkRoot := filepath.Join(t.TempDir(), "sdk")
	for _, path := range []string{
		filepath.Join(sdkRoot, "platforms", "android-35"),
		filepath.Join(projectRoot, "app"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wrapper := "gradlew"
	if goruntime.GOOS == "windows" {
		wrapper = "gradlew.bat"
	}
	if err := os.WriteFile(filepath.Join(projectRoot, wrapper), []byte("wrapper"), 0o755); err != nil {
		t.Fatal(err)
	}
	buildFile := filepath.Join(projectRoot, "app", "build.gradle")
	unsigned := "android { compileSdk 35\n defaultConfig { applicationId \"dev.mob.sample\" } }"
	if err := os.WriteFile(buildFile, []byte(unsigned), 0o644); err != nil {
		t.Fatal(err)
	}
	store := state.New(mobHome)
	config := state.Default()
	config.Android.SDKs = []state.AndroidSDK{{Name: "managed", Path: sdkRoot, Ownership: state.OwnershipManaged}}
	if err := store.Save(config); err != nil {
		t.Fatal(err)
	}
	r := runtime{home: mobHome, store: store}
	info := &project.Info{Root: projectRoot, Kind: project.KindAndroid, Targets: []string{"android"}}
	data, ready, err := r.releaseCheckData(info)
	if err != nil || ready {
		t.Fatalf("unsigned preflight ready=%t data=%#v err=%v", ready, data, err)
	}
	checks := data["checks"].([]check)
	if checks[len(checks)-1].ID != "release-signing" || checks[len(checks)-1].Status != "missing" {
		t.Fatalf("missing release-signing check: %#v", checks)
	}

	signed := unsigned + "\n buildTypes { release { signingConfig signingConfigs.release } }"
	if err := os.WriteFile(buildFile, []byte(signed), 0o644); err != nil {
		t.Fatal(err)
	}
	_, ready, err = r.releaseCheckData(info)
	if err != nil || !ready {
		t.Fatalf("signed preflight ready=%t err=%v", ready, err)
	}
}
