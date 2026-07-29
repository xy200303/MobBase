package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// AndroidRequirements are the SDK components declared by an existing Gradle
// project. Mob reads them without rewriting project build files.
type AndroidRequirements struct {
	CompileSDK  int    `json:"compileSdk,omitempty"`
	BuildTools  string `json:"buildTools,omitempty"`
	NDKVersion  string `json:"ndkVersion,omitempty"`
	JavaVersion int    `json:"javaVersion,omitempty"`
}

var (
	compileSDKPattern    = regexp.MustCompile(`(?m)\bcompileSdk(?:Version)?\s*(?:=\s*)?(\d+)\b`)
	buildToolsPattern    = regexp.MustCompile(`(?m)\bbuildToolsVersion\s*(?:=\s*)?["']([^"']+)["']`)
	ndkVersionPattern    = regexp.MustCompile(`(?m)\bndkVersion\s*(?:=\s*)?["']([^"']+)["']`)
	javaVersionPattern   = regexp.MustCompile(`(?m)\b(?:sourceCompatibility|targetCompatibility)\s*(?:=\s*)?(?:JavaVersion\.VERSION_(?:1_)?|JavaVersion\.VERSION_?1_)?(\d+)\b`)
	jvmToolchainPattern  = regexp.MustCompile(`(?m)\b(?:jvmToolchain|JavaLanguageVersion\.of)\s*\(?\s*(\d+)\b`)
	applicationIDPattern = regexp.MustCompile(`(?m)\bapplicationId\s*(?:=\s*)?["']([^"']+)["']`)
)

// AndroidRequirementsFor scans Gradle source files for explicit Android
// component requirements. Dynamic Gradle expressions deliberately remain
// unresolved so Mob never guesses a version to install.
func AndroidRequirementsFor(root string) (AndroidRequirements, error) {
	requirements := AndroidRequirements{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".gradle" || entry.Name() == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if name != "build.gradle" && name != "build.gradle.kts" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		content := string(data)
		if requirements.CompileSDK == 0 {
			if match := compileSDKPattern.FindStringSubmatch(content); len(match) == 2 {
				requirements.CompileSDK, _ = strconv.Atoi(match[1])
			}
		}
		if requirements.BuildTools == "" {
			if match := buildToolsPattern.FindStringSubmatch(content); len(match) == 2 {
				requirements.BuildTools = match[1]
			}
		}
		if requirements.NDKVersion == "" {
			if match := ndkVersionPattern.FindStringSubmatch(content); len(match) == 2 {
				requirements.NDKVersion = match[1]
			}
		}
		if requirements.JavaVersion == 0 {
			if match := javaVersionPattern.FindStringSubmatch(content); len(match) == 2 {
				requirements.JavaVersion, _ = strconv.Atoi(match[1])
			} else if match := jvmToolchainPattern.FindStringSubmatch(content); len(match) == 2 {
				requirements.JavaVersion, _ = strconv.Atoi(match[1])
			}
		}
		return nil
	})
	if err != nil {
		return AndroidRequirements{}, fmt.Errorf("inspect Android Gradle requirements: %w", err)
	}
	return requirements, nil
}

// AndroidApplicationID returns the explicit applicationId declared by an
// Android app module. It intentionally does not infer namespace-derived IDs.
func AndroidApplicationID(root string) (string, error) {
	var applicationID string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".gradle" || entry.Name() == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if applicationID != "" || (entry.Name() != "build.gradle" && entry.Name() != "build.gradle.kts") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if match := applicationIDPattern.FindStringSubmatch(string(data)); len(match) == 2 {
			applicationID = match[1]
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("inspect Android application ID: %w", err)
	}
	if applicationID == "" {
		return "", fmt.Errorf("no explicit Android applicationId was found")
	}
	return applicationID, nil
}
