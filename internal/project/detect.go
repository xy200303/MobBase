// Package project identifies existing mobile project layouts without changing
// their files. Mob uses this information to select official runners later.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Kind string

const (
	KindAndroid             Kind = "android"
	KindIOS                 Kind = "ios"
	KindFlutter             Kind = "flutter"
	KindReactNative         Kind = "react-native"
	KindKotlinMultiplatform Kind = "kotlin-multiplatform"
)

type Info struct {
	Root    string   `json:"root"`
	Kind    Kind     `json:"kind"`
	Targets []string `json:"targets"`
}

// Detect walks from start toward the filesystem root and returns the closest
// supported project. An unrecognized directory returns (nil, nil).
func Detect(start string) (*Info, error) {
	if start == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve working directory: %w", err)
		}
		start = workingDirectory
	}
	directory, err := filepath.Abs(start)
	if err != nil {
		return nil, fmt.Errorf("resolve project directory: %w", err)
	}
	for {
		if info, found, err := inspect(directory); err != nil || found {
			return info, err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return nil, nil
		}
		directory = parent
	}
}

func inspect(root string) (*Info, bool, error) {
	if isFlutter(root) {
		return &Info{Root: root, Kind: KindFlutter, Targets: nativeTargets(root)}, true, nil
	}
	if reactNative, err := isReactNative(root); err != nil {
		return nil, false, err
	} else if reactNative {
		return &Info{Root: root, Kind: KindReactNative, Targets: nativeTargets(root)}, true, nil
	}
	if kmp, targets, err := isKotlinMultiplatform(root); err != nil {
		return nil, false, err
	} else if kmp {
		return &Info{Root: root, Kind: KindKotlinMultiplatform, Targets: targets}, true, nil
	}
	if isAndroid(root) {
		return &Info{Root: root, Kind: KindAndroid, Targets: []string{"android"}}, true, nil
	}
	if isIOS(root) {
		return &Info{Root: root, Kind: KindIOS, Targets: []string{"ios"}}, true, nil
	}
	return nil, false, nil
}

func isFlutter(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "pubspec.yaml"))
	if err != nil || !hasDirectory(root, "android") {
		return false
	}
	return strings.Contains(string(data), "flutter:")
}

func isReactNative(root string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read package.json: %w", err)
	}
	var manifest struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return false, fmt.Errorf("parse package.json: %w", err)
	}
	_, dependency := manifest.Dependencies["react-native"]
	_, devDependency := manifest.DevDependencies["react-native"]
	return (dependency || devDependency) && (hasDirectory(root, "android") || hasDirectory(root, "ios")), nil
}

func isKotlinMultiplatform(root string) (bool, []string, error) {
	if !hasSettingsGradle(root) {
		return false, nil, nil
	}
	content, err := readBuildScripts(root)
	if err != nil {
		return false, nil, err
	}
	if !strings.Contains(content, "multiplatform") && !strings.Contains(content, "Multiplatform") {
		return false, nil, nil
	}
	targets := make([]string, 0, 2)
	if strings.Contains(content, "androidTarget") || strings.Contains(content, "android()") {
		targets = append(targets, "android")
	}
	if strings.Contains(content, "ios") || strings.Contains(content, "iOS") {
		targets = append(targets, "ios")
	}
	return true, targets, nil
}

func isAndroid(root string) bool {
	return hasSettingsGradle(root) && (fileExists(root, "build.gradle") || fileExists(root, "build.gradle.kts") || hasDirectory(root, "app"))
}

// isIOS recognizes an Xcode project package rather than merely an ios-named
// directory. This keeps unrelated folders from becoming mobile projects.
func isIOS(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".xcodeproj") && fileExists(filepath.Join(root, entry.Name()), "project.pbxproj") {
			return true
		}
	}
	return false
}

// IOSProjectPath returns the only native Xcode project package directly owned
// by root. Multiple projects are intentionally not guessed; callers should
// pass an explicit xcodebuild command through Mob's command separator.
func IOSProjectPath(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("read iOS project directory: %w", err)
	}
	projects := make([]string, 0, 1)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".xcodeproj") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if fileExists(path, "project.pbxproj") {
			projects = append(projects, path)
		}
	}
	switch len(projects) {
	case 1:
		return projects[0], nil
	case 0:
		return "", fmt.Errorf("no .xcodeproj containing project.pbxproj was found")
	default:
		return "", fmt.Errorf("multiple .xcodeproj packages were found; pass an explicit xcodebuild command after --")
	}
}

func nativeTargets(root string) []string {
	targets := make([]string, 0, 2)
	if hasDirectory(root, "android") {
		targets = append(targets, "android")
	}
	if hasDirectory(root, "ios") {
		targets = append(targets, "ios")
	}
	return targets
}

func hasSettingsGradle(root string) bool {
	return fileExists(root, "settings.gradle") || fileExists(root, "settings.gradle.kts")
}
func fileExists(root, name string) bool {
	info, err := os.Stat(filepath.Join(root, name))
	return err == nil && !info.IsDir()
}
func hasDirectory(root, name string) bool {
	info, err := os.Stat(filepath.Join(root, name))
	return err == nil && info.IsDir()
}

func readBuildScripts(root string) (string, error) {
	var content strings.Builder
	for _, name := range []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("read %s: %w", name, err)
		}
		content.Write(data)
	}
	return content.String(), nil
}
