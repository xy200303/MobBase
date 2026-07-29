package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	androidVersionNamePattern = regexp.MustCompile(`(?m)\bversionName\s*(?:=\s*)?["']([^"']+)["']`)
	flutterVersionPattern     = regexp.MustCompile(`(?m)^version:\s*([^\s#]+)`)
)

// ReleaseVersion returns a statically declared project version. Dynamic
// Gradle expressions intentionally produce an empty value instead of a guess.
func ReleaseVersion(info *Info) (string, error) {
	if info.Kind == KindFlutter {
		data, err := os.ReadFile(filepath.Join(info.Root, "pubspec.yaml"))
		if err != nil {
			return "", fmt.Errorf("read pubspec.yaml: %w", err)
		}
		match := flutterVersionPattern.FindStringSubmatch(string(data))
		if len(match) == 2 {
			return match[1], nil
		}
		return "", nil
	}
	if info.Kind != KindAndroid {
		return "", nil
	}
	version := ""
	err := filepath.WalkDir(info.Root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "build" || entry.Name() == ".gradle" {
				return filepath.SkipDir
			}
			return nil
		}
		if version != "" || (entry.Name() != "build.gradle" && entry.Name() != "build.gradle.kts") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if match := androidVersionNamePattern.FindStringSubmatch(string(data)); len(match) == 2 {
			version = strings.TrimSpace(match[1])
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("inspect Android version name: %w", err)
	}
	return version, nil
}
