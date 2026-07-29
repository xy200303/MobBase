package android

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/xy200303/MobBase/internal/config"
	"github.com/xy200303/MobBase/internal/platform"
	"github.com/xy200303/MobBase/internal/system"
)

const defaultCommandLineToolsURL = "https://dl.google.com/android/repository/commandlinetools-win-11076708_latest.zip"

type Service struct {
	Config config.Android
}

func (s Service) ID() string { return "android" }

func (s Service) Doctor(context.Context) (platform.Report, error) {
	checks := make([]platform.Check, 0, 7)
	sdkRoot := s.SDKRoot()

	javaPath, hasJava := system.LookPath("java")
	if !hasJava && s.Config.JavaHome != "" {
		javaPath = executable(filepath.Join(s.Config.JavaHome, "bin", "java"))
		_, err := os.Stat(javaPath)
		hasJava = err == nil
	}
	checks = append(checks, check("java", "JDK", hasJava, true, javaPath, "Install JDK 17, or run mob env setup android --install-jdk --yes."))
	checks = append(checks, check("android-sdk", "Android SDK root", directoryExists(sdkRoot), true, sdkRoot, "Run mob env setup android --yes."))

	sdkmanager := s.SDKManager()
	checks = append(checks, check("sdkmanager", "Android SDK Manager", fileExists(sdkmanager), true, sdkmanager, "Run mob env setup android --yes."))
	adb := s.ADB()
	checks = append(checks, check("adb", "Android Debug Bridge", fileExists(adb), true, adb, "Install platform-tools with mob env setup android --yes."))
	emulator := s.Emulator()
	checks = append(checks, check("emulator", "Android Emulator", fileExists(emulator), false, emulator, "Install emulator with mob env setup android --yes --with-emulator."))
	platformDir := filepath.Join(sdkRoot, "platforms", fmt.Sprintf("android-%d", s.Config.APILevel))
	checks = append(checks, check("android-platform", fmt.Sprintf("Android API %d", s.Config.APILevel), directoryExists(platformDir), true, platformDir, "Install the configured Android platform with mob env setup android --yes."))
	buildToolsDir := filepath.Join(sdkRoot, "build-tools", s.Config.BuildTools)
	checks = append(checks, check("build-tools", "Android Build Tools", directoryExists(buildToolsDir), true, buildToolsDir, "Install build tools with mob env setup android --yes."))

	ready := true
	for _, item := range checks {
		if item.Required && item.Status != "ok" {
			ready = false
		}
	}
	return platform.Report{Platform: s.ID(), Ready: ready, Checks: checks}, nil
}

func (s Service) SDKRoot() string {
	for _, value := range []string{s.Config.SDKRoot, os.Getenv("ANDROID_SDK_ROOT"), os.Getenv("ANDROID_HOME")} {
		if value != "" {
			return filepath.Clean(value)
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "Android", "Sdk")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Android", "Sdk")
	}
	return ""
}

func (s Service) SDKManager() string {
	root := s.SDKRoot()
	for _, candidate := range []string{
		filepath.Join(root, "cmdline-tools", "latest", "bin", executable("sdkmanager")),
		filepath.Join(root, "cmdline-tools", "bin", executable("sdkmanager")),
	} {
		if fileExists(candidate) {
			return candidate
		}
	}
	return filepath.Join(root, "cmdline-tools", "latest", "bin", executable("sdkmanager"))
}

func (s Service) ADB() string {
	path := filepath.Join(s.SDKRoot(), "platform-tools", executable("adb"))
	if fileExists(path) {
		return path
	}
	if resolved, ok := system.LookPath(executable("adb")); ok {
		return resolved
	}
	return path
}

func (s Service) Emulator() string {
	return filepath.Join(s.SDKRoot(), "emulator", executable("emulator"))
}

func (s Service) AVDManager() string {
	return filepath.Join(s.SDKRoot(), "cmdline-tools", "latest", "bin", executable("avdmanager"))
}

func (s Service) Environment(device string) []string {
	entries := []string{"ANDROID_SDK_ROOT=" + s.SDKRoot(), "ANDROID_HOME=" + s.SDKRoot()}
	if s.Config.JavaHome != "" {
		entries = append(entries, "JAVA_HOME="+s.Config.JavaHome)
	}
	if s.Config.GradleUserHome != "" {
		entries = append(entries, "GRADLE_USER_HOME="+s.Config.GradleUserHome)
	}
	if device != "" {
		entries = append(entries, "ANDROID_SERIAL="+device)
	}
	return entries
}

type SetupOptions struct {
	Yes          bool
	InstallJDK   bool
	WithEmulator bool
	Persist      bool
}

type SetupResult struct {
	SDKRoot      string   `json:"sdkRoot"`
	Installed    []string `json:"installed"`
	Persisted    bool     `json:"persisted"`
	NextCommands []string `json:"nextCommands"`
}

