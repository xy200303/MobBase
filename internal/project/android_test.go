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
	compileOptions { sourceCompatibility = JavaVersion.VERSION_1_8 }
	plugins { id("com.android.application") version "8.7.3" apply false }
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

func TestAndroidRequirementsIgnoreBytecodeCompatibilityWithoutRuntimeRequirement(t *testing.T) {
	root := t.TempDir()
	content := `android {
    compileSdk = 35
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_1_8
        targetCompatibility = JavaVersion.VERSION_1_8
    }
}`
	if err := os.WriteFile(filepath.Join(root, "build.gradle.kts"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	requirements, err := AndroidRequirementsFor(root)
	if err != nil {
		t.Fatal(err)
	}
	if requirements.JavaVersion != 0 {
		t.Fatalf("bytecode compatibility must not select Gradle runtime JDK: %#v", requirements)
	}
}

func TestAndroidRequirementsUseExplicitToolchainAndAGPMinimum(t *testing.T) {
	root := t.TempDir()
	content := `plugins { id("com.android.application") version "7.4.2" apply false }
kotlin { jvmToolchain(21) }`
	if err := os.WriteFile(filepath.Join(root, "build.gradle.kts"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	requirements, err := AndroidRequirementsFor(root)
	if err != nil {
		t.Fatal(err)
	}
	if requirements.JavaVersion != 21 {
		t.Fatalf("Java version = %d, want 21", requirements.JavaVersion)
	}
}
