package android

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/xy200303/MobBase/internal/state"
)

type SDK struct {
	Name       string          `json:"name"`
	Path       string          `json:"path"`
	Ownership  state.Ownership `json:"ownership"`
	Current    bool            `json:"current"`
	Components Components      `json:"components"`
}

type Components struct {
	Platforms      []string `json:"platforms"`
	BuildTools     []string `json:"buildTools"`
	NDK            []string `json:"ndk"`
	SystemImages   []string `json:"systemImages"`
	PlatformTools  bool     `json:"platformTools"`
	CommandLineSDK bool     `json:"commandLineTools"`
	Emulator       bool     `json:"emulator"`
}

// Discover returns valid standard SDKs plus persisted imported/managed SDKs.
// A persisted name takes precedence when its path matches a standard location.
func Discover(config state.Config) ([]SDK, error) {
	registered := append([]state.AndroidSDK(nil), config.Android.SDKs...)
	for _, path := range standardPaths() {
		if path == "" || !validSDKRoot(path) || registeredPath(registered, path) {
			continue
		}
		registered = append(registered, state.AndroidSDK{
			Name:      uniqueName("system", registered),
			Path:      path,
			Ownership: state.OwnershipDiscovered,
		})
	}

	result := make([]SDK, 0, len(registered))
	seen := make(map[string]bool)
	for _, entry := range registered {
		path, err := cleanPath(entry.Path)
		if err != nil || !validSDKRoot(path) || seen[strings.ToLower(path)] {
			continue
		}
		seen[strings.ToLower(path)] = true
		result = append(result, SDK{
			Name:       entry.Name,
			Path:       path,
			Ownership:  entry.Ownership,
			Current:    entry.Name == config.Android.CurrentSDK,
			Components: inspectComponents(path),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func ValidateSDKRoot(path string) (string, error) {
	clean, err := cleanPath(path)
	if err != nil {
		return "", err
	}
	if !validSDKRoot(clean) {
		return "", fmt.Errorf("%s is not an Android SDK root", clean)
	}
	return clean, nil
}

func inspectComponents(root string) Components {
	components := Components{
		Platforms:      directoryNames(filepath.Join(root, "platforms")),
		BuildTools:     directoryNames(filepath.Join(root, "build-tools")),
		NDK:            directoryNames(filepath.Join(root, "ndk")),
		SystemImages:   systemImagePackages(filepath.Join(root, "system-images")),
		PlatformTools:  directory(filepath.Join(root, "platform-tools")),
		CommandLineSDK: directory(filepath.Join(root, "cmdline-tools")),
		Emulator:       directory(filepath.Join(root, "emulator")),
	}
	return components
}

func standardPaths() []string {
	paths := []string{os.Getenv("ANDROID_SDK_ROOT"), os.Getenv("ANDROID_HOME")}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		paths = append(paths, filepath.Join(local, "Android", "Sdk"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		switch runtime.GOOS {
		case "darwin":
			paths = append(paths, filepath.Join(home, "Library", "Android", "sdk"))
		default:
			paths = append(paths, filepath.Join(home, "Android", "Sdk"))
		}
	}
	return paths
}

func registeredPath(entries []state.AndroidSDK, path string) bool {
	clean, err := cleanPath(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		entryPath, err := cleanPath(entry.Path)
		if err == nil && strings.EqualFold(entryPath, clean) {
			return true
		}
	}
	return false
}

func uniqueName(base string, entries []state.AndroidSDK) string {
	for suffix := 1; ; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}
		found := false
		for _, entry := range entries {
			if entry.Name == candidate {
				found = true
				break
			}
		}
		if !found {
			return candidate
		}
	}
}

func cleanPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("SDK path is required")
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve SDK path: %w", err)
	}
	return abs, nil
}

func validSDKRoot(path string) bool {
	if !directory(path) {
		return false
	}
	for _, child := range []string{"platform-tools", "platforms", "build-tools", "cmdline-tools", "emulator", "ndk"} {
		if directory(filepath.Join(path, child)) {
			return true
		}
	}
	return false
}

func directory(path string) bool { info, err := os.Stat(path); return err == nil && info.IsDir() }

func directoryNames(path string) []string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return []string{}
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			result = append(result, entry.Name())
		}
	}
	sort.Strings(result)
	return result
}

func systemImagePackages(root string) []string {
	apis, err := os.ReadDir(root)
	if err != nil {
		return []string{}
	}
	packages := make([]string, 0)
	for _, api := range apis {
		if !api.IsDir() {
			continue
		}
		vendors, err := os.ReadDir(filepath.Join(root, api.Name()))
		if err != nil {
			continue
		}
		for _, vendor := range vendors {
			if !vendor.IsDir() {
				continue
			}
			abis, err := os.ReadDir(filepath.Join(root, api.Name(), vendor.Name()))
			if err != nil {
				continue
			}
			for _, abi := range abis {
				if abi.IsDir() {
					packages = append(packages, strings.Join([]string{"system-images", api.Name(), vendor.Name(), abi.Name()}, ";"))
				}
			}
		}
	}
	sort.Strings(packages)
	return packages
}
