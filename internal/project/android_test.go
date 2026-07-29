package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAndroidRequirementsFor(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatalf("create app: %v", err)
	}
	content := `android {
    compileSdk = 35
    buildToolsVersion "35.0.0"
    ndkVersion = "27.2.12479018"
	compileOptions { sourceCompatibility = JavaVersion.VERSION_17 }
}`
	if err := os.WriteFile(filepath.Join(app, "build.gradle.kts"), []byte(content), 0o644); err != nil {
		t.Fatalf("write Gradle file: %v", err)
	}
	requirements, err := AndroidRequirementsFor(root)
	if err != nil {
		t.Fatalf("requirements: %v", err)
	}
	if requirements.CompileSDK != 35 || requirements.BuildTools != "35.0.0" || requirements.NDKVersion != "27.2.12479018" || requirements.JavaVersion != 17 {
		t.Fatalf("unexpected requirements: %#v", requirements)
	}
}
