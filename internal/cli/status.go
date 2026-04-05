package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/djtouchette/rally/internal/model"
	"github.com/djtouchette/rally/internal/store"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show connection health, secrets, and ticket summary",
		RunE:  runStatus,
	}
	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, _, err := store.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Show connections
	if len(cfg.Connections) == 0 {
		fmt.Println("No providers connected.")
		fmt.Println("Run `rally connect jira` or `rally connect linear` to get started.")
		return nil
	}

	fmt.Println("Connections:")
	for _, conn := range cfg.Connections {
		project := ""
		if conn.Project != "" {
			project = fmt.Sprintf(" (project: %s)", conn.Project)
		}
		fmt.Printf("  %s%s\n", conn.Provider, project)
	}

	// Show secrets status
	if len(cfg.Secrets) > 0 {
		fmt.Println("\nSecrets:")
		for _, s := range cfg.Secrets {
			present := os.Getenv(s.Name) != ""
			indicator := "missing"
			if present {
				indicator = "ok"
			}
			fmt.Printf("  %s: %s", s.Name, indicator)
			if !present && s.Required {
				domains := strings.Join(s.Domains, ",")
				fmt.Printf("\n    vaulty set %s --domains %s", s.Name, domains)
			}
			fmt.Println()
		}

		missing := cfg.MissingSecrets()
		if len(missing) > 0 {
			fmt.Printf("\n%d required secret(s) missing from environment.\n", len(missing))
			fmt.Println("Store them in vaulty, then run commands via:")

			// Collect all secret names for the vaulty exec hint
			var names []string
			for _, s := range missing {
				names = append(names, s.Name)
			}
			fmt.Printf("  vaulty exec --secrets %s -- rally <command>\n", strings.Join(names, ","))
		}
	}

	// Show sync state
	state, err := store.LoadState()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if state.LastSync.IsZero() {
		fmt.Println("\nNever synced. Run `rally sync`.")
	} else {
		fmt.Printf("\nLast sync: %s\n", state.LastSync.Format("2006-01-02 15:04:05"))
	}

	// Show ticket counts
	tickets, err := loadLocalTickets()
	if err != nil {
		return err
	}

	if len(tickets) == 0 {
		fmt.Println("No local tickets.")
		return nil
	}

	counts := make(map[model.Status]int)
	for _, t := range tickets {
		counts[t.Status]++
	}

	fmt.Printf("\nTickets: %d total\n", len(tickets))
	for _, s := range []model.Status{model.StatusBacklog, model.StatusTodo, model.StatusInProgress, model.StatusInReview, model.StatusDone} {
		if c := counts[s]; c > 0 {
			fmt.Printf("  %s: %d\n", s, c)
		}
	}

	return nil
}
