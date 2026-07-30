package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xy200303/MobBase/internal/state"
)

func TestParseJavaImport(t *testing.T) {
	path, name, err := parseJavaImport([]string{"--path", ".", "--name", "temurin-17"})
	if err != nil || name != "temurin-17" || path == "" {
		t.Fatalf("parse import: %q %q %v", path, name, err)
	}
	if _, _, err := parseJavaImport([]string{"--path", ".", "--name", "jdk/17"}); err == nil {
		t.Fatal("expected invalid JDK name")
	}
}

func TestJavaUseSelectsRegisteredSDK(t *testing.T) {
	home := t.TempDir()
	r := runtime{home: home, store: state.New(home), out: &bytes.Buffer{}, events: &eventStream{}}
	config := state.Default()
	config.Java.SDKs = []state.JavaSDK{{Name: "temurin-17", Version: 17, Path: filepath.Join(home, "jdk-17"), Ownership: state.OwnershipImported}}
	if err := r.store.Save(config); err != nil {
		t.Fatal(err)
	}
	if err := r.javaUse([]string{"temurin-17"}); err != nil {
		t.Fatal(err)
	}
	config, err := r.store.Load()
	if err != nil || config.Java.CurrentSDK != "temurin-17" {
		t.Fatalf("current Java: %#v %v", config.Java, err)
	}
}

func TestJavaEnvironmentIsProcessScoped(t *testing.T) {
	path := t.TempDir()
	env := javaEnvironment(state.JavaSDK{Path: path})
	if env[0] != "JAVA_HOME="+path || env[1][:5] != "PATH=" {
		t.Fatalf("unexpected Java environment: %#v", env)
	}
}

func TestJavaHomesInListsOnlyDirectJDKDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"liberica-17", "temurin-21"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "not-a-jdk"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	homes := javaHomesIn(root, "")
	if len(homes) != 2 || homes[0] != filepath.Join(root, "liberica-17") || homes[1] != filepath.Join(root, "temurin-21") {
		t.Fatalf("unexpected homes: %#v", homes)
	}
}

func TestNextDiscoveredJavaNameAvoidsCollisions(t *testing.T) {
	existing := []state.JavaSDK{{Name: "discovered-java-17"}, {Name: "discovered-java-17-2"}}
	if name := nextDiscoveredJavaName(existing, 17); name != "discovered-java-17-3" {
		t.Fatalf("name = %q", name)
	}
}

func TestJavaRemoveDeletesManagedSDK(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "toolchains", "java", "17.0.14+7")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	r := runtime{home: home, store: state.New(home), out: &bytes.Buffer{}, events: &eventStream{}}
	config := state.Default()
	config.Java.SDKs = []state.JavaSDK{{Name: "temurin-17", Version: 17, Path: path, Ownership: state.OwnershipManaged}}
	if err := r.store.Save(config); err != nil {
		t.Fatal(err)
	}
	if err := r.javaRemove([]string{"temurin-17", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("JDK still exists: %v", err)
	}
}
