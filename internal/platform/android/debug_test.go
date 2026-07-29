package android

import (
	"context"
	"testing"
)

func TestContainsPID(t *testing.T) {
	if !containsPID("124\n  800\n", 800) {
		t.Fatal("expected PID to be found")
	}
	if containsPID("124\n800\n", 80) {
		t.Fatal("unexpected partial PID match")
	}
}

func TestForwardJDWPRejectsInvalidPIDBeforeLookingUpADB(t *testing.T) {
	_, err := ForwardJDWP(context.Background(), nil, "emulator-5554", 0)
	if err == nil || err.Error() != "Android JDWP PID must be positive" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveJDWPForwardRejectsInvalidPortBeforeLookingUpADB(t *testing.T) {
	if err := RemoveJDWPForward(context.Background(), nil, "emulator-5554", 0); err == nil || err.Error() != "Android JDWP port must be between 1 and 65535" {
		t.Fatalf("unexpected remove error: %v", err)
	}
}
