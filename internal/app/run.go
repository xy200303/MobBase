package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/system"
)

type runOptions struct {
	Platform       string
	Device         string
	Command        []string
	NoDeviceCreate bool
	NoInstall      bool
	AcceptLicenses bool
	Mirror         bool
	Headless       bool
}

func (r runtime) run(ctx context.Context, args []string) error {
	return r.runAs(ctx, args, "mob run")
}

// runAs shares Android and Flutter launch preparation with workflows that
// delegate to their official run command while retaining their own CLI and
// JSON event identity.
func (r runtime) runAs(ctx context.Context, args []string, command string) error {
	options, err := parseRunAs(args, command)
	if err != nil {
		return err
	}
	currentProject, err := project.Detect("")
	if err != nil {
		return err
	}
	if currentProject == nil {
		return &codedError{Code: "MOB_PROJECT_UNRECOGNIZED", Message: "The current directory is not a supported mobile project.", Remediation: "Run mob status, or open a supported project directory in your terminal."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	platform, err := selectRunPlatform(currentProject, options, config.Device.DefaultID)
	if err != nil {
		return err
	}
	if platform != "android" {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "Android is the only run platform implemented in this Mob release.", Remediation: "Choose --platform android for a project that declares an Android target."}
	}
	if currentProject.Kind != project.KindAndroid && currentProject.Kind != project.KindFlutter && currentProject.Kind != project.KindKotlinMultiplatform {
		return &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: "The " + string(currentProject.Kind) + " run adapter is not available yet.", Remediation: "Use the framework's official runner for now, or run a native Android Gradle project with mob run."}
	}
	var androidApplication *project.AndroidApplication
	if currentProject.Kind == project.KindKotlinMultiplatform && len(options.Command) == 0 {
		application, err := project.KotlinMultiplatformAndroidApplication(currentProject.Root)
		if err != nil {
			return &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: err.Error(), Remediation: "Include exactly one Android application module with com.android.application and an explicit applicationId, or pass the project's official run command after --."}
		}
		androidApplication = &application
	}
	sdk, requirements, err := r.prepareAndroidSDK(ctx, currentProject.Root, command, true, options.NoInstall, options.AcceptLicenses)
	if err != nil {
		return err
	}
	java, err := r.selectProjectJava(ctx, requirements.JavaVersion, options.NoInstall)
	if err != nil {
		return err
	}
	config, err = r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	if err := r.emit("started", command, true, map[string]interface{}{"phase": "run", "platform": "android", "project": currentProject.Root, "sdk": sdk.Name}, nil); err != nil {
		return err
	}
	if currentProject.Kind == project.KindFlutter && len(options.Command) == 0 {
		if _, err := r.ensureFlutterRunner(ctx, currentProject.Root, options.NoInstall, command); err != nil {
			return err
		}
	}
	devices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return androidCommandError(err, "Run mob android sdk inspect <name> and ensure platform-tools is installed.")
	}
	device, err := selectAndroidRunDevice(devices, options.Device, config.Device.DefaultID)
	if err != nil {
		if options.Device != "" || options.NoDeviceCreate {
			return err
		}
		sdk, err = r.prepareAndroidEmulator(ctx, sdk, requirements, command, options.NoInstall, options.AcceptLicenses)
		if err != nil {
			return err
		}
		config, err = r.store.Load()
		if err != nil {
			return err
		}
		sdks, err = android.Discover(config)
		if err != nil {
			return err
		}
		device, err = r.createAndStartDefaultEmulator(ctx, sdks, sdk, requirements.CompileSDK, options.Headless, command)
		if err != nil {
			return err
		}
		config.Device.DefaultID = device.ID
		if err := r.store.Save(config); err != nil {
			return err
		}
	}
	if options.Mirror {
		if device.Kind != "physical" {
			return &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: "--mirror requires a connected physical Android device.", Remediation: "Use the official Emulator window without --mirror, or choose a physical device with --device."}
		}
		if _, err := r.startAndroidMirror(ctx, device.NativeID, command); err != nil {
			return err
		}
		if err := r.emit("preview", command, true, map[string]interface{}{"kind": "physical-mirror", "device": device, "client": "scrcpy"}, nil); err != nil {
			return err
		}
	}
	program, commandArgs, launchesApplication, applicationID, err := runProjectCommand(currentProject, options.Command, device.NativeID, androidApplication)
	if err != nil {
		return err
	}
	machineFlutterDebug := shouldUseFlutterMachineDebug(command, currentProject, options, r.eventMode)
	if machineFlutterDebug {
		if len(options.Command) > 0 {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "mob debug --json=events for Flutter cannot forward a custom command.", Remediation: "Omit the command after -- so Mob can invoke flutter run --machine and report the Dart VM Service target."}
		}
		commandArgs = append(commandArgs, "--machine")
	}
	if launchesApplication {
		if androidApplication != nil {
			applicationID = androidApplication.ApplicationID
		} else {
			applicationID, err = project.AndroidApplicationID(currentProject.Root)
			if err != nil {
				return &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: err.Error(), Remediation: "Declare applicationId in the Android app Gradle module, or pass an explicit command after --."}
			}
		}
	}
	program, commandArgs = replaceRunTokens(program, commandArgs, device)
	program, commandArgs = system.BatchCommand(program, commandArgs...)
	environment := append(androidEnvironment(sdk), javaEnvironment(java)...)
	environment = append(environment, "ANDROID_SERIAL="+device.NativeID)
	var result system.CommandResult
	var commandErr error
	if machineFlutterDebug {
		handler := newFlutterMachineHandler(r, command, device)
		commandErr = system.StreamLines(ctx, program, commandArgs, environment, currentProject.Root, handler.stdout, handler.stderr)
	} else {
		result, commandErr = r.executeWorkflowCommand(ctx, command, program, commandArgs, environment, currentProject.Root)
	}
	if result.Output != "" {
		if r.json {
			if err := r.emit("log", command, true, map[string]string{"stream": "combined", "output": result.Output}, nil); err != nil {
				return err
			}
		} else {
			fmt.Fprint(r.out, result.Output)
		}
	}
	if commandErr != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Android run command failed: " + commandErr.Error(), Remediation: "Review the Gradle output, selected device, and Android SDK."}
	}
	if launchesApplication {
		if err := android.LaunchPackage(ctx, sdks, device.NativeID, applicationID); err != nil {
			return androidCommandError(err, "Verify the app package and that the selected Android device remains ready.")
		}
	}
	data := map[string]interface{}{"platform": "android", "project": currentProject.Root, "sdk": sdk.Name, "java": java, "device": device, "command": append([]string{program}, commandArgs...), "workflow": command}
	if r.json {
		return r.result(command, data)
	}
	fmt.Fprintf(r.out, "Android application is running on %s.\n", device.ID)
	return nil
}

