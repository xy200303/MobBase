package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	gort "runtime"
	"testing"

	"github.com/xy200303/MobBase/internal/state"
)

func TestFlutterRunnerUsesFVMMarkerWithoutParsingIt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".fvmrc"), []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	runner, err := flutterRunnerWithLookup(root, func(name string) (string, bool) { return name + ".cmd", name == "fvm" })
	if err != nil {
		t.Fatalf("runner: %v", err)
	}
	if runner.Program != "fvm.cmd" || !reflect.DeepEqual(runner.Prefix, []string{"flutter"}) {
		t.Fatalf("unexpected runner: %#v", runner)
	}
}

func TestFlutterRunnerUsesFlutterWithoutFVMMarker(t *testing.T) {
	runner, err := flutterRunnerWithLookup(t.TempDir(), func(name string) (string, bool) { return "flutter", name == "flutter" })
	if err != nil {
		t.Fatalf("runner: %v", err)
	}
	if runner.Program != "flutter" || len(runner.Prefix) != 0 {
		t.Fatalf("unexpected runner: %#v", runner)
	}
}

func TestParseFlutterCreate(t *testing.T) {
	name, platforms, err := parseFlutterCreate([]string{"travel_app", "--platforms", "android,ios"})
	if err != nil || name != "travel_app" || platforms != "android,ios" {
		t.Fatalf("create options: %q %q %v", name, platforms, err)
	}
	if _, _, err := parseFlutterCreate([]string{"Travel-App"}); err == nil {
		t.Fatal("expected invalid project name")
	}
}

func TestFlutterRunnerPrefersManagedSDK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MOB_HOME", home)
	path := filepath.Join(home, "toolchains", "flutter", "3.29.0")
	config := state.Default()
	config.Flutter.SDKs = []state.FlutterSDK{{Version: "3.29.0", Path: path}}
	config.Flutter.CurrentSDK = "3.29.0"
	if err := state.New(home).Save(config); err != nil {
		t.Fatal(err)
	}
	executable := managedFlutterExecutable(path)
	runner, err := flutterRunnerWithLookup(t.TempDir(), func(candidate string) (string, bool) {
		return candidate, candidate == executable
	})
	if err != nil || runner.Program != executable {
		t.Fatalf("runner: %#v %v", runner, err)
	}
}

func TestFlutterUseSelectsInstalledSDK(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "flutter")
	executable := managedFlutterExecutable(path)
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runtime{home: home, store: state.New(home), out: &bytes.Buffer{}, events: &eventStream{}}
	config := state.Default()
	config.Flutter.SDKs = []state.FlutterSDK{{Version: "3.29.0", Path: path}}
	if err := r.store.Save(config); err != nil {
		t.Fatal(err)
	}
	if err := r.flutterUse([]string{"3.29.0"}); err != nil {
		t.Fatal(err)
	}
	config, err := r.store.Load()
	if err != nil || config.Flutter.CurrentSDK != "3.29.0" {
		t.Fatalf("current: %#v %v", config.Flutter, err)
	}
}

func managedFlutterExecutable(path string) string {
	executable := filepath.Join(path, "bin", "flutter")
	if gort.GOOS == "windows" {
		executable += ".bat"
	}
	return executable
}

func TestFlutterRemoveDeletesManagedSDK(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "toolchains", "flutter", "3.29.0")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	r := runtime{home: home, store: state.New(home), out: &bytes.Buffer{}, events: &eventStream{}}
	config := state.Default()
	config.Flutter.SDKs = []state.FlutterSDK{{Version: "3.29.0", Path: path}}
	if err := r.store.Save(config); err != nil {
		t.Fatal(err)
	}
	if err := r.flutterRemove([]string{"3.29.0", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Flutter still exists: %v", err)
	}
}

func TestEnsureFlutterRunnerHonorsNoInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MOB_HOME", home)
	t.Setenv("PATH", "")
	r := runtime{home: home, store: state.New(home), out: &bytes.Buffer{}, events: &eventStream{}}
	_, err := r.ensureFlutterRunner(context.Background(), t.TempDir(), true, "mob build")
	if err == nil {
		t.Fatal("expected missing Flutter error")
	}
	var coded *codedError
	if !errors.As(err, &coded) || coded.Code != "MOB_TOOLCHAIN_MISSING" {
		t.Fatalf("unexpected error: %v", err)
	}
}
