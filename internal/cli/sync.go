package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/djtouchette/rally/internal/markdown"
	"github.com/djtouchette/rally/internal/provider"
	"github.com/djtouchette/rally/internal/store"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Pull assigned tickets from connected providers",
		Long:  "Reads access tokens from environment variables (injected by vaulty exec).",
		RunE:  runSync,
	}
	return cmd
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, _, err := store.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if len(cfg.Connections) == 0 {
		return fmt.Errorf("no providers connected — run: rally connect <provider>")
	}

	state, err := store.LoadState()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	ticketsDir := store.TicketsDir()
	if err := os.MkdirAll(ticketsDir, 0755); err != nil {
		return fmt.Errorf("creating tickets directory: %w", err)
	}

	ctx := context.Background()
	totalNew, totalUpdated, totalRemoved := 0, 0, 0

	for _, conn := range cfg.Connections {
		prov, err := provider.New(conn.Provider)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			continue
		}

		// Read token from environment (injected by vaulty exec)
		token := os.Getenv("RALLY_" + upperName(conn.Provider) + "_TOKEN")
		if token == "" {
			fmt.Fprintf(os.Stderr, "warning: RALLY_%s_TOKEN not in environment\n", upperName(conn.Provider))
			fmt.Fprintf(os.Stderr, "  run: vaulty exec --secrets RALLY_%s_TOKEN -- rally sync\n", upperName(conn.Provider))
			continue
		}

		opts := provider.FetchOpts{Project: conn.Project}
		tickets, err := prov.FetchAssigned(ctx, token, opts)
		if err != nil {
			return fmt.Errorf("fetching %s tickets: %w", conn.Provider, err)
		}

		// Set provider IDs with cloud context for Jira
		for i := range tickets {
			if conn.Provider == "jira" && conn.CloudID != "" {
				tickets[i].ProviderID = conn.CloudID + ":" + tickets[i].ProviderID
			}
			tickets[i].SyncedAt = time.Now()
		}

		// Track which tickets we saw this sync
		seen := make(map[string]bool)

		for _, t := range tickets {
			seen[t.ID] = true
			hash := ticketHash(t)
			t.SyncHash = hash

			filename := markdown.Filename(t)
			path := filepath.Join(ticketsDir, filename)

			oldHash, exists := state.Tickets[t.ID]
			if !exists {
				totalNew++
			} else if oldHash != hash {
				totalUpdated++
			} else {
				continue // unchanged
			}

			md := markdown.Write(t)
			if err := os.WriteFile(path, []byte(md), 0644); err != nil {
				return fmt.Errorf("writing %s: %w", filename, err)
			}
			state.Tickets[t.ID] = hash
		}

		// Remove tickets that are no longer assigned
		for id := range state.Tickets {
			if !seen[id] {
				filename := conn.Provider + "-" + id + ".md"
				path := filepath.Join(ticketsDir, filename)
				if _, err := os.Stat(path); err == nil {
					os.Remove(path)
					delete(state.Tickets, id)
					totalRemoved++
				}
			}
		}
	}

	state.LastSync = time.Now()
	if err := store.SaveState(state); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Printf("Synced: %d new, %d updated, %d removed\n", totalNew, totalUpdated, totalRemoved)
	return nil
}

func ticketHash(t interface{}) string {
	data, _ := json.Marshal(t)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}
