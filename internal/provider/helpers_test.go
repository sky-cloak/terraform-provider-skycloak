package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileSHA256(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "theme.zip")
	if err := os.WriteFile(p, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	// echo -n hello | sha256sum
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	got, err := fileSHA256(p)
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}
	if got != want {
		t.Fatalf("fileSHA256 = %q, want %q", got, want)
	}
}

func TestFileSHA256Missing(t *testing.T) {
	if _, err := fileSHA256(filepath.Join(t.TempDir(), "nope.zip")); err == nil {
		t.Fatalf("expected an error for a missing file")
	}
}
