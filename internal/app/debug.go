package app

import (
	"context"
	"fmt"
	"time"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/system"
)

// debug starts a native Android debug build and emits a JDWP target that a
// VS Code Java/Kotlin debug extension can attach to.
func (r runtime) debug(ctx context.Context, args []string) error {
	options, err := parseRun(args)
	if err != nil {
		return err
	}
	currentProject, err := project.Detect("")
	if err != nil {
		return err
	}
	if currentProject == nil {
		return &codedError{Code: "MOB_PROJECT_UNRECOGNIZED", Message: "The current directory is not a supported mobile project.", Remediation: "Open a supported project directory in your terminal."}
	}
	if currentProject.Kind == project.KindFlutter {
		return r.runAs(ctx, args, "mob debug")
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	platform, err := selectRunPlatform(currentProject, options, config.Device.DefaultID)
	if err != nil {
		return err
	}
	if platform != "android" || currentProject.Kind != project.KindAndroid {
		return &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: "Native Android is the only non-Flutter debug adapter implemented in this Mob release.", Remediation: "Use --platform android with a native Android Gradle project, or use the framework's official debugger."}
	}
	sdk, requirements, err := r.prepareAndroidSDK(ctx, currentProject.Root, "mob debug", true, options.NoInstall, options.AcceptLicenses)
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
	if err := r.emit("started", "mob debug", true, map[string]interface{}{"phase": "debug", "platform": "android", "project": currentProject.Root, "sdk": sdk.Name}, nil); err != nil {
		return err
	}
	devices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return androidCommandError(err, "Verify that platform-tools is installed in the selected Android SDK.")
	}
	device, err := selectAndroidRunDevice(devices, options.Device, config.Device.DefaultID)
	if err != nil {
		if options.Device != "" || options.NoDeviceCreate {
			return err
		}
		sdk, err = r.prepareAndroidEmulator(ctx, sdk, requirements, "mob debug", options.NoInstall, options.AcceptLicenses)
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
		device, err = r.createAndStartDefaultEmulator(ctx, sdks, sdk, requirements.CompileSDK, options.Headless, "mob debug")
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
		if _, err := r.startAndroidMirror(ctx, device.NativeID, "mob debug"); err != nil {
			return err
		}
		if err := r.emit("preview", "mob debug", true, map[string]interface{}{"kind": "physical-mirror", "device": device, "client": "scrcpy"}, nil); err != nil {
			return err
		}
	}
	program, commandArgs, _, _, err := runCommand(currentProject.Root, options.Command)
	if err != nil {
		return err
	}
	program, commandArgs = replaceRunTokens(program, commandArgs, device)
	program, commandArgs = system.BatchCommand(program, commandArgs...)
	environment := append(androidEnvironment(sdk), javaEnvironment(java)...)
	environment = append(environment, "ANDROID_SERIAL="+device.NativeID)
	result, commandErr := r.executeWorkflowCommand(ctx, "mob debug", program, commandArgs, environment, currentProject.Root)
	if result.Output != "" {
		if r.json {
			if err := r.emit("log", "mob debug", true, map[string]string{"stream": "combined", "output": result.Output}, nil); err != nil {
				return err
			}
		} else {
			fmt.Fprint(r.out, result.Output)
		}
	}
	if commandErr != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Android debug build failed: " + commandErr.Error(), Remediation: "Review the Gradle output, selected device, and Android SDK."}
	}
	applicationID, err := project.AndroidApplicationID(currentProject.Root)
	if err != nil {
		return &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: err.Error(), Remediation: "Declare applicationId in the Android app Gradle module before using mob debug."}
	}
	if err := android.SetDebugApp(ctx, sdks, device.NativeID, applicationID); err != nil {
		return androidCommandError(err, "Verify the app package and that the selected Android device remains ready.")
	}
	if err := android.LaunchPackage(ctx, sdks, device.NativeID, applicationID); err != nil {
		return androidCommandError(err, "Verify the app package and that the selected Android device remains ready.")
	}
	jdwpContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pid, err := android.WaitForJDWPProcess(jdwpContext, sdks, device.NativeID, applicationID)
	if err != nil {
		return androidCommandError(err, "Ensure the debug build is debuggable and retry.")
	}
	port, err := android.ForwardJDWP(ctx, sdks, device.NativeID, pid)
	if err != nil {
		return androidCommandError(err, "Verify that ADB can forward a local port to the selected device, then retry.")
	}
	target := androidJDWPTarget(device, applicationID, pid, port)
	if err := r.emit("debugTarget", "mob debug", true, target, nil); err != nil {
		return err
	}
	if r.json {
		return r.result("mob debug", target)
	}
	fmt.Fprintf(r.out, "Android debug target ready: JDWP PID %d at 127.0.0.1:%d for %s.\n", pid, port, device.ID)
	return nil
}

func androidJDWPTarget(device android.Device, applicationID string, pid, port int) map[string]interface{} {
	return map[string]interface{}{
		"platform":  "android",
		"transport": "jdwp",
		"host":      "127.0.0.1",
		"port":      port,
		"device":    device,
		"package":   applicationID,
		"pid":       pid,
	}
}
