package app

import "testing"

func TestParseSDKInstall(t *testing.T) {
	options, err := parseSDKInstall([]string{"managed", "--package", "platform-tools", "--api", "35", "--accept-licenses"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if options.Name != "managed" || options.API != 35 || len(options.Packages) != 1 || !options.AcceptLicenses {
		t.Fatalf("unexpected options: %#v", options)
	}
	if _, err := parseSDKInstall([]string{"managed", "--api", "35.0"}); err == nil {
		t.Fatal("expected decimal API level to be rejected")
	}
}

func TestParseNDKInstall(t *testing.T) {
	options, err := parseNDKInstall([]string{"27.2.12479018", "--sdk", "managed", "--accept-licenses"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if options.Version != "27.2.12479018" || options.SDKName != "managed" || !options.AcceptLicenses {
		t.Fatalf("unexpected options: %#v", options)
	}
	if _, err := parseNDKInstall([]string{"27.2.12479018", "--accept-licenses"}); err == nil {
		t.Fatal("expected --sdk to be required")
	}
}

func TestParseSDKAvailable(t *testing.T) {
	options, err := parseSDKAvailable([]string{"--api", "35", "--refresh"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if options.API != 35 || !options.Refresh {
		t.Fatalf("unexpected options: %#v", options)
	}
	if _, err := parseSDKAvailable([]string{"--api", "preview"}); err == nil {
		t.Fatal("expected invalid API level")
	}
}

func TestParseRefresh(t *testing.T) {
	refresh, err := parseRefresh([]string{"--refresh"}, "mob android ndk available")
	if err != nil || !refresh {
		t.Fatalf("refresh = %t, err = %v", refresh, err)
	}
}

func TestEmulatorImageInstallRequiresSystemImageAndSDK(t *testing.T) {
	r := runtime{}
	if err := r.emulatorImageInstall(t.Context(), []string{"platform-tools", "--sdk", "managed"}); err == nil {
		t.Fatal("expected non-image package to fail")
	}
	if err := r.emulatorImageInstall(t.Context(), []string{"system-images;android-35;google_apis;x86_64"}); err == nil {
		t.Fatal("expected missing SDK to fail")
	}
}
