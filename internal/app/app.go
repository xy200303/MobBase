// Package app contains the command boundary used by the terminal, VS Code and CI.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	gort "runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/xy200303/MobBase/internal/app/ui"
	"github.com/xy200303/MobBase/internal/home"
	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/platform/ios"
	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/state"
)

const schemaVersion = "1.0"

var helpOptionPattern = regexp.MustCompile(`--[A-Za-z][A-Za-z0-9-]*(?:=[^\s\]\)]+)?`)

type codedError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

func (e *codedError) Error() string { return e.Message }

type event struct {
	SchemaVersion string      `json:"schemaVersion"`
	Event         string      `json:"event"`
	Command       string      `json:"command"`
	Sequence      int         `json:"sequence"`
	OK            bool        `json:"ok"`
	Data          interface{} `json:"data,omitempty"`
	Error         *codedError `json:"error,omitempty"`
}

type eventStream struct {
	sequence int
	mu       sync.Mutex
}

type runtime struct {
	home      string
	store     state.Store
	json      bool
	eventMode bool
	out       io.Writer
	err       io.Writer
	events    *eventStream
	terminal  *ui.UI
}

// Run is deliberately side-effect free for list, status, doctor and help.
// Commands that register an SDK persist only references, never SDK contents.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	args, jsonOutput, eventMode := takeOutput(args)
	if isVersionCommand(args) {
		run := runtime{json: jsonOutput, eventMode: eventMode, out: stdout, err: stderr, events: &eventStream{}, terminal: ui.New(stdout, stderr)}
		return run.version()
	}
	homePath, err := home.Resolve()
	if err != nil {
		return writeFailure(runtime{json: jsonOutput, eventMode: eventMode, out: stdout, err: stderr, events: &eventStream{}}, "mob", err)
	}
	run := runtime{home: homePath, store: state.New(homePath), json: jsonOutput, eventMode: eventMode, out: stdout, err: stderr, events: &eventStream{}, terminal: ui.New(stdout, stderr)}
	if err := run.execute(ctx, args); err != nil {
		return writeFailure(run, commandName(args), err)
	}
	return 0
}

func (r runtime) execute(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return r.help(args[1:])
	}
	if helpPath, requested := inlineHelpPath(args); requested {
		return r.help(helpPath)
	}
	switch args[0] {
	case "version", "--version", "-V":
		if len(args) != 1 {
			return invalidCommand(strings.Join(args, " "))
		}
		return r.versionResult()
	case "status":
		return r.status()
	case "doctor":
		return r.doctorAs(ctx, "mob doctor", args[1:])
	case "catalog":
		return r.catalog(ctx, args[1:])
	case "build":
		return r.build(ctx, args[1:])
	case "run":
		return r.run(ctx, args[1:])
	case "debug":
		return r.debug(ctx, args[1:])
	case "release":
		return r.release(ctx, args[1:])
	case "logs":
		return r.logs(ctx, args[1:])
	case "test":
		return r.test(ctx, args[1:])
	case "flutter":
		return r.flutter(ctx, args[1:])
	case "fvm":
		return r.fvm(ctx, args[1:])
	case "env":
		return r.env(args[1:])
	case "home":
		return r.homeCommand(args[1:])
	case "support":
		return r.support(args[1:])
	case "java":
		return r.java(ctx, args[1:])
	case "android":
		return r.android(ctx, args[1:])
	case "ios":
		return r.ios(ctx, args[1:])
	case "harmony":
		return r.reservedPlatform("harmony", args[1:])
	case "device":
		return r.device(ctx, args[1:])
	default:
		return invalidCommand(strings.Join(args, " "))
	}
}

func inlineHelpPath(args []string) ([]string, bool) {
	path := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return path, true
		}
		path = append(path, arg)
	}
	return nil, false
}

func isVersionCommand(args []string) bool {
	return len(args) == 1 && (args[0] == "version" || args[0] == "--version" || args[0] == "-V")
}

func (r runtime) version() int {
	if err := r.versionResult(); err != nil {
		return writeFailure(r, "mob version", err)
	}
	return 0
}

func (r runtime) versionResult() error {
	data := map[string]string{"version": cliVersion}
	if r.json {
		return r.result("mob version", data)
	}
	fmt.Fprintf(r.out, "mob %s\n", cliVersion)
	return nil
}

// reservedPlatform keeps the cross-platform CLI surface explicit while Android
// is the only implemented toolchain. It lets the VS Code extension distinguish
// an unsupported host from a platform adapter that has not been shipped yet.
func (r runtime) reservedPlatform(platform string, args []string) error {
	command := "mob " + platform
	if len(args) > 0 {
		command += " " + strings.Join(args, " ")
	}
	if platform == "ios" && gort.GOOS != "darwin" {
		return &codedError{Code: "MOB_HOST_UNSUPPORTED", Message: command + " requires macOS with Xcode.", Remediation: "Run this command on macOS after installing and accepting the Xcode license."}
	}
	return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: command + " is reserved but not implemented in this Mob release.", Remediation: "Use the platform's official tooling for now; current Mob toolchain management is available for Android."}
}

func (r runtime) homeCommand(args []string) error {
	if len(args) == 1 && args[0] == "show" {
		selected, err := home.Selected()
		if err != nil {
			return err
		}
		data := map[string]string{"path": r.home, "selectedPath": selected, "environmentOverride": strings.TrimSpace(os.Getenv(home.EnvironmentVariable))}
		if r.json {
			return r.result("mob home show", data)
		}
		fmt.Fprintln(r.out, r.home)
		return nil
	}
	if len(args) != 2 || args[0] != "set" || strings.TrimSpace(args[1]) == "" {
		return invalidCommand("mob home " + strings.Join(args, " "))
	}
	if override := strings.TrimSpace(os.Getenv(home.EnvironmentVariable)); override != "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "MOB_HOME is set and takes precedence over mob home set.", Remediation: "Update or remove MOB_HOME in the calling environment, then rerun mob home set."}
	}
	destination, err := filepath.Abs(filepath.Clean(args[1]))
	if err != nil {
		return err
	}
	if err := home.Migrate(r.home, destination); err != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Move Mob home: " + err.Error(), Remediation: "Choose an empty destination directory on a writable drive, then retry."}
	}
	if err := home.Select(destination); err != nil {
		return err
	}
	data := map[string]string{"previousPath": r.home, "path": destination}
	if r.json {
		return r.result("mob home set", data)
	}
	fmt.Fprintf(r.out, "Mob home moved to %s.\n", destination)
	return nil
}

func (r runtime) device(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return invalidCommand("mob device")
	}
	switch args[0] {
	case "list":
		return r.deviceList(ctx, args)
	case "open":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Device ID is required.", Remediation: "Run mob device list, then use mob device open android:<native-id>."}
		}
		return r.deviceOpen(ctx, args[1])
	case "mirror":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Device ID is required.", Remediation: "Run mob device list, then use mob device mirror android:<native-id>."}
		}
		return r.deviceMirror(ctx, args[1])
	case "screenshot":
		return r.deviceScreenshot(ctx, args[1:])
	case "ui-tree":
		return r.deviceUITree(ctx, args[1:])
	case "wait":
		return r.deviceWait(ctx, args[1:])
	case "input":
		return r.deviceInput(ctx, args[1:])
	case "record":
		return r.deviceRecord(ctx, args[1:])
	case "forward":
		return r.deviceForward(ctx, args[1:])
	case "use":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Device ID is required.", Remediation: "Run mob device list, then use mob device use <platform:native-id>."}
		}
		return r.deviceUse(ctx, args[1])
	default:
		return invalidCommand("mob device " + strings.Join(args, " "))
	}
}

type deviceForwardRemoveOptions struct {
	DeviceID string
	Port     int
}

func (r runtime) deviceForward(ctx context.Context, args []string) error {
	options, err := parseDeviceForwardRemove(args)
	if err != nil {
		return err
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	if err := android.RemoveJDWPForward(ctx, sdks, strings.TrimPrefix(options.DeviceID, "android:"), options.Port); err != nil {
		return androidCommandError(err, "Ensure Android platform-tools is installed and the selected ADB device is still connected.")
	}
	data := map[string]interface{}{"device": options.DeviceID, "port": options.Port}
	if r.json {
		return r.result("mob device forward remove", data)
	}
	fmt.Fprintf(r.out, "Removed Android JDWP forward at 127.0.0.1:%d for %s.\n", options.Port, options.DeviceID)
	return nil
}

func parseDeviceForwardRemove(args []string) (deviceForwardRemoveOptions, error) {
	if len(args) != 4 || args[0] != "remove" || !strings.HasPrefix(args[1], "android:") || strings.TrimSpace(strings.TrimPrefix(args[1], "android:")) == "" || args[2] != "--port" {
		return deviceForwardRemoveOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob device forward remove android:<native-id> --port <1-65535>."}
	}
	port, err := strconv.Atoi(args[3])
	if err != nil || port < 1 || port > 65535 {
		return deviceForwardRemoveOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--port must be an integer between 1 and 65535."}
	}
	return deviceForwardRemoveOptions{DeviceID: args[1], Port: port}, nil
}

func (r runtime) deviceOpen(ctx context.Context, id string) error {
	if !strings.HasPrefix(id, "android:") {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "Only connected Android devices can be opened in this Mob release."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	devices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return androidCommandError(err, "Ensure Android platform-tools is installed.")
	}
	device, err := findReadyAndroidDevice(devices, id, true)
	if err != nil {
		return err
	}
	mode := "wake"
	if device.Kind == "physical" {
		if _, err := r.startAndroidMirror(ctx, device.NativeID, "mob device open"); err != nil {
			return err
		}
		mode = "mirror"
	} else if err := android.WakeDevice(ctx, sdks, device.NativeID); err != nil {
		return androidCommandError(err, "Verify the selected emulator remains ready and has ADB access.")
	}
	data := map[string]interface{}{"device": device, "mode": mode}
	if r.json {
		return r.result("mob device open", data)
	}
	if mode == "mirror" {
		fmt.Fprintf(r.out, "Opened Android device mirror for %s.\n", device.ID)
	} else {
		fmt.Fprintf(r.out, "Woke Android emulator %s in its official Emulator window.\n", device.ID)
	}
	return nil
}

func (r runtime) deviceMirror(ctx context.Context, id string) error {
	if !strings.HasPrefix(id, "android:") {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "Only connected Android devices can be mirrored in this Mob release."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	devices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return androidCommandError(err, "Ensure Android platform-tools is installed.")
	}
	device, err := findReadyAndroidDevice(devices, id, true)
	if err != nil {
		return err
	}
	if device.Kind != "physical" {
		return &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: "Device mirroring is intended for connected physical Android devices.", Remediation: "Use the official Emulator window for an Android virtual device."}
	}
	if _, err := r.startAndroidMirror(ctx, device.NativeID, "mob device mirror"); err != nil {
		return err
	}
	data := map[string]interface{}{"device": device, "client": "scrcpy"}
	if r.json {
		return r.result("mob device mirror", data)
	}
	fmt.Fprintf(r.out, "Started Android device mirror for %s.\n", device.ID)
	return nil
}

func (r runtime) deviceScreenshot(ctx context.Context, args []string) error {
	deviceID, output := "", ""
	for len(args) > 0 {
		switch args[0] {
		case "--output", "--out":
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: args[0] + " requires a PNG path."}
			}
			output, args = args[1], args[2:]
		default:
			if deviceID != "" || !strings.HasPrefix(args[0], "android:") {
				return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob device screenshot [android:<native-id>] [--output|--out <path>]."}
			}
			deviceID, args = args[0], args[1:]
		}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	devices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return androidCommandError(err, "Ensure Android platform-tools is installed.")
	}
	device, err := selectAndroidRunDevice(devices, deviceID, config.Device.DefaultID)
	if err != nil {
		return err
	}
	if output == "" {
		output = "mob-" + strings.ReplaceAll(device.NativeID, ":", "-") + ".png"
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := android.ScreenshotDevice(ctx, sdks, device.NativeID, output); err != nil {
		return androidCommandError(err, "Verify the selected device remains ready and has ADB access.")
	}
	data := map[string]interface{}{"device": device, "path": output}
	if r.json {
		return r.result("mob device screenshot", data)
	}
	fmt.Fprintf(r.out, "Saved screenshot: %s\n", output)
	return nil
}

func (r runtime) deviceRecord(ctx context.Context, args []string) error {
	if len(args) < 1 || !strings.HasPrefix(args[0], "android:") {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob device record android:<native-id> [--output <path>] [--seconds <1-180>]."}
	}
	output, seconds := "", 30
	for rest := args[1:]; len(rest) > 0; {
		if len(rest) < 2 {
			return invalidCommand("mob device record")
		}
		if rest[0] == "--output" {
			output = rest[1]
		} else if rest[0] == "--seconds" {
			value, err := strconv.Atoi(rest[1])
			if err != nil || value < 1 || value > 180 {
				return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--seconds must be between 1 and 180."}
			}
			seconds = value
		} else {
			return invalidCommand("mob device record")
		}
		rest = rest[2:]
	}
	if output == "" {
		output = "mob-" + strings.ReplaceAll(strings.TrimPrefix(args[0], "android:"), ":", "-") + ".mp4"
	}
	output, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	devices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return androidCommandError(err, "Ensure Android platform-tools is installed.")
	}
	device, err := findReadyAndroidDevice(devices, args[0], true)
	if err != nil {
		return err
	}
	if err := android.RecordDevice(ctx, sdks, device.NativeID, output, seconds); err != nil {
		return androidCommandError(err, "Verify the selected device remains ready and supports screen recording.")
	}
	data := map[string]interface{}{"device": device, "path": output, "seconds": seconds}
	if r.json {
		return r.result("mob device record", data)
	}
	fmt.Fprintf(r.out, "Saved recording: %s\n", output)
	return nil
}

