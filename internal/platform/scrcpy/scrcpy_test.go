package scrcpy

import "testing"

func TestParseLatestReleaseSelectsVerifiedWindowsAsset(t *testing.T) {
	release, err := parseLatestRelease([]byte(`{
  "tag_name": "v4.1",
  "assets": [
    {"name":"scrcpy-win64-v4.1.zip","browser_download_url":"https://github.com/Genymobile/scrcpy/releases/download/v4.1/scrcpy-win64-v4.1.zip","digest":"sha256:5b12172b3264b2889f4583ee64752ce832e29bc8b1089dca81093459697165db","size":11305298}
  ]
}`))
	if err != nil || release.Version != "v4.1" || release.SHA256 != "5b12172b3264b2889f4583ee64752ce832e29bc8b1089dca81093459697165db" {
		t.Fatalf("release = %#v, err = %v", release, err)
	}
}

func TestParseLatestReleaseRejectsAssetWithoutDigest(t *testing.T) {
	_, err := parseLatestRelease([]byte(`{"tag_name":"v4.1","assets":[{"name":"scrcpy-win64-v4.1.zip","browser_download_url":"https://github.com/Genymobile/scrcpy/releases/download/v4.1/scrcpy-win64-v4.1.zip","size":1}]}`))
	if err == nil {
		t.Fatal("expected release without checksum to fail")
	}
}
