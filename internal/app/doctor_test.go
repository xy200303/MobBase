package app

import (
	"os"
	"path/filepath"
	gort "runtime"
	"testing"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
)

func TestAndroidRequirementDetail(t *testing.T) {
	detail := androidRequirementDetail(project.AndroidRequirements{CompileSDK: 35, BuildTools: "35.0.0", NDKVersion: "27.2.12479018"}, nil)
	if detail != "compileSdk=35, buildTools=35.0.0, ndk=27.2.12479018" {
		t.Fatalf("detail: %q", detail)
	}
}

func TestAndroidDeviceToolChecks(t *testing.T) {
	checks := androidDeviceToolChecks(android.SDK{Name: "managed"})
	if len(checks) != 2 || checks[0].ID != "android-adb" || checks[0].Status != "missing" || checks[0].Required {
		t.Fatalf("unexpected ADB check: %#v", checks)
	}
	if checks[1].ID != "android-emulator" || checks[1].Status != "missing" || checks[1].Required {
		t.Fatalf("unexpected Emulator check: %#v", checks)
	}
	checks = androidDeviceToolChecks(android.SDK{Name: "managed", Components: android.Components{PlatformTools: true, Emulator: true}})
	if checks[0].Status != "ok" || checks[0].Fix != "" || checks[1].Status != "ok" || checks[1].Fix != "" {
		t.Fatalf("unexpected available checks: %#v", checks)
	}
}

func TestNativeGradleWrapperCheckUsesProjectWrapper(t *testing.T) {
	root := t.TempDir()
	wrapper := "gradlew"
	if gort.GOOS == "windows" {
		wrapper = "gradlew.bat"
	}
	if err := os.WriteFile(filepath.Join(root, wrapper), []byte(""), 0o644); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	if _, _, err := buildCommand(root, nil); err != nil {
		t.Fatalf("recognize wrapper: %v", err)
	}
}
