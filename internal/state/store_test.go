package state

import (
	"path/filepath"
	"testing"
)

func TestStoreSaveReplacesExistingConfiguration(t *testing.T) {
	store := New(t.TempDir())
	first := Default()
	first.Android.CurrentSDK = "first"
	first.Device.DefaultID = "android:emulator-5554"
	if err := store.Save(first); err != nil {
		t.Fatalf("save first configuration: %v", err)
	}
	second := Default()
	second.Android.CurrentSDK = "second"
	second.Device.DefaultID = "android:emulator-5554"
	if err := store.Save(second); err != nil {
		t.Fatalf("replace configuration: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	if loaded.Android.CurrentSDK != "second" {
		t.Fatalf("current SDK = %q, want second", loaded.Android.CurrentSDK)
	}
	if loaded.Device.DefaultID != "android:emulator-5554" {
		t.Fatalf("default device = %q", loaded.Device.DefaultID)
	}
	if store.Path != filepath.Join(filepath.Dir(store.Path), FileName) {
		t.Fatalf("unexpected configuration path: %s", store.Path)
	}
}
