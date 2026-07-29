package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/xy200303/MobBase/internal/platform/android"
)

func TestDebugUsesRunOptionContract(t *testing.T) {
	options, err := parseRun([]string{"--device", "android:emulator-5554", "--mirror", "--no-install", "--accept-licenses"})
	if err != nil {
		t.Fatalf("parse debug options: %v", err)
	}
	if options.Device != "android:emulator-5554" || !options.Mirror || !options.NoInstall || !options.AcceptLicenses {
		t.Fatalf("unexpected debug options: %#v", options)
	}
}

func TestAndroidJDWPTargetUsesLoopbackEndpoint(t *testing.T) {
	target := androidJDWPTarget(android.Device{ID: "android:emulator-5554", NativeID: "emulator-5554"}, "com.example.notes", 2418, 41234)
	if target["transport"] != "jdwp" || target["host"] != "127.0.0.1" || target["port"] != 41234 || target["pid"] != 2418 || target["package"] != "com.example.notes" {
		t.Fatalf("unexpected JDWP target: %#v", target)
	}
	device, ok := target["device"].(android.Device)
	if !ok || device.ID != "android:emulator-5554" {
		t.Fatalf("unexpected target device: %#v", target["device"])
	}
}

func TestDebugOptionErrorsUseDebugCommand(t *testing.T) {
	_, err := parseRunAs([]string{"--unknown"}, "mob debug")
	var coded *codedError
	if !errors.As(err, &coded) || coded.Code != "MOB_INVALID_COMMAND" {
		t.Fatalf("unexpected error: %v", err)
	}
	if coded.Message != "Unknown or invalid command: mob debug --unknown" {
		t.Fatalf("unexpected debug command message: %q", coded.Message)
	}
}

func TestParseDeviceForwardRemove(t *testing.T) {
	options, err := parseDeviceForwardRemove([]string{"remove", "android:emulator-5554", "--port", "41234"})
	if err != nil || options.DeviceID != "android:emulator-5554" || options.Port != 41234 {
		t.Fatalf("unexpected options: %#v, %v", options, err)
	}
	if _, err := parseDeviceForwardRemove([]string{"remove", "android:emulator-5554", "--port", "0"}); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestAndroidPairRejectsInvalidCodeAsArgumentError(t *testing.T) {
	t.Setenv("MOB_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"android", "device", "pair", "192.168.1.20:37123", "--code", "123", "--json"}, &stdout, &stderr); exit == 0 {
		t.Fatal("expected pairing command to fail")
	}
	var event map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &event); err != nil {
		t.Fatalf("decode JSON error event: %v (%s)", err, stdout.String())
	}
	errorData, ok := event["error"].(map[string]interface{})
	if !ok || errorData["code"] != "MOB_INVALID_ARGUMENT" {
		t.Fatalf("unexpected pairing error: %#v", event)
	}
}
