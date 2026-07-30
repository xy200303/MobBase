//go:build !windows

package system

import (
	"fmt"
	"os"
	"path/filepath"
)

// AddUserPath exposes Gradle through ~/.local/bin, the standard user command
// directory on Unix-like systems. Shells that do not include it can still use
// the managed Gradle path reported by Mob.
func AddUserPath(directory string) (bool, error) {
	directory, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return false, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return false, err
	}
	link := filepath.Join(bin, "gradle")
	target := filepath.Join(directory, "gradle")
	if existing, err := os.Readlink(link); err == nil {
		if existing == target {
			return false, nil
		}
		return false, fmt.Errorf("%s already points to %s", link, existing)
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect %s: %w", link, err)
	}
	if err := os.Symlink(target, link); err != nil {
		return false, fmt.Errorf("create %s: %w", link, err)
	}
	return true, nil
}
