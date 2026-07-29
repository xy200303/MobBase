package android

import "testing"

func TestParseAvailableSystemImages(t *testing.T) {
	output := `Available Packages:
  Path                                                                            | Version | Description
  -------                                                                         | ------- | -------
  system-images;android-35;google_apis;x86_64                                     | 9       | Google APIs Intel x86_64 Atom System Image
  platform-tools                                                                  | 37.0.1  | Android SDK Platform-Tools
  system-images;android-35;google_apis;x86_64                                     | 9       | duplicate
`
	items := ParseAvailableSystemImages(output)
	if len(items) != 1 || items[0].PackageID != "system-images;android-35;google_apis;x86_64" || items[0].Version != "9" {
		t.Fatalf("items = %#v", items)
	}
}
