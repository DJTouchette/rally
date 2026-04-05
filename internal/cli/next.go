package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/djtouchette/rally/internal/model"
	"github.com/spf13/cobra"
)

func newNextCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "next",
		Short: "Show the next recommended ticket to work on",
		RunE: func(cmd *cobra.Command, args []string) error {
			tickets, err := loadLocalTickets()
			if err != nil {
				return err
			}

			// Filter to actionable statuses
			var actionable []model.Ticket
			for _, t := range tickets {
				if t.Status == model.StatusTodo || t.Status == model.StatusBacklog {
					actionable = append(actionable, t)
				}
			}

			if len(actionable) == 0 {
				fmt.Println("No actionable tickets. All done or run `rally sync`.")
				return nil
			}

			model.SortByPriorityThenAge(actionable)
			next := actionable[0]

			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(next)
			}

			fmt.Printf("Next ticket:\n\n")
			fmt.Printf("  %s: %s\n", next.ID, next.Title)
			fmt.Printf("  Priority: %s\n", next.Priority)
			fmt.Printf("  Status:   %s\n", next.Status)
			if next.Project != "" {
				fmt.Printf("  Project:  %s\n", next.Project)
			}
			if next.URL != "" {
				fmt.Printf("  URL:      %s\n", next.URL)
			}
			fmt.Printf("\nRun `rally start %s` to begin work.\n", next.ID)

			return nil
		},
	}

	cmd.Flags().BoolVar(&outputJSON, "json", false, "output as JSON")

	return cmd
}
