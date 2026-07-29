package android

import (
	"context"
	"testing"
)

func TestParseDevices(t *testing.T) {
	devices := ParseDevices("List of devices attached\nemulator-5554 device product:sdk model:Pixel_8 device:emu64xa transport_id:1\nR58N123456A unauthorized usb:1-2 transport_id:2\n")
	if len(devices) != 2 {
		t.Fatalf("device count = %d, want 2", len(devices))
	}
	if devices[0].ID != "android:emulator-5554" || devices[0].Kind != "emulator" || devices[0].State != "ready" || devices[0].Name != "Pixel 8" {
		t.Fatalf("unexpected emulator: %#v", devices[0])
	}
	if devices[1].Kind != "physical" || devices[1].State != "unauthorized" {
		t.Fatalf("unexpected physical device: %#v", devices[1])
	}
}

func TestParseEmulators(t *testing.T) {
	emulators := ParseEmulators("mob-android-api-35\n\npixel-api-27\nmob-android-api-35\n")
	if len(emulators) != 2 {
		t.Fatalf("emulator count = %d, want 2", len(emulators))
	}
	if emulators[0].Name != "mob-android-api-35" || emulators[1].Name != "pixel-api-27" {
		t.Fatalf("unexpected emulators: %#v", emulators)
	}
}

func TestPairDeviceRejectsInvalidCodeBeforeLookingUpADB(t *testing.T) {
	_, err := PairDevice(context.Background(), nil, "192.168.1.20:37123", "123")
	if err == nil || err.Error() != "Android pairing code must contain exactly 6 digits" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultEmulatorName(t *testing.T) {
	name := DefaultEmulatorName("system-images;android-35;google_apis;x86_64")
	if name != "mob-android-api-35" {
		t.Fatalf("default AVD name = %q", name)
	}
}

func TestReadyEmulatorNotIn(t *testing.T) {
	devices := []Device{
		{ID: "android:emulator-5554", Kind: "emulator", State: "ready"},
		{ID: "android:emulator-5556", Kind: "emulator", State: "offline"},
		{ID: "android:emulator-5558", Kind: "emulator", State: "ready"},
	}
	excluded := map[string]struct{}{"android:emulator-5554": {}}
	device, found := readyEmulatorNotIn(devices, excluded)
	if !found || device.ID != "android:emulator-5558" {
		t.Fatalf("device = %#v, found = %t", device, found)
	}
}

func TestReadyEmulatorCanReusePreviouslyOfflineID(t *testing.T) {
	known := []Device{{ID: "android:emulator-5554", Kind: "emulator", State: "offline"}}
	device, found := readyEmulatorNotIn([]Device{{ID: "android:emulator-5554", Kind: "emulator", State: "ready"}}, readyEmulatorIDs(known))
	if !found || device.ID != "android:emulator-5554" {
		t.Fatalf("device = %#v, found = %t", device, found)
	}
}
