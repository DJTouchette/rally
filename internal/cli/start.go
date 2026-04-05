package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/djtouchette/rally/internal/markdown"
	"github.com/djtouchette/rally/internal/model"
	"github.com/djtouchette/rally/internal/provider"
	"github.com/djtouchette/rally/internal/store"
	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	var noPush bool

	cmd := &cobra.Command{
		Use:   "start <id>",
		Short: "Mark a ticket as in-progress and push to provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(args[0], model.StatusInProgress, noPush)
		},
	}

	cmd.Flags().BoolVar(&noPush, "local", false, "only update locally, don't push to provider")

	return cmd
}

func runTransition(ticketID string, status model.Status, localOnly bool) error {
	tickets, err := loadLocalTickets()
	if err != nil {
		return err
	}

	var target *model.Ticket
	for i := range tickets {
		if tickets[i].ID == ticketID {
			target = &tickets[i]
			break
		}
	}

	if target == nil {
		return fmt.Errorf("ticket %s not found — run `rally sync` first", ticketID)
	}

	oldStatus := target.Status
	target.Status = status

	// Write updated markdown
	ticketsDir := store.TicketsDir()
	filename := markdown.Filename(*target)
	path := filepath.Join(ticketsDir, filename)
	md := markdown.Write(*target)
	if err := os.WriteFile(path, []byte(md), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", filename, err)
	}

	// Push to provider if token is in environment
	if !localOnly && target.ProviderID != "" {
		token := os.Getenv("RALLY_" + upperName(target.Provider) + "_TOKEN")
		if token == "" {
			fmt.Fprintf(os.Stderr, "warning: RALLY_%s_TOKEN not in environment — status not pushed\n", upperName(target.Provider))
			fmt.Fprintf(os.Stderr, "  run via: vaulty exec --secrets RALLY_%s_TOKEN -- rally start %s\n", upperName(target.Provider), ticketID)
		} else {
			prov, err := provider.New(target.Provider)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			} else {
				ctx := context.Background()
				if err := prov.UpdateStatus(ctx, token, target.ProviderID, status); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not push status to %s: %v\n", target.Provider, err)
				} else {
					fmt.Printf("Pushed status to %s.\n", target.Provider)
				}
			}
		}
	}

	fmt.Printf("%s: %s → %s\n", ticketID, oldStatus, status)
	return nil
}
