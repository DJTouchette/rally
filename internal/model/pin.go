package model

import "time"

// Pin marks a ticket as actively in the user's working context.
// Pinned tickets are surfaced into MCP-driven chat sessions so the agent
// always sees what the user is currently working on.
type Pin struct {
	TicketID string    `json:"ticket_id"`
	PinnedAt time.Time `json:"pinned_at"`
	Note     string    `json:"note,omitempty"`
}
