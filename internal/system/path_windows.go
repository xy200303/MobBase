//go:build windows

package system

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// AddUserPath appends a directory to the current user's PATH without using
// setx, which can truncate an existing long PATH value on Windows.
func AddUserPath(directory string) (bool, error) {
	directory, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return false, err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("open user environment: %w", err)
	}
	defer key.Close()
	current, valueType, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return false, fmt.Errorf("read user PATH: %w", err)
	}
	for _, entry := range strings.Split(current, ";") {
		if strings.EqualFold(filepath.Clean(strings.TrimSpace(entry)), directory) {
			return false, nil
		}
	}
	updated := directory
	if strings.TrimSpace(current) != "" {
		updated = strings.TrimRight(current, ";") + ";" + directory
	}
	if valueType == registry.EXPAND_SZ {
		err = key.SetExpandStringValue("Path", updated)
	} else {
		err = key.SetStringValue("Path", updated)
	}
	if err != nil {
		return false, fmt.Errorf("update user PATH: %w", err)
	}
	return true, nil
}
