package app

import (
	"fmt"
	"strings"
)

func (r runtime) env(args []string) error {
	if len(args) != 1 || args[0] != "show" {
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
	if r.json {
		return r.result("mob env show", data)
	}
	fmt.Fprintf(r.out, "MOB_HOME=%s\n", r.home)
	fmt.Fprintf(r.out, "Android SDK=%s\nJava=%s\nFlutter=%s\nDefault device=%s\n", config.Android.CurrentSDK, config.Java.CurrentSDK, config.Flutter.CurrentSDK, config.Device.DefaultID)
	fmt.Fprintln(r.out, "Mob only injects ANDROID_SDK_ROOT, ANDROID_HOME, JAVA_HOME, PATH, and ANDROID_SERIAL into its child processes.")
	return nil
}
