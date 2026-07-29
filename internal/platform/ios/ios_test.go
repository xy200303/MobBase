package ios

import "testing"

func TestParseVersion(t *testing.T) {
	toolchain, err := ParseVersion("Xcode 16.2\nBuild version 16C5032a\n")
	if err != nil {
		t.Fatalf("ParseVersion returned error: %v", err)
	}
	if toolchain.Version != "16.2" || toolchain.BuildVersion != "16C5032a" {
		t.Fatalf("unexpected toolchain: %#v", toolchain)
	}
}

func TestParseVersionRejectsIncompleteOutput(t *testing.T) {
	if _, err := ParseVersion("Xcode 16.2\n"); err == nil {
		t.Fatal("ParseVersion accepted incomplete output")
	}
}

func TestParseDevices(t *testing.T) {
	devices, err := ParseDevices(`{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-18-2":[{"udid":"A1","name":"iPhone 16","state":"Shutdown","isAvailable":true},{"udid":"B2","name":"iPhone 16 Pro","state":"Booted","isAvailable":true},{"udid":"C3","name":"Unavailable","state":"Shutdown","isAvailable":false}]}}`)
	if err != nil {
		t.Fatalf("ParseDevices returned error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("device count = %d: %#v", len(devices), devices)
	}
	if devices[0].ID != "ios:B2" || devices[0].State != "ready" || devices[0].Kind != "simulator" {
		t.Fatalf("unexpected first device: %#v", devices[0])
	}
	if devices[1].ID != "ios:A1" || devices[1].State != "shutdown" {
		t.Fatalf("unexpected second device: %#v", devices[1])
	}
}

func TestParseDevicesRejectsInvalidJSON(t *testing.T) {
	if _, err := ParseDevices("not JSON"); err == nil {
		t.Fatal("ParseDevices accepted invalid JSON")
	}
}
