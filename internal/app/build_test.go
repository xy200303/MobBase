package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	gort "runtime"
	"testing"

	"github.com/xy200303/MobBase/internal/project"
)

func TestParseBuildForwardsArgumentsAfterSeparator(t *testing.T) {
	args, jsonOutput := takeJSON([]string{"build", "--json", "--platform", "android", "--no-install", "--accept-licenses", "--", "gradlew.bat", "assembleDebug", "--console", "plain"})
	if !jsonOutput {
		t.Fatal("expected Mob JSON flag before the separator")
	}
	options, err := parseBuild(args[1:])
	if err != nil {
		t.Fatalf("parse build: %v", err)
	}
	want := []string{"gradlew.bat", "assembleDebug", "--console", "plain"}
	if options.Platform != "android" || !options.NoInstall || !options.AcceptLicenses || !reflect.DeepEqual(options.Command, want) {
		t.Fatalf("unexpected options: %#v", options)
	}
}

func TestSelectBuildPlatformRequiresExplicitMultiTarget(t *testing.T) {
	info := &project.Info{Kind: project.KindFlutter, Targets: []string{"android", "ios"}}
	if _, err := selectBuildPlatform(info, ""); err == nil {
		t.Fatal("expected multi-target project to require --platform")
	}
	platform, err := selectBuildPlatform(info, "android")
	if err != nil || platform != "android" {
		t.Fatalf("platform = %q, err = %v", platform, err)
	}
}

func TestBuildProjectCommandUsesFlutterRunner(t *testing.T) {
	root := t.TempDir()
	if _, _, err := buildProjectCommand(&project.Info{Root: root, Kind: project.KindFlutter}, []string{"echo", "build"}); err != nil {
		t.Fatalf("forwarded Flutter command: %v", err)
	}
}

func TestIOSBuildCommandUsesSingleXcodeProject(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "Notes.xcodeproj")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "project.pbxproj"), []byte("// !$*UTF8*$!"), 0o644); err != nil {
		t.Fatal(err)
	}
	program, arguments, err := iosBuildCommand(root, nil)
	if err != nil {
		t.Fatalf("ios build command: %v", err)
	}
	want := []string{"-project", projectPath, "-configuration", "Debug", "build"}
	if program != "xcodebuild" || !reflect.DeepEqual(arguments, want) {
		t.Fatalf("unexpected iOS build command: %q %#v", program, arguments)
	}
}

func TestBuildIOSReportsUnsupportedHost(t *testing.T) {
	if gort.GOOS == "darwin" {
		t.Skip("requires a non-macOS host")
	}
	err := (runtime{}).buildIOS(context.Background(), &project.Info{Kind: project.KindIOS, Root: t.TempDir(), Targets: []string{"ios"}}, buildOptions{})
	var coded *codedError
	if !errors.As(err, &coded) || coded.Code != "MOB_HOST_UNSUPPORTED" {
		t.Fatalf("expected MOB_HOST_UNSUPPORTED, got %v", err)
	}
}
