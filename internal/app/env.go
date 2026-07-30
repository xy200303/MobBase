package app

import (
	"context"
	"fmt"
	"os"
	goruntime "runtime"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/android"
)

func (r runtime) env(args []string) error {
	shell, show, err := parseEnv(args)
	if err != nil {
		return err
	}
	if !show {
		return invalidCommand("mob env " + strings.Join(args, " "))
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	data := map[string]interface{}{
		"mobHome":       r.home,
		"androidSdk":    config.Android.CurrentSDK,
		"java":          config.Java.CurrentSDK,
		"flutter":       config.Flutter.CurrentSDK,
		"defaultDevice": config.Device.DefaultID,
		"scope":         "child-process-only",
	}
	if shell != "" {
		sdks, err := android.Discover(config)
		if err != nil {
			return err
		}
		sdk, found := selectAndroidBuildSDK(sdks)
		if !found {
			return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No Android SDK is available to export.", Remediation: "Run mob android sdk list, then install or import an Android SDK."}
		}
		javaSDKs, err := discoverJava(context.Background(), config)
		if err != nil || len(javaSDKs) == 0 {
			return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No JDK is available to export.", Remediation: "Run mob java list, then install or import a JDK."}
		}
		java, err := r.selectProjectJava(context.Background(), 0, true)
		if err != nil {
			return err
		}
		environment := map[string]string{"ANDROID_SDK_ROOT": sdk.Path, "ANDROID_HOME": sdk.Path, "JAVA_HOME": java.Path, "PATH": fmt.Sprintf("%s%c%s", java.Path+string(os.PathSeparator)+"bin", os.PathListSeparator, os.Getenv("PATH"))}
		data["shell"] = shell
		data["environment"] = environment
		if r.json {
			return r.result("mob env", data)
		}
		return writeShellEnvironment(r.out, shell, environment)
	}
	if r.json {
		return r.result("mob env show", data)
	}
	fmt.Fprintf(r.out, "MOB_HOME=%s\n", r.home)
	fmt.Fprintf(r.out, "Android SDK=%s\nJava=%s\nFlutter=%s\nDefault device=%s\n", config.Android.CurrentSDK, config.Java.CurrentSDK, config.Flutter.CurrentSDK, config.Device.DefaultID)
	fmt.Fprintln(r.out, "Mob only injects ANDROID_SDK_ROOT, ANDROID_HOME, JAVA_HOME, PATH, and ANDROID_SERIAL into its child processes.")
	return nil
}

func parseEnv(args []string) (shell string, show bool, err error) {
	if len(args) == 1 && args[0] == "show" {
		return "", true, nil
	}
	if len(args) == 2 && args[0] == "show" && args[1] == "--shell" {
		return defaultEnvShell(), true, nil
	}
	if len(args) == 3 && args[0] == "show" && args[1] == "--shell" {
		shell = args[2]
	} else if len(args) == 2 && args[0] == "--shell" {
		shell = args[1]
	}
	if shell != "" && (shell == "sh" || shell == "powershell" || shell == "cmd") {
		return shell, true, nil
	}
	return "", false, invalidCommand("mob env " + strings.Join(args, " "))
}

func defaultEnvShell() string {
	if goruntime.GOOS == "windows" {
		return "powershell"
	}
	return "sh"
}

func writeShellEnvironment(out interface{ Write([]byte) (int, error) }, shell string, environment map[string]string) error {
	for _, key := range []string{"ANDROID_SDK_ROOT", "ANDROID_HOME", "JAVA_HOME", "PATH"} {
		value := environment[key]
		switch shell {
		case "powershell":
			fmt.Fprintf(out, "$env:%s = '%s'\n", key, strings.ReplaceAll(value, "'", "''"))
		case "cmd":
			fmt.Fprintf(out, "set \"%s=%s\"\n", key, value)
		default:
			fmt.Fprintf(out, "export %s='%s'\n", key, strings.ReplaceAll(value, "'", "'\\\"'\\\"'"))
		}
	}
	return nil
}
