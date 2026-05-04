package cli

import (
	"fmt"

	"github.com/djtouchette/rally/internal/store"
	"github.com/spf13/cobra"
)

func newUnpinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpin <id>",
		Short: "Remove a ticket from the pinned set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ticketID := args[0]
			if err := store.RemovePin(ticketID); err != nil {
				return err
			}
			fmt.Printf("Unpinned %s\n", ticketID)
			return nil
		},
	}
}