func (r runtime) status() error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	currentProject, err := project.Detect("")
	if err != nil {
		return err
	}
	data := struct {
		Home       string          `json:"home"`
		AndroidSDK []android.SDK   `json:"androidSdks"`
		Java       []state.JavaSDK `json:"javaSdks"`
		Flutter    state.Flutter   `json:"flutter"`
		Device     state.Device    `json:"device"`
		Project    *project.Info   `json:"project,omitempty"`
	}{Home: r.home, AndroidSDK: sdks, Flutter: config.Flutter, Device: config.Device, Project: currentProject}
	javaSDKs, javaErr := discoverJava(context.Background(), config)
	if javaErr == nil {
		data.Java = javaSDKs
	}
	if r.json {
		return r.result("mob status", data)
	}
	fmt.Fprintf(r.out, "Mob home: %s\nAndroid SDKs: %d\nJDKs: %d\nFlutter SDKs: %d\nDefault device: %s\n", r.home, len(sdks), len(data.Java), len(config.Flutter.SDKs), config.Device.DefaultID)
	if currentProject != nil {
		fmt.Fprintf(r.out, "Project: %s (%s)\n", currentProject.Root, currentProject.Kind)
	}
	for _, sdk := range sdks {
		fmt.Fprintf(r.out, "  %s  [%s]  %s\n", sdk.Name, sdk.Ownership, sdk.Path)
	}
	return nil
}

