package markdown

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"github.com/djtouchette/rally/internal/model"
)

// Parse reads a rally markdown file and returns a Ticket.
func Parse(content string) (model.Ticket, error) {
	var t model.Ticket
	scanner := bufio.NewScanner(strings.NewReader(content))

	// Parse title line: # ID: Title
	if scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimPrefix(line, "# ")
		if idx := strings.Index(line, ": "); idx != -1 {
			t.ID = line[:idx]
			t.Title = line[idx+2:]
		} else {
			t.Title = line
		}
	}

	var section string
	var descLines []string
	var labels []string

	for scanner.Scan() {
		line := scanner.Text()

		// Detect section headers
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimPrefix(line, "## ")
			continue
		}

		// Parse bold-key metadata fields
		if strings.HasPrefix(line, "**") && strings.Contains(line, ":**") {
			key, val := parseBoldField(line)
			switch key {
			case "Provider":
				t.Provider = val
			case "Status":
				t.Status = model.Status(val)
			case "Priority":
				t.Priority = model.Priority(val)
			case "Type":
				t.Type = val
			case "Project":
				t.Project = val
			case "Team":
				t.Team = val
			case "Epic":
				t.Epic = val
			case "Assignee":
				t.Assignee = val
			case "Created":
				if parsed, err := time.Parse("2006-01-02", val); err == nil {
					t.CreatedAt = parsed
				}
			case "Due":
				if parsed, err := time.Parse("2006-01-02", val); err == nil {
					t.DueDate = &parsed
				}
			case "URL":
				t.URL = val
			}
			continue
		}

		// Collect section content
		switch section {
		case "Description":
			descLines = append(descLines, line)
		case "Labels":
			trimmed := strings.TrimPrefix(line, "- ")
			if trimmed != "" && trimmed != line {
				labels = append(labels, trimmed)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return t, fmt.Errorf("reading markdown: %w", err)
	}

	t.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
	t.Labels = labels

	return t, nil
}

func parseBoldField(line string) (string, string) {
	// Format: **Key:** value
	line = strings.TrimPrefix(line, "**")
	idx := strings.Index(line, ":**")
	if idx == -1 {
		return "", ""
	}
	key := line[:idx]
	val := strings.TrimSpace(line[idx+3:])
	return key, val
}
