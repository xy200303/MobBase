package app

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	gort "runtime"
	"strings"
	"time"

	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/state"
)

// cliVersion is overridden for release artifacts with Go's -ldflags -X.
// Development builds deliberately remain identifiable without requiring Git.
var cliVersion = "dev"

// support creates a deliberately small diagnostic archive. It uses only Mob's
// own configuration and process metadata so that it cannot accidentally ship
// application source, device logs, environment variables, or credentials.
func (r runtime) support(args []string) error {
	if len(args) == 0 || args[0] != "bundle" {
		return invalidCommand("mob support " + strings.Join(args, " "))
	}
	output, err := supportOutputPath(args[1:])
	if err != nil {
		return err
	}
	if err := r.createSupportBundle(output); err != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Create support bundle: " + err.Error(), Remediation: "Choose a writable output path that does not already exist, then retry."}
	}
	data := map[string]string{"path": output}
	if r.json {
		return r.result("mob support bundle", data)
	}
	fmt.Fprintf(r.out, "Created redacted support bundle at %s.\n", output)
	return nil
}

func supportOutputPath(args []string) (string, error) {
	var output string
	if len(args) == 0 {
		output = fmt.Sprintf("mob-support-%s.zip", time.Now().UTC().Format("20060102T150405Z"))
	} else if len(args) == 2 && args[0] == "--output" && strings.TrimSpace(args[1]) != "" {
		output = args[1]
	} else {
		return "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob support bundle [--output <path>]."}
	}
	if !strings.EqualFold(filepath.Ext(output), ".zip") {
		return "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Support bundle output must use a .zip extension."}
	}
	path, err := filepath.Abs(filepath.Clean(output))
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Support bundle output already exists: " + path, Remediation: "Choose a new output filename; Mob never overwrites support bundles."}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

func (r runtime) createSupportBundle(output string) error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	completed := false
	defer func() {
		file.Close()
		if !completed {
			_ = os.Remove(output)
		}
	}()

	archive := zip.NewWriter(file)
	if err := writeBundleJSON(archive, "manifest.json", supportManifest()); err != nil {
		archive.Close()
		return err
	}
	if err := writeBundleJSON(archive, "toolchains.json", redactSupportConfig(config, r.home)); err != nil {
		archive.Close()
		return err
	}
	if err := archive.Close(); err != nil {
		return err
	}
	completed = true
	return nil
}

func writeBundleJSON(archive *zip.Writer, name string, value interface{}) error {
	entry, err := archive.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func supportManifest() map[string]string {
	manifest := map[string]string{
		"schemaVersion": "1.0",
		"mobVersion":    cliVersion,
		"capturedAt":    time.Now().UTC().Format(time.RFC3339),
		"goos":          gort.GOOS,
		"goarch":        gort.GOARCH,
		"contents":      "Redacted Mob configuration and registered toolchain inventory only.",
		"excluded":      "Project files, environment variables, logs, proxy settings, credentials, raw host paths, and device identifiers.",
	}
	if info, err := project.Detect(""); err == nil && info != nil {
		manifest["projectKind"] = string(info.Kind)
	}
	return manifest
}

type supportConfig struct {
	Version int `json:"version"`
	Android struct {
		SDKs       []supportAndroidSDK `json:"sdks"`
		CurrentSDK string              `json:"currentSdk,omitempty"`
	} `json:"android"`
	Flutter struct {
		SDKs       []supportFlutterSDK `json:"sdks"`
		CurrentSDK string              `json:"currentSdk,omitempty"`
	} `json:"flutter"`
	FVM struct {
		SDKs       []supportFVMSDK `json:"sdks"`
		CurrentSDK string          `json:"currentSdk,omitempty"`
	} `json:"fvm"`
	Java struct {
		SDKs       []supportJavaSDK `json:"sdks"`
		CurrentSDK string           `json:"currentSdk,omitempty"`
	} `json:"java"`
	DeviceConfigured bool `json:"deviceConfigured"`
}

type supportAndroidSDK struct {
	Name      string          `json:"name"`
	Path      string          `json:"path"`
	Ownership state.Ownership `json:"ownership"`
}

type supportFlutterSDK struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

type supportFVMSDK struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

type supportJavaSDK struct {
	Name      string          `json:"name"`
	Version   int             `json:"version"`
	Path      string          `json:"path"`
	Ownership state.Ownership `json:"ownership"`
}

func redactSupportConfig(config state.Config, mobHome string) supportConfig {
	result := supportConfig{Version: config.Version, DeviceConfigured: config.Device.DefaultID != ""}
	result.Android.CurrentSDK = config.Android.CurrentSDK
	for _, sdk := range config.Android.SDKs {
		result.Android.SDKs = append(result.Android.SDKs, supportAndroidSDK{Name: sdk.Name, Path: redactSupportPath(sdk.Path, mobHome), Ownership: sdk.Ownership})
	}
	result.Flutter.CurrentSDK = config.Flutter.CurrentSDK
	for _, sdk := range config.Flutter.SDKs {
		result.Flutter.SDKs = append(result.Flutter.SDKs, supportFlutterSDK{Version: sdk.Version, Path: redactSupportPath(sdk.Path, mobHome)})
	}
	result.FVM.CurrentSDK = config.FVM.CurrentSDK
	for _, sdk := range config.FVM.SDKs {
		result.FVM.SDKs = append(result.FVM.SDKs, supportFVMSDK{Version: sdk.Version, Path: redactSupportPath(sdk.Path, mobHome)})
	}
	result.Java.CurrentSDK = config.Java.CurrentSDK
	for _, sdk := range config.Java.SDKs {
		result.Java.SDKs = append(result.Java.SDKs, supportJavaSDK{Name: sdk.Name, Version: sdk.Version, Path: redactSupportPath(sdk.Path, mobHome), Ownership: sdk.Ownership})
	}
	return result
}

func redactSupportPath(path, mobHome string) string {
	path = filepath.Clean(path)
	home := filepath.Clean(mobHome)
	if relative, err := filepath.Rel(home, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(filepath.Join("<mob-home>", relative))
	}
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "<external>"
	}
	return filepath.ToSlash(filepath.Join("<external>", base))
}
