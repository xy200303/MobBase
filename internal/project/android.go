package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
	compileSDKPattern = regexp.MustCompile(`(?m)\bcompileSdk(?:Version)?\s*(?:=\s*)?(\d+)\b`)
	buildToolsPattern = regexp.MustCompile(`(?m)\bbuildToolsVersion\s*(?:=\s*)?["']([^"']+)["']`)
	ndkVersionPattern = regexp.MustCompile(`(?m)\bndkVersion\s*(?:=\s*)?["']([^"']+)["']`)
	// sourceCompatibility and targetCompatibility intentionally do not appear
	// here. They select bytecode compatibility, not the JDK that runs Gradle.
	jvmToolchainPattern     = regexp.MustCompile(`(?m)\b(?:jvmToolchain|JavaLanguageVersion\.of)\s*\(?\s*(\d+)\b`)
	androidPluginPattern    = regexp.MustCompile(`(?m)\bid\s*(?:\(\s*)?["']com\.android\.(?:application|library|test)["']\s*\)?\s*version\s*["'](\d+)`)
	androidClasspathPattern = regexp.MustCompile(`(?m)com\.android\.tools\.build:gradle:(\d+)`)
	applicationIDPattern    = regexp.MustCompile(`(?m)\bapplicationId\s*(?:=\s*)?["']([^"']+)["']`)
	androidAppPluginPattern = regexp.MustCompile(`(?m)(?:id\s*\(\s*["']com\.android\.application["']\s*\)|id\s+["']com\.android\.application["']|alias\s*\(\s*libs\.plugins\.android\.application\s*\))`)
	applyFalsePattern       = regexp.MustCompile(`(?i)^\s*(?:version\s+(?:["'][^"']+["']|\S+)\s+)?apply\s+false\b`)
)

// AndroidApplication describes the Gradle module that produces an Android
// application package. Module uses Gradle's colon-prefixed project notation.
type AndroidApplication struct {
	Module        string `json:"module"`
	Directory     string `json:"directory"`
	ApplicationID string `json:"applicationId"`
}

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
		if match := jvmToolchainPattern.FindStringSubmatch(content); len(match) == 2 {
			requirements.JavaVersion = maxJavaVersion(requirements.JavaVersion, javaVersion(match[1]))
		}
		if match := androidPluginPattern.FindStringSubmatch(content); len(match) == 2 {
			requirements.JavaVersion = maxJavaVersion(requirements.JavaVersion, minimumJavaForAGP(javaVersion(match[1])))
		}
		if match := androidClasspathPattern.FindStringSubmatch(content); len(match) == 2 {
			requirements.JavaVersion = maxJavaVersion(requirements.JavaVersion, minimumJavaForAGP(javaVersion(match[1])))
		}
		return nil
	})
	if err != nil {
		return AndroidRequirements{}, fmt.Errorf("inspect Android Gradle requirements: %w", err)
	}
	return requirements, nil
}

func javaVersion(value string) int {
	version, _ := strconv.Atoi(value)
	return version
}

func maxJavaVersion(current, candidate int) int {
	if candidate > current {
		return candidate
	}
	return current
}

// minimumJavaForAGP follows Android Gradle Plugin runtime requirements. It
// deliberately returns zero for pre-7 releases: without an explicit JVM
// toolchain, Mob should respect the user's selected JDK instead of forcing 8.
func minimumJavaForAGP(major int) int {
	switch {
	case major >= 8:
		return 17
	case major == 7:
		return 11
	default:
		return 0
	}
}

// AndroidApplicationID returns the explicit applicationId declared by an
// Android app module. It intentionally does not infer namespace-derived IDs.
func AndroidApplicationID(root string) (string, error) {
	application, err := AndroidApplicationFor(root)
	if err != nil {
		return "", err
	}
	if application.ApplicationID == "" {
		return "", fmt.Errorf("no explicit Android applicationId was found")
	}
	return application.ApplicationID, nil
}

// KotlinMultiplatformAndroidApplication finds the Android application module
// of a KMP project. A KMP root is often a library-only project, so callers
// must not assume that root-level installDebug targets a launchable app.
func KotlinMultiplatformAndroidApplication(root string) (AndroidApplication, error) {
	application, err := androidApplicationFor(root, false)
	if err != nil {
		return AndroidApplication{}, err
	}
	if application.Module == "" {
		return AndroidApplication{}, fmt.Errorf("no Android application module was found in the Kotlin Multiplatform project")
	}
	if application.ApplicationID == "" {
		return AndroidApplication{}, fmt.Errorf("Android application module %s does not declare an explicit applicationId", application.Module)
	}
	return application, nil
}

// AndroidApplicationFor finds a single Android application module. It uses
// Gradle's declared module layout first, then retains a narrow fallback for
// conventional projects that omit a settings include in incomplete fixtures.
func AndroidApplicationFor(root string) (AndroidApplication, error) {
	application, err := androidApplicationFor(root, true)
	if err != nil {
		return AndroidApplication{}, err
	}
	if application.Module != "" || application.Directory != "" {
		return application, nil
	}
	return fallbackAndroidApplication(root)
}

func androidApplicationFor(root string, includeRoot bool) (AndroidApplication, error) {
	modules, err := includedGradleModules(root)
	if err != nil {
		return AndroidApplication{}, fmt.Errorf("inspect Android application module: %w", err)
	}
	type candidate struct {
		module    string
		directory string
	}
	candidates := make([]candidate, 0, len(modules)+1)
	if includeRoot {
		candidates = append(candidates, candidate{directory: root})
	}
	for _, module := range modules {
		candidates = append(candidates, candidate{
			module:    ":" + strings.ReplaceAll(filepath.ToSlash(module), "/", ":"),
			directory: filepath.Join(root, module),
		})
	}
	applications := make([]AndroidApplication, 0, 1)
	for _, candidate := range candidates {
		content, found, err := moduleBuildScript(candidate.directory)
		if err != nil {
			return AndroidApplication{}, err
		}
		if !found || !declaresAndroidApplicationPlugin(content) {
			continue
		}
		application := AndroidApplication{Module: candidate.module, Directory: candidate.directory}
		if match := applicationIDPattern.FindStringSubmatch(content); len(match) == 2 {
			application.ApplicationID = match[1]
		}
		applications = append(applications, application)
	}
	if len(applications) > 1 {
		modules := make([]string, 0, len(applications))
		for _, application := range applications {
			if application.Module == "" {
				modules = append(modules, ":")
			} else {
				modules = append(modules, application.Module)
			}
		}
		return AndroidApplication{}, fmt.Errorf("multiple Android application modules were found: %s", strings.Join(modules, ", "))
	}
	if len(applications) == 1 {
		return applications[0], nil
	}
	return AndroidApplication{}, nil
}

func declaresAndroidApplicationPlugin(content string) bool {
	for _, location := range androidAppPluginPattern.FindAllStringIndex(content, -1) {
		if !applyFalsePattern.MatchString(content[location[1]:]) {
			return true
		}
	}
	return false
}

func moduleBuildScript(directory string) (string, bool, error) {
	for _, name := range []string{"build.gradle.kts", "build.gradle"} {
		path := filepath.Join(directory, name)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", false, fmt.Errorf("read %s: %w", path, err)
		}
		return string(data), true, nil
	}
	return "", false, nil
}

func fallbackAndroidApplication(root string) (AndroidApplication, error) {
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
		return AndroidApplication{}, fmt.Errorf("inspect Android application ID: %w", err)
	}
	return AndroidApplication{Directory: root, ApplicationID: applicationID}, nil
}
