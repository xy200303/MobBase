package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	gort "runtime"
	"strings"
	"testing"

	"github.com/xy200303/MobBase/internal/home"
	"github.com/xy200303/MobBase/internal/state"
)

func TestSDKImportListAndUseJSONContract(t *testing.T) {
	mobHome := t.TempDir()
	t.Setenv("MOB_HOME", mobHome)
	sdkRoot := filepath.Join(t.TempDir(), "Sdk")
	if err := os.MkdirAll(filepath.Join(sdkRoot, "platforms", "android-35"), 0o755); err != nil {
		t.Fatalf("create SDK: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"android", "sdk", "import", "--path", sdkRoot, "--name", "shared", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("import exit = %d, stderr = %s", exit, stderr.String())
	}
	assertEvent(t, stdout.Bytes(), true, "mob android sdk import")

	stdout.Reset()
	if exit := Run(context.Background(), []string{"android", "sdk", "use", "shared", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("use exit = %d, stderr = %s", exit, stderr.String())
	}
	assertEvent(t, stdout.Bytes(), true, "mob android sdk use")

	stdout.Reset()
	if exit := Run(context.Background(), []string{"android", "sdk", "list", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("list exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob android sdk list")
	data := event["data"].(map[string]interface{})
	for _, rawSDK := range data["sdks"].([]interface{}) {
		sdk := rawSDK.(map[string]interface{})
		if sdk["name"] == "shared" && sdk["current"] == true {
			return
		}
	}
	if len(data["sdks"].([]interface{})) == 0 {
		t.Fatalf("unexpected SDK response: %s", stdout.String())
	}
	t.Fatalf("imported SDK is missing from response: %s", stdout.String())
}

func TestSDKAddUsesPositionalNameAndCompletedEvent(t *testing.T) {
	mobHome := t.TempDir()
	t.Setenv("MOB_HOME", mobHome)
	sdkRoot := filepath.Join(t.TempDir(), "Sdk")
	if err := os.MkdirAll(filepath.Join(sdkRoot, "platform-tools"), 0o755); err != nil {
		t.Fatalf("create SDK: %v", err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{"android", "sdk", "add", "studio", "--path", sdkRoot, "--json"}
	if exit := Run(context.Background(), args, &stdout, &stderr); exit != 0 {
		t.Fatalf("add exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob android sdk add")
	if event["event"] != "completed" {
		t.Fatalf("event kind = %q, want completed", event["event"])
	}
	data := event["data"].(map[string]interface{})
	sdk := data["sdk"].(map[string]interface{})
	if sdk["name"] != "studio" {
		t.Fatalf("SDK name = %q, want studio", sdk["name"])
	}
}

func TestJSONEventStreamUsesOrderedLifecycleEvents(t *testing.T) {
	var output bytes.Buffer
	run := runtime{json: true, eventMode: true, out: &output, events: &eventStream{}}
	if err := run.emit("started", "mob android sdk install", true, map[string]string{"phase": "install"}, nil); err != nil {
		t.Fatalf("write started event: %v", err)
	}
	if err := run.result("mob android sdk install", map[string]string{"sdk": "managed"}); err != nil {
		t.Fatalf("write completed event: %v", err)
	}

	decoder := json.NewDecoder(&output)
	var started, completed map[string]interface{}
	if err := decoder.Decode(&started); err != nil {
		t.Fatalf("decode started event: %v", err)
	}
	if err := decoder.Decode(&completed); err != nil {
		t.Fatalf("decode completed event: %v", err)
	}
	if started["event"] != "started" || started["sequence"] != float64(1) {
		t.Fatalf("unexpected started event: %#v", started)
	}
	if completed["event"] != "completed" || completed["sequence"] != float64(2) {
		t.Fatalf("unexpected completed event: %#v", completed)
	}
}

func TestStandardJSONSuppressesLifecycleEvents(t *testing.T) {
	var output bytes.Buffer
	run := runtime{json: true, out: &output, events: &eventStream{}}
	if err := run.emit("started", "mob build", true, map[string]string{"phase": "build"}, nil); err != nil {
		t.Fatalf("emit started: %v", err)
	}
	if err := run.result("mob build", map[string]string{"platform": "android"}); err != nil {
		t.Fatalf("emit completed: %v", err)
	}
	event := assertEvent(t, output.Bytes(), true, "mob build")
	if event["event"] != "completed" || event["sequence"] != float64(1) {
		t.Fatalf("unexpected standard JSON event: %#v", event)
	}
}

func TestTakeJSONAcceptsEventStreamSelector(t *testing.T) {
	args, jsonOutput := takeJSON([]string{"logs", "--json=events", "--follow"})
	if !jsonOutput {
		t.Fatal("expected --json=events to enable JSON output")
	}
	if len(args) != 2 || args[0] != "logs" || args[1] != "--follow" {
		t.Fatalf("unexpected remaining arguments: %#v", args)
	}
}

func TestTakeOutputDistinguishesStandardJSONFromEventStream(t *testing.T) {
	args, jsonOutput, eventMode := takeOutput([]string{"build", "--json"})
	if !jsonOutput || eventMode || len(args) != 1 || args[0] != "build" {
		t.Fatalf("standard JSON mode = %#v, %v, %v", args, jsonOutput, eventMode)
	}
	args, jsonOutput, eventMode = takeOutput([]string{"build", "--json=events"})
	if !jsonOutput || !eventMode || len(args) != 1 || args[0] != "build" {
		t.Fatalf("event mode = %#v, %v, %v", args, jsonOutput, eventMode)
	}
}

func TestEnvShowJSONContract(t *testing.T) {
	mobHome := t.TempDir()
	t.Setenv("MOB_HOME", mobHome)
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"env", "show", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("env show exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob env show")
	data := event["data"].(map[string]interface{})
	if data["mobHome"] != mobHome || data["scope"] != "child-process-only" {
		t.Fatalf("unexpected environment response: %s", stdout.String())
	}
}

func TestNDKListFiltersBySDK(t *testing.T) {
	mobHome := t.TempDir()
	t.Setenv("MOB_HOME", mobHome)
	sdkRoot := filepath.Join(t.TempDir(), "Sdk")
	if err := os.MkdirAll(filepath.Join(sdkRoot, "ndk", "27.2.12479018"), 0o755); err != nil {
		t.Fatalf("create NDK: %v", err)
	}

	var stdout, stderr bytes.Buffer
	addArgs := []string{"android", "sdk", "add", "managed-copy", "--path", sdkRoot, "--json"}
	if exit := Run(context.Background(), addArgs, &stdout, &stderr); exit != 0 {
		t.Fatalf("add exit = %d, stderr = %s", exit, stderr.String())
	}
	stdout.Reset()
	listArgs := []string{"android", "ndk", "list", "--sdk", "managed-copy", "--json"}
	if exit := Run(context.Background(), listArgs, &stdout, &stderr); exit != 0 {
		t.Fatalf("list exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob android ndk list")
	data := event["data"].(map[string]interface{})
	ndks := data["ndks"].([]interface{})
	if len(ndks) != 1 {
		t.Fatalf("NDK list = %#v", ndks)
	}
	entry := ndks[0].(map[string]interface{})
	versions := entry["versions"].([]interface{})
	if entry["sdk"] != "managed-copy" || len(versions) != 1 || versions[0] != "27.2.12479018" {
		t.Fatalf("unexpected NDK entry: %#v", entry)
	}
}

func TestHelpUsesFullCommandPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"help", "android", "sdk", "import", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("help exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob help")
	data := event["data"].(map[string]interface{})
	if data["command"] != "mob android sdk import" {
		t.Fatalf("command = %q", data["command"])
	}
}

func TestAndroidSDKInstallHelpContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"help", "android", "sdk", "install", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("help exit = %d, stderr = %s", exit, stderr.String())
	}
	data := assertEvent(t, stdout.Bytes(), true, "mob help")["data"].(map[string]interface{})
	if data["command"] != "mob android sdk install" || !strings.Contains(data["usage"].(string), "--package <package-id>") {
		t.Fatalf("unexpected SDK install help: %#v", data)
	}
	valueSyntax := map[string]string{}
	for _, raw := range data["options"].([]interface{}) {
		option := raw.(map[string]interface{})
		if value, ok := option["valueSyntax"].(string); ok {
			valueSyntax[option["name"].(string)] = value
		}
	}
	if valueSyntax["--api"] != "<level>" || valueSyntax["--package"] != "<package-id>" {
		t.Fatalf("SDK install option contract is incomplete: %#v", valueSyntax)
	}
}

func TestDeviceOpenHelpContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"help", "device", "open", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("help exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob help")
	data := event["data"].(map[string]interface{})
	if data["usage"] != "mob device open <android:native-id> [--json]" {
		t.Fatalf("unexpected device open help: %s", stdout.String())
	}
}

func TestHelpSupportsMarkdownAndStructuredJSONFormats(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"help", "run", "--format", "markdown"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("markdown help exit = %d, stderr = %s", exit, stderr.String())
	}
	markdown := stdout.String()
	if !strings.Contains(markdown, "# `mob run`") || !strings.Contains(markdown, "## Options") || !strings.Contains(markdown, "`--device <platform:native-id>`") {
		t.Fatalf("unexpected markdown help: %s", markdown)
	}

	stdout.Reset()
	if exit := Run(context.Background(), []string{"help", "run", "--format=json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("JSON format help exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob help")
	data := event["data"].(map[string]interface{})
	if data["command"] != "mob run" {
		t.Fatalf("command = %#v", data["command"])
	}
	platforms := data["platforms"].([]interface{})
	if len(platforms) != 1 || platforms[0] != "android" {
		t.Fatalf("platforms = %#v", platforms)
	}
	options := data["options"].([]interface{})
	foundDevice := false
	for _, raw := range options {
		option := raw.(map[string]interface{})
		if option["name"] == "--device" && option["valueSyntax"] == "<platform:native-id>" {
			foundDevice = true
		}
	}
	if !foundDevice {
		t.Fatalf("device option missing: %#v", options)
	}
	if data["supportsEventStream"] != true || !strings.Contains(data["usage"].(string), "--json=events") {
		t.Fatalf("event stream contract missing: %#v", data)
	}
}

func TestDebugHelpDeclaresFlutterMachineProtocol(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"help", "debug", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("debug help exit = %d, stderr = %s", exit, stderr.String())
	}
	data := assertEvent(t, stdout.Bytes(), true, "mob help")["data"].(map[string]interface{})
	if !strings.Contains(data["description"].(string), "Dart VM Service") {
		t.Fatalf("Flutter machine protocol missing: %#v", data)
	}
	errors := data["errors"].([]interface{})
	foundInvalidArgument := false
	for _, value := range errors {
		if value == "MOB_INVALID_ARGUMENT" {
			foundInvalidArgument = true
		}
	}
	if !foundInvalidArgument {
		t.Fatalf("missing forwarded-command restriction error: %#v", errors)
	}
}

func TestHelpRejectsUnknownFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"help", "run", "--format", "html"}, &stdout, &stderr); exit == 0 {
		t.Fatal("expected unknown help format to fail")
	}
	if !strings.Contains(stderr.String(), "--format supports text, markdown, or json") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestHelpGroupsNamespacesAndRejectsUnknownCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"help", "android", "sdk", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("namespace help exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob help")
	if event["data"].(map[string]interface{})["command"] != "mob android sdk" {
		t.Fatalf("unexpected namespace help: %s", stdout.String())
	}
	stdout.Reset()
	if exit := Run(context.Background(), []string{"help", "android", "sdk", "does-not-exist", "--json"}, &stdout, &stderr); exit == 0 {
		t.Fatal("expected unknown help target to fail")
	}
	event = assertEvent(t, stdout.Bytes(), false, "mob help android sdk does-not-exist")
	if event["error"].(map[string]interface{})["code"] != "MOB_INVALID_COMMAND" {
		t.Fatalf("unexpected unknown help error: %s", stdout.String())
	}
}

func TestSupportBundleIsRedactedAndDoesNotOverwrite(t *testing.T) {
	mobHome := t.TempDir()
	t.Setenv("MOB_HOME", mobHome)
	config := state.Default()
	config.Android.ProxyURL = "https://proxy.example.test:7890"
	config.Android.SDKs = []state.AndroidSDK{{Name: "managed", Path: filepath.Join(mobHome, "sdk"), Ownership: state.OwnershipManaged}}
	config.Device.DefaultID = "android:private-device-id"
	if err := state.New(mobHome).Save(config); err != nil {
		t.Fatalf("save config: %v", err)
	}
	output := filepath.Join(t.TempDir(), "support.zip")
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"support", "bundle", "--output", output, "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("bundle exit = %d, stderr = %s", exit, stderr.String())
	}
	assertEvent(t, stdout.Bytes(), true, "mob support bundle")
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("open support bundle: %v", err)
	}
	defer archive.Close()
	if len(archive.File) != 2 || archive.File[0].Name != "manifest.json" || archive.File[1].Name != "toolchains.json" {
		t.Fatalf("unexpected support archive: %#v", archive.File)
	}
	entry, err := archive.File[1].Open()
	if err != nil {
		t.Fatalf("open inventory: %v", err)
	}
	data, err := io.ReadAll(entry)
	entry.Close()
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "proxy.example.test") || strings.Contains(text, "private-device-id") || strings.Contains(text, mobHome) {
		t.Fatalf("support archive contains private config: %s", text)
	}
	stdout.Reset()
	if exit := Run(context.Background(), []string{"support", "bundle", "--output", output, "--json"}, &stdout, &stderr); exit == 0 {
		t.Fatal("expected existing support output to be rejected")
	}
}

func TestFVMHelpAndVersionContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"help", "fvm", "install", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("help exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob help")
	data := event["data"].(map[string]interface{})
	if data["usage"] != "mob fvm install [--version <version>] [--json|--json=events]" || data["supportsEventStream"] != true {
		t.Fatalf("unexpected FVM install help: %s", stdout.String())
	}
	if version, err := parseFVMVersion([]string{"--version", "4.1.2"}, "mob fvm install"); err != nil || version != "4.1.2" {
		t.Fatalf("valid version = %q, %v", version, err)
	}
	if _, err := parseFVMVersion([]string{"--version", "../../bad"}, "mob fvm install"); err == nil {
		t.Fatal("expected invalid FVM version error")
	}
}

func TestFVMUseSelectsManagedLauncher(t *testing.T) {
	mobHome := t.TempDir()
	t.Setenv("MOB_HOME", mobHome)
	launcherRoot := filepath.Join(mobHome, "toolchains", "fvm", "4.1.2")
	launcher := fvmLauncherPath(launcherRoot)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatalf("create launcher path: %v", err)
	}
	if err := os.WriteFile(launcher, []byte("launcher"), 0o755); err != nil {
		t.Fatalf("create launcher: %v", err)
	}
	run := runtime{home: mobHome, store: state.New(mobHome), json: true, out: &bytes.Buffer{}, events: &eventStream{}}
	config, err := run.store.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	config.FVM.SDKs = []state.FVMSDK{{Version: "4.1.2", Path: launcherRoot, SHA256: "checksum"}}
	if err := run.store.Save(config); err != nil {
		t.Fatalf("save config: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"fvm", "use", "4.1.2", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("use exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob fvm use")
	if event["data"].(map[string]interface{})["version"] != "4.1.2" {
		t.Fatalf("unexpected response: %s", stdout.String())
	}
	config, err = run.store.Load()
	if err != nil || config.FVM.CurrentSDK != "4.1.2" {
		t.Fatalf("selected FVM = %#v, %v", config.FVM, err)
	}
}

func TestHomeSetMigratesMobOwnedData(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	source := filepath.Join(t.TempDir(), "mob-home")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := home.Select(source); err != nil {
		t.Fatalf("select source: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "mobile-tools")
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"home", "set", destination, "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("home set exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob home set")
	if event["data"].(map[string]interface{})["path"] != destination {
		t.Fatalf("unexpected response: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(destination, "config.yaml")); err != nil {
		t.Fatalf("migrated config not found: %v", err)
	}
	selected, err := home.Selected()
	if err != nil || selected != destination {
		t.Fatalf("selected home = %q, %v", selected, err)
	}
}

func TestAndroidDoctorUsesPlatformCommandName(t *testing.T) {
	mobHome := t.TempDir()
	t.Setenv("MOB_HOME", mobHome)
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"android", "doctor", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("android doctor exit = %d, stderr = %s", exit, stderr.String())
	}
	event := assertEvent(t, stdout.Bytes(), true, "mob android doctor")
	if event["data"].(map[string]interface{})["platform"] != "android" {
		t.Fatalf("unexpected response: %s", stdout.String())
	}
}

func TestReservedPlatformNamespacesReturnCodedErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"harmony", "doctor", "--json"}, &stdout, &stderr); exit == 0 {
		t.Fatal("expected unsupported HarmonyOS namespace to fail")
	}
	event := assertEvent(t, stdout.Bytes(), false, "mob harmony doctor")
	errorData := event["error"].(map[string]interface{})
	if errorData["code"] != "MOB_PLATFORM_NOT_SUPPORTED" {
		t.Fatalf("unexpected HarmonyOS error: %#v", errorData)
	}
	if gort.GOOS == "darwin" {
		return
	}

	stdout.Reset()
	if exit := Run(context.Background(), []string{"ios", "doctor", "--json"}, &stdout, &stderr); exit == 0 {
		t.Fatal("expected reserved iOS namespace to fail")
	}
	event = assertEvent(t, stdout.Bytes(), false, "mob ios doctor")
	errorData = event["error"].(map[string]interface{})
	if errorData["code"] != "MOB_HOST_UNSUPPORTED" && errorData["code"] != "MOB_PLATFORM_NOT_SUPPORTED" {
		t.Fatalf("unexpected iOS error: %#v", errorData)
	}
}

