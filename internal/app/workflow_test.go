package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
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
