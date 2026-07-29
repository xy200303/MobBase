package app

import "testing"

func TestDeviceMirrorRequiresAndroidID(t *testing.T) {
	r := runtime{}
	if err := r.deviceMirror(t.Context(), "ios:123"); err == nil {
		t.Fatal("expected non-Android device to fail")
	}
}