func (r runtime) doctorAs(ctx context.Context, command string, args []string) error {
	fix, licenses := false, false
	for len(args) > 0 {
		switch args[0] {
		case "--fix":
			fix = true
			args = args[1:]
		case "--accept-licenses":
			licenses = true
			args = args[1:]
		case "--platform":
			if len(args) != 2 || args[1] != "android" {
				return invalidCommand(command + " --platform")
			}
			args = nil
		default:
			return invalidCommand(command + " " + strings.Join(args, " "))
		}
	}
	if fix {
		current, err := project.Detect("")
		if err != nil {
			return err
		}
		if current == nil || !targetDeclared(current, "android") {
			return &codedError{Code: "MOB_PROJECT_UNRECOGNIZED", Message: "mob doctor --fix requires a project that declares an Android target."}
		}
		if _, requirements, err := r.prepareAndroidSDK(ctx, current.Root, command, false, false, licenses); err != nil {
			return err
		} else if _, err := r.selectProjectJava(ctx, requirements.JavaVersion, false); err != nil {
			return err
		}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	currentProject, err := project.Detect("")
	if err != nil {
		return err
	}
	javaSDKs, javaErr := discoverJava(context.Background(), config)
	hasJava := javaErr == nil && len(javaSDKs) > 0
	checks := []check{
		{ID: "android-sdk", Label: "Android SDK", Status: status(len(sdks) > 0), Required: true, Detail: "No Android SDK was found.", Fix: "Run mob android sdk add <name> --path <sdk-root>, or run mob build in an Android project to prepare android:managed."},
		{ID: "java", Label: "JDK", Status: status(hasJava), Required: true, Detail: "java was not found on PATH.", Fix: "Install a supported JDK, then run mob java list."},
	}
	if hasJava {
		selected := javaSDKs[0]
		for _, sdk := range javaSDKs {
			if sdk.Name == config.Java.CurrentSDK {
				selected = sdk
				break
			}
		}
		checks[1].Detail = fmt.Sprintf("%s (Java %d)", selected.Path, selected.Version)
		checks[1].Fix = ""
	}
	if len(sdks) > 0 {
		selected, found := selectAndroidBuildSDK(sdks)
		if !found {
			selected = sdks[0]
		}
		checks[0].Detail = selected.Path
		checks[0].Fix = ""
		checks = append(checks, androidDeviceToolChecks(selected)...)
	}
	projectReady := true
	if currentProject != nil && (currentProject.Kind == project.KindAndroid || currentProject.Kind == project.KindFlutter || currentProject.Kind == project.KindKotlinMultiplatform) && targetDeclared(currentProject, "android") {
		requirements, requirementsErr := project.AndroidRequirementsFor(currentProject.Root)
		matching := requirementsErr == nil
		if matching {
			_, matching = matchingAndroidSDK(sdks, requirements, false)
		}
		detail := androidRequirementDetail(requirements, requirementsErr)
		fix := ""
		if !matching {
			fix = "Run mob build --accept-licenses to prepare android:managed, or install the declared Android components."
			for index := range checks {
				if checks[index].ID == "android-sdk" {
					checks[index].Status = "missing"
					checks[index].Detail = "No installed SDK satisfies project requirements: " + detail
					checks[index].Fix = fix
					break
				}
			}
		}
		checks = append(checks, check{ID: "android-project-requirements", Label: "Android project requirements", Status: status(matching), Required: true, Detail: detail, Fix: fix})
		projectReady = matching
	}
	if currentProject != nil && (currentProject.Kind == project.KindAndroid || currentProject.Kind == project.KindKotlinMultiplatform) {
		_, _, wrapperErr := buildCommand(currentProject.Root, nil)
		detail, fix := "Gradle Wrapper is available.", ""
		if wrapperErr != nil {
			detail = wrapperErr.Error()
			fix = "Restore gradlew/gradlew.bat, or regenerate the project wrapper with the official Gradle command."
		}
		checks = append(checks, check{ID: "gradle-wrapper", Label: "Gradle Wrapper", Status: status(wrapperErr == nil), Required: true, Detail: detail, Fix: fix})
		projectReady = projectReady && wrapperErr == nil
	}
	if currentProject != nil && currentProject.Kind == project.KindFlutter {
		runner, runnerErr := flutterRunner(currentProject.Root)
		detail := "available"
		fix := ""
		if runnerErr != nil {
			detail = runnerErr.Error()
			fix = "Install the required Flutter or FVM launcher, then rerun mob doctor."
		} else {
			detail = runner.Program
		}
		checks = append(checks, check{ID: "flutter-runner", Label: "Flutter runner", Status: status(runnerErr == nil), Required: true, Detail: detail, Fix: fix})
		projectReady = projectReady && runnerErr == nil
	}
	ready := hasJava && len(sdks) > 0 && projectReady
	data := struct {
		Platform string          `json:"platform"`
		Ready    bool            `json:"ready"`
		Checks   []check         `json:"checks"`
		SDKs     []android.SDK   `json:"sdks"`
		JavaSDKs []state.JavaSDK `json:"javaSdks"`
	}{Platform: "android", Ready: ready, Checks: checks, SDKs: sdks, JavaSDKs: javaSDKs}
	if r.json {
		return r.result(command, data)
	}
	for _, item := range checks {
		fmt.Fprintf(r.out, "%s: %s\n", item.Label, item.Status)
		if item.Fix != "" {
			fmt.Fprintf(r.out, "  %s\n", item.Fix)
		}
	}
	return nil
}

func androidDeviceToolChecks(sdk android.SDK) []check {
	adb := check{
		ID:       "android-adb",
		Label:    "Android Debug Bridge",
		Status:   status(sdk.Components.PlatformTools),
		Required: false,
		Detail:   "platform-tools is not installed.",
		Fix:      "Run mob android sdk install " + sdk.Name + " --package platform-tools --accept-licenses.",
	}
	if sdk.Components.PlatformTools {
		adb.Detail = "platform-tools is available."
		adb.Fix = ""
	}
	emulator := check{
		ID:       "android-emulator",
		Label:    "Android Emulator",
		Status:   status(sdk.Components.Emulator),
		Required: false,
		Detail:   "Emulator is not installed; connect a physical device or install the official Emulator package.",
		Fix:      "Run mob android sdk install " + sdk.Name + " --package emulator --accept-licenses.",
	}
	if sdk.Components.Emulator {
		emulator.Detail = "Emulator package is available."
		emulator.Fix = ""
	}
	return []check{adb, emulator}
}

func androidRequirementDetail(requirements project.AndroidRequirements, err error) string {
	if err != nil {
		return err.Error()
	}
	values := make([]string, 0, 3)
	if requirements.CompileSDK > 0 {
		values = append(values, fmt.Sprintf("compileSdk=%d", requirements.CompileSDK))
	}
	if requirements.BuildTools != "" {
		values = append(values, "buildTools="+requirements.BuildTools)
	}
	if requirements.NDKVersion != "" {
		values = append(values, "ndk="+requirements.NDKVersion)
	}
	if len(values) == 0 {
		return "No explicit numeric compileSdk, buildToolsVersion, or ndkVersion found."
	}
	return strings.Join(values, ", ")
}

type check struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Detail   string `json:"detail,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

func status(ok bool) string {
	if ok {
		return "ok"
	}
	return "missing"
}

func (r runtime) android(ctx context.Context, args []string) error {
	if len(args) >= 1 && args[0] == "proxy" {
		return r.androidProxy(args[1:])
	}
	if len(args) == 1 && args[0] == "doctor" {
		return r.doctorAs(ctx, "mob android doctor", []string{"--platform", "android"})
	}
	if len(args) >= 1 && args[0] == "create" {
		return r.androidCreate(ctx, args[1:])
	}
	if len(args) >= 1 && args[0] == "device" {
		return r.androidDevice(ctx, args[1:])
	}
	if len(args) >= 1 && args[0] == "emulator" {
		return r.androidEmulator(ctx, args[1:])
	}
	if len(args) >= 1 && args[0] == "ndk" {
		return r.androidNDK(ctx, args[1:])
	}
	if len(args) < 2 || args[0] != "sdk" {
		return invalidCommand("mob android " + strings.Join(args, " "))
	}
	switch args[1] {
	case "list":
		if len(args) != 2 {
			return invalidCommand("mob android sdk list")
		}
		return r.sdkList()
	case "available":
		return r.sdkAvailable(ctx, args[2:])
	case "add":
		return r.sdkRegister(args[2:], "add")
	case "import":
		return r.sdkRegister(args[2:], "import")
	case "inspect":
		return r.sdkInspect(args[2:])
	case "use":
		return r.sdkUse(args[2:])
	case "install":
		return r.sdkInstall(ctx, args[2:])
	case "remove":
		return r.sdkRemove(args[2:])
	default:
		return invalidCommand("mob android sdk " + strings.Join(args[1:], " "))
	}
}

func (r runtime) androidDevice(ctx context.Context, args []string) error {
	if len(args) == 2 && args[0] == "connect" {
		return r.androidDeviceConnect(ctx, args[1])
	}
	if len(args) == 4 && args[0] == "pair" && args[2] == "--code" {
		if !validAndroidPairingCode(args[3]) {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--code must contain exactly 6 digits."}
		}
		return r.androidDevicePair(ctx, args[1], args[3])
	}
	return invalidCommand("mob android device " + strings.Join(args, " "))
}

func validAndroidPairingCode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func (r runtime) androidProxy(args []string) error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "show" {
		data := map[string]string{"url": config.Android.ProxyURL}
		if r.json {
			return r.result("mob android proxy show", data)
		}
		fmt.Fprintln(r.out, config.Android.ProxyURL)
		return nil
	}
	if len(args) == 2 && args[0] == "set" && validProxyURL(args[1]) {
		config.Android.ProxyURL = args[1]
		if err := r.store.Save(config); err != nil {
			return err
		}
		if r.json {
			return r.result("mob android proxy set", map[string]string{"url": args[1]})
		}
		fmt.Fprintln(r.out, "Android download proxy updated.")
		return nil
	}
	if len(args) == 1 && args[0] == "clear" {
		config.Android.ProxyURL = ""
		if err := r.store.Save(config); err != nil {
			return err
		}
		if r.json {
			return r.result("mob android proxy clear", map[string]string{})
		}
		fmt.Fprintln(r.out, "Android download proxy cleared.")
		return nil
	}
	return invalidCommand("mob android proxy " + strings.Join(args, " "))
}

func validProxyURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.User == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func androidProxyEnvironment(config state.Config) []string {
	if config.Android.ProxyURL == "" {
		return nil
	}
	return []string{"HTTP_PROXY=" + config.Android.ProxyURL, "HTTPS_PROXY=" + config.Android.ProxyURL}
}

func (r runtime) androidEmulator(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return invalidCommand("mob android emulator")
	}
	switch args[0] {
	case "image":
		return r.emulatorImage(ctx, args[1:])
	case "list":
		if len(args) != 1 {
			return invalidCommand("mob android emulator list")
		}
		return r.emulatorList(ctx)
	case "create":
		return r.emulatorCreate(ctx, args[1:])
	case "start":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Android virtual device name is required.", Remediation: "Run mob android emulator list, then use mob android emulator start <avd-name>."}
		}
		return r.emulatorStart(ctx, args[1])
	case "stop":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Android emulator device ID is required.", Remediation: "Run mob device list, then use mob android emulator stop android:emulator-5554."}
		}
		return r.emulatorStop(ctx, args[1])
	default:
		return invalidCommand("mob android emulator " + strings.Join(args, " "))
	}
}

func (r runtime) emulatorImage(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return invalidCommand("mob android emulator image")
	}
	switch args[0] {
	case "available":
		return r.emulatorImageAvailable(ctx, args[1:])
	case "install":
		return r.emulatorImageInstall(ctx, args[1:])
	default:
		return invalidCommand("mob android emulator image " + strings.Join(args, " "))
	}
}

type emulatorCreateOptions struct {
	Name  string
	Image string
	SDK   string
}

func (r runtime) emulatorCreate(ctx context.Context, args []string) error {
	options, err := parseEmulatorCreate(args)
	if err != nil {
		return err
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	sdk, found := selectEmulatorSDK(sdks, options.SDK)
	if !found {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No Android SDK is available for emulator creation.", Remediation: "Run mob android sdk list, then add an SDK containing command-line tools, Emulator, and a system image."}
	}
	image := options.Image
	if image == "" {
		var available bool
		image, available = android.DefaultSystemImage(sdk)
		if !available {
			return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No Android system image is installed in SDK " + sdk.Name + ".", Remediation: "Install a system image, then rerun mob android emulator create."}
		}
	}
	name := options.Name
	if name == "" {
		name = android.DefaultEmulatorName(image)
	}
	if err := r.emit("started", "mob android emulator create", true, map[string]string{"phase": "create", "sdk": sdk.Name, "image": image, "avd": name}, nil); err != nil {
		return err
	}
	if err := android.CreateEmulator(ctx, sdk, name, image); err != nil {
		return androidCommandError(err, "Verify that the selected SDK contains the Emulator, command-line tools, and requested system image.")
	}
	data := map[string]string{"sdk": sdk.Name, "image": image, "avd": name}
	if r.json {
		return r.result("mob android emulator create", data)
	}
	fmt.Fprintf(r.out, "Created Android virtual device %s.\n", name)
	return nil
}

func parseEmulatorCreate(args []string) (emulatorCreateOptions, error) {
	options := emulatorCreateOptions{}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		options.Name = args[0]
		args = args[1:]
	}
	for len(args) > 0 {
		switch args[0] {
		case "--image":
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				return emulatorCreateOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--image requires an installed Android system image package ID."}
			}
			options.Image = args[1]
			args = args[2:]
		case "--sdk":
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				return emulatorCreateOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--sdk requires an Android SDK name."}
			}
			options.SDK = args[1]
			args = args[2:]
		default:
			return emulatorCreateOptions{}, invalidCommand("mob android emulator create " + strings.Join(args, " "))
		}
	}
	return options, nil
}

func selectEmulatorSDK(sdks []android.SDK, name string) (android.SDK, bool) {
	if name != "" {
		for _, sdk := range sdks {
			if sdk.Name == name {
				return sdk, true
			}
		}
		return android.SDK{}, false
	}
	return selectAndroidBuildSDK(sdks)
}

func (r runtime) emulatorList(ctx context.Context) error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	emulators, err := android.ListEmulators(ctx, sdks)
	if err != nil {
		return androidCommandError(err, "Run mob android sdk inspect <name> and ensure the Emulator package is installed.")
	}
	if r.json {
		return r.result("mob android emulator list", map[string]interface{}{"emulators": emulators})
	}
	if len(emulators) == 0 {
		fmt.Fprintln(r.out, "No Android virtual devices found.")
		return nil
	}
	for _, emulator := range emulators {
		fmt.Fprintln(r.out, emulator.Name)
	}
	return nil
}

func (r runtime) emulatorStart(ctx context.Context, avd string) error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	if err := r.emit("started", "mob android emulator start", true, map[string]string{"phase": "start", "avd": avd}, nil); err != nil {
		return err
	}
	if err := android.StartEmulator(ctx, sdks, avd); err != nil {
		return androidCommandError(err, "Run mob android emulator list and verify that the Emulator package is installed.")
	}
	data := map[string]string{"avd": avd}
	if r.json {
		return r.result("mob android emulator start", data)
	}
	fmt.Fprintf(r.out, "Started Android virtual device %s.\n", avd)
	return nil
}

func (r runtime) emulatorStop(ctx context.Context, id string) error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	if err := android.StopEmulator(ctx, sdks, id); err != nil {
		return androidCommandError(err, "Run mob device list and ensure platform-tools is installed.")
	}
	data := map[string]string{"device": id}
	if r.json {
		return r.result("mob android emulator stop", data)
	}
	fmt.Fprintf(r.out, "Stopped Android emulator %s.\n", id)
	return nil
}

func (r runtime) androidNDK(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return invalidCommand("mob android ndk")
	}
	switch args[0] {
	case "list":
		return r.ndkList(args[1:])
	case "available":
		return r.ndkAvailable(ctx, args[1:])
	case "install":
		return r.ndkInstall(ctx, args[1:])
	case "remove":
		return r.ndkRemove(args[1:])
	default:
		return invalidCommand("mob android ndk " + strings.Join(args, " "))
	}
}

type sdkInstallOptions struct {
	Name               string
	Packages           []string
	API                int
	AllowExternalWrite bool
	AcceptLicenses     bool
	Yes                bool
}

func (r runtime) sdkInstall(ctx context.Context, args []string) error {
	return r.sdkInstallAs(ctx, args, "mob android sdk install")
}

func (r runtime) sdkInstallAs(ctx context.Context, args []string, command string) error {
	options, err := parseSDKInstall(args)
	if err != nil {
		return err
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	target, err := r.installTarget(options.Name, sdks)
	if err != nil {
		return err
	}
	if target.Ownership != state.OwnershipManaged && (!options.AllowExternalWrite || !options.Yes) {
		return &codedError{Code: "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", Message: "Android SDK " + target.Name + " is not Mob-managed.", Remediation: "Review the target path, then repeat with --allow-external-write --yes."}
	}
	if !options.AcceptLicenses {
		return &codedError{Code: "MOB_LICENSE_REQUIRED", Message: "Android SDK licenses must be accepted before installation.", Remediation: "Review the Android SDK license, then repeat with --accept-licenses."}
	}
	packages := append([]string(nil), options.Packages...)
	if options.API > 0 {
		packages = append(packages, fmt.Sprintf("platforms;android-%d", options.API))
	}
	catalog, err := r.validateAndroidPackages(ctx, packages, false, target.Path)
	if err != nil {
		return err
	}
	if err := r.emit("started", command, true, map[string]interface{}{"phase": "install", "tool": "android-sdkmanager", "sdk": target.Name, "packages": packages}, nil); err != nil {
		return err
	}
	if err := r.bootstrapManagedSDK(ctx, target, catalog, command); err != nil {
		return err
	}
	result, err := android.InstallPackages(ctx, android.InstallRequest{Root: target.Path, Packages: packages, AcceptLicenses: true, Environment: androidProxyEnvironment(config), Output: r.sdkManagerOutput()})
	if err != nil {
		if strings.Contains(err.Error(), "command-line tools were not found") {
			return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: err.Error(), Remediation: "Install or import Android command-line tools first. Mob-managed command-line-tools bootstrap will be available through the verified catalog installer."}
		}
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Check your selected Android source, network access, and SDK licenses."}
	}
	if target.Ownership == state.OwnershipManaged {
		r.registerManagedSDK(&config, target)
	}
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]interface{}{"sdk": target, "installation": result}
	if r.json {
		return r.result(command, data)
	}
	fmt.Fprintf(r.out, "Installed %s in Android SDK %s.\n", strings.Join(result.Packages, ", "), target.Name)
	return nil
}

func (r runtime) installTarget(name string, sdks []android.SDK) (android.SDK, error) {
	if name == "managed" {
		for _, sdk := range sdks {
			if sdk.Name == name {
				return sdk, nil
			}
		}
		return android.SDK{Name: "managed", Path: filepath.Join(r.home, "toolchains", "android", "managed", "sdk"), Ownership: state.OwnershipManaged}, nil
	}
	for _, sdk := range sdks {
		if sdk.Name == name {
			return sdk, nil
		}
	}
	return android.SDK{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Android SDK " + name + " is not available.", Remediation: "Run mob android sdk list, or import an existing SDK."}
}

func (r runtime) registerManagedSDK(config *state.Config, sdk android.SDK) {
	for index := range config.Android.SDKs {
		if config.Android.SDKs[index].Name == sdk.Name {
			config.Android.SDKs[index].Path = sdk.Path
			return
		}
	}
	config.Android.SDKs = append(config.Android.SDKs, state.AndroidSDK{Name: sdk.Name, Path: sdk.Path, Ownership: state.OwnershipManaged})
}

func parseSDKInstall(args []string) (sdkInstallOptions, error) {
	if len(args) == 0 {
		return sdkInstallOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Android SDK name is required.", Remediation: "Use mob android sdk install <name> --package <id> or --api <level>."}
	}
	options := sdkInstallOptions{Name: args[0]}
	for args = args[1:]; len(args) > 0; {
		switch args[0] {
		case "--package":
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				return sdkInstallOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--package requires a package ID."}
			}
			options.Packages = append(options.Packages, args[1])
			args = args[2:]
		case "--api":
			if len(args) < 2 {
				return sdkInstallOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--api requires an API level."}
			}
			api, err := strconv.Atoi(args[1])
			if err != nil || api < 1 {
				return sdkInstallOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--api must be a positive integer."}
			}
			options.API = api
			args = args[2:]
		case "--allow-external-write":
			options.AllowExternalWrite = true
			args = args[1:]
		case "--accept-licenses":
			options.AcceptLicenses = true
			args = args[1:]
		case "--yes":
			options.Yes = true
			args = args[1:]
		default:
			return sdkInstallOptions{}, invalidCommand("mob android sdk install " + strings.Join(args, " "))
		}
	}
	if len(options.Packages) == 0 && options.API == 0 {
		return sdkInstallOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "At least one --package or --api option is required."}
	}
	return options, nil
}

type ndkInstallOptions struct {
	Version            string
	SDKName            string
	AllowExternalWrite bool
	AcceptLicenses     bool
	Yes                bool
}

func (r runtime) ndkList(args []string) error {
	sdkName, err := optionalSDKName(args)
	if err != nil {
		return err
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	type ndkEntry struct {
		SDK      string   `json:"sdk"`
		Path     string   `json:"path"`
		Versions []string `json:"versions"`
	}
	entries := make([]ndkEntry, 0, len(sdks))
	for _, sdk := range sdks {
		if sdkName != "" && sdk.Name != sdkName {
			continue
		}
		entries = append(entries, ndkEntry{SDK: sdk.Name, Path: sdk.Path, Versions: sdk.Components.NDK})
	}
	if sdkName != "" && len(entries) == 0 {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Android SDK " + sdkName + " is not available.", Remediation: "Run mob android sdk list, then add or select an available SDK."}
	}
	if r.json {
		return r.result("mob android ndk list", map[string]interface{}{"ndks": entries})
	}
	if len(entries) == 0 {
		fmt.Fprintln(r.out, "No Android SDK found.")
		return nil
	}
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		versions := strings.Join(entry.Versions, ", ")
		if versions == "" {
			versions = "none"
		}
		rows = append(rows, []string{entry.SDK, versions})
	}
	if !r.terminal.Table([]string{"SDK", "VERSIONS"}, rows) {
		for _, row := range rows {
			fmt.Fprintf(r.out, "%s\t%s\n", row[0], row[1])
		}
	}
	return nil
}

func (r runtime) ndkInstall(ctx context.Context, args []string) error {
	options, err := parseNDKInstall(args)
	if err != nil {
		return err
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	target, err := r.installTarget(options.SDKName, sdks)
	if err != nil {
		return err
	}
	if target.Ownership != state.OwnershipManaged && (!options.AllowExternalWrite || !options.Yes) {
		return &codedError{Code: "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", Message: "Android SDK " + target.Name + " is not Mob-managed.", Remediation: "Review the target path, then repeat with --allow-external-write --yes."}
	}
	if !options.AcceptLicenses {
		return &codedError{Code: "MOB_LICENSE_REQUIRED", Message: "Android SDK licenses must be accepted before installation.", Remediation: "Review the Android SDK license, then repeat with --accept-licenses."}
	}
	packageID := "ndk;" + options.Version
	catalog, err := r.validateAndroidPackages(ctx, []string{packageID}, true, target.Path)
	if err != nil {
		return err
	}
	if err := r.emit("started", "mob android ndk install", true, map[string]interface{}{"phase": "install", "tool": "android-sdkmanager", "sdk": target.Name, "packages": []string{packageID}}, nil); err != nil {
		return err
	}
	if err := r.bootstrapManagedSDK(ctx, target, catalog, "mob android ndk install"); err != nil {
		return err
	}
	result, err := android.InstallPackages(ctx, android.InstallRequest{Root: target.Path, Packages: []string{packageID}, AcceptLicenses: true, Environment: androidProxyEnvironment(config), Output: r.sdkManagerOutput()})
	if err != nil {
		if strings.Contains(err.Error(), "command-line tools were not found") {
			return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: err.Error(), Remediation: "Install or add Android command-line tools first."}
		}
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Check the selected Android SDK, network access, and SDK licenses."}
	}
	if target.Ownership == state.OwnershipManaged {
		r.registerManagedSDK(&config, target)
	}
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]interface{}{"sdk": target, "installation": result}
	if r.json {
		return r.result("mob android ndk install", data)
	}
	fmt.Fprintf(r.out, "Installed Android NDK %s in SDK %s.\n", options.Version, target.Name)
	return nil
}

func (r runtime) ndkRemove(args []string) error {
	if len(args) != 4 || args[1] != "--sdk" || args[3] != "--yes" || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[2]) == "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Removing an Android NDK requires <version> --sdk <name> --yes.", Remediation: "Use mob android ndk list --sdk managed, then confirm mob android ndk remove <version> --sdk managed --yes."}
	}
	version, sdkName := args[0], args[2]
	if filepath.Base(version) != version || version == "." {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Android NDK version must not contain a path separator."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	var entry *state.AndroidSDK
	for index := range config.Android.SDKs {
		if config.Android.SDKs[index].Name == sdkName {
			entry = &config.Android.SDKs[index]
			break
		}
	}
	if entry == nil {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Android SDK " + sdkName + " is not registered with Mob.", Remediation: "Run mob android sdk list."}
	}
	managedRoot, err := filepath.Abs(filepath.Join(r.home, "toolchains", "android", "managed", "sdk"))
	if err != nil {
		return err
	}
	sdkPath, err := filepath.Abs(filepath.Clean(entry.Path))
	if err != nil {
		return err
	}
	if entry.Ownership != state.OwnershipManaged || !strings.EqualFold(sdkPath, managedRoot) {
		return &codedError{Code: "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", Message: "Only NDK versions inside the Mob-managed Android SDK can be removed.", Remediation: "Imported and discovered SDKs are references only."}
	}
	target := filepath.Join(sdkPath, "ndk", version)
	info, err := os.Stat(target)
	if os.IsNotExist(err) {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Android NDK " + version + " is not installed in SDK " + sdkName + "."}
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Android NDK path is not a directory."}
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove Mob-managed Android NDK: %w", err)
	}
	data := map[string]string{"sdk": sdkName, "version": version, "path": target}
	if r.json {
		return r.result("mob android ndk remove", data)
	}
	fmt.Fprintf(r.out, "Removed Android NDK %s from SDK %s.\n", version, sdkName)
	return nil
}

func (r runtime) validateAndroidPackages(ctx context.Context, packages []string, ndkOnly bool, sdkRoot string) (android.Catalog, error) {
	catalog, err := android.LoadCatalog(ctx, android.CatalogCachePath(r.home), false)
	if err != nil {
		return android.Catalog{}, &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Run mob android sdk available --refresh after restoring access to the Android official repository."}
	}
	items := catalog.SDKItems(0)
	if ndkOnly {
		items = catalog.NDKItems()
	}
	for _, packageID := range packages {
		if !android.ContainsPackage(items, packageID) {
			if !ndkOnly && strings.HasPrefix(packageID, "system-images;") {
				available, availableErr := android.HasAvailableSystemImage(ctx, sdkRoot, packageID)
				if availableErr == nil && available {
					continue
				}
			}
			return android.Catalog{}, &codedError{Code: "MOB_PACKAGE_NOT_AVAILABLE", Message: "Android package " + packageID + " is not in the current catalog.", Remediation: "Run mob android sdk available --refresh and choose a package ID returned by the catalog."}
		}
	}
	return catalog, nil
}

func (r runtime) bootstrapManagedSDK(ctx context.Context, target android.SDK, catalog android.Catalog, command string) error {
	if target.Ownership != state.OwnershipManaged {
		return nil
	}
	if _, found := android.SDKManager(target.Path); found {
		return nil
	}
	item, found := catalog.FindPackage("cmdline-tools;latest")
	if !found {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: "The Android catalog does not contain command-line tools.", Remediation: "Refresh the Android catalog and retry."}
	}
	if err := r.emit("progress", command, true, map[string]string{"phase": "bootstrap-command-line-tools", "sdk": target.Name}, nil); err != nil {
		return err
	}
	cacheDirectory := filepath.Join(r.home, "cache", "downloads")
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	var report func(android.DownloadProgress)
	if callback := r.download("Downloading Android command-line tools"); callback != nil {
		report = func(progress android.DownloadProgress) {
			callback(progress.Downloaded, progress.Total)
		}
	}
	if err := android.BootstrapCommandLineTools(ctx, target.Path, cacheDirectory, item, config.Android.ProxyURL, report); err != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Check Android repository access and retry; existing SDK installations are not modified."}
	}
	return nil
}

func parseNDKInstall(args []string) (ndkInstallOptions, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return ndkInstallOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Android NDK version is required.", Remediation: "Use mob android ndk install <version> --sdk <name> --accept-licenses."}
	}
	options := ndkInstallOptions{Version: args[0]}
	for args = args[1:]; len(args) > 0; {
		switch args[0] {
		case "--sdk":
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				return ndkInstallOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--sdk requires an Android SDK name."}
			}
			options.SDKName = args[1]
			args = args[2:]
		case "--allow-external-write":
			options.AllowExternalWrite = true
			args = args[1:]
		case "--accept-licenses":
			options.AcceptLicenses = true
			args = args[1:]
		case "--yes":
			options.Yes = true
			args = args[1:]
		default:
			return ndkInstallOptions{}, invalidCommand("mob android ndk install " + strings.Join(args, " "))
		}
	}
	if options.SDKName == "" {
		return ndkInstallOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--sdk is required for Android NDK installation.", Remediation: "Run mob android sdk list, then use --sdk <name>."}
	}
	return options, nil
}

func optionalSDKName(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) == 2 && args[0] == "--sdk" && strings.TrimSpace(args[1]) != "" {
		return args[1], nil
	}
	return "", invalidCommand("mob android ndk list " + strings.Join(args, " "))
}

func (r runtime) deviceList(ctx context.Context, args []string) error {
	platform, err := parseDeviceList(args)
	if err != nil {
		return err
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	devices, err := r.listDevices(ctx, platform)
	if err != nil {
		return err
	}
	if r.json {
		return r.result("mob device list", map[string]interface{}{"devices": devices, "defaultDevice": config.Device.DefaultID})
	}
	if len(devices) == 0 {
		fmt.Fprintln(r.out, "No mobile devices found.")
		return nil
	}
	rows := make([][]string, 0, len(devices))
	for _, device := range devices {
		marker := ""
		if device.ID == config.Device.DefaultID {
			marker = "*"
		}
		rows = append(rows, []string{marker, device.ID, device.Kind, device.State, device.Name})
	}
	if !r.terminal.Table([]string{"DEFAULT", "ID", "KIND", "STATE", "NAME"}, rows) {
		for _, row := range rows {
			marker := row[0]
			if marker == "" {
				marker = " "
			}
			fmt.Fprintf(r.out, "%s %s\t%s\t%s\t%s\n", marker, row[1], row[2], row[3], row[4])
		}
	}
	return nil
}

type listedDevice struct {
	ID       string `json:"id"`
	Platform string `json:"platform"`
	NativeID string `json:"nativeId"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	State    string `json:"state"`
}

