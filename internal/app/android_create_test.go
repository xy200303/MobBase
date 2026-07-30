package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAndroidCreateAndTemplate(t *testing.T) {
	o, err := parseAndroidCreate([]string{"notes", "--language", "java", "--ui", "views", "--min-sdk", "24"})
	if err != nil || o.Language != "java" || o.UI != "views" || o.MinSDK != 24 {
		t.Fatalf("options: %#v %v", o, err)
	}
	root := t.TempDir()
	if err := writeAndroidTemplate(root, o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "build.gradle.kts")); err != nil {
		t.Fatalf("missing build file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "src", "main", "java", "com", "example", "notes", "MainActivity.java")); err != nil {
		t.Fatalf("missing activity: %v", err)
	}
}

func TestAndroidCreateRejectsJavaCompose(t *testing.T) {
	if _, err := parseAndroidCreate([]string{"notes", "--language", "java", "--ui", "compose"}); err == nil {
		t.Fatal("expected Java Compose template to be rejected")
	}
}

func TestGradleWrapperHelpers(t *testing.T) {
	if checksum := parseSHA256("A67EC9A87755A07D5F3115C233A065FA5C54C48A1DCB61C151A1A2E5DAEA9C2C gradle.zip"); checksum != "a67ec9a87755a07d5f3115c233a065fa5c54c48a1dcb61c151a1a2e5daea9c2c" {
		t.Fatalf("checksum = %q", checksum)
	}
	if parseSHA256("not-a-checksum") != "" {
		t.Fatal("invalid checksum was accepted")
	}
	path := gradleExecutable(`C:\\mob\\gradle`)
	if filepath.Base(path) != "gradle.bat" && filepath.Base(path) != "gradle" {
		t.Fatalf("unexpected Gradle executable path: %s", path)
	}
}
