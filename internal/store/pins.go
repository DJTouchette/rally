package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/djtouchette/rally/internal/model"
)

const pinsFile = ".rally/pins.json"

type pinsFileFormat struct {
	Pins []model.Pin `json:"pins"`
}

// LoadPins reads pinned tickets from disk, ordered oldest-first.
// Returns an empty slice if the file does not exist.
func LoadPins() ([]model.Pin, error) {
	data, err := os.ReadFile(pinsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading pins: %w", err)
	}

	var f pinsFileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decoding pins: %w", err)
	}
	sort.SliceStable(f.Pins, func(i, j int) bool {
		return f.Pins[i].PinnedAt.Before(f.Pins[j].PinnedAt)
	})
	return f.Pins, nil
}

// SavePins writes pinned tickets to disk.
func SavePins(pins []model.Pin) error {
	dir := filepath.Dir(pinsFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating pins directory: %w", err)
	}

	data, err := json.MarshalIndent(pinsFileFormat{Pins: pins}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling pins: %w", err)
	}

	if err := os.WriteFile(pinsFile, data, 0644); err != nil {
		return fmt.Errorf("writing pins: %w", err)
	}
	return nil
}

// AddPin pins a ticket. If already pinned, the existing entry is kept
// (PinnedAt is not refreshed) but the note is updated when non-empty.
func AddPin(ticketID, note string) error {
	pins, err := LoadPins()
	if err != nil {
		return err
	}
	for i := range pins {
		if pins[i].TicketID == ticketID {
			if note != "" {
				pins[i].Note = note
				return SavePins(pins)
			}
			return nil
		}
	}
	pins = append(pins, model.Pin{
		TicketID: ticketID,
		PinnedAt: time.Now().UTC(),
		Note:     note,
	})
	return SavePins(pins)
}

// RemovePin unpins a ticket. Removing an unpinned ticket is a no-op.
func RemovePin(ticketID string) error {
	pins, err := LoadPins()
	if err != nil {
		return err
	}
	out := pins[:0]
	for _, p := range pins {
		if p.TicketID != ticketID {
			out = append(out, p)
		}
	}
	if len(out) == len(pins) {
		return nil
	}
	return SavePins(out)
}

// IsPinned reports whether a ticket is currently pinned.
func IsPinned(ticketID string) (bool, error) {
	pins, err := LoadPins()
	if err != nil {
		return false, err
	}
	for _, p := range pins {
		if p.TicketID == ticketID {
			return true, nil
		}
	}
	return false, nil
}
