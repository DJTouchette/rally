package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/djtouchette/rally/internal/markdown"
	"github.com/djtouchette/rally/internal/model"
	"github.com/djtouchette/rally/internal/store"
	"github.com/spf13/cobra"
)

var (
	listStatus   string
	listPriority string
	listProvider string
	listJSON     bool
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List synced tickets",
		RunE:  runList,
	}

	cmd.Flags().StringVar(&listStatus, "status", "", "filter by status (todo, in_progress, done, ...)")
	cmd.Flags().StringVar(&listPriority, "priority", "", "filter by priority (urgent, high, medium, low)")
	cmd.Flags().StringVar(&listProvider, "provider", "", "filter by provider (jira, linear)")
	cmd.Flags().BoolVar(&listJSON, "json", false, "output as JSON")

	return cmd
}

func runList(cmd *cobra.Command, args []string) error {
	tickets, err := loadLocalTickets()
	if err != nil {
		return err
	}

	filter := model.Filter{
		Status:   model.Status(listStatus),
		Priority: model.Priority(listPriority),
		Provider: listProvider,
	}

	var filtered []model.Ticket
	for _, t := range tickets {
		if filter.Match(t) {
			filtered = append(filtered, t)
		}
	}

	model.SortByPriorityThenAge(filtered)

	if listJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	if len(filtered) == 0 {
		fmt.Println("No tickets found. Run `rally sync` to pull tickets.")
		return nil
	}

	for _, t := range filtered {
		printTicketLine(t)
	}
	fmt.Printf("\n%d ticket(s)\n", len(filtered))

	return nil
}

func printTicketLine(t model.Ticket) {
	priority := padRight(string(t.Priority), 6)
	status := padRight(string(t.Status), 11)
	fmt.Printf("  %-12s  %s  %s  %s\n", t.ID, priority, status, t.Title)
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func loadLocalTickets() ([]model.Ticket, error) {
	ticketsDir := store.TicketsDir()

	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading tickets directory: %w", err)
	}

	var tickets []model.Ticket
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ticketsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		t, err := markdown.Parse(string(data))
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		tickets = append(tickets, t)
	}

	return tickets, nil
}
