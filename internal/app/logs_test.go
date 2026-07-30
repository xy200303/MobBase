package app

import "testing"

func TestParseLogs(t *testing.T) {
	options, err := parseLogs([]string{"--follow", "--device", "android:emulator-5554", "--app", "com.example.app"})
	if err != nil || options.Device != "android:emulator-5554" || options.App != "com.example.app" || !options.Follow {
		t.Fatalf("options = %#v, err = %v", options, err)
	}
	if _, err := parseLogs([]string{"--device"}); err == nil {
		t.Fatal("expected invalid logs options")
	}
}