func runProjectCommand(info *project.Info, forwarded []string, nativeDeviceID string, androidApplication *project.AndroidApplication) (string, []string, bool, string, error) {
	if info.Kind == project.KindFlutter {
		program, args, err := flutterRunCommand(info.Root, forwarded, nativeDeviceID)
		return program, args, false, "", err
	}
	if info.Kind == project.KindKotlinMultiplatform && androidApplication != nil && len(forwarded) == 0 {
		program, _, _, _, err := runCommand(info.Root, nil)
		if err != nil {
			return "", nil, false, "", err
		}
		return program, []string{androidApplication.Module + ":installDebug"}, true, androidApplication.ApplicationID, nil
	}
	return runCommand(info.Root, forwarded)
}

func parseRun(args []string) (runOptions, error) {
	return parseRunAs(args, "mob run")
}

func parseRunAs(args []string, command string) (runOptions, error) {
	options := runOptions{}
	for len(args) > 0 {
		if args[0] == "--" {
			if len(args) == 1 {
				return runOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "-- must be followed by an official run command."}
			}
			options.Command = append([]string(nil), args[1:]...)
			return options, nil
		}
		switch args[0] {
		case "--platform":
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				return runOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--platform requires a platform ID."}
			}
			options.Platform = args[1]
			args = args[2:]
		case "--device":
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				return runOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--device requires a device ID."}
			}
			options.Device = args[1]
			args = args[2:]
		case "--no-device-create":
			options.NoDeviceCreate = true
			args = args[1:]
		case "--no-install":
			options.NoInstall = true
			args = args[1:]
		case "--accept-licenses":
			options.AcceptLicenses = true
			args = args[1:]
		case "--mirror":
			options.Mirror = true
			args = args[1:]
		case "--headless":
			options.Headless = true
			args = args[1:]
		default:
			return runOptions{}, invalidCommand(command + " " + strings.Join(args, " "))
		}
	}
	return options, nil
}