func parseDeviceList(args []string) (string, error) {
	if len(args) == 1 && args[0] == "list" {
		return "", nil
	}
	if len(args) == 3 && args[0] == "list" && args[1] == "--platform" && (args[2] == "android" || args[2] == "ios") {
		return args[2], nil
	}
	return "", invalidCommand("mob device " + strings.Join(args, " "))
}

func (r runtime) listDevices(ctx context.Context, platform string) ([]listedDevice, error) {
	if platform == "ios" {
		return r.listIOSDevices(ctx)
	}
	androidDevices, androidErr := r.listAndroidDevices(ctx)
	if platform == "android" {
		return androidDevices, androidErr
	}

	devices := androidDevices
	iosAvailable := false
	if gort.GOOS == "darwin" {
		iosDevices, iosErr := r.listIOSDevices(ctx)
		if iosErr == nil {
			iosAvailable = true
			devices = append(devices, iosDevices...)
		}
	}
	if androidErr != nil && !iosAvailable {
		return nil, androidErr
	}
	return devices, nil
}

func (r runtime) listAndroidDevices(ctx context.Context) ([]listedDevice, error) {
	config, err := r.store.Load()
	if err != nil {
		return nil, err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return nil, err
	}
	devices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return nil, androidCommandError(err, "Run mob android sdk inspect <name> and ensure platform-tools is installed.")
	}
	result := make([]listedDevice, 0, len(devices))
	for _, device := range devices {
		result = append(result, listedDevice{ID: device.ID, Platform: device.Platform, NativeID: device.NativeID, Kind: device.Kind, Name: device.Name, State: device.State})
	}
	return result, nil
}

func (r runtime) listIOSDevices(ctx context.Context) ([]listedDevice, error) {
	if _, err := r.iosToolchain(ctx, "mob device list --platform ios"); err != nil {
		return nil, err
	}
	devices, err := ios.ListDevices(ctx)
	if err != nil {
		return nil, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: err.Error(), Remediation: "Install and select Xcode, then rerun mob ios doctor."}
	}
	result := make([]listedDevice, 0, len(devices))
	for _, device := range devices {
		result = append(result, listedDevice{ID: device.ID, Platform: device.Platform, NativeID: device.NativeID, Kind: device.Kind, Name: device.Name, State: device.State})
	}
	return result, nil
}

func (r runtime) deviceUse(ctx context.Context, id string) error {
	platform, _, found := strings.Cut(id, ":")
	if !found || platform == "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Device ID must use the form <platform>:<native-id>.", Remediation: "Run mob device list and copy an ID such as android:emulator-5554."}
	}
	if platform != "android" {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "Device platform " + platform + " is not available in this Mob installation.", Remediation: "Run mob device list --platform android."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	devices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return androidCommandError(err, "Run mob android sdk inspect <name> and ensure platform-tools is installed.")
	}
	for _, device := range devices {
		if device.ID != id {
			continue
		}
		if device.State != "ready" {
			return &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: "Device " + id + " is not ready.", Remediation: "Wait for the device to become ready, then run mob device use " + id + "."}
		}
		config.Device.DefaultID = id
		if err := r.store.Save(config); err != nil {
			return err
		}
		if r.json {
			return r.result("mob device use", map[string]string{"defaultDevice": id})
		}
		fmt.Fprintf(r.out, "Default device: %s\n", id)
		return nil
	}
	return &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: "Device " + id + " is not connected.", Remediation: "Run mob device list, connect or start the device, then try again."}
}

func (r runtime) androidDeviceConnect(ctx context.Context, address string) error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	output, err := android.ConnectDevice(ctx, sdks, address)
	if err != nil {
		return androidCommandError(err, "Check the host:port address, USB/Wi-Fi debugging authorization, and platform-tools.")
	}
	if r.json {
		return r.result("mob android device connect", map[string]interface{}{"address": address, "output": output})
	}
	fmt.Fprintln(r.out, output)
	return nil
}

func (r runtime) androidDevicePair(ctx context.Context, address, code string) error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	output, err := android.PairDevice(ctx, sdks, address, code)
	if err != nil {
		return androidCommandError(err, "Check the pairing address and six-digit code shown in Android Wireless debugging, then ensure platform-tools is installed.")
	}
	data := map[string]interface{}{"address": address, "output": output}
	if r.json {
		return r.result("mob android device pair", data)
	}
	fmt.Fprintln(r.out, output)
	return nil
}

func androidCommandError(err error, remediation string) error {
	if strings.Contains(err.Error(), "Debug Bridge was not found") || strings.Contains(err.Error(), "Android Emulator was not found") || strings.Contains(err.Error(), "AVD Manager was not found") || strings.Contains(err.Error(), "system image") {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: err.Error(), Remediation: remediation}
	}
	return &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: remediation}
}

func (r runtime) sdkList() error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	if r.json {
		return r.result("mob android sdk list", map[string]interface{}{"sdks": sdks})
	}
	if len(sdks) == 0 {
		fmt.Fprintln(r.out, "No Android SDK found.")
		return nil
	}
	rows := make([][]string, 0, len(sdks))
	for _, sdk := range sdks {
		rows = append(rows, []string{sdk.Name, string(sdk.Ownership), sdk.Path})
	}
	if !r.terminal.Table([]string{"NAME", "OWNERSHIP", "PATH"}, rows) {
		for _, row := range rows {
			fmt.Fprintf(r.out, "%s\t%s\t%s\n", row[0], row[1], row[2])
		}
	}
	return nil
}

func (r runtime) sdkRegister(args []string, verb string) error {
	path, name, err := sdkRegisterOptions(args, verb)
	if err != nil {
		return err
	}
	path, err = android.ValidateSDKRoot(path)
	if err != nil {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: err.Error(), Remediation: "Pass the root directory containing platforms, build-tools, platform-tools, cmdline-tools, emulator, or ndk."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	for _, entry := range config.Android.SDKs {
		entryPath, _ := filepath.Abs(entry.Path)
		if strings.EqualFold(filepath.Clean(entryPath), path) {
			data := map[string]interface{}{"sdk": entry, "alreadyRegistered": true}
			if r.json {
				return r.result("mob android sdk "+verb, data)
			}
			fmt.Fprintf(r.out, "Android SDK %s is already registered.\n", entry.Name)
			return nil
		}
	}
	if name == "" {
		name = generatedName(path, config.Android.SDKs)
	}
	if name == "managed" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "managed is reserved for the Mob-managed Android SDK.", Remediation: "Choose a different --name."}
	}
	if !validName(name) {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "SDK name must contain only letters, numbers, hyphens, and underscores.", Remediation: "Provide a name as the first argument, for example mob android sdk add shared --path <sdk-root>."}
	}
	for _, entry := range config.Android.SDKs {
		if entry.Name == name {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "An Android SDK named " + name + " already exists.", Remediation: "Choose another --name, or use mob android sdk inspect " + name + "."}
		}
	}
	entry := state.AndroidSDK{Name: name, Path: path, Ownership: state.OwnershipImported}
	config.Android.SDKs = append(config.Android.SDKs, entry)
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]interface{}{"sdk": entry, "alreadyRegistered": false}
	if r.json {
		return r.result("mob android sdk "+verb, data)
	}
	fmt.Fprintf(r.out, "Registered Android SDK %s at %s.\n", entry.Name, entry.Path)
	return nil
}

func sdkRegisterOptions(args []string, verb string) (string, string, error) {
	if verb != "add" {
		return importOptions(args)
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Android SDK name is required.", Remediation: "Use mob android sdk add <name> --path <sdk-root>."}
	}
	name := args[0]
	path, suppliedName, err := importOptions(args[1:])
	if err != nil {
		return "", "", err
	}
	if suppliedName != "" {
		return "", "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "mob android sdk add accepts the SDK name as its first argument.", Remediation: "Use mob android sdk add <name> --path <sdk-root>."}
	}
	return path, name, nil
}

