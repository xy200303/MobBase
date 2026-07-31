package session

import "testing"

func TestMetadataSupportsDeclaredControl(t *testing.T) {
	metadata := Metadata{Controls: []string{ControlTap, ControlClose}}
	if !metadata.Supports(ControlTap) {
		t.Fatal("declared control was not supported")
	}
	if metadata.Supports(ControlSwipe) {
		t.Fatal("undeclared control was supported")
	}
}
