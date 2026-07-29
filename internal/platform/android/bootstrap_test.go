package android

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifySHA1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	if err := verifySHA1(path, "a9993e364706816aba3e25717850c26c9cd0d89d"); err != nil {
		t.Fatalf("verify expected checksum: %v", err)
	}
	if err := verifySHA1(path, "0000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected mismatched checksum to fail")
	}
}
