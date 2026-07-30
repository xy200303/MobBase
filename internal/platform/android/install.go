package android

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/xy200303/MobBase/internal/system"
)

type InstallRequest struct {
	Root           string
	Packages       []string
	AcceptLicenses bool
	// Environment is injected only into sdkmanager subprocesses. It supports
	// network proxies without changing the user's global environment.
	Environment []string
	// Output receives the live sdkmanager transcript for interactive callers.
	// JSON callers leave it nil so stdout remains machine-readable.
	Output io.Writer
}

type InstallResult struct {
	SDKManager string   `json:"sdkManager"`
	Packages   []string `json:"packages"`
	// Output is reserved for concise diagnostic output. Successful installs
	// stream their transcript to interactive callers and intentionally leave it
	// empty so JSON consumers do not receive shell-specific batch-file echoes.
	Output string `json:"output,omitempty"`
}

// InstallPackages delegates package installation to the Android SDK's official
// sdkmanager. The caller owns target selection and external-write consent.
func InstallPackages(ctx context.Context, request InstallRequest) (InstallResult, error) {
	if len(request.Packages) == 0 {
		return InstallResult{}, fmt.Errorf("at least one Android SDK package is required")
	}
	manager, found := SDKManager(request.Root)
	if !found {
		return InstallResult{}, fmt.Errorf("Android SDK command-line tools were not found in %s", request.Root)
	}
	if !request.AcceptLicenses {
		return InstallResult{}, fmt.Errorf("Android SDK licenses must be accepted before installation")
	}
	program, args := system.BatchCommand(manager, "--sdk_root="+request.Root, "--licenses")
	if result, err := system.Run(ctx, program, args, request.Environment, "", strings.Repeat("y\n", 64)); err != nil {
		return InstallResult{}, fmt.Errorf("accept Android SDK licenses: %w: %s", err, sdkManagerDiagnostic(result.Output))
	}
	packages := append([]string(nil), request.Packages...)
	sort.Strings(packages)
	program, args = system.BatchCommand(manager, append([]string{"--sdk_root=" + request.Root, "--install"}, packages...)...)
	result, err := system.RunWithOutput(ctx, program, args, request.Environment, "", "", request.Output)
	if flusher, ok := request.Output.(interface{ Flush() }); ok {
		flusher.Flush()
	}
	if err != nil {
		return InstallResult{}, fmt.Errorf("install Android SDK packages: %w: %s", err, sdkManagerDiagnostic(result.Output))
	}
	return InstallResult{SDKManager: manager, Packages: packages}, nil
}

// sdkManagerDiagnostic removes cmd.exe's echo of sdkmanager.bat internals while
// retaining the SDK manager's actual diagnostic messages.
func sdkManagerDiagnostic(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	meaningful := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || system.IsWindowsBatchEcho(line) {
			continue
		}
		meaningful = append(meaningful, line)
	}
	return strings.Join(meaningful, "\n")
}

// SDKManager finds a command-line-tools installation without depending on a
// global PATH. Older SDK layouts are retained for existing installations.
func SDKManager(root string) (string, bool) {
	return commandLineTool(root, sdkManagerExecutable())
}

func AVDManager(root string) (string, bool) {
	return commandLineTool(root, avdManagerExecutable())
}

func commandLineTool(root, executable string) (string, bool) {
	candidates := []string{
		filepath.Join(root, "cmdline-tools", "latest", "bin", executable),
		filepath.Join(root, "cmdline-tools", "bin", executable),
	}
	entries, err := os.ReadDir(filepath.Join(root, "cmdline-tools"))
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				candidates = append(candidates, filepath.Join(root, "cmdline-tools", entry.Name(), "bin", executable))
			}
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return filepath.Join(root, "cmdline-tools", "latest", "bin", executable), false
}

func sdkManagerExecutable() string {
	if runtime.GOOS == "windows" {
		return "sdkmanager.bat"
	}
	return "sdkmanager"
}

func avdManagerExecutable() string {
	if runtime.GOOS == "windows" {
		return "avdmanager.bat"
	}
	return "avdmanager"
}
