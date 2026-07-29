package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFlutterFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "pubspec.yaml"), "dependencies:\n  flutter:\n    sdk: flutter\n")
	makeDirectory(t, filepath.Join(root, "android"))
	makeDirectory(t, filepath.Join(root, "ios"))
	nested := filepath.Join(root, "lib", "features")
	makeDirectory(t, nested)

	info, err := Detect(nested)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info == nil || info.Root != root || info.Kind != KindFlutter || len(info.Targets) != 2 {
		t.Fatalf("unexpected project: %#v", info)
	}
}

func TestDetectReactNative(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"react-native":"0.76.0"}}`)
	makeDirectory(t, filepath.Join(root, "android"))

	info, err := Detect(root)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info == nil || info.Kind != KindReactNative || info.Targets[0] != "android" {
		t.Fatalf("unexpected project: %#v", info)
	}
}

func TestDetectKotlinMultiplatformBeforeAndroid(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "settings.gradle.kts"), "rootProject.name = \"sample\"")
	writeFile(t, filepath.Join(root, "build.gradle.kts"), "plugins { kotlin(\"multiplatform\") }\nkotlin { androidTarget(); iosArm64() }")

	info, err := Detect(root)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info == nil || info.Kind != KindKotlinMultiplatform || len(info.Targets) != 2 {
		t.Fatalf("unexpected project: %#v", info)
	}
}

func TestDetectNativeAndroid(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "settings.gradle"), "rootProject.name = 'sample'")
	writeFile(t, filepath.Join(root, "build.gradle"), "plugins {}")

	info, err := Detect(root)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info == nil || info.Kind != KindAndroid || len(info.Targets) != 1 {
		t.Fatalf("unexpected project: %#v", info)
	}
}

func TestDetectNativeIOS(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Notes.xcodeproj", "project.pbxproj"), "// !$*UTF8*$!")

	info, err := Detect(root)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info == nil || info.Kind != KindIOS || len(info.Targets) != 1 || info.Targets[0] != "ios" {
		t.Fatalf("unexpected project: %#v", info)
	}
}

func TestDoesNotTreatAnIOSNamedDirectoryAsANativeIOSProject(t *testing.T) {
	root := t.TempDir()
	makeDirectory(t, filepath.Join(root, "ios"))

	info, err := Detect(root)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info != nil {
		t.Fatalf("unexpected project: %#v", info)
	}
}

func TestIOSProjectPathRejectsAmbiguousProjectRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "One.xcodeproj", "project.pbxproj"), "one")
	writeFile(t, filepath.Join(root, "Two.xcodeproj", "project.pbxproj"), "two")

	if _, err := IOSProjectPath(root); err == nil {
		t.Fatal("IOSProjectPath accepted an ambiguous project root")
	}
	info, err := Detect(root)
	if err != nil || info == nil || info.Kind != KindIOS {
		t.Fatalf("Detect did not retain an ambiguous native iOS project: %#v, %v", info, err)
	}
}

func TestAndroidApplicationID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app", "build.gradle.kts"), "android { defaultConfig { applicationId = \"com.example.notes\" } }")
	applicationID, err := AndroidApplicationID(root)
	if err != nil || applicationID != "com.example.notes" {
		t.Fatalf("application ID = %q, err = %v", applicationID, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	makeDirectory(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func makeDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
}
