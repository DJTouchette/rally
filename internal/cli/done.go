package cli

import (
	"fmt"

	"github.com/djtouchette/rally/internal/model"
	"github.com/spf13/cobra"
)

func newDoneCmd() *cobra.Command {
	var noPush bool

	cmd := &cobra.Command{
		Use:   "done [id]",
		Short: "Mark a ticket as done and push to provider",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var ticketID string

			if len(args) == 1 {
				ticketID = args[0]
			} else {
				// Find the single in-progress ticket
				tickets, err := loadLocalTickets()
				if err != nil {
					return err
				}

				var inProgress []model.Ticket
				for _, t := range tickets {
					if t.Status == model.StatusInProgress {
						inProgress = append(inProgress, t)
					}
				}

				switch len(inProgress) {
				case 0:
					return fmt.Errorf("no in-progress tickets found — specify a ticket ID")
				case 1:
					ticketID = inProgress[0].ID
				default:
					fmt.Println("Multiple in-progress tickets:")
					for _, t := range inProgress {
						fmt.Printf("  %s: %s\n", t.ID, t.Title)
					}
					return fmt.Errorf("specify which ticket to complete")
				}
			}

			return runTransition(ticketID, model.StatusDone, noPush)
		},
	}

	cmd.Flags().BoolVar(&noPush, "local", false, "only update locally, don't push to provider")

	return cmd
}
