// Package project identifies existing mobile project layouts without changing
// their files. Mob uses this information to select official runners later.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	lowerContent := strings.ToLower(content)
	if !strings.Contains(lowerContent, "multiplatform") && !strings.Contains(lowerContent, "kuikly") {
		return false, nil, nil
	}
	targets := make([]string, 0, 2)
	if declaresAndroidTarget(content) {
		targets = append(targets, "android")
	}
	if strings.Contains(content, "ios") || strings.Contains(content, "iOS") {
		targets = append(targets, "ios")
	}
	return true, targets, nil
}

func declaresAndroidTarget(content string) bool {
	return strings.Contains(content, "androidTarget") ||
		strings.Contains(content, "android()") ||
		strings.Contains(content, "com.android.application") ||
		strings.Contains(content, "com.android.library") ||
		strings.Contains(content, "libs.plugins.android.application") ||
		strings.Contains(content, "libs.plugins.android.library")
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
	return len(settingsScriptNames(root)) > 0
}

// settingsScriptNames accepts Gradle's conventional settings files as well as
// versioned settings variants used by multi-module builds such as Kuikly.
func settingsScriptNames(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	names := make([]string, 0, 2)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "settings.") {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".gradle") || strings.HasSuffix(entry.Name(), ".gradle.kts") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
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
	for _, name := range append([]string{"build.gradle", "build.gradle.kts"}, settingsScriptNames(root)...) {
		if err := appendBuildScript(&content, filepath.Join(root, name)); err != nil {
			return "", err
		}
	}
	modules, err := includedGradleModules(root)
	if err != nil {
		return "", err
	}
	for _, module := range modules {
		for _, name := range []string{"build.gradle", "build.gradle.kts"} {
			if err := appendBuildScript(&content, filepath.Join(root, module, name)); err != nil {
				return "", err
			}
		}
	}
	return content.String(), nil
}

func appendBuildScript(content *strings.Builder, path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	content.Write(data)
	return nil
}

var (
	includeCallPattern  = regexp.MustCompile(`(?s)\binclude\s*\((.*?)\)`)
	includeLinePattern  = regexp.MustCompile(`(?m)^\s*include\s+(.+)$`)
	gradleModulePattern = regexp.MustCompile(`["'](:[A-Za-z0-9_.-]+)["']`)
)

// includedGradleModules returns conventional module directories declared by
// Gradle's include syntax. Custom projectDir mappings remain Gradle-owned and
// are intentionally not guessed by source inspection.
func includedGradleModules(root string) ([]string, error) {
	var settings strings.Builder
	for _, name := range settingsScriptNames(root) {
		if err := appendBuildScript(&settings, filepath.Join(root, name)); err != nil {
			return nil, err
		}
	}
	seen := make(map[string]struct{})
	modules := make([]string, 0)
	addMatches := func(value string) {
		for _, match := range gradleModulePattern.FindAllStringSubmatch(value, -1) {
			module := filepath.Join(strings.Split(strings.TrimPrefix(match[1], ":"), ":")...)
			if _, exists := seen[module]; exists {
				continue
			}
			seen[module] = struct{}{}
			modules = append(modules, module)
		}
	}
	for _, match := range includeCallPattern.FindAllStringSubmatch(settings.String(), -1) {
		addMatches(match[1])
	}
	for _, match := range includeLinePattern.FindAllStringSubmatch(settings.String(), -1) {
		addMatches(match[1])
	}
	return modules, nil
}
