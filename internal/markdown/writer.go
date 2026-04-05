package markdown

import (
	"fmt"
	"strings"

	"github.com/djtouchette/rally/internal/model"
)

// Write serializes a Ticket to the rally markdown format.
func Write(t model.Ticket) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s: %s\n", t.ID, t.Title)
	b.WriteString("\n")
	fmt.Fprintf(&b, "**Provider:** %s\n", t.Provider)
	fmt.Fprintf(&b, "**Status:** %s\n", t.Status)
	fmt.Fprintf(&b, "**Priority:** %s\n", t.Priority)

	if t.Type != "" {
		fmt.Fprintf(&b, "**Type:** %s\n", t.Type)
	}
	if t.Project != "" {
		fmt.Fprintf(&b, "**Project:** %s\n", t.Project)
	}
	if t.Team != "" {
		fmt.Fprintf(&b, "**Team:** %s\n", t.Team)
	}
	if t.Epic != "" {
		fmt.Fprintf(&b, "**Epic:** %s\n", t.Epic)
	}
	if t.Assignee != "" {
		fmt.Fprintf(&b, "**Assignee:** %s\n", t.Assignee)
	}
	if !t.CreatedAt.IsZero() {
		fmt.Fprintf(&b, "**Created:** %s\n", t.CreatedAt.Format("2006-01-02"))
	}
	if t.DueDate != nil && !t.DueDate.IsZero() {
		fmt.Fprintf(&b, "**Due:** %s\n", t.DueDate.Format("2006-01-02"))
	}
	if t.URL != "" {
		fmt.Fprintf(&b, "**URL:** %s\n", t.URL)
	}

	if t.Description != "" {
		b.WriteString("\n## Description\n\n")
		b.WriteString(t.Description)
		b.WriteString("\n")
	}

	if len(t.Labels) > 0 {
		b.WriteString("\n## Labels\n\n")
		for _, l := range t.Labels {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}

	return b.String()
}

// Filename returns the expected markdown filename for a ticket.
func Filename(t model.Ticket) string {
	return fmt.Sprintf("%s-%s.md", t.Provider, t.ID)
}
