package store

import (
	"os"
	"path/filepath"
)

// candidateDirs are the locations rally looks for an existing data directory,
// in priority order, when RALLY_DIR is not set. ".rivet/rally" lets rally nest
// its data under a rivet workspace.
var candidateDirs = []string{".rally", ".rivet/rally"}

// BaseDir returns rally's working directory for config, state, tickets, and
// pins. Resolution order:
//
//  1. $RALLY_DIR, when set (explicit override).
//  2. the first of candidateDirs that already exists on disk — this is how
//     rally finds a relocated ".rivet/rally" even under `vaulty exec`, which
//     runs commands with a sanitized environment and would drop $RALLY_DIR.
//  3. ".rally" as the default for a first-time `connect`.
//
// Note: to bootstrap a fresh install directly into ".rivet/rally", set
// RALLY_DIR for the `connect` command (which runs outside `vaulty exec`).
func BaseDir() string {
	if d := os.Getenv("RALLY_DIR"); d != "" {
		return d
	}
	for _, dir := range candidateDirs {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
	}
	return ".rally"
}

// On-disk locations, all derived from BaseDir so RALLY_DIR moves them together.
func configPath() string  { return filepath.Join(BaseDir(), "config.yaml") }
func statePath() string   { return filepath.Join(BaseDir(), "state.json") }
func ticketsPath() string { return filepath.Join(BaseDir(), "tickets") }
func pinsPath() string    { return filepath.Join(BaseDir(), "pins.json") }
