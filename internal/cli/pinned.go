package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/djtouchette/rally/internal/model"
	"github.com/djtouchette/rally/internal/store"
	"github.com/spf13/cobra"
)

func newPinnedCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "pinned",
		Short: "List pinned tickets",
		RunE: func(cmd *cobra.Command, args []string) error {
			pins, err := store.LoadPins()
			if err != nil {
				return err
			}

			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(pins)
			}

			if len(pins) == 0 {
				fmt.Println("No pinned tickets. Use `rally pin <id>` to pin one.")
				return nil
			}

			tickets, err := loadLocalTickets()
			if err != nil {
				return err
			}
			byID := make(map[string]model.Ticket, len(tickets))
			for _, t := range tickets {
				byID[t.ID] = t
			}

			for _, p := range pins {
				title := "(ticket not synced)"
				if t, ok := byID[p.TicketID]; ok {
					title = t.Title
				}
				fmt.Printf("  %-12s  %s\n", p.TicketID, title)
				if p.Note != "" {
					fmt.Printf("                — %s\n", p.Note)
				}
			}
			fmt.Printf("\n%d pinned\n", len(pins))
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}
