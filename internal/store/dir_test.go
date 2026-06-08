package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaseDir(t *testing.T) {
	// 1. RALLY_DIR override always wins.
	t.Setenv("RALLY_DIR", "custom/dir")
	if got := BaseDir(); got != "custom/dir" {
		t.Fatalf("RALLY_DIR override: got %q, want custom/dir", got)
	}

	// Clear the override for the discovery cases.
	t.Setenv("RALLY_DIR", "")
	t.Chdir(t.TempDir())

	// 2. Nothing on disk -> default .rally.
	if got := BaseDir(); got != ".rally" {
		t.Fatalf("default: got %q, want .rally", got)
	}

	// 3. Only .rivet/rally present -> discovered (the vaulty-exec case, no env).
	if err := os.MkdirAll(filepath.Join(".rivet", "rally"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := BaseDir(); got != ".rivet/rally" {
		t.Fatalf("discovery: got %q, want .rivet/rally", got)
	}

	// 4. .rally present -> takes priority over .rivet/rally.
	if err := os.MkdirAll(".rally", 0o755); err != nil {
		t.Fatal(err)
	}
	if got := BaseDir(); got != ".rally" {
		t.Fatalf("priority: got %q, want .rally", got)
	}
}
