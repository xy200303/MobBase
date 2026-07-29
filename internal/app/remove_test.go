package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xy200303/MobBase/internal/state"
)

func TestSDKRemoveDeletesOnlyManagedSDK(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "toolchains", "android", "managed", "sdk")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	r := runtime{home: home, store: state.New(home), out: &bytes.Buffer{}, events: &eventStream{}}
	config := state.Default()
	config.Android.SDKs = []state.AndroidSDK{{Name: "managed", Path: path, Ownership: state.OwnershipManaged}}
	if err := r.store.Save(config); err != nil {
		t.Fatal(err)
	}
	if err := r.sdkRemove([]string{"managed", "--yes"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("SDK still exists: %v", err)
	}
}