func (s Service) Setup(ctx context.Context, options SetupOptions) (SetupResult, error) {
	if !options.Yes {
		return SetupResult{}, fmt.Errorf("confirmation required: rerun with --yes after reviewing the Android SDK license and download size")
	}
	if runtime.GOOS != "windows" && options.InstallJDK {
		return SetupResult{}, fmt.Errorf("automatic JDK installation is currently implemented for Windows only")
	}
	if _, found := system.LookPath("java"); !found && options.InstallJDK {
		if _, found := system.LookPath("winget"); !found {
			return SetupResult{}, fmt.Errorf("JDK missing and winget is unavailable; install a JDK 17 manually")
		}
		if _, err := system.Run(ctx, "winget", []string{"install", "--id", "EclipseAdoptium.Temurin.17.JDK", "--exact", "--accept-package-agreements", "--accept-source-agreements"}, nil, "", ""); err != nil {
			return SetupResult{}, fmt.Errorf("install JDK 17: %w", err)
		}
	}

	root := s.SDKRoot()
	if root == "" {
		return SetupResult{}, fmt.Errorf("cannot determine Android SDK root")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return SetupResult{}, fmt.Errorf("create Android SDK root: %w", err)
	}
	if !fileExists(s.SDKManager()) {
		if err := s.installCommandLineTools(ctx, root); err != nil {
			return SetupResult{}, err
		}
	}

	packages := []string{"platform-tools", fmt.Sprintf("platforms;android-%d", s.Config.APILevel), "build-tools;" + s.Config.BuildTools}
	if options.WithEmulator {
		packages = append(packages, "emulator", fmt.Sprintf("system-images;android-%d;google_apis;x86_64", s.Config.APILevel))
	}
	manager := s.SDKManager()
	program, args := system.BatchCommand(manager, "--sdk_root="+root, "--licenses")
	if _, err := system.Run(ctx, program, args, s.Environment(""), "", strings.Repeat("y\n", 64)); err != nil {
		return SetupResult{}, fmt.Errorf("accept Android SDK licenses: %w", err)
	}
	program, args = system.BatchCommand(manager, append([]string{"--sdk_root=" + root, "--install"}, packages...)...)
	if result, err := system.Run(ctx, program, args, s.Environment(""), "", ""); err != nil {
		return SetupResult{}, fmt.Errorf("install Android SDK packages: %w\n%s", err, result.Output)
	}

	if options.Persist {
		if err := persistEnvironment(ctx, "ANDROID_SDK_ROOT", root); err != nil {
			return SetupResult{}, err
		}
		if err := persistEnvironment(ctx, "ANDROID_HOME", root); err != nil {
			return SetupResult{}, err
		}
	}
	return SetupResult{SDKRoot: root, Installed: packages, Persisted: options.Persist, NextCommands: []string{"mob doctor --platform android", "mob emulator create pixel-api" + strconv.Itoa(s.Config.APILevel), "mob device list"}}, nil
}

func (s Service) installCommandLineTools(ctx context.Context, root string) error {
	url := s.Config.CommandLineToolsURL
	if url == "" {
		if runtime.GOOS != "windows" {
			return fmt.Errorf("Android command-line tools URL must be configured with mob env setup android --cmdline-tools-url on this operating system")
		}
		url = defaultCommandLineToolsURL
	}
	response, err := (&http.Client{Timeout: 30 * time.Minute}).Get(url)
	if err != nil {
		return fmt.Errorf("download Android command-line tools: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("download Android command-line tools: server returned %s", response.Status)
	}
	temporary, err := os.CreateTemp("", "mob-commandlinetools-*.zip")
	if err != nil {
		return err
	}
	archivePath := temporary.Name()
	defer os.Remove(archivePath)
	if _, err := temporary.ReadFrom(response.Body); err != nil {
		temporary.Close()
		return fmt.Errorf("save Android command-line tools: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	destination := filepath.Join(root, "cmdline-tools", "latest")
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("replace command-line tools: %w", err)
	}
	if err := system.ExtractZipPrefix(archivePath, destination, "cmdline-tools"); err != nil {
		return fmt.Errorf("extract Android command-line tools: %w", err)
	}
	return nil
}

func persistEnvironment(ctx context.Context, key, value string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("persisting environment variables is currently implemented for Windows only")
	}
	if _, err := system.Run(ctx, "setx", []string{key, value}, nil, "", ""); err != nil {
		return fmt.Errorf("persist %s: %w", key, err)
	}
	return nil
}

func check(id, label string, ok, required bool, detail, fix string) platform.Check {
	status := "missing"
	if ok {
		status = "ok"
		fix = ""
	}
	return platform.Check{ID: id, Label: label, Status: status, Required: required, Detail: detail, Fix: fix}
}

func executable(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
