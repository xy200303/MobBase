// Package state persists Mob-owned registrations. External toolchains are
// referenced only; their files are never modified by this package.
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const FileName = "config.yaml"

type Ownership string

const (
	OwnershipDiscovered Ownership = "discovered"
	OwnershipImported   Ownership = "imported"
	OwnershipManaged    Ownership = "managed"
)

type AndroidSDK struct {
	Name      string    `yaml:"name" json:"name"`
	Path      string    `yaml:"path" json:"path"`
	Ownership Ownership `yaml:"ownership" json:"ownership"`
}

type Android struct {
	SDKs       []AndroidSDK `yaml:"sdks" json:"sdks"`
	CurrentSDK string       `yaml:"currentSdk,omitempty" json:"currentSdk,omitempty"`
	ProxyURL   string       `yaml:"proxyUrl,omitempty" json:"proxyUrl,omitempty"`
}

type FlutterSDK struct {
	Version string `yaml:"version" json:"version"`
	Path    string `yaml:"path" json:"path"`
}

type Flutter struct {
	SDKs       []FlutterSDK `yaml:"sdks" json:"sdks"`
	CurrentSDK string       `yaml:"currentSdk,omitempty" json:"currentSdk,omitempty"`
}

// FVMSDK is a Mob-managed FVM launcher. Its package cache is isolated from
// the user's global Dart PUB_CACHE so installing Mob never changes it.
type FVMSDK struct {
	Version string `yaml:"version" json:"version"`
	Path    string `yaml:"path" json:"path"`
	SHA256  string `yaml:"sha256" json:"sha256"`
}

type FVM struct {
	SDKs       []FVMSDK `yaml:"sdks" json:"sdks"`
	CurrentSDK string   `yaml:"currentSdk,omitempty" json:"currentSdk,omitempty"`
}

type JavaSDK struct {
	Name      string    `yaml:"name" json:"name"`
	Version   int       `yaml:"version" json:"version"`
	Path      string    `yaml:"path" json:"path"`
	Ownership Ownership `yaml:"ownership" json:"ownership"`
}

type Java struct {
	SDKs       []JavaSDK `yaml:"sdks" json:"sdks"`
	CurrentSDK string    `yaml:"currentSdk,omitempty" json:"currentSdk,omitempty"`
}

type Device struct {
	DefaultID string `yaml:"defaultId,omitempty" json:"defaultId,omitempty"`
}

type Config struct {
	Version int     `yaml:"version" json:"version"`
	Android Android `yaml:"android" json:"android"`
	Flutter Flutter `yaml:"flutter" json:"flutter"`
	FVM     FVM     `yaml:"fvm" json:"fvm"`
	Java    Java    `yaml:"java" json:"java"`
	Device  Device  `yaml:"device" json:"device"`
}

func Default() Config {
	return Config{
		Version: 1,
		Android: Android{SDKs: []AndroidSDK{}},
		Flutter: Flutter{SDKs: []FlutterSDK{}},
		FVM:     FVM{SDKs: []FVMSDK{}},
		Java:    Java{SDKs: []JavaSDK{}},
	}
}

type Store struct{ Path string }

func New(home string) Store { return Store{Path: filepath.Join(home, FileName)} }

func (s Store) Load() (Config, error) {
	config := Default()
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read Mob configuration: %w", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("parse Mob configuration: %w", err)
	}
	if config.Version == 0 {
		config.Version = 1
	}
	if config.Android.SDKs == nil {
		config.Android.SDKs = []AndroidSDK{}
	}
	if config.Flutter.SDKs == nil {
		config.Flutter.SDKs = []FlutterSDK{}
	}
	if config.FVM.SDKs == nil {
		config.FVM.SDKs = []FVMSDK{}
	}
	if config.Java.SDKs == nil {
		config.Java.SDKs = []JavaSDK{}
	}
	return config, nil
}

// Save writes a temporary file in the destination directory and atomically
// replaces the previous configuration only after serialization succeeds.
func (s Store) Save(config Config) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("create Mob home: %w", err)
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode Mob configuration: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.Path), "config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary configuration: %w", err)
	}
	if err := replaceFile(temporaryPath, s.Path); err != nil {
		return fmt.Errorf("replace Mob configuration: %w", err)
	}
	return nil
}
