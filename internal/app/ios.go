package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/ios"
)

func (r runtime) ios(ctx context.Context, args []string) error {
	if len(args) == 3 && args[0] == "simulator" && args[1] == "start" {
		return r.iosSimulatorStart(ctx, args[2])
	}
	if len(args) != 1 || args[0] != "doctor" {
		return invalidCommand("mob ios " + strings.Join(args, " "))
	}
	toolchain, err := r.iosToolchain(ctx, "mob ios doctor")
	if err != nil {
		return err
	}
	if r.json {
		return r.result("mob ios doctor", toolchain)
	}
	fmt.Fprintf(r.out, "iOS toolchain is ready.\nDeveloper directory: %s\nXcode: %s (build %s)\n", toolchain.DeveloperDir, toolchain.Version, toolchain.BuildVersion)
	return nil
}

func (r runtime) iosSimulatorStart(ctx context.Context, id string) error {
	rawID := strings.TrimSpace(id)
	if !strings.HasPrefix(rawID, "ios:") {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob ios simulator start ios:<simulator-udid>.", Remediation: "Run mob device list --platform ios and copy a Simulator ID."}
	}
	nativeID := strings.TrimPrefix(rawID, "ios:")
	if nativeID == "" || strings.Contains(nativeID, ":") {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob ios simulator start ios:<simulator-udid>.", Remediation: "Run mob device list --platform ios and copy a Simulator ID."}
	}
	if _, err := r.iosToolchain(ctx, "mob ios simulator start"); err != nil {
		return err
	}
	devices, err := ios.ListDevices(ctx)
	if err != nil {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: err.Error(), Remediation: "Install and select Xcode, then rerun mob ios doctor."}
	}
	var device ios.Device
	found := false
	for _, item := range devices {
		if item.NativeID == nativeID {
			device, found = item, true
			break
		}
	}
	if !found {
		return &codedError{Code: "MOB_DEVICE_UNAVAILABLE", Message: "iOS Simulator " + id + " is not available.", Remediation: "Run mob device list --platform ios, then choose an available Simulator."}
	}
	if err := ios.StartSimulator(ctx, nativeID); err != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Verify the selected Simulator and active Xcode installation, then rerun mob ios simulator start."}
	}
	if r.json {
		return r.result("mob ios simulator start", map[string]interface{}{"device": device, "started": device.State != "ready"})
	}
	fmt.Fprintf(r.out, "Opened iOS Simulator %s.\n", device.ID)
	return nil
}

func (r runtime) iosToolchain(ctx context.Context, command string) (ios.Toolchain, error) {
	toolchain, err := ios.Discover(ctx)
	if err == nil {
		return toolchain, nil
	}
	switch {
	case errors.Is(err, ios.ErrHostUnsupported):
		return ios.Toolchain{}, &codedError{Code: "MOB_HOST_UNSUPPORTED", Message: command + " requires macOS with Xcode.", Remediation: "Run this command on macOS after installing and accepting the Xcode license."}
	case errors.Is(err, ios.ErrToolchainMissing):
		return ios.Toolchain{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: err.Error(), Remediation: "Install Xcode through Apple, select it with xcode-select, and accept its license before rerunning " + command + "."}
	default:
		return ios.Toolchain{}, err
	}
}
