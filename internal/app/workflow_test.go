package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/xy200303/MobBase/internal/app/ui"
)

func TestExecuteWorkflowCommandStreamsEvents(t *testing.T) {
	if os.Getenv("MOB_WORKFLOW_HELPER") == "1" {
		fmt.Fprintln(os.Stdout, "workflow stdout")
		fmt.Fprintln(os.Stderr, "workflow stderr")
		return
	}
	var output bytes.Buffer
	r := runtime{json: true, eventMode: true, out: &output, events: &eventStream{}}
	_, err := r.executeWorkflowCommand(context.Background(), "mob build", os.Args[0], []string{"-test.run=^TestExecuteWorkflowCommandStreamsEvents$"}, []string{"MOB_WORKFLOW_HELPER=1"}, "")
	if err != nil {
		t.Fatalf("execute workflow: %v", err)
	}
	decoder := json.NewDecoder(&output)
	var first, second map[string]interface{}
	if err := decoder.Decode(&first); err != nil {
		t.Fatalf("decode first event: %v", err)
	}
	if err := decoder.Decode(&second); err != nil {
		t.Fatalf("decode second event: %v", err)
	}
	if first["event"] != "log" || second["event"] != "log" || first["command"] != "mob build" || second["command"] != "mob build" {
		t.Fatalf("unexpected events: %#v %#v", first, second)
	}
}

func TestExecuteWorkflowCommandUsesExternalToolOutput(t *testing.T) {
	if os.Getenv("MOB_WORKFLOW_EXTERNAL_HELPER") == "1" {
		fmt.Fprintln(os.Stdout, "Gradle task detail")
		return
	}
	var stdout, stderr bytes.Buffer
	r := runtime{out: &stdout, err: &stderr, events: &eventStream{}, terminal: ui.New(&stdout, &stderr)}
	_, err := r.executeWorkflowCommand(context.Background(), "mob build", os.Args[0], []string{"-test.run=^TestExecuteWorkflowCommandUsesExternalToolOutput$"}, []string{"MOB_WORKFLOW_EXTERNAL_HELPER=1"}, "")
	if err != nil {
		t.Fatalf("execute workflow: %v", err)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "[mob] "+externalToolLabel(os.Args[0], nil)+": Gradle task detail") {
		t.Fatalf("unexpected workflow streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExternalToolLabelRecognizesOfficialRunners(t *testing.T) {
	tests := []struct {
		program string
		args    []string
		want    string
	}{
		{program: "cmd.exe", args: []string{"/c", `C:\\project\\gradlew.bat`, "assembleDebug"}, want: "Gradle"},
		{program: "/opt/flutter/bin/flutter", want: "Flutter"},
		{program: "xcodebuild", want: "Xcode"},
	}
	for _, test := range tests {
		if got := externalToolLabel(test.program, test.args); got != test.want {
			t.Errorf("externalToolLabel(%q, %q) = %q, want %q", test.program, test.args, got, test.want)
		}
	}
}
