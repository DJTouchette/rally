package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func chdirTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestLoadPins_MissingFile(t *testing.T) {
	chdirTemp(t)
	pins, err := LoadPins()
	if err != nil {
		t.Fatalf("LoadPins on missing file: %v", err)
	}
	if len(pins) != 0 {
		t.Fatalf("expected empty pins, got %d", len(pins))
	}
}

func TestAddPin_Idempotent(t *testing.T) {
	chdirTemp(t)
	if err := AddPin("RAL-1", "first"); err != nil {
		t.Fatal(err)
	}
	pinsBefore, _ := LoadPins()
	pinnedAt := pinsBefore[0].PinnedAt

	time.Sleep(2 * time.Millisecond)
	if err := AddPin("RAL-1", ""); err != nil {
		t.Fatal(err)
	}
	pins, _ := LoadPins()

	if len(pins) != 1 {
		t.Fatalf("expected 1 pin after duplicate add, got %d", len(pins))
	}
	if !pins[0].PinnedAt.Equal(pinnedAt) {
		t.Fatalf("PinnedAt should not refresh on duplicate add")
	}
	if pins[0].Note != "first" {
		t.Fatalf("note should be preserved when re-add has empty note, got %q", pins[0].Note)
	}
}

func TestAddPin_UpdatesNote(t *testing.T) {
	chdirTemp(t)
	if err := AddPin("RAL-1", "first"); err != nil {
		t.Fatal(err)
	}
	if err := AddPin("RAL-1", "second"); err != nil {
		t.Fatal(err)
	}
	pins, _ := LoadPins()
	if pins[0].Note != "second" {
		t.Fatalf("expected note=second, got %q", pins[0].Note)
	}
}

func TestRemovePin(t *testing.T) {
	chdirTemp(t)
	_ = AddPin("RAL-1", "")
	_ = AddPin("RAL-2", "")

	if err := RemovePin("RAL-1"); err != nil {
		t.Fatal(err)
	}
	pins, _ := LoadPins()
	if len(pins) != 1 || pins[0].TicketID != "RAL-2" {
		t.Fatalf("after remove, expected only RAL-2, got %+v", pins)
	}

	if err := RemovePin("nope"); err != nil {
		t.Fatalf("removing missing pin should be no-op, got %v", err)
	}
}

func TestLoadPins_OrderedByPinnedAt(t *testing.T) {
	chdirTemp(t)
	_ = AddPin("RAL-1", "")
	time.Sleep(2 * time.Millisecond)
	_ = AddPin("RAL-2", "")
	time.Sleep(2 * time.Millisecond)
	_ = AddPin("RAL-3", "")

	pins, _ := LoadPins()
	if len(pins) != 3 {
		t.Fatalf("expected 3 pins, got %d", len(pins))
	}
	for i := 1; i < len(pins); i++ {
		if pins[i].PinnedAt.Before(pins[i-1].PinnedAt) {
			t.Fatalf("pins not ordered by PinnedAt at index %d", i)
		}
	}
}

func TestIsPinned(t *testing.T) {
	chdirTemp(t)
	_ = AddPin("RAL-1", "")

	yes, err := IsPinned("RAL-1")
	if err != nil || !yes {
		t.Fatalf("expected RAL-1 pinned, got yes=%v err=%v", yes, err)
	}
	no, err := IsPinned("RAL-2")
	if err != nil || no {
		t.Fatalf("expected RAL-2 not pinned, got no=%v err=%v", no, err)
	}
}

func TestSavePins_CreatesDir(t *testing.T) {
	chdirTemp(t)
	if err := AddPin("RAL-1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(".rally", "pins.json")); err != nil {
		t.Fatalf("pins.json not created: %v", err)
	}
}
