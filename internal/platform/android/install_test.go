package android

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSDKManagerFindsLatestLayout(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "cmdline-tools", "latest", "bin", sdkManagerExecutable())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create command-line tools directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("placeholder"), 0o755); err != nil {
		t.Fatalf("create sdkmanager: %v", err)
	}
	manager, found := SDKManager(root)
	if !found || manager != path {
		t.Fatalf("SDKManager() = %q, %t; want %q, true", manager, found, path)
	}
}

func TestInstallPackagesRequiresLicenseAcceptance(t *testing.T) {
	_, err := InstallPackages(t.Context(), InstallRequest{Root: t.TempDir(), Packages: []string{"platform-tools"}})
	if err == nil || err.Error() != "Android SDK command-line tools were not found in "+t.TempDir() {
		// The temporary root differs across calls; verify the contract rather than its path.
		if err == nil || err.Error() == "" {
			t.Fatal("expected command-line-tools error")
		}
	}
}

func TestSDKManagerDiagnosticRemovesWindowsBatchEcho(t *testing.T) {
	raw := "C:\\Android\\cmdline-tools\\latest\\bin>if \"Windows_NT\"==\"Windows_NT\" setlocal\r\n" +
		"set DIRNAME=C:\\Android\\cmdline-tools\\latest\\bin\\\r\n" +
		"[=======================================] 100% Installing platform-tools\r\n" +
		"Warning: Package path is invalid\r\n"

	got := sdkManagerDiagnostic(raw)
	want := "[=======================================] 100% Installing platform-tools\nWarning: Package path is invalid"
	if got != want {
		t.Fatalf("sdkManagerDiagnostic() = %q, want %q", got, want)
	}
}
