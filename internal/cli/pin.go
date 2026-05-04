package cli

import (
	"fmt"

	"github.com/djtouchette/rally/internal/store"
	"github.com/spf13/cobra"
)

func newPinCmd() *cobra.Command {
	var note string

	cmd := &cobra.Command{
		Use:   "pin <id>",
		Short: "Pin a ticket so it stays in chat context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ticketID := args[0]
			tickets, err := loadLocalTickets()
			if err != nil {
				return err
			}
			var found bool
			for _, t := range tickets {
				if t.ID == ticketID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("ticket %s not found — run `rally sync` first", ticketID)
			}
			if err := store.AddPin(ticketID, note); err != nil {
				return err
			}
			fmt.Printf("Pinned %s\n", ticketID)
			return nil
		},
	}

	cmd.Flags().StringVar(&note, "note", "", "short note for why this ticket is pinned")

	return cmd
}