func (r runtime) sdkInspect(args []string) error {
	if len(args) != 1 {
		return invalidCommand("mob android sdk inspect")
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	for _, sdk := range sdks {
		if sdk.Name != args[0] {
			continue
		}
		if r.json {
			return r.result("mob android sdk inspect", map[string]interface{}{"sdk": sdk})
		}
		fmt.Fprintf(r.out, "%s\nPath: %s\nOwnership: %s\n", sdk.Name, sdk.Path, sdk.Ownership)
		return nil
	}
	return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Android SDK " + args[0] + " is not available.", Remediation: "Run mob android sdk list, then import or select an available SDK."}
}

func (r runtime) sdkUse(args []string) error {
	if len(args) != 1 {
		return invalidCommand("mob android sdk use")
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	var chosen *android.SDK
	for i := range sdks {
		if sdks[i].Name == args[0] {
			chosen = &sdks[i]
			break
		}
	}
	if chosen == nil {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Android SDK " + args[0] + " is not available.", Remediation: "Run mob android sdk list."}
	}
	updated := false
	for i := range config.Android.SDKs {
		if config.Android.SDKs[i].Name == chosen.Name {
			config.Android.SDKs[i].Path = chosen.Path
			updated = true
			break
		}
	}
	if !updated {
		config.Android.SDKs = append(config.Android.SDKs, state.AndroidSDK{Name: chosen.Name, Path: chosen.Path, Ownership: chosen.Ownership})
	}
	config.Android.CurrentSDK = chosen.Name
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]interface{}{"currentSdk": chosen.Name, "path": chosen.Path}
	if r.json {
		return r.result("mob android sdk use", data)
	}
	fmt.Fprintf(r.out, "Current Android SDK: %s\n", chosen.Name)
	return nil
}

func (r runtime) sdkRemove(args []string) error {
	if len(args) != 2 || args[1] != "--yes" || strings.TrimSpace(args[0]) == "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Removing an Android SDK requires <name> and --yes.", Remediation: "Use mob android sdk remove managed --yes after verifying the SDK name with mob android sdk list."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	index := -1
	for current, entry := range config.Android.SDKs {
		if entry.Name == args[0] {
			index = current
			break
		}
	}
	if index < 0 {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Android SDK " + args[0] + " is not registered with Mob.", Remediation: "Run mob android sdk list."}
	}
	entry := config.Android.SDKs[index]
	managedRoot := filepath.Join(r.home, "toolchains", "android", "managed", "sdk")
	entryPath, err := filepath.Abs(filepath.Clean(entry.Path))
	if err != nil {
		return err
	}
	expectedPath, err := filepath.Abs(filepath.Clean(managedRoot))
	if err != nil {
		return err
	}
	if entry.Ownership != state.OwnershipManaged || !strings.EqualFold(entryPath, expectedPath) {
		return &codedError{Code: "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", Message: "Only the Mob-managed Android SDK can be removed.", Remediation: "Imported and discovered SDKs are references only and must be removed with their owning tool."}
	}
	if err := os.RemoveAll(entryPath); err != nil {
		return fmt.Errorf("remove Mob-managed Android SDK: %w", err)
	}
	config.Android.SDKs = append(config.Android.SDKs[:index], config.Android.SDKs[index+1:]...)
	if config.Android.CurrentSDK == entry.Name {
		config.Android.CurrentSDK = ""
	}
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]string{"sdk": entry.Name, "path": entryPath}
	if r.json {
		return r.result("mob android sdk remove", data)
	}
	fmt.Fprintf(r.out, "Removed Mob-managed Android SDK %s.\n", entry.Name)
	return nil
}

func (r runtime) help(args []string) error {
	pathArgs, format, err := parseHelpFormat(args)
	if err != nil {
		return err
	}
	if format == helpFormatMarkdown && r.json {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--json cannot be combined with --format markdown.", Remediation: "Use either --json/--format json for machine output, or --format markdown for human-readable output."}
	}
	path := "mob"
	if len(pathArgs) > 0 {
		path += " " + strings.Join(pathArgs, " ")
	}
	data, known := helpData(path)
	if !known {
		return invalidCommand("mob help " + strings.Join(pathArgs, " "))
	}
	if path == "mob" && (r.json || format == helpFormatJSON) {
		data.Subcommands = topLevelHelpContracts()
	}
	if r.json || format == helpFormatJSON {
		jsonRuntime := r
		jsonRuntime.json = true
		return jsonRuntime.result("mob help", data)
	}
	if format == helpFormatMarkdown {
		fmt.Fprint(r.out, markdownHelp(data))
		return nil
	}
	fmt.Fprintf(r.out, "Usage: %s\n\n%s\n", data.Usage, data.Description)
	if len(data.Examples) > 0 {
		fmt.Fprintln(r.out, "\nExamples:")
		for _, example := range data.Examples {
			fmt.Fprintf(r.out, "  %s\n", example)
		}
	}
	if len(data.Related) > 0 {
		fmt.Fprintf(r.out, "\nRelated: %s\n", strings.Join(data.Related, ", "))
	}
	return nil
}

type helpFormat string

const (
	helpFormatText     helpFormat = "text"
	helpFormatMarkdown helpFormat = "markdown"
	helpFormatJSON     helpFormat = "json"
)

func parseHelpFormat(args []string) ([]string, helpFormat, error) {
	path := make([]string, 0, len(args))
	format := helpFormatText
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, "--format=") {
			format = helpFormat(strings.TrimPrefix(arg, "--format="))
		} else if arg == "--format" {
			if index+1 >= len(args) {
				return nil, "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--format requires text, markdown, or json."}
			}
			index++
			format = helpFormat(args[index])
		} else {
			path = append(path, arg)
		}
	}
	if format != helpFormatText && format != helpFormatMarkdown && format != helpFormatJSON {
		return nil, "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--format supports text, markdown, or json."}
	}
	return path, format, nil
}

func markdownHelp(data helpResponse) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# `%s`\n\n%s\n\n## Usage\n\n```text\n%s\n```\n", data.Command, data.Description, data.Usage)
	if len(data.Arguments) > 0 {
		builder.WriteString("\n## Arguments\n\n")
		for _, argument := range data.Arguments {
			requirement := "optional"
			if argument.Required {
				requirement = "required"
			}
			fmt.Fprintf(&builder, "- `%s`: %s\n", argument.Name, requirement)
		}
	}
	if len(data.Options) > 0 {
		builder.WriteString("\n## Options\n\n")
		for _, option := range data.Options {
			if option.ValueSyntax == "" {
				fmt.Fprintf(&builder, "- `%s`\n", option.Name)
			} else {
				fmt.Fprintf(&builder, "- `%s %s`\n", option.Name, option.ValueSyntax)
			}
		}
	}
	if len(data.Prerequisites) > 0 {
		builder.WriteString("\n## Prerequisites\n\n")
		for _, prerequisite := range data.Prerequisites {
			fmt.Fprintf(&builder, "- %s\n", prerequisite)
		}
	}
	if data.SideEffects != "none" {
		fmt.Fprintf(&builder, "\n## Side Effects\n\n%s\n", data.SideEffects)
	}
	if len(data.Examples) > 0 {
		builder.WriteString("\n## Examples\n\n")
		for _, example := range data.Examples {
			fmt.Fprintf(&builder, "```text\n%s\n```\n", example)
		}
	}
	return builder.String()
}

type helpArgument struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

type helpOption struct {
	Name        string `json:"name"`
	ValueSyntax string `json:"valueSyntax,omitempty"`
}

func completeHelpContract(data *helpResponse) {
	data.Arguments = usageArguments(data.Usage)
	data.Options = usageOptions(data.Usage)
	if data.Prerequisites == nil {
		data.Prerequisites = []string{}
	}
	if data.Platforms == nil {
		data.Platforms = []string{}
	}
	switch data.Command {
	case "mob build", "mob run", "mob debug", "mob test", "mob release", "mob release check", "mob logs":
		data.Platforms = []string{"android"}
		data.Prerequisites = []string{"Run from a recognized native Android or Flutter project."}
	case "mob android doctor", "mob android sdk available", "mob android sdk install", "mob android ndk available", "mob android ndk install", "mob android emulator image available", "mob android emulator image install":
		data.Platforms = []string{"android"}
	}
	if eventStreamCommand(data.Command) {
		data.SupportsEventStream = true
		data.Usage = strings.Replace(data.Usage, "[--json]", "[--json|--json=events]", 1)
		data.Options = usageOptions(data.Usage)
	}
}

func eventStreamCommand(command string) bool {
	switch command {
	case "mob android sdk install", "mob android ndk install", "mob android emulator create", "mob android emulator start", "mob java install", "mob flutter install", "mob flutter create", "mob fvm install", "mob fvm update", "mob build", "mob run", "mob debug", "mob test", "mob release", "mob logs":
		return true
	default:
		return false
	}
}

func usageArguments(usage string) []helpArgument {
	command := strings.Fields(usage)
	arguments := make([]helpArgument, 0)
	for index := 0; index < len(command); index++ {
		raw := command[index]
		token := strings.Trim(raw, "[]")
		if strings.HasPrefix(token, "--") {
			if index+1 < len(command) {
				next := strings.Trim(command[index+1], "[]")
				if (strings.HasPrefix(next, "<") && strings.HasSuffix(next, ">")) || strings.Contains(next, "|") {
					index++
				}
			}
			continue
		}
		if !strings.HasPrefix(token, "<") || !strings.HasSuffix(token, ">") {
			continue
		}
		arguments = append(arguments, helpArgument{Name: strings.Trim(token, "<>"), Required: !strings.HasPrefix(raw, "[")})
	}
	return arguments
}

func usageOptions(usage string) []helpOption {
	options := make([]helpOption, 0)
	seen := make(map[string]bool)
	for _, match := range helpOptionPattern.FindAllStringIndex(usage, -1) {
		name := usage[match[0]:match[1]]
		if seen[name] {
			continue
		}
		option := helpOption{Name: name}
		remainder := strings.TrimLeft(usage[match[1]:], " \t[]()")
		if strings.HasPrefix(remainder, "<") {
			if closing := strings.Index(remainder, ">"); closing >= 0 {
				option.ValueSyntax = remainder[:closing+1]
			}
		}
		seen[name] = true
		options = append(options, option)
	}
	return options
}

type helpResponse struct {
	CLIVersion          string         `json:"cliVersion"`
	Command             string         `json:"command"`
	Usage               string         `json:"usage"`
	Description         string         `json:"description"`
	Arguments           []helpArgument `json:"arguments"`
	Options             []helpOption   `json:"options"`
	Prerequisites       []string       `json:"prerequisites"`
	Platforms           []string       `json:"platforms"`
	SupportsJSON        bool           `json:"supportsJson"`
	SupportsEventStream bool           `json:"supportsEventStream"`
	SideEffects         string         `json:"sideEffects"`
	Examples            []string       `json:"examples"`
	Related             []string       `json:"related"`
	Errors              []string       `json:"errors"`
	Subcommands         []helpResponse `json:"subcommands,omitempty"`
}

func topLevelHelpContracts() []helpResponse {
	commands := []string{"version", "status", "doctor", "catalog", "build", "run", "debug", "test", "logs", "release", "env", "home", "support", "java", "flutter", "fvm", "android", "ios", "harmony", "device"}
	contracts := make([]helpResponse, 0, len(commands))
	for _, command := range commands {
		if contract, known := helpData("mob " + command); known {
			contracts = append(contracts, contract)
		}
	}
	return contracts
}

