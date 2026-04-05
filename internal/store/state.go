package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SyncState tracks the last sync time and per-ticket hashes.
type SyncState struct {
	LastSync time.Time         `json:"last_sync"`
	Tickets  map[string]string `json:"tickets"` // ticket ID -> sync hash
}

const stateFile = ".rally/state.json"

// LoadState loads sync state from disk.
func LoadState() (*SyncState, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &SyncState{Tickets: make(map[string]string)}, nil
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decoding state: %w", err)
	}
	if state.Tickets == nil {
		state.Tickets = make(map[string]string)
	}

	return &state, nil
}

// SaveState writes sync state to disk.
func SaveState(state *SyncState) error {
	dir := filepath.Dir(stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	return nil
}

// TicketsDir returns the directory where synced ticket markdown files live.
func TicketsDir() string {
	return ".rally/tickets"
}
