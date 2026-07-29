package app

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
)

func TestFlutterMachineDebugSelection(t *testing.T) {
	flutter := &project.Info{Kind: project.KindFlutter}
	if !shouldUseFlutterMachineDebug("mob debug", flutter, runOptions{}, true) {
		t.Fatal("expected Flutter debug event mode to use the machine protocol")
	}
	if shouldUseFlutterMachineDebug("mob run", flutter, runOptions{}, true) {
		t.Fatal("mob run must not use the Flutter debug machine protocol")
	}
	if !shouldUseFlutterMachineDebug("mob debug", flutter, runOptions{Command: []string{"flutter", "run"}}, true) {
		t.Fatal("forwarded debug commands must enter the machine-protocol validation path")
	}
}

func TestFlutterMachineHandlerEmitsDartVMTarget(t *testing.T) {
	var output bytes.Buffer
	r := runtime{json: true, eventMode: true, out: &output, events: &eventStream{}}
	handler := newFlutterMachineHandler(r, "mob debug", android.Device{ID: "android:emulator-5554", NativeID: "emulator-5554", Platform: "android"})
	line := `{"event":"app.debugPort","params":{"appId":"app-1","port":50123,"wsUri":"ws://127.0.0.1:50123/abc=/ws"}}`
	if err := handler.stdout(line); err != nil {
		t.Fatalf("handle machine event: %v", err)
	}
	if err := handler.stdout(line); err != nil {
		t.Fatalf("handle duplicate machine event: %v", err)
	}
	var event map[string]interface{}
	if err := json.NewDecoder(&output).Decode(&event); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if event["event"] != "debugTarget" || event["command"] != "mob debug" {
		t.Fatalf("unexpected event: %#v", event)
	}
	data := event["data"].(map[string]interface{})
	if data["transport"] != "dart-vm-service" || data["port"] != float64(50123) || data["wsUri"] != "ws://127.0.0.1:50123/abc=/ws" {
		t.Fatalf("unexpected target: %#v", data)
	}
	if output.Len() != 0 {
		t.Fatalf("duplicate target emitted: %s", output.String())
	}
}
