package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultAPILevel = 35
const DefaultBuildTools = "35.0.0"

type Android struct {
	SDKRoot             string `json:"sdkRoot"`
	JavaHome            string `json:"javaHome,omitempty"`
	APILevel            int    `json:"apiLevel"`
	BuildTools          string `json:"buildTools"`
	GradleUserHome      string `json:"gradleUserHome,omitempty"`
	CommandLineToolsURL string `json:"commandLineToolsUrl,omitempty"`
}

type Config struct {
	Android Android `json:"android"`
}

func Default() Config {
	return Config{Android: Android{APILevel: DefaultAPILevel, BuildTools: DefaultBuildTools}}
}

type Store struct {
	Path string
}

func NewStore() (Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Store{}, fmt.Errorf("resolve user config directory: %w", err)
	}
	return Store{Path: filepath.Join(dir, "mob", "config.json")}, nil
}

func (s Store) Load() (Config, error) {
	config := Default()
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", s.Path, err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", s.Path, err)
	}
	if config.Android.APILevel == 0 {
		config.Android.APILevel = DefaultAPILevel
	}
	if config.Android.BuildTools == "" {
		config.Android.BuildTools = DefaultBuildTools
	}
	return config, nil
}

func (s Store) Save(config Config) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(s.Path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", s.Path, err)
	}
	return nil
}
