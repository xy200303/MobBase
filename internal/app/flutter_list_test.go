package app

import (
	"context"
	"testing"
)

func TestFlutterListRejectsOtherSubcommands(t *testing.T) {
	runtime := runtime{}
	if err := runtime.flutter(context.Background(), []string{"unknown"}); err == nil {
		t.Fatal("expected an invalid command error")
	}
}
