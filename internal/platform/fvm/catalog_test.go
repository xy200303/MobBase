package fvm

import "testing"

func TestParseCatalogKeepsVerifiedReleasesAndCurrentVersion(t *testing.T) {
	data := []byte(`{
  "latest":{"version":"4.1.2"},
  "versions":[
    {"version":"4.0.0","archive_url":"https://pub.dev/api/archives/fvm-4.0.0.tar.gz","archive_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pubspec":{"environment":{"sdk":">=3.6.0 <4.0.0"}}},
    {"version":"4.1.2","archive_url":"https://pub.dev/api/archives/fvm-4.1.2.tar.gz","archive_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","pubspec":{"environment":{"sdk":">=3.6.0 <4.0.0"}}},
    {"version":"broken","archive_url":"https://example.test/fvm.tar.gz","archive_sha256":"short"}
  ]
}`)
	catalog, err := ParseCatalog(data)
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	if catalog.Source != OfficialCatalogURL || len(catalog.Releases) != 2 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	if catalog.Releases[0].Version != "4.1.2" || !catalog.Releases[0].Current || catalog.Releases[0].SHA256 != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("unexpected selected release: %#v", catalog.Releases[0])
	}
}
