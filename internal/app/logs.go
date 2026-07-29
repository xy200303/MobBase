package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
)

func (r runtime) logs(ctx context.Context, args []string) error {
	options, err := parseLogs(args)
	if err != nil {
		return err
	}
	currentProject, err := project.Detect("")
	if err != nil {
		return err
	}
	if currentProject == nil || (currentProject.Kind != project.KindAndroid && currentProject.Kind != project.KindFlutter) {
		return &codedError{Code: "MOB_PROJECT_UNRECOGNIZED", Message: "Open a native Android or Flutter project before reading Android logs."}
	}
	applicationID, err := project.AndroidApplicationID(currentProject.Root)
	if err != nil {
		return &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: err.Error(), Remediation: "Declare applicationId in the Android app Gradle module."}
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
	device, err := selectAndroidRunDevice(devices, options.Device, config.Device.DefaultID)
	if err != nil {
		return err
	}
	if options.Follow {
		if r.json && !r.eventMode {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "mob logs --follow requires --json=events for machine output.", Remediation: "Use mob logs --follow --json=events, or omit --json for interactive terminal output."}
		}
		if r.json {
			if err := r.emit("started", "mob logs", true, map[string]interface{}{"phase": "follow-logs", "platform": "android", "device": device, "package": applicationID}, nil); err != nil {
				return err
			}
			pid, err := android.FollowPackageLogsLines(ctx, sdks, device.NativeID, applicationID, func(line string) error {
				return r.emit("log", "mob logs", true, map[string]interface{}{"stream": "stdout", "output": line, "device": device.ID, "package": applicationID}, nil)
			})
			if err != nil {
				return androidCommandError(err, "Start the application on the selected device, then retry.")
			}
			return r.result("mob logs", map[string]interface{}{"platform": "android", "device": device, "package": applicationID, "pid": pid, "followed": true})
		}
		_, err := android.FollowPackageLogs(ctx, sdks, device.NativeID, applicationID, r.out)
		if err != nil {
			return androidCommandError(err, "Start the application on the selected device, then retry.")
		}
		return nil
	}
	output, pid, err := android.PackageLogs(ctx, sdks, device.NativeID, applicationID)
	if err != nil {
		return androidCommandError(err, "Start the application on the selected device, then retry.")
	}
	data := map[string]interface{}{"platform": "android", "device": device, "package": applicationID, "pid": pid, "output": output}
	if r.json {
		return r.result("mob logs", data)
	}
	fmt.Fprint(r.out, output)
	return nil
}

type logsOptions struct {
	Device string
	Follow bool
}

func parseLogs(args []string) (logsOptions, error) {
	var options logsOptions
	for len(args) > 0 {
		if args[0] == "--follow" {
			options.Follow = true
			args = args[1:]
			continue
		}
		if len(args) >= 2 && args[0] == "--device" && strings.TrimSpace(args[1]) != "" {
			options.Device = args[1]
			args = args[2:]
			continue
		}
		return logsOptions{}, invalidCommand("mob logs " + strings.Join(args, " "))
	}
	return options, nil
}
