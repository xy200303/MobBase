package app

import (
	"os"
	"path/filepath"
	"reflect"
	gort "runtime"
	"testing"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
)

func TestParseRunAndReplaceDeviceTokens(t *testing.T) {
	options, err := parseRun([]string{"--platform", "android", "--device", "android:emulator-5554", "--no-device-create", "--no-install", "--accept-licenses", "--mirror", "--headless", "--", "gradlew.bat", "installDebug", "--serial", "{{mob.device.nativeId}}"})
	if err != nil {
		t.Fatalf("parse run: %v", err)
	}
	if options.Platform != "android" || options.Device != "android:emulator-5554" || !options.NoDeviceCreate || !options.NoInstall || !options.AcceptLicenses || !options.Mirror || !options.Headless {
		t.Fatalf("unexpected options: %#v", options)
	}
	device := android.Device{ID: "android:emulator-5554", NativeID: "emulator-5554", Platform: "android"}
	program, arguments := replaceRunTokens(options.Command[0], options.Command[1:], device)
	if program != "gradlew.bat" || !reflect.DeepEqual(arguments, []string{"installDebug", "--serial", "emulator-5554"}) {
		t.Fatalf("unexpected replacement: %q %#v", program, arguments)
	}
}

func TestSelectRunPlatformAndDevice(t *testing.T) {
	info := &project.Info{Kind: project.KindKotlinMultiplatform, Targets: []string{"android", "ios"}}
	platform, err := selectRunPlatform(info, runOptions{Device: "android:emulator-5554"}, "")
	if err != nil || platform != "android" {
		t.Fatalf("platform = %q, err = %v", platform, err)
	}
	devices := []android.Device{
		{ID: "android:physical", State: "unauthorized"},
		{ID: "android:emulator-5554", NativeID: "emulator-5554", State: "ready"},
	}
	device, err := selectAndroidRunDevice(devices, "", "android:physical")
	if err != nil || device.ID != "android:emulator-5554" {
		t.Fatalf("device = %#v, err = %v", device, err)
	}
}

func TestParseEmulatorCreate(t *testing.T) {
	options, err := parseEmulatorCreate([]string{"mob-android-api-35", "--image", "system-images;android-35;google_apis;x86_64", "--sdk", "managed"})
	if err != nil {
		t.Fatalf("parse emulator create: %v", err)
	}
	if options.Name != "mob-android-api-35" || options.SDK != "managed" || options.Image != "system-images;android-35;google_apis;x86_64" {
		t.Fatalf("unexpected options: %#v", options)
	}
}

func TestRunProjectCommandForwardsFlutterCommand(t *testing.T) {
	info := &project.Info{Kind: project.KindFlutter, Root: t.TempDir()}
	program, arguments, launches, _, err := runProjectCommand(info, []string{"fvm", "flutter", "run"}, "emulator-5554", nil)
	if err != nil || program != "fvm" || !reflect.DeepEqual(arguments, []string{"flutter", "run"}) || launches {
		t.Fatalf("unexpected Flutter run command: %q %#v %t %v", program, arguments, launches, err)
	}
}

func TestRunProjectCommandUsesKotlinMultiplatformAppModule(t *testing.T) {
	root := t.TempDir()
	wrapper := "gradlew"
	if gort.GOOS == "windows" {
		wrapper = "gradlew.bat"
	}
	if err := os.WriteFile(filepath.Join(root, wrapper), []byte("wrapper"), 0o644); err != nil {
		t.Fatalf("write Gradle Wrapper: %v", err)
	}
	info := &project.Info{Kind: project.KindKotlinMultiplatform, Root: root}
	application := &project.AndroidApplication{Module: ":androidApp", ApplicationID: "com.example.kmp"}
	program, arguments, launches, applicationID, err := runProjectCommand(info, nil, "emulator-5554", application)
	if err != nil || program != filepath.Join(root, wrapper) || !reflect.DeepEqual(arguments, []string{":androidApp:installDebug"}) || !launches || applicationID != "com.example.kmp" {
		t.Fatalf("unexpected KMP command: %q %#v %t %q %v", program, arguments, launches, applicationID, err)
	}
}

func TestSelectDefaultAndroidAVD(t *testing.T) {
	emulators := []android.Emulator{
		{Name: "mob-android-api-27"},
		{Name: "pixel-api-35"},
		{Name: "mob-android-api-35"},
	}
	avd, found := selectDefaultAndroidAVD(emulators, "mob-android-api-35")
	if !found || avd != "mob-android-api-35" {
		t.Fatalf("selected AVD = %q, found = %t", avd, found)
	}
	if avd, found := selectDefaultAndroidAVD(emulators, "mob-android-api-34"); found || avd != "" {
		t.Fatalf("unexpected fallback AVD = %q, found = %t", avd, found)
	}
}
