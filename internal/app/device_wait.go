package app

import (
	"context"
	"time"

	"github.com/xy200303/MobBase/internal/platform/android"
)

func (r runtime) deviceWait(ctx context.Context, args []string) error {
	if len(args) != 1 || (args[0] != "--boot" && args[0] != "--idle") {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob device wait --boot or mob device wait --idle."}
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
	device, err := selectAndroidRunDevice(devices, "", config.Device.DefaultID)
	if err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if args[0] == "--boot" {
		err = android.WaitForBoot(waitCtx, sdks, device.NativeID)
	} else {
		err = android.WaitForIdle(waitCtx, sdks, device.NativeID)
	}
	if err != nil {
		return androidCommandError(err, "Verify the selected Android device remains connected and ready.")
	}
	if r.json {
		return r.result("mob device wait", map[string]interface{}{"device": device, "state": args[0][2:]})
	}
	return nil
}
