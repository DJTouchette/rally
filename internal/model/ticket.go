package model

import "time"

// Status is the normalized lifecycle state across all providers.
type Status string

const (
	StatusBacklog    Status = "backlog"
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusInReview   Status = "in_review"
	StatusDone       Status = "done"
	StatusCancelled  Status = "cancelled"
)

// Priority is the normalized urgency level across all providers.
type Priority string

const (
	PriorityUrgent Priority = "urgent"
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
	PriorityNone   Priority = "none"
)

// PriorityRank returns a numeric rank for sorting (lower = more urgent).
func PriorityRank(p Priority) int {
	switch p {
	case PriorityUrgent:
		return 0
	case PriorityHigh:
		return 1
	case PriorityMedium:
		return 2
	case PriorityLow:
		return 3
	case PriorityNone:
		return 4
	default:
		return 5
	}
}

// Ticket is the provider-agnostic representation of a work item.
type Ticket struct {
	// Identity
	ID         string `json:"id" yaml:"id"`
	ProviderID string `json:"provider_id" yaml:"provider_id"`
	Provider   string `json:"provider" yaml:"provider"`
	URL        string `json:"url" yaml:"url"`

	// Content
	Title       string   `json:"title" yaml:"title"`
	Description string   `json:"description" yaml:"description"`
	Labels      []string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// Classification
	Status   Status   `json:"status" yaml:"status"`
	Priority Priority `json:"priority" yaml:"priority"`
	Type     string   `json:"type,omitempty" yaml:"type,omitempty"`

	// Hierarchy
	Project string `json:"project,omitempty" yaml:"project,omitempty"`
	Team    string `json:"team,omitempty" yaml:"team,omitempty"`
	Epic    string `json:"epic,omitempty" yaml:"epic,omitempty"`
	Parent  string `json:"parent,omitempty" yaml:"parent,omitempty"`

	// People
	Assignee string `json:"assignee" yaml:"assignee"`
	Creator  string `json:"creator,omitempty" yaml:"creator,omitempty"`

	// Dates
	CreatedAt time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" yaml:"updated_at"`
	DueDate   *time.Time `json:"due_date,omitempty" yaml:"due_date,omitempty"`

	// Sync metadata
	SyncedAt time.Time `json:"synced_at" yaml:"-"`
	SyncHash string    `json:"sync_hash" yaml:"-"`
}
