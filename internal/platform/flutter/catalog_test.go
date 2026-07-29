package flutter

import "testing"

func TestParseCatalog(t *testing.T) {
	data := []byte(`{"current_release":{"stable":"a"},"releases":[{"hash":"a","channel":"stable","version":"3.29.0","archive":"stable/windows/flutter.zip","sha256":"abc"},{"hash":"b","channel":"beta","version":"3.30.0","archive":"beta.zip","sha256":"def"}]}`)
	catalog, err := ParseCatalog(data, "test")
	if err != nil || len(catalog.Releases) != 1 || !catalog.Releases[0].Current {
		t.Fatalf("catalog: %#v %v", catalog, err)
	}
}
