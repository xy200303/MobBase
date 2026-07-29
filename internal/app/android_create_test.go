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
