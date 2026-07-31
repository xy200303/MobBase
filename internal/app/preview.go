package app

import (
	"context"
	"path/filepath"

	"github.com/xy200303/MobBase/internal/platform/android"
	scrcpyplatform "github.com/xy200303/MobBase/internal/platform/scrcpy"
)

// startAndroidMirror resolves Mob's internal preview runtime. It is not a
// user-managed toolchain and is automatically prepared when preview is used.
func (r runtime) startAndroidMirror(ctx context.Context, nativeID, command string) (string, error) {
	client, _, _, err := r.androidPreviewRuntime(ctx, command)
	if err != nil {
		return "", err
	}
	if err := android.MirrorDeviceWithClient(ctx, nativeID, client); err != nil {
		return "", &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Check USB or Wireless debugging authorization and reconnect the Android device."}
	}
	return client, nil
}

// androidPreviewRuntime keeps scrcpy as a Mob-internal runtime and verifies
// that both its native client and server are available before use.
func (r runtime) androidPreviewRuntime(ctx context.Context, command string) (string, string, string, error) {
	client, clientFound := scrcpyplatform.RuntimeExecutable(r.home)
	server, serverFound := scrcpyplatform.RuntimeServer(r.home)
	if !clientFound || !serverFound {
		release, err := scrcpyplatform.LatestRelease(ctx)
		if err != nil {
			return "", "", "", &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Check access to the official Genymobile scrcpy release, then retry the preview command."}
		}
		if err := r.emit("progress", command, true, map[string]interface{}{"phase": "bootstrap-scrcpy", "version": release.Version, "size": release.Size}, nil); err != nil {
			return "", "", "", err
		}
		var report func(scrcpyplatform.Progress)
		if callback := r.download("Downloading Android device preview"); callback != nil {
			report = func(progress scrcpyplatform.Progress) {
				callback(progress.Downloaded, progress.Total)
			}
		}
		if err := scrcpyplatform.Install(ctx, scrcpyplatform.RuntimeRoot(r.home), filepath.Join(r.home, "cache", "downloads"), release, report); err != nil {
			return "", "", "", &codedError{Code: "MOB_COMMAND_FAILED", Message: "Install Android device preview runtime: " + err.Error(), Remediation: "Check free disk space and access to the official Genymobile scrcpy release, then retry."}
		}
		client, clientFound = scrcpyplatform.RuntimeExecutable(r.home)
		server, serverFound = scrcpyplatform.RuntimeServer(r.home)
		if !clientFound || !serverFound {
			return "", "", "", &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Mob installed the Android device preview runtime but its client or server component was not found.", Remediation: "Run the preview command again after checking the Mob runtime directory."}
		}
	}
	version, err := scrcpyplatform.RuntimeVersion(ctx, client)
	if err != nil {
		return "", "", "", &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Reinstall Mob's Android device preview runtime, then retry."}
	}
	return client, server, version, nil
}