func (r runtime) createAndStartDefaultEmulator(ctx context.Context, sdks []android.SDK, sdk android.SDK, compileSDK int, headless bool, command string) (android.Device, error) {
	image, found := android.SystemImageForAPI(sdk, compileSDK)
	if compileSDK == 0 {
		image, found = android.DefaultSystemImage(sdk)
	}
	if !found {
		return android.Device{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No Android system image is available to create an emulator.", Remediation: "Install a system image, or connect a physical Android device and rerun " + command + "."}
	}
	desiredAVD := android.DefaultEmulatorName(image)
	emulators, err := android.ListEmulators(ctx, sdks)
	if err != nil {
		return android.Device{}, androidCommandError(err, "Install the Android Emulator package, then retry "+command+".")
	}
	avd, exists := selectDefaultAndroidAVD(emulators, desiredAVD)
	if !exists {
		avd = desiredAVD
		if err := r.emit("progress", command, true, map[string]string{"phase": "create-device", "avd": avd, "image": image}, nil); err != nil {
			return android.Device{}, err
		}
		if err := android.CreateEmulator(ctx, sdk, avd, image); err != nil {
			return android.Device{}, androidCommandError(err, "Verify that the selected SDK contains the Emulator, command-line tools, and system image.")
		}
	}
	if err := r.emit("progress", command, true, map[string]interface{}{"phase": "start-device", "avd": avd, "headless": headless}, nil); err != nil {
		return android.Device{}, err
	}
	existingDevices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return android.Device{}, androidCommandError(err, "Run mob android sdk inspect <name> and ensure platform-tools is installed.")
	}
	if err := android.StartEmulatorWithOptions(ctx, sdks, avd, headless); err != nil {
		return android.Device{}, androidCommandError(err, "Run mob android emulator list and verify that the Emulator package is installed.")
	}
	if err := r.emit("progress", command, true, map[string]string{"phase": "wait-device", "avd": avd}, nil); err != nil {
		return android.Device{}, err
	}
	waitContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	device, err := android.WaitForNewReadyEmulator(waitContext, sdks, existingDevices)
	if err != nil {
		return android.Device{}, &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: err.Error(), Remediation: "Wait for the emulator to boot, then rerun " + command + " or choose a ready device with --device."}
	}
	return device, nil
}

// selectDefaultAndroidAVD only reuses the Mob-managed AVD for the project's
// selected system image. Other AVDs remain available through --device.
func selectDefaultAndroidAVD(emulators []android.Emulator, desired string) (string, bool) {
	for _, emulator := range emulators {
		if emulator.Name == desired {
			return emulator.Name, true
		}
	}
	return "", false
}

func selectRunPlatform(info *project.Info, options runOptions, defaultDevice string) (string, error) {
	requested := options.Platform
	if options.Device != "" {
		platform, _, found := strings.Cut(options.Device, ":")
		if !found || platform == "" {
			return "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--device must use the form <platform>:<native-id>."}
		}
		if requested != "" && requested != platform {
			return "", &codedError{Code: "MOB_PLATFORM_DEVICE_MISMATCH", Message: "--platform and --device point to different platforms.", Remediation: "Use an Android device with --platform android, or omit one of the options."}
		}
		requested = platform
	}
	if requested == "" && defaultDevice != "" {
		platform, _, found := strings.Cut(defaultDevice, ":")
		if found && targetDeclared(info, platform) {
			requested = platform
		}
	}
	return selectBuildPlatform(info, requested)
}

func selectAndroidRunDevice(devices []android.Device, explicit, defaultID string) (android.Device, error) {
	if explicit != "" {
		return findReadyAndroidDevice(devices, explicit, true)
	}
	if strings.HasPrefix(defaultID, "android:") {
		if device, err := findReadyAndroidDevice(devices, defaultID, false); err == nil {
			return device, nil
		}
	}
	for _, device := range devices {
		if device.State == "ready" {
			return device, nil
		}
	}
	return android.Device{}, &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: "No ready Android device is available.", Remediation: "Connect a device or start an existing AVD with mob android emulator start <avd-name>."}
}

func findReadyAndroidDevice(devices []android.Device, id string, explicit bool) (android.Device, error) {
	for _, device := range devices {
		if device.ID != id {
			continue
		}
		if device.State == "ready" {
			return device, nil
		}
		return android.Device{}, &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: "Device " + id + " is not ready.", Remediation: "Wait for the device to become ready, then try again."}
	}
	if explicit {
		return android.Device{}, &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: "Requested device " + id + " is not connected.", Remediation: "Run mob device list and choose a ready device."}
	}
	return android.Device{}, &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: "Default device " + id + " is not ready.", Remediation: "Run mob device list."}
}

func targetDeclared(info *project.Info, target string) bool {
	for _, declared := range info.Targets {
		if declared == target {
			return true
		}
	}
	return false
}

func runCommand(root string, forwarded []string) (string, []string, bool, string, error) {
	if len(forwarded) > 0 {
		return forwarded[0], forwarded[1:], false, "", nil
	}
	wrapper := "gradlew"
	if goruntime.GOOS == "windows" {
		wrapper = "gradlew.bat"
	}
	path := filepath.Join(root, wrapper)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", nil, false, "", &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: "Gradle Wrapper was not found in the Android project.", Remediation: "Restore gradlew/gradlew.bat from the project, or pass an explicit official run command after --."}
	}
	return path, []string{"installDebug"}, true, "", nil
}

func replaceRunTokens(program string, args []string, device android.Device) (string, []string) {
	replace := func(value string) string {
		switch value {
		case "{{mob.device.nativeId}}":
			return device.NativeID
		case "{{mob.device.id}}":
			return device.ID
		case "{{mob.platform}}":
			return device.Platform
		default:
			return value
		}
	}
	updated := make([]string, len(args))
	for index, value := range args {
		updated[index] = replace(value)
	}
	return replace(program), updated
}