func TestParseDeviceListAcceptsPlatformNamespaces(t *testing.T) {
	for _, platform := range []string{"android", "ios"} {
		got, err := parseDeviceList([]string{"list", "--platform", platform})
		if err != nil || got != platform {
			t.Fatalf("platform %q parsed as %q with error %v", platform, got, err)
		}
	}
	if _, err := parseDeviceList([]string{"list", "--platform", "harmony"}); err == nil {
		t.Fatal("unsupported device platform was accepted")
	}
}

func TestIOSSimulatorStartRequiresIOSDeviceID(t *testing.T) {
	err := (runtime{}).iosSimulatorStart(context.Background(), "android:emulator-5554")
	var coded *codedError
	if !errors.As(err, &coded) || coded.Code != "MOB_INVALID_ARGUMENT" {
		t.Fatalf("expected MOB_INVALID_ARGUMENT, got %v", err)
	}
}

func TestAndroidProxyPersistsAndBuildsChildEnvironment(t *testing.T) {
	mobHome := t.TempDir()
	t.Setenv("MOB_HOME", mobHome)
	var stdout, stderr bytes.Buffer
	if exit := Run(context.Background(), []string{"android", "proxy", "set", "http://127.0.0.1:7890", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("proxy set exit = %d, stderr = %s", exit, stderr.String())
	}
	assertEvent(t, stdout.Bytes(), true, "mob android proxy set")
	config, err := state.New(mobHome).Load()
	if err != nil || config.Android.ProxyURL != "http://127.0.0.1:7890" {
		t.Fatalf("proxy config = %#v, %v", config.Android, err)
	}
	environment := androidProxyEnvironment(config)
	if len(environment) != 2 || environment[0] != "HTTP_PROXY=http://127.0.0.1:7890" || environment[1] != "HTTPS_PROXY=http://127.0.0.1:7890" {
		t.Fatalf("proxy environment = %#v", environment)
	}
	if validProxyURL("ftp://example.test") || validProxyURL("not-a-url") || validProxyURL("http://user:secret@example.test:8080") {
		t.Fatal("invalid proxy URL accepted")
	}
}

func assertEvent(t *testing.T, data []byte, wantOK bool, command string) map[string]interface{} {
	t.Helper()
	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("parse event: %v\n%s", err, data)
	}
	if event["schemaVersion"] != "1.0" || event["ok"] != wantOK || event["command"] != command {
		t.Fatalf("unexpected event: %s", data)
	}
	return event
}
