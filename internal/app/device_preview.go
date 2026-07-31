package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/android"
)

// devicePreviewServe owns one loopback H.264/control session. It intentionally
// requires the event protocol so callers receive the endpoint and credential
// before the long-running command waits for the preview to close.
func (r runtime) devicePreviewServe(ctx context.Context, args []string) error {
	if len(args) != 2 || args[0] != "serve" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob device preview serve <platform:native-id> --json=events."}
	}
	platform, nativeID, found := strings.Cut(args[1], ":")
	if !found || strings.TrimSpace(platform) == "" || strings.TrimSpace(nativeID) == "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob device preview serve <platform:native-id> --json=events."}
	}
	if r.json && !r.eventMode {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "mob device preview serve requires --json=events.", Remediation: "Use mob device preview serve <platform:native-id> --json=events."}
	}
	if platform != "android" {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: fmt.Sprintf("Device preview is not available for %s yet.", platform), Remediation: "Android is currently the only supported device preview platform."}
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
	device, err := findReadyAndroidDevice(devices, args[1], true)
	if err != nil {
		return err
	}
	_, server, version, err := r.androidPreviewRuntime(ctx, "mob device preview serve")
	if err != nil {
		return err
	}
	session, err := android.StartPreview(ctx, sdks, device.NativeID, server, version)
	if err != nil {
		return androidCommandError(err, "Verify the selected Android device remains ready and authorizes ADB screen capture.")
	}
	defer session.Close()
	metadata := session.Metadata()
	data := map[string]interface{}{
		"device": device, "protocol": metadata.Protocol, "platform": metadata.Platform, "deviceId": metadata.DeviceID, "endpoint": metadata.Endpoint,
		"token": metadata.Token, "video": metadata.Video, "controls": metadata.Controls,
	}
	if r.json {
		if err := r.emit("preview", "mob device preview serve", true, data, nil); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(r.out, "Android preview service started for %s. Use a compatible Mob client to connect.\n", device.ID)
	}
	if err := session.Wait(); err != nil {
		return androidCommandError(err, "Reconnect the device and start a new preview session.")
	}
	return nil
}
