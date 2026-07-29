// Package home resolves the directory owned by Mob.
package home

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const EnvironmentVariable = "MOB_HOME"

// Resolve uses MOB_HOME when configured and otherwise ~/.mob.
func Resolve() (string, error) {
	if value := strings.TrimSpace(os.Getenv(EnvironmentVariable)); value != "" {
		return filepath.Abs(filepath.Clean(value))
	}
	if selected, err := Selected(); err != nil {
		return "", err
	} else if selected != "" {
		return selected, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".mob"), nil
}

// Selected returns a user-level Mob home selection. MOB_HOME deliberately
// takes precedence in Resolve so CI can still override the local choice.
func Selected() (string, error) {
	path, err := selectionPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read Mob home selection: %w", err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", nil
	}
	return filepath.Abs(filepath.Clean(value))
}

// Select persists a Mob-owned home selection without modifying PATH or any
// language/toolchain environment variable.
func Select(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	selection, err := selectionPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(selection), 0o755); err != nil {
		return fmt.Errorf("create Mob user configuration directory: %w", err)
	}
	if err := os.WriteFile(selection, []byte(absolute+"\n"), 0o600); err != nil {
		return fmt.Errorf("save Mob home selection: %w", err)
	}
	return nil
}

// Migrate moves only the currently selected Mob-owned root. A non-empty
// destination is rejected so no unrelated user files are merged or replaced.
func Migrate(source, destination string) error {
	source, err := filepath.Abs(filepath.Clean(source))
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(filepath.Clean(destination))
	if err != nil {
		return err
	}
	if strings.EqualFold(source, destination) {
		return os.MkdirAll(destination, 0o755)
	}
	if info, err := os.Stat(destination); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("Mob home destination is a file: %s", destination)
		}
		entries, readErr := os.ReadDir(destination)
		if readErr != nil {
			return readErr
		}
		if len(entries) != 0 {
			return fmt.Errorf("Mob home destination must be empty: %s", destination)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return os.MkdirAll(destination, 0o755)
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	if err := copyTree(source, destination); err != nil {
		return err
	}
	return os.RemoveAll(source)
}

func selectionPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve Mob user configuration directory: %w", err)
	}
	return filepath.Join(directory, "mob", "home"), nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Mob home contains unsupported symbolic link: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeOut := out.Close()
		closeIn := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOut != nil {
			return closeOut
		}
		return closeIn
	})
}
