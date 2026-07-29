package java

import "testing"

func TestParseLatest(t *testing.T) {
	data := []byte(`[{"version":{"semver":"17.0.14+7"},"binary":{"package":{"link":"https://example.test/jdk.zip","checksum":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}]`)
	release, err := ParseLatest(data, 17, "windows")
	if err != nil || release.Major != 17 || release.Version != "17.0.14+7" || release.SHA256 == "" {
		t.Fatalf("release: %#v %v", release, err)
	}
}
