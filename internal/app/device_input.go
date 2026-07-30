package app

import (
	"context"
	"github.com/xy200303/MobBase/internal/platform/android"
	"strconv"
)

func (r runtime) deviceInput(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob device input tap <x> <y>, swipe <x1> <y1> <x2> <y2> [--duration <ms>], text <value>, or key <keycode>."}
	}
	command := args[0]
	input := []string{command}
	switch command {
	case "tap":
		if len(args) != 3 || !coordinates(args[1:]) {
			return invalidDeviceInput()
		}
		input = append(input, args[1:]...)
	case "swipe":
		if len(args) != 5 && len(args) != 7 {
			return invalidDeviceInput()
		}
		if !coordinates(args[1:5]) || (len(args) == 7 && (args[5] != "--duration" || !positive(args[6]))) {
			return invalidDeviceInput()
		}
		input = append(input, args[1:5]...)
		if len(args) == 7 {
			input = append(input, args[6])
		}
	case "text", "key":
		if len(args) != 2 {
			return invalidDeviceInput()
		}
		input = append(input, args[1])
	default:
		return invalidDeviceInput()
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
	if err := android.Input(ctx, sdks, device.NativeID, input); err != nil {
		return androidCommandError(err, "Verify the selected Android device remains ready.")
	}
	if r.json {
		return r.result("mob device input", map[string]interface{}{"device": device, "input": input})
	}
	return nil
}
func coordinates(values []string) bool {
	for _, v := range values {
		if !positive(v) {
			return false
		}
	}
	return true
}
func positive(value string) bool { _, err := strconv.Atoi(value); return err == nil }
func invalidDeviceInput() error {
	return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Invalid device input. Run mob device input --help."}
}
