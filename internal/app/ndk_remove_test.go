package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xy200303/MobBase/internal/state"
)

func TestNDKRemoveDeletesOnlyManagedVersion(t *testing.T) {
	home := t.TempDir()
	sdk := filepath.Join(home, "toolchains", "android", "managed", "sdk")
	ndk := filepath.Join(sdk, "ndk", "27.2.12479018")
	if err := os.MkdirAll(ndk, 0o755); err != nil {
		t.Fatal(err)
	}
	r := runtime{home: home, store: state.New(home), out: &bytes.Buffer{}, events: &eventStream{}}
	config := state.Default()
	config.Android.SDKs = []state.AndroidSDK{{Name: "managed", Path: sdk, Ownership: state.OwnershipManaged}}
	if err := r.store.Save(config); err != nil {
		t.Fatal(err)
	}
	if err := r.ndkRemove([]string{"27.2.12479018", "--sdk", "managed", "--yes"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(ndk); !os.IsNotExist(err) {
		t.Fatalf("NDK still exists: %v", err)
	}
}
