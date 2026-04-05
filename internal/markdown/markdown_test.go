package markdown

import (
	"testing"
	"time"

	"github.com/djtouchette/rally/internal/model"
)

func TestRoundTrip(t *testing.T) {
	due := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	ticket := model.Ticket{
		ID:          "PROJ-123",
		Provider:    "jira",
		URL:         "https://myco.atlassian.net/browse/PROJ-123",
		Title:       "Fix payment retry logic",
		Description: "The payment retry logic silently drops failures after 3 attempts.\nNeed to add exponential backoff.",
		Labels:      []string{"backend", "payments"},
		Status:      model.StatusInProgress,
		Priority:    model.PriorityHigh,
		Type:        "bug",
		Project:     "Backend",
		Team:        "Platform",
		Assignee:    "djtouchette",
		CreatedAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		DueDate:     &due,
	}

	md := Write(ticket)
	got, err := Parse(md)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got.ID != ticket.ID {
		t.Errorf("ID: got %q, want %q", got.ID, ticket.ID)
	}
	if got.Title != ticket.Title {
		t.Errorf("Title: got %q, want %q", got.Title, ticket.Title)
	}
	if got.Provider != ticket.Provider {
		t.Errorf("Provider: got %q, want %q", got.Provider, ticket.Provider)
	}
	if got.Status != ticket.Status {
		t.Errorf("Status: got %q, want %q", got.Status, ticket.Status)
	}
	if got.Priority != ticket.Priority {
		t.Errorf("Priority: got %q, want %q", got.Priority, ticket.Priority)
	}
	if got.Type != ticket.Type {
		t.Errorf("Type: got %q, want %q", got.Type, ticket.Type)
	}
	if got.Project != ticket.Project {
		t.Errorf("Project: got %q, want %q", got.Project, ticket.Project)
	}
	if got.Team != ticket.Team {
		t.Errorf("Team: got %q, want %q", got.Team, ticket.Team)
	}
	if got.Assignee != ticket.Assignee {
		t.Errorf("Assignee: got %q, want %q", got.Assignee, ticket.Assignee)
	}
	if got.URL != ticket.URL {
		t.Errorf("URL: got %q, want %q", got.URL, ticket.URL)
	}
	if got.Description != ticket.Description {
		t.Errorf("Description: got %q, want %q", got.Description, ticket.Description)
	}
	if len(got.Labels) != len(ticket.Labels) {
		t.Fatalf("Labels count: got %d, want %d", len(got.Labels), len(ticket.Labels))
	}
	for i := range ticket.Labels {
		if got.Labels[i] != ticket.Labels[i] {
			t.Errorf("Labels[%d]: got %q, want %q", i, got.Labels[i], ticket.Labels[i])
		}
	}
	if !got.CreatedAt.Equal(ticket.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, ticket.CreatedAt)
	}
	if got.DueDate == nil {
		t.Fatal("DueDate: got nil, want non-nil")
	}
	if !got.DueDate.Equal(*ticket.DueDate) {
		t.Errorf("DueDate: got %v, want %v", got.DueDate, ticket.DueDate)
	}
}

func TestFilename(t *testing.T) {
	ticket := model.Ticket{ID: "PROJ-123", Provider: "jira"}
	got := Filename(ticket)
	want := "jira-PROJ-123.md"
	if got != want {
		t.Errorf("Filename: got %q, want %q", got, want)
	}
}

func TestParseMinimal(t *testing.T) {
	md := "# ABC-1: Simple task\n\n**Provider:** linear\n**Status:** todo\n**Priority:** medium\n"
	got, err := Parse(md)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.ID != "ABC-1" {
		t.Errorf("ID: got %q, want %q", got.ID, "ABC-1")
	}
	if got.Status != model.StatusTodo {
		t.Errorf("Status: got %q, want %q", got.Status, model.StatusTodo)
	}
	if got.Description != "" {
		t.Errorf("Description: got %q, want empty", got.Description)
	}
}
