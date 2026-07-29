// Package ios discovers the locally installed Apple toolchain. It never
// downloads, configures, or otherwise changes Xcode.
package ios

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/xy200303/MobBase/internal/system"
)

var (
	ErrHostUnsupported  = errors.New("iOS tooling requires macOS")
	ErrToolchainMissing = errors.New("Xcode toolchain is unavailable")
)

// Toolchain is the subset of Xcode identity that Mob needs for diagnostics.
type Toolchain struct {
	DeveloperDir string `json:"developerDir"`
	Version      string `json:"version"`
	BuildVersion string `json:"buildVersion"`
}

// Device is the iOS projection of Mob's cross-platform device model.
type Device struct {
	ID       string `json:"id"`
	Platform string `json:"platform"`
	NativeID string `json:"nativeId"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	State    string `json:"state"`
}

// Discover returns the active Xcode developer directory and version. It is
// intentionally read-only: installation, license acceptance, signing, and
// simulator management remain Xcode responsibilities.
func Discover(ctx context.Context) (Toolchain, error) {
	if runtime.GOOS != "darwin" {
		return Toolchain{}, ErrHostUnsupported
	}
	if _, found := system.LookPath("xcode-select"); !found {
		return Toolchain{}, fmt.Errorf("%w: xcode-select was not found", ErrToolchainMissing)
	}
	developer, err := system.Run(ctx, "xcode-select", []string{"-p"}, nil, "", "")
	if err != nil {
		return Toolchain{}, fmt.Errorf("%w: %v", ErrToolchainMissing, err)
	}
	developerDir := strings.TrimSpace(developer.Output)
	if developerDir == "" {
		return Toolchain{}, fmt.Errorf("%w: xcode-select returned an empty developer directory", ErrToolchainMissing)
	}
	if _, found := system.LookPath("xcodebuild"); !found {
		return Toolchain{}, fmt.Errorf("%w: xcodebuild was not found", ErrToolchainMissing)
	}
	version, err := system.Run(ctx, "xcodebuild", []string{"-version"}, nil, "", "")
	if err != nil {
		return Toolchain{}, fmt.Errorf("%w: %v", ErrToolchainMissing, err)
	}
	toolchain, err := ParseVersion(version.Output)
	if err != nil {
		return Toolchain{}, fmt.Errorf("%w: %v", ErrToolchainMissing, err)
	}
	toolchain.DeveloperDir = developerDir
	return toolchain, nil
}

// ParseVersion parses the stable fields emitted by xcodebuild -version.
func ParseVersion(output string) (Toolchain, error) {
	var toolchain Toolchain
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Xcode "):
			toolchain.Version = strings.TrimSpace(strings.TrimPrefix(line, "Xcode "))
		case strings.HasPrefix(line, "Build version "):
			toolchain.BuildVersion = strings.TrimSpace(strings.TrimPrefix(line, "Build version "))
		}
	}
	if toolchain.Version == "" || toolchain.BuildVersion == "" {
		return Toolchain{}, errors.New("xcodebuild -version did not report both Xcode and Build version")
	}
	return toolchain, nil
}

// ListDevices reads available iOS Simulators from the active Xcode toolchain.
// It intentionally does not boot, create, delete, or otherwise alter them.
func ListDevices(ctx context.Context) ([]Device, error) {
	if runtime.GOOS != "darwin" {
		return nil, ErrHostUnsupported
	}
	if _, found := system.LookPath("xcrun"); !found {
		return nil, fmt.Errorf("%w: xcrun was not found", ErrToolchainMissing)
	}
	result, err := system.Run(ctx, "xcrun", []string{"simctl", "list", "devices", "available", "-j"}, nil, "", "")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrToolchainMissing, err)
	}
	devices, err := ParseDevices(result.Output)
	if err != nil {
		return nil, fmt.Errorf("list iOS Simulators: %w", err)
	}
	return devices, nil
}

// StartSimulator boots an existing available Simulator, waits until it is
// usable, and opens Xcode's official Simulator application. It never creates
// a simulator definition or changes its contents.
func StartSimulator(ctx context.Context, nativeID string) error {
	if runtime.GOOS != "darwin" {
		return ErrHostUnsupported
	}
	if strings.TrimSpace(nativeID) == "" {
		return errors.New("iOS Simulator device ID is required")
	}
	if _, found := system.LookPath("xcrun"); !found {
		return fmt.Errorf("%w: xcrun was not found", ErrToolchainMissing)
	}
	result, err := system.Run(ctx, "xcrun", []string{"simctl", "boot", nativeID}, nil, "", "")
	if err != nil && !strings.Contains(strings.ToLower(result.Output), "current state: booted") {
		return fmt.Errorf("boot iOS Simulator %s: %w: %s", nativeID, err, strings.TrimSpace(result.Output))
	}
	result, err = system.Run(ctx, "xcrun", []string{"simctl", "bootstatus", nativeID, "-b"}, nil, "", "")
	if err != nil {
		return fmt.Errorf("wait for iOS Simulator %s: %w: %s", nativeID, err, strings.TrimSpace(result.Output))
	}
	if _, found := system.LookPath("open"); !found {
		return fmt.Errorf("%w: macOS open command was not found", ErrToolchainMissing)
	}
	if err := system.Start(ctx, "open", []string{"-a", "Simulator"}, nil, ""); err != nil {
		return fmt.Errorf("open Simulator application: %w", err)
	}
	return nil
}

// ParseDevices converts xcrun simctl's JSON into Mob's stable device model.
func ParseDevices(output string) ([]Device, error) {
	var payload struct {
		Devices map[string][]struct {
			UDID        string `json:"udid"`
			Name        string `json:"name"`
			State       string `json:"state"`
			IsAvailable bool   `json:"isAvailable"`
		} `json:"devices"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return nil, fmt.Errorf("parse simctl JSON: %w", err)
	}
	devices := make([]Device, 0)
	for _, runtimeDevices := range payload.Devices {
		for _, item := range runtimeDevices {
			if !item.IsAvailable || strings.TrimSpace(item.UDID) == "" {
				continue
			}
			state := strings.ToLower(strings.TrimSpace(item.State))
			if state == "booted" {
				state = "ready"
			} else if state == "" {
				state = "unknown"
			}
			devices = append(devices, Device{ID: "ios:" + item.UDID, Platform: "ios", NativeID: item.UDID, Kind: "simulator", Name: strings.TrimSpace(item.Name), State: state})
		}
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].State != devices[j].State {
			return devices[i].State == "ready"
		}
		return devices[i].Name < devices[j].Name
	})
	return devices, nil
}