func helpData(path string) (helpResponse, bool) {
	base := helpResponse{CLIVersion: cliVersion, Command: path, SupportsJSON: true, SideEffects: "none", Errors: []string{"MOB_INVALID_COMMAND"}}
	known := true
	switch path {
	case "mob", "mob help":
		base.Usage = "mob <version|status|doctor|catalog|build|run|debug|test|logs|release|env|home|support|java|flutter|fvm|android|ios|harmony|device> [--json]"
		base.Description = "Manage mobile development toolchains."
		base.Examples = []string{"mob --version", "mob status", "mob android sdk list --json", "mob device list"}
		base.Related = []string{"mob version", "mob status", "mob doctor", "mob catalog", "mob build", "mob run", "mob debug", "mob test", "mob logs", "mob release", "mob env show", "mob home show", "mob support bundle", "mob java list", "mob flutter list", "mob fvm status", "mob android sdk list", "mob ios doctor", "mob harmony doctor", "mob device list"}
	case "mob version":
		base.Usage = "mob version [--json]"
		base.Description = "Print the version embedded in the current Mob executable. --version and -V are equivalent aliases."
		base.Examples = []string{"mob --version", "mob version --json"}
		base.Related = []string{"mob help", "mob status"}
	case "mob android":
		base.Usage = "mob android <doctor|create|sdk|ndk|emulator|device|proxy> [--json]"
		base.Description = "Manage Android SDKs, NDKs, emulators, ADB devices, and Android download proxy settings."
		base.Platforms = []string{"android"}
		base.Examples = []string{"mob android sdk list", "mob android emulator list", "mob android proxy show"}
		base.Related = []string{"mob android sdk", "mob android ndk", "mob android emulator", "mob android device"}
	case "mob android sdk":
		base.Usage = "mob android sdk <list|available|import|inspect|install|use|remove> [--json]"
		base.Description = "Discover, register, inspect, install, select, and remove Android SDKs and their official components."
		base.Platforms = []string{"android"}
		base.Examples = []string{"mob android sdk list", "mob android sdk available --api 35", "mob android sdk import --path <sdk-root>"}
		base.Related = []string{"mob android sdk available", "mob android sdk install", "mob android sdk inspect"}
	case "mob android ndk":
		base.Usage = "mob android ndk <list|available|install|remove> [--json]"
		base.Description = "List and manage Android NDK packages within an Android SDK."
		base.Platforms = []string{"android"}
		base.Examples = []string{"mob android ndk list", "mob android ndk available"}
		base.Related = []string{"mob android ndk install", "mob android sdk inspect"}
	case "mob android emulator":
		base.Usage = "mob android emulator <list|create|start|stop|image> [--json]"
		base.Description = "Manage Android Virtual Devices through the official Android Emulator tools."
		base.Platforms = []string{"android"}
		base.Examples = []string{"mob android emulator list", "mob android emulator image available"}
		base.Related = []string{"mob android emulator create", "mob device list"}
	case "mob android emulator image":
		base.Usage = "mob android emulator image <available|install> [--json]"
		base.Description = "List or install Android Emulator system images from the Android repository catalog."
		base.Platforms = []string{"android"}
		base.Examples = []string{"mob android emulator image available --api 35"}
		base.Related = []string{"mob android emulator create", "mob android sdk inspect"}
	case "mob android proxy":
		base.Usage = "mob android proxy <show|set|clear> [--json]"
		base.Description = "Manage the HTTP(S) proxy injected only into Android installation subprocesses."
		base.Platforms = []string{"android"}
		base.Examples = []string{"mob android proxy show", "mob android proxy set http://127.0.0.1:7890"}
		base.Related = []string{"mob android sdk install"}
	case "mob android device":
		base.Usage = "mob android device <connect|pair> <host:port> [--code <6-digits>] [--json]"
		base.Description = "Pair or connect an Android device through ADB over TCP/IP."
		base.Platforms = []string{"android"}
		base.Examples = []string{"mob android device pair 192.168.1.20:37123 --code 123456", "mob android device connect 192.168.1.20:5555"}
		base.Related = []string{"mob device list"}
	case "mob device":
		base.Usage = "mob device <list|use|open|mirror|screenshot|ui-tree|record|forward> [--json]"
		base.Description = "List and operate on connected Android emulators and physical devices."
		base.Platforms = []string{"android"}
		base.Examples = []string{"mob device list", "mob device use android:emulator-5554"}
		base.Related = []string{"mob run", "mob android emulator list"}
	case "mob java":
		base.Usage = "mob java <list|available|import|install|use|remove> [--json]"
		base.Description = "Discover and manage JDKs used only by Mob child processes."
		base.Examples = []string{"mob java list", "mob java install 17"}
		base.Related = []string{"mob doctor", "mob build"}
	case "mob flutter":
		base.Usage = "mob flutter <list|available|install|use|remove|create> [--json]"
		base.Description = "Discover and manage Flutter SDKs without changing project configuration."
		base.Examples = []string{"mob flutter available", "mob flutter install"}
		base.Related = []string{"mob fvm status", "mob build", "mob run"}
	case "mob fvm":
		base.Usage = "mob fvm <status|list|available|install|update|use|remove> [--json]"
		base.Description = "Manage isolated Mob FVM launchers while preserving each project's .fvmrc."
		base.Examples = []string{"mob fvm status", "mob fvm install"}
		base.Related = []string{"mob flutter list", "mob run"}
	case "mob home":
		base.Usage = "mob home <show|set> [--json]"
		base.Description = "Show or migrate the Mob-owned home directory."
		base.Examples = []string{"mob home show", "mob home set D:\\mobile-tools"}
		base.Related = []string{"mob env show", "mob status"}
	case "mob support":
		base.Usage = "mob support bundle [--output <path>] [--json]"
		base.Description = "Create a redacted support archive for Mob diagnostics."
		base.Examples = []string{"mob support bundle"}
		base.Related = []string{"mob support bundle", "mob doctor"}
	case "mob env":
		base.Usage = "mob env show [--json]"
		base.Description = "Show selections and the child-process-only environment scope."
		base.Examples = []string{"mob env show"}
		base.Related = []string{"mob status"}
	case "mob ios", "mob ios doctor":
		base.Usage = "mob ios doctor [--json]"
		base.Description = "Read the active Xcode developer directory and Xcode version without changing the Apple toolchain."
		base.Platforms = []string{"ios"}
		base.Prerequisites = []string{"macOS with an installed and licensed Xcode."}
		base.Examples = []string{"mob ios doctor --json"}
		base.Related = []string{"mob doctor", "mob status"}
		base.Errors = []string{"MOB_HOST_UNSUPPORTED", "MOB_TOOLCHAIN_MISSING"}
	case "mob ios simulator start":
		base.Usage = "mob ios simulator start ios:<simulator-udid> [--json]"
		base.Description = "Boot an existing available iOS Simulator and open Xcode's official Simulator application."
		base.Platforms = []string{"ios"}
		base.Prerequisites = []string{"macOS with installed and licensed Xcode.", "An available Simulator from mob device list --platform ios."}
		base.SideEffects = "boots the selected Simulator and opens the official Simulator application"
		base.Examples = []string{"mob ios simulator start ios:9A31B7F9-0B9D-4ED1-B8BC-5FBAC2112345"}
		base.Related = []string{"mob device list --platform ios", "mob ios doctor"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_HOST_UNSUPPORTED", "MOB_TOOLCHAIN_MISSING", "MOB_DEVICE_UNAVAILABLE", "MOB_COMMAND_FAILED"}
	case "mob harmony", "mob harmony doctor":
		base.Usage = "mob harmony doctor [--json]"
		base.Description = "Report the reserved HarmonyOS namespace. HarmonyOS toolchain management is not implemented in this Mob release."
		base.Platforms = []string{"harmony"}
		base.Examples = []string{"mob harmony doctor --json"}
		base.Related = []string{"mob doctor", "mob status"}
		base.Errors = []string{"MOB_PLATFORM_NOT_SUPPORTED"}
	case "mob support bundle":
		base.Usage = "mob support bundle [--output <path>] [--json]"
		base.Description = "Create a ZIP containing a redacted Mob configuration and registered toolchain inventory. It never includes project files, environment variables, logs, proxy settings, credentials, or raw host paths."
		base.SideEffects = "writes a new support ZIP without overwriting an existing file"
		base.Examples = []string{"mob support bundle", "mob support bundle --output artifacts/mob-support.zip --json"}
		base.Related = []string{"mob status", "mob doctor", "mob env show"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_COMMAND_FAILED"}
	case "mob catalog":
		base.Usage = "mob catalog [--platform android] [--component sdk|ndk|system-image|java] [--refresh] [--json]"
		base.Description = "Show the aggregated Android SDK, NDK, system-image, and Temurin JDK packages available from verified official catalogs."
		base.Examples = []string{"mob catalog", "mob catalog --refresh --json"}
		base.Related = []string{"mob android sdk available", "mob android ndk available", "mob android emulator image available"}
		base.Errors = []string{"MOB_CATALOG_UNAVAILABLE"}
	case "mob android doctor":
		base.Usage = "mob android doctor [--json]"
		base.Description = "Diagnose the Android SDK, JDK, and current Android or Flutter project's declared requirements."
		base.Examples = []string{"mob android doctor", "mob android doctor --json"}
		base.Related = []string{"mob doctor", "mob android sdk list", "mob java list"}
		base.Errors = []string{"MOB_TOOLCHAIN_MISSING", "MOB_PROJECT_UNRECOGNIZED"}
	case "mob android proxy show":
		base.Usage = "mob android proxy show [--json]"
		base.Description = "Show the Android sdkmanager HTTP(S) proxy Mob injects only into installation subprocesses."
		base.Examples = []string{"mob android proxy show --json"}
		base.Related = []string{"mob android proxy set", "mob android sdk install"}
	case "mob android proxy set":
		base.Usage = "mob android proxy set <http://host:port|https://host:port> [--json]"
		base.Description = "Set a Mob-managed HTTP(S) proxy for Android sdkmanager downloads without changing system proxy variables."
		base.SideEffects = "writes Mob's Android download proxy preference"
		base.Examples = []string{"mob android proxy set http://127.0.0.1:7890"}
		base.Related = []string{"mob android proxy show", "mob android proxy clear"}
		base.Errors = []string{"MOB_INVALID_COMMAND"}
	case "mob android proxy clear":
		base.Usage = "mob android proxy clear [--json]"
		base.Description = "Remove Mob's Android sdkmanager proxy preference."
		base.SideEffects = "clears Mob's Android download proxy preference"
		base.Examples = []string{"mob android proxy clear"}
		base.Related = []string{"mob android proxy show"}
	case "mob test":
		base.Usage = "mob test [--platform android] [--no-install] [--accept-licenses] [-- <official-command> [args...]] [--json]"
		base.Description = "Run the current project's official Android or Flutter unit-test command."
		base.SideEffects = "runs project tests and creates normal test reports"
		base.Examples = []string{"mob test", "mob test --platform android", "mob test -- flutter test test/widget_test.dart"}
		base.Related = []string{"mob build", "mob run"}
		base.Errors = []string{"MOB_PROJECT_UNRECOGNIZED", "MOB_PLATFORM_NOT_SUPPORTED", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob logs":
		base.Usage = "mob logs [--device <android:native-id>] [--follow] [--json|--json=events]"
		base.Description = "Read the current Android log buffer for the running application in this project."
		base.Examples = []string{"mob logs", "mob logs --follow", "mob logs --device android:emulator-5554 --json"}
		base.Related = []string{"mob run", "mob device list"}
		base.Errors = []string{"MOB_PROJECT_UNRECOGNIZED", "MOB_DEVICE_UNAVAILABLE", "MOB_RUNNER_UNAVAILABLE", "MOB_COMMAND_FAILED"}
	case "mob fvm status":
		base.Usage = "mob fvm status [--json]"
		base.Description = "Show system and Mob-managed FVM launchers without reading project .fvmrc contents."
		base.Examples = []string{"mob fvm status", "mob fvm status --json"}
		base.Related = []string{"mob fvm available", "mob fvm install"}
		base.Errors = []string{"MOB_PROJECT_UNRECOGNIZED"}
	case "mob fvm list":
		base.Usage = "mob fvm list [--json]"
		base.Description = "List Mob-managed FVM versions and the selected launcher."
		base.Examples = []string{"mob fvm list", "mob fvm list --json"}
		base.Related = []string{"mob fvm use", "mob fvm remove"}
	case "mob fvm available":
		base.Usage = "mob fvm available [--refresh] [--json]"
		base.Description = "List FVM releases and SHA-256 checksums from the official pub.dev catalog."
		base.Examples = []string{"mob fvm available --json", "mob fvm available --refresh"}
		base.Related = []string{"mob fvm install", "mob fvm update"}
		base.Errors = []string{"MOB_CATALOG_UNAVAILABLE"}
	case "mob fvm install":
		base.Usage = "mob fvm install [--version <version>] [--json]"
		base.Description = "Verify and install an isolated Mob-managed FVM launcher using Dart supplied by Flutter."
		base.SideEffects = "downloads a verified FVM package and creates a Mob-owned isolated Dart package cache"
		base.Examples = []string{"mob fvm install", "mob fvm install --version 4.1.2"}
		base.Related = []string{"mob fvm available", "mob fvm status", "mob flutter install"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_CATALOG_UNAVAILABLE", "MOB_SOURCE_INVALID", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob fvm update":
		base.Usage = "mob fvm update [--json]"
		base.Description = "Install and select the current FVM release from the official catalog without changing any project .fvmrc."
		base.SideEffects = "downloads a verified FVM package and changes Mob's selected FVM launcher"
		base.Examples = []string{"mob fvm update"}
		base.Related = []string{"mob fvm available", "mob fvm install"}
		base.Errors = []string{"MOB_CATALOG_UNAVAILABLE", "MOB_SOURCE_INVALID", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob fvm use":
		base.Usage = "mob fvm use <version> [--json]"
		base.Description = "Select one installed Mob-managed FVM launcher without changing any project files."
		base.SideEffects = "changes Mob's selected FVM launcher"
		base.Examples = []string{"mob fvm use 4.1.2"}
		base.Related = []string{"mob fvm list", "mob fvm install"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING"}
	case "mob fvm remove":
		base.Usage = "mob fvm remove <version> --yes [--json]"
		base.Description = "Remove one Mob-managed FVM launcher and its isolated package cache."
		base.SideEffects = "permanently deletes a Mob-managed FVM directory"
		base.Examples = []string{"mob fvm remove 4.1.2 --yes"}
		base.Related = []string{"mob fvm list", "mob fvm use"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", "MOB_TOOLCHAIN_MISSING"}
	case "mob release":
		base.Usage = "mob release [--platform android] [--artifact aab|apk] [--output <path>] [--no-install] [--accept-licenses] [--json]"
		base.Description = "Build a configured Android release artifact and return its absolute path, size, and SHA-256."
		base.SideEffects = "runs the project release build and may copy the output artifact"
		base.Examples = []string{"mob release", "mob release --artifact apk --output dist", "mob release --json"}
		base.Related = []string{"mob build", "mob doctor"}
		base.Errors = []string{"MOB_PROJECT_UNRECOGNIZED", "MOB_PLATFORM_NOT_SUPPORTED", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob release check":
		base.Usage = "mob release check [--platform android] [--json]"
		base.Description = "Check the Android release runner, SDK requirements, application ID, and configured release signing before building."
		base.Examples = []string{"mob release check", "mob release check --json"}
		base.Related = []string{"mob release", "mob doctor"}
		base.Errors = []string{"MOB_PROJECT_UNRECOGNIZED", "MOB_PLATFORM_NOT_SUPPORTED"}
	case "mob flutter list":
		base.Usage = "mob flutter list [--json]"
		base.Description = "List discovered Flutter and FVM launchers without modifying either toolchain."
		base.Examples = []string{"mob flutter list", "mob flutter list --json"}
		base.Related = []string{"mob build", "mob run", "mob help run"}
		base.Errors = []string{"MOB_INVALID_COMMAND"}
	case "mob env show":
		base.Usage = "mob env show [--json]"
		base.Description = "Show Mob's current toolchain selections and the environment variables Mob injects only into child processes."
		base.Examples = []string{"mob env show", "mob env show --json"}
		base.Related = []string{"mob status", "mob android sdk use", "mob java use", "mob flutter use"}
	case "mob home show":
		base.Usage = "mob home show [--json]"
		base.Description = "Show the resolved Mob home, persistent selection, and any MOB_HOME override."
		base.Examples = []string{"mob home show", "mob home show --json"}
		base.Related = []string{"mob home set", "mob env show"}
	case "mob home set":
		base.Usage = "mob home set <path> [--json]"
		base.Description = "Move the Mob-owned home to an empty destination and persist the new user-level selection."
		base.SideEffects = "moves Mob-owned toolchains and cache files, then updates Mob's home selection"
		base.Examples = []string{"mob home set D:\\mobile-tools"}
		base.Related = []string{"mob home show", "mob status"}
		base.Errors = []string{"MOB_INVALID_COMMAND", "MOB_INVALID_ARGUMENT", "MOB_COMMAND_FAILED"}
	case "mob device screenshot":
		base.Usage = "mob device screenshot [android:<native-id>] [--output|--out <path>] [--json]"
		base.Description = "Capture a PNG screenshot from a ready Android device through its selected SDK's ADB."
		base.SideEffects = "writes a PNG image file"
		base.Examples = []string{"mob device screenshot", "mob device screenshot android:R58N123456A --out artifacts/phone.png"}
		base.Related = []string{"mob device list", "mob device mirror"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_DEVICE_UNAVAILABLE", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob device ui-tree":
		base.Usage = "mob device ui-tree [--device android:<native-id>] [--json]"
		base.Description = "Return the current Android UI Automator hierarchy as structured JSON without exposing Android temporary paths to the calling shell."
		base.Examples = []string{"mob device ui-tree --json", "mob device ui-tree --device android:emulator-5554 --json"}
		base.Related = []string{"mob device screenshot", "mob logs"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_DEVICE_UNAVAILABLE", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob device record":
		base.Usage = "mob device record android:<native-id> [--output <path>] [--seconds <1-180>] [--json]"
		base.Description = "Record a ready Android device screen through ADB and save an MP4 on the host."
		base.SideEffects = "records the device and writes an MP4 file"
		base.Examples = []string{"mob device record android:emulator-5554", "mob device record android:R58N123456A --seconds 60 --output artifacts/demo.mp4"}
		base.Related = []string{"mob device screenshot", "mob device mirror"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_DEVICE_UNAVAILABLE", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob java list":
		base.Usage = "mob java list [--json]"
		base.Description = "List Mob-registered and discovered JDKs without modifying the system environment."
		base.Examples = []string{"mob java list", "mob java list --json"}
		base.Related = []string{"mob java import", "mob java use", "mob doctor"}
	case "mob java available":
		base.Usage = "mob java available [--refresh] [--json]"
		base.Description = "List the current 8, 11, 17, and 21 Eclipse Temurin JDK releases for this host with official SHA-256 checksums."
		base.Examples = []string{"mob java available", "mob java available --refresh --json"}
		base.Related = []string{"mob java import", "mob catalog"}
		base.Errors = []string{"MOB_CATALOG_UNAVAILABLE"}
	case "mob java install":
		base.Usage = "mob java install <8|11|17|21> [--json]"
		base.Description = "Download a verified Eclipse Temurin JDK into Mob's managed toolchain directory."
		base.SideEffects = "downloads and installs a Mob-managed JDK"
		base.Examples = []string{"mob java install 17"}
		base.Related = []string{"mob java available", "mob java use"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_CATALOG_UNAVAILABLE", "MOB_COMMAND_FAILED"}
	case "mob java remove":
		base.Usage = "mob java remove <name> --yes [--json]"
		base.Description = "Remove one Mob-managed JDK. Imported and discovered JDKs are never modified."
		base.SideEffects = "permanently deletes the selected Mob-managed JDK directory"
		base.Examples = []string{"mob java remove temurin-17 --yes"}
		base.Related = []string{"mob java list", "mob java install"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING", "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", "MOB_TOOLCHAIN_IN_USE"}
	case "mob java import":
		base.Usage = "mob java import --path <jdk-root> --name <name> [--json]"
		base.Description = "Register an existing JDK after validating its java executable and version."
		base.SideEffects = "writes an external JDK reference to config.yaml; JDK files are never modified"
		base.Examples = []string{"mob java import --path C:\\Java\\jdk-17 --name temurin-17"}
		base.Related = []string{"mob java list", "mob java use"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT"}
	case "mob java use":
		base.Usage = "mob java use <name> [--json]"
		base.Description = "Select a registered JDK for later Mob Android child processes without changing system JAVA_HOME."
		base.SideEffects = "writes the current Mob JDK reference to config.yaml"
		base.Examples = []string{"mob java use temurin-17"}
		base.Related = []string{"mob java list", "mob build"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING"}
	case "mob flutter available":
		base.Usage = "mob flutter available [--refresh] [--json]"
		base.Description = "List stable Flutter SDK releases for the current host from Flutter's official release directory."
		base.Examples = []string{"mob flutter available", "mob flutter available --refresh --json"}
		base.Related = []string{"mob flutter list", "mob flutter create"}
		base.Errors = []string{"MOB_CATALOG_UNAVAILABLE"}
	case "mob flutter install":
		base.Usage = "mob flutter install [--version <version>] [--json]"
		base.Description = "Download and install a stable Flutter SDK from the official release catalog with SHA-256 verification."
		base.SideEffects = "downloads and installs a Mob-managed Flutter SDK"
		base.Examples = []string{"mob flutter install", "mob flutter install --version 3.29.0"}
		base.Related = []string{"mob flutter available", "mob flutter list"}
		base.Errors = []string{"MOB_CATALOG_UNAVAILABLE", "MOB_PACKAGE_NOT_AVAILABLE", "MOB_COMMAND_FAILED"}
	case "mob flutter use":
		base.Usage = "mob flutter use <version> [--json]"
		base.Description = "Select an installed Mob-managed Flutter SDK for later Mob project commands."
		base.SideEffects = "writes the current Mob Flutter SDK reference to config.yaml"
		base.Examples = []string{"mob flutter use 3.29.0"}
		base.Related = []string{"mob flutter list", "mob flutter install"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING"}
	case "mob flutter remove":
		base.Usage = "mob flutter remove <version> --yes [--json]"
		base.Description = "Remove one Mob-managed Flutter SDK. System Flutter and FVM installations are never modified."
		base.SideEffects = "permanently deletes the selected Mob-managed Flutter directory"
		base.Examples = []string{"mob flutter remove 3.29.0 --yes"}
		base.Related = []string{"mob flutter list", "mob flutter install"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", "MOB_TOOLCHAIN_MISSING"}
	case "mob flutter create":
		base.Usage = "mob flutter create <name> [--platforms android,ios] [--json]"
		base.Description = "Create a standard Flutter project through Flutter. Mob automatically installs the verified current stable SDK when no regular Flutter runner exists."
		base.SideEffects = "may download a Mob-managed Flutter SDK and creates a Flutter project directory"
		base.Examples = []string{"mob flutter create travel_app", "mob flutter create travel_app --platforms android,ios"}
		base.Related = []string{"mob flutter list", "mob run"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob status":
		base.Usage = "mob status [--json]"
		base.Description = "Show the Mob home directory and discovered Android SDKs."
		base.Examples = []string{"mob status", "mob status --json"}
		base.Related = []string{"mob doctor", "mob android sdk list"}
	case "mob doctor":
		base.Usage = "mob doctor [--platform android] [--json]"
		base.Description = "Diagnose Android SDK and JDK availability without changing the host."
		base.Examples = []string{"mob doctor", "mob doctor --platform android --json"}
		base.Related = []string{"mob status", "mob android sdk import", "mob support bundle"}
		base.Errors = []string{"MOB_INVALID_COMMAND"}
	case "mob build":
		base.Usage = "mob build [--platform <id>] [--force --platform android -- <official-command> [args...]] [--no-install] [--accept-licenses] [-- <official-command> [args...]] [--json]"
		base.Description = "Build the current project through its supported official runner. Native Android and Android KMP use the Gradle Wrapper; Flutter Android uses flutter build apk; native iOS invokes xcodebuild on macOS. --force permits an explicit Android Gradle command when project detection is incomplete."
		base.SideEffects = "runs the project build and creates normal project build outputs"
		base.Examples = []string{"mob build", "mob build --accept-licenses", "mob build --platform android -- gradlew.bat assembleRelease", "mob build --force --platform android -- .\\gradlew.bat :androidApp:assembleDebug", "mob build --platform ios"}
		base.Related = []string{"mob status", "mob doctor", "mob android sdk use"}
		base.Errors = []string{"MOB_PROJECT_UNRECOGNIZED", "MOB_PLATFORM_REQUIRED", "MOB_PLATFORM_NOT_SUPPORTED", "MOB_TOOLCHAIN_MISSING", "MOB_RUNNER_UNAVAILABLE", "MOB_COMMAND_FAILED"}
	case "mob run":
		base.Usage = "mob run [--platform <id>] [--device <platform:native-id>] [--mirror] [--headless] [--no-device-create] [--no-install] [--accept-licenses] [-- <official-command> [args...]] [--json]"
		base.Description = "Build, install and launch the current project through its supported official runner. Native Android uses Gradle and ADB; Flutter uses flutter run and automatically prepares a verified Flutter SDK when needed."
		base.SideEffects = "runs the project, installs the app, and launches it on the selected device"
		base.Examples = []string{"mob run", "mob run --accept-licenses", "mob run --headless", "mob run --mirror --device android:R58N123456A", "mob run --no-device-create", "mob run --device android:emulator-5554", "mob run -- fvm flutter run -d {{mob.device.nativeId}}"}
		base.Related = []string{"mob device list", "mob device use", "mob android emulator start"}
		base.Errors = []string{"MOB_PROJECT_UNRECOGNIZED", "MOB_PLATFORM_REQUIRED", "MOB_PLATFORM_DEVICE_MISMATCH", "MOB_DEVICE_UNAVAILABLE", "MOB_TOOLCHAIN_MISSING", "MOB_RUNNER_UNAVAILABLE", "MOB_COMMAND_FAILED"}
	case "mob debug":
		base.Usage = "mob debug [--platform <id>] [--device <platform:native-id>] [--mirror] [--headless] [--no-device-create] [--no-install] [--accept-licenses] [-- <official-command> [args...]] [--json]"
		base.Description = "Build and launch a native Android project in wait-for-debugger mode and return a JDWP target, or start the official Flutter debug runner. Flutter --json=events reports its Dart VM Service target and cannot be combined with a forwarded command."
		base.SideEffects = "runs the project, installs the debug app, and starts the platform's official debug session"
		base.Examples = []string{"mob debug --device android:emulator-5554", "mob debug --accept-licenses --json", "mob debug --json=events"}
		base.Related = []string{"mob run", "mob device list", "mob android emulator start"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_PROJECT_UNRECOGNIZED", "MOB_PLATFORM_REQUIRED", "MOB_DEVICE_UNAVAILABLE", "MOB_TOOLCHAIN_MISSING", "MOB_RUNNER_UNAVAILABLE", "MOB_COMMAND_FAILED"}
	case "mob android sdk list":
		base.Usage = "mob android sdk list [--json]"
		base.Description = "List valid Android SDKs discovered on the host or registered with Mob."
		base.Examples = []string{"mob android sdk list", "mob android sdk list --json"}
		base.Related = []string{"mob android sdk available", "mob android sdk add", "mob android sdk inspect", "mob android sdk use"}
	case "mob android create":
		base.Usage = "mob android create <name> [--language kotlin|java] [--ui compose|views] [--min-sdk <level>] [--json]"
		base.Description = "Create a standard native Android Gradle project and generate its Gradle Wrapper through local Gradle."
		base.SideEffects = "creates a new Android project directory and standard Gradle files"
		base.Examples = []string{"mob android create notes", "mob android create notes --language java --ui views --min-sdk 24"}
		base.Related = []string{"mob build", "mob run"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob android sdk available":
		base.Usage = "mob android sdk available [--api <level>] [--refresh] [--json]"
		base.Description = "List installable Android SDK components from the official repository catalog or its local cache."
		base.Examples = []string{"mob android sdk available", "mob android sdk available --api 35 --refresh --json"}
		base.Related = []string{"mob android sdk install", "mob android emulator create"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_CATALOG_UNAVAILABLE"}
	case "mob android sdk install":
		base.Usage = "mob android sdk install <name> (--api <level>|--package <package-id>)... [--accept-licenses] [--allow-external-write --yes] [--json]"
		base.Description = "Install Android SDK Platform, Build Tools, platform-tools, Emulator, or catalog-listed packages through the selected SDK's official sdkmanager."
		base.SideEffects = "downloads and installs Android SDK components"
		base.Examples = []string{"mob android sdk install managed --api 35 --accept-licenses", "mob android sdk install managed --package platform-tools --accept-licenses"}
		base.Related = []string{"mob android sdk available", "mob android sdk inspect", "mob android emulator image install"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_LICENSE_REQUIRED", "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob android sdk inspect":
		base.Usage = "mob android sdk inspect <name> [--json]"
		base.Description = "Inspect platforms, build tools, NDKs and device tools in one Android SDK."
		base.Examples = []string{"mob android sdk inspect shared --json"}
		base.Related = []string{"mob android sdk list"}
		base.Errors = []string{"MOB_TOOLCHAIN_MISSING"}
	case "mob android sdk use":
		base.Usage = "mob android sdk use <name> [--json]"
		base.Description = "Select a registered or discovered Android SDK for later Mob commands."
		base.SideEffects = "writes the current Android SDK reference to config.yaml"
		base.Examples = []string{"mob android sdk use shared"}
		base.Related = []string{"mob android sdk list", "mob android sdk inspect"}
		base.Errors = []string{"MOB_TOOLCHAIN_MISSING"}
	case "mob android sdk remove":
		base.Usage = "mob android sdk remove <name> --yes [--json]"
		base.Description = "Remove a Mob-managed Android SDK and its downloaded components. Imported and discovered SDKs are never removed."
		base.SideEffects = "permanently deletes the selected Mob-managed SDK directory"
		base.Examples = []string{"mob android sdk remove managed --yes"}
		base.Related = []string{"mob android sdk list", "mob android sdk install"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", "MOB_TOOLCHAIN_MISSING"}
	case "mob android sdk add":
		base.Usage = "mob android sdk add <name> --path <sdk-root> [--json]"
		base.Description = "Register an existing Android SDK as read-only."
		base.SideEffects = "writes a Mob-owned SDK reference to config.yaml"
		base.Examples = []string{"mob android sdk add shared --path E:\\Android\\Sdk"}
		base.Related = []string{"mob android sdk list", "mob android sdk use"}
		base.Errors = []string{"MOB_TOOLCHAIN_MISSING", "MOB_INVALID_ARGUMENT"}
	case "mob android sdk import":
		base.Usage = "mob android sdk import --path <sdk-root> [--name <name>] [--json]"
		base.Description = "Compatibility alias for mob android sdk add."
		base.SideEffects = "writes a Mob-owned SDK reference to config.yaml"
		base.Examples = []string{"mob android sdk add shared --path E:\\Android\\Sdk"}
		base.Related = []string{"mob android sdk add", "mob android sdk list"}
		base.Errors = []string{"MOB_TOOLCHAIN_MISSING", "MOB_INVALID_ARGUMENT"}
	case "mob android ndk list":
		base.Usage = "mob android ndk list [--sdk <name>] [--json]"
		base.Description = "List installed NDK versions in discovered Android SDKs."
		base.Examples = []string{"mob android ndk list", "mob android ndk list --sdk managed --json"}
		base.Related = []string{"mob android sdk list", "mob android ndk install"}
		base.Errors = []string{"MOB_TOOLCHAIN_MISSING", "MOB_INVALID_ARGUMENT"}
	case "mob android ndk available":
		base.Usage = "mob android ndk available [--refresh] [--json]"
		base.Description = "List installable Android NDK versions from the official repository catalog or its local cache."
		base.Examples = []string{"mob android ndk available --json", "mob android ndk available --refresh"}
		base.Related = []string{"mob android ndk install", "mob android sdk available"}
		base.Errors = []string{"MOB_CATALOG_UNAVAILABLE"}
	case "mob android ndk install":
		base.Usage = "mob android ndk install <version> --sdk <name> --accept-licenses [--allow-external-write --yes] [--json]"
		base.Description = "Install one Android NDK version through the selected SDK's official sdkmanager."
		base.SideEffects = "downloads and installs an Android SDK component"
		base.Examples = []string{"mob android ndk install 27.2.12479018 --sdk managed --accept-licenses"}
		base.Related = []string{"mob android ndk list", "mob android sdk inspect"}
		base.Errors = []string{"MOB_LICENSE_REQUIRED", "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob android ndk remove":
		base.Usage = "mob android ndk remove <version> --sdk <name> --yes [--json]"
		base.Description = "Remove one NDK version from the Mob-managed Android SDK. External SDKs are never modified."
		base.SideEffects = "permanently deletes the selected Mob-managed NDK directory"
		base.Examples = []string{"mob android ndk remove 27.2.12479018 --sdk managed --yes"}
		base.Related = []string{"mob android ndk list", "mob android ndk install"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", "MOB_TOOLCHAIN_MISSING"}
	case "mob android emulator list":
		base.Usage = "mob android emulator list [--json]"
		base.Description = "List Android Virtual Device definitions reported by the official Emulator."
		base.Examples = []string{"mob android emulator list --json"}
		base.Related = []string{"mob android emulator start", "mob device list"}
		base.Errors = []string{"MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob android emulator image available":
		base.Usage = "mob android emulator image available [--api <level>] [--refresh] [--json]"
		base.Description = "List installable Android Emulator system images from official sdkmanager metadata when an SDK is available, otherwise from the Android repository catalog."
		base.Examples = []string{"mob android emulator image available --api 35 --json"}
		base.Related = []string{"mob android emulator image install", "mob android emulator create"}
		base.Errors = []string{"MOB_CATALOG_UNAVAILABLE"}
	case "mob android emulator image install":
		base.Usage = "mob android emulator image install <package-id> --sdk <name> --accept-licenses [--allow-external-write --yes] [--json]"
		base.Description = "Install one catalog-listed Android system image through the selected SDK's official sdkmanager."
		base.SideEffects = "downloads and installs an Android Emulator system image"
		base.Examples = []string{"mob android emulator image install system-images;android-35;google_apis;x86_64 --sdk managed --accept-licenses"}
		base.Related = []string{"mob android emulator image available", "mob android emulator create"}
		base.Errors = []string{"MOB_LICENSE_REQUIRED", "MOB_PACKAGE_NOT_AVAILABLE", "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", "MOB_COMMAND_FAILED"}
	case "mob android emulator create":
		base.Usage = "mob android emulator create [<avd-name>] [--image <package-id>] [--sdk <name>] [--json]"
		base.Description = "Create an Android Virtual Device from an already installed SDK system image without overwriting existing AVDs."
		base.SideEffects = "creates an Android Virtual Device definition"
		base.Examples = []string{"mob android emulator create", "mob android emulator create mob-android-api-35 --image system-images;android-35;google_apis;x86_64 --sdk managed"}
		base.Related = []string{"mob android emulator list", "mob android emulator start", "mob android sdk inspect"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob android emulator start":
		base.Usage = "mob android emulator start <avd-name> [--json]"
		base.Description = "Launch an existing Android Virtual Device in the official emulator window."
		base.SideEffects = "starts an Android Emulator process"
		base.Examples = []string{"mob android emulator start mob-android-api-35"}
		base.Related = []string{"mob android emulator list", "mob device list"}
		base.Errors = []string{"MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob android emulator stop":
		base.Usage = "mob android emulator stop <android:emulator-id> [--json]"
		base.Description = "Stop a running Android Emulator through ADB."
		base.SideEffects = "stops the selected Android Emulator"
		base.Examples = []string{"mob android emulator stop android:emulator-5554"}
		base.Related = []string{"mob device list", "mob android emulator start"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob device list":
		base.Usage = "mob device list [--platform android|ios] [--json]"
		base.Description = "List Android devices reported by ADB and, on macOS, iOS Simulators reported by Xcode simctl."
		base.Examples = []string{"mob device list", "mob device list --platform android --json", "mob device list --platform ios"}
		base.Related = []string{"mob device use", "mob android device connect", "mob android sdk inspect", "mob ios doctor"}
		base.Errors = []string{"MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob device use":
		base.Usage = "mob device use <platform:native-id> [--json]"
		base.Description = "Persist a ready connected device as Mob's default run target."
		base.SideEffects = "writes Mob's default device to config.yaml"
		base.Examples = []string{"mob device use android:emulator-5554"}
		base.Related = []string{"mob device list", "mob android emulator start"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_DEVICE_UNAVAILABLE", "MOB_PLATFORM_NOT_SUPPORTED", "MOB_TOOLCHAIN_MISSING"}
	case "mob device open":
		base.Usage = "mob device open <android:native-id> [--json]"
		base.Description = "Wake a ready Android emulator or open a physical Android device in Mob's automatically prepared preview window."
		base.SideEffects = "sends an ADB wake event or downloads Mob's internal preview runtime and starts a mirror process"
		base.Examples = []string{"mob device open android:emulator-5554", "mob device open android:R58N123456A"}
		base.Related = []string{"mob device list", "mob device mirror", "mob android emulator start"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_DEVICE_UNAVAILABLE", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob device mirror":
		base.Usage = "mob device mirror <android:native-id> [--json]"
		base.Description = "Open a low-latency mirror window for a ready physical Android device; Mob automatically prepares its internal preview runtime when needed."
		base.SideEffects = "may download Mob's internal preview runtime and starts a mirror process"
		base.Examples = []string{"mob device mirror android:R58N123456A"}
		base.Related = []string{"mob device list", "mob run"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_DEVICE_UNAVAILABLE", "MOB_TOOLCHAIN_MISSING"}
	case "mob device forward":
		base.Usage = "mob device forward remove android:<native-id> --port <1-65535> [--json]"
		base.Description = "Remove a local ADB JDWP forward created for an Android debug session."
		base.Platforms = []string{"android"}
		base.SideEffects = "removes one active ADB port forward"
		base.Examples = []string{"mob device forward remove android:emulator-5554 --port 41234"}
		base.Related = []string{"mob debug", "mob device list"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob android device connect":
		base.Usage = "mob android device connect <host:port> [--json]"
		base.Description = "Ask ADB to connect to a wireless Android device."
		base.SideEffects = "establishes an ADB device connection"
		base.Examples = []string{"mob android device connect 192.168.1.20:5555"}
		base.Related = []string{"mob device list"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	case "mob android device pair":
		base.Usage = "mob android device pair <host:port> --code <6-digits> [--json]"
		base.Description = "Pair with Android Wireless debugging using the address and six-digit code shown on the device."
		base.Platforms = []string{"android"}
		base.SideEffects = "creates an ADB Wireless debugging trust relationship"
		base.Examples = []string{"mob android device pair 192.168.1.20:37123 --code 123456"}
		base.Related = []string{"mob android device connect", "mob device list"}
		base.Errors = []string{"MOB_INVALID_ARGUMENT", "MOB_TOOLCHAIN_MISSING", "MOB_COMMAND_FAILED"}
	default:
		known = false
	}
	completeHelpContract(&base)
	return base, known
}

func (r runtime) result(command string, data interface{}) error {
	return r.emit("completed", command, true, data, nil)
}

func (r runtime) emit(kind, command string, ok bool, data interface{}, coded *codedError) error {
	if !r.json {
		if kind == "started" || kind == "progress" {
			phase := eventPhase(data)
			if phase == "" {
				phase = kind
			}
			r.terminal.Phase(command, phaseLabel(phase))
		}
		return nil
	}
	if !r.eventMode && kind != "completed" && kind != "error" {
		return nil
	}
	r.events.mu.Lock()
	defer r.events.mu.Unlock()
	r.events.sequence++
	return json.NewEncoder(r.out).Encode(event{
		SchemaVersion: schemaVersion,
		Event:         kind,
		Command:       command,
		Sequence:      r.events.sequence,
		OK:            ok,
		Data:          data,
		Error:         coded,
	})
}

func (r runtime) interactiveOutput() io.Writer {
	if r.json {
		return nil
	}
	return r.err
}

// download returns the shared download-progress callback for human mode, or
// nil in JSON mode so platform code never renders terminal UI there.
func (r runtime) download(label string) func(downloaded, total int64) {
	if r.json || r.terminal == nil {
		return nil
	}
	return r.terminal.Download(label)
}

func (r runtime) sdkManagerOutput() io.Writer {
	if r.json {
		return nil
	}
	return newSDKManagerOutput(r.err)
}

func takeJSON(args []string) ([]string, bool) {
	clean, jsonOutput, _ := takeOutput(args)
	return clean, jsonOutput
}

func takeOutput(args []string) ([]string, bool, bool) {
	result := make([]string, 0, len(args))
	jsonOutput := false
	eventMode := false
	helpCommand := len(args) > 0 && args[0] == "help"
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			result = append(result, args[index:]...)
			break
		}
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		if arg == "--json=events" {
			jsonOutput = true
			eventMode = true
			continue
		}
		if strings.HasPrefix(arg, "--format=") && helpCommand {
			jsonOutput = strings.TrimPrefix(arg, "--format=") == "json"
			result = append(result, arg)
			continue
		}
		if arg == "--format" && index+1 < len(args) && helpCommand {
			jsonOutput = args[index+1] == "json"
			result = append(result, arg, args[index+1])
			index++
			continue
		}
		result = append(result, arg)
	}
	return result, jsonOutput, eventMode
}
func commandName(args []string) string {
	if len(args) == 0 {
		return "mob"
	}
	return "mob " + strings.Join(args, " ")
}
func writeFailure(run runtime, command string, err error) int {
	var coded *codedError
	if !errors.As(err, &coded) {
		coded = &codedError{Code: "MOB_INTERNAL_ERROR", Message: err.Error()}
	}
	if run.json {
		_ = run.emit("error", command, false, nil, coded)
	} else {
		fmt.Fprintf(run.err, "mob: %s\n", coded.Message)
		if coded.Remediation != "" {
			fmt.Fprintf(run.err, "%s\n", coded.Remediation)
		}
	}
	return 1
}
func invalidCommand(command string) error {
	return &codedError{Code: "MOB_INVALID_COMMAND", Message: "Unknown or invalid command: " + command, Remediation: "Run mob help for available commands, or mob help <command> for command-specific usage; use --json for machine output."}
}
func importOptions(args []string) (string, string, error) {
	var path, name string
	for len(args) > 0 {
		switch args[0] {
		case "--path":
			if len(args) < 2 {
				return "", "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--path requires a value."}
			}
			path = args[1]
			args = args[2:]
		case "--name":
			if len(args) < 2 {
				return "", "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--name requires a value."}
			}
			name = args[1]
			args = args[2:]
		default:
			return "", "", invalidCommand("mob android sdk import " + strings.Join(args, " "))
		}
	}
	if path == "" {
		return "", "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--path is required.", Remediation: "Pass the root of an existing Android SDK."}
	}
	return path, name, nil
}

var sdkName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func validName(value string) bool { return sdkName.MatchString(value) }
func generatedName(path string, entries []state.AndroidSDK) string {
	base := strings.ToLower(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(filepath.Base(path), "-"))
	base = strings.Trim(base, "-")
	if base == "" {
		base = "imported"
	}
	candidate := base
	for suffix := 2; ; suffix++ {
		exists := false
		for _, entry := range entries {
			if entry.Name == candidate {
				exists = true
				break
			}
		}
		if !exists {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
}
