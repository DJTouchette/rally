package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/djtouchette/rally/internal/model"
)

func TestNormalizeLinearStatus(t *testing.T) {
	cases := []struct {
		state linearState
		want  model.Status
	}{
		{linearState{Name: "Backlog", Type: "backlog"}, model.StatusBacklog},
		{linearState{Name: "Triage", Type: "triage"}, model.StatusTodo},
		{linearState{Name: "Todo", Type: "unstarted"}, model.StatusTodo},
		{linearState{Name: "In Progress", Type: "started"}, model.StatusInProgress},
		{linearState{Name: "In Review", Type: "started"}, model.StatusInReview},
		{linearState{Name: "Done", Type: "completed"}, model.StatusDone},
		{linearState{Name: "Canceled", Type: "canceled"}, model.StatusCancelled},
		{linearState{Name: "Weird", Type: "unknown"}, model.StatusTodo},
	}
	for _, c := range cases {
		if got := normalizeLinearStatus(c.state); got != c.want {
			t.Errorf("normalizeLinearStatus(%+v) = %q, want %q", c.state, got, c.want)
		}
	}
}

func TestNormalizeLinearPriority(t *testing.T) {
	cases := map[int]model.Priority{
		0: model.PriorityNone,
		1: model.PriorityUrgent,
		2: model.PriorityHigh,
		3: model.PriorityMedium,
		4: model.PriorityLow,
	}
	for in, want := range cases {
		if got := normalizeLinearPriority(in); got != want {
			t.Errorf("normalizeLinearPriority(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestLinearFetchAssigned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok123" {
			t.Errorf("Authorization = %q, want Bearer tok123", got)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"viewer":{"assignedIssues":{
			"nodes":[{
				"id":"uuid-1","identifier":"ENG-42","title":"Fix login","description":"desc",
				"url":"https://linear.app/x/issue/ENG-42","priority":1,
				"createdAt":"2026-01-02T03:04:05.000Z","updatedAt":"2026-01-03T03:04:05.000Z",
				"state":{"name":"In Progress","type":"started"},
				"labels":{"nodes":[{"name":"bug"},{"name":"auth"}]},
				"team":{"key":"ENG","name":"Engineering"},
				"assignee":{"name":"dj","displayName":"DJ T"},
				"creator":{"name":"alice"},
				"parent":{"identifier":"ENG-10"}
			}],
			"pageInfo":{"hasNextPage":false,"endCursor":""}
		}}}}`)
	}))
	defer srv.Close()
	orig := linearAPIURL
	linearAPIURL = srv.URL
	defer func() { linearAPIURL = orig }()

	l := &Linear{}
	tickets, err := l.FetchAssigned(context.Background(), "tok123", FetchOpts{})
	if err != nil {
		t.Fatalf("FetchAssigned: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("got %d tickets, want 1", len(tickets))
	}
	got := tickets[0]
	checks := map[string]struct{ got, want string }{
		"ID":         {got.ID, "ENG-42"},
		"ProviderID": {got.ProviderID, "uuid-1"},
		"Provider":   {got.Provider, "linear"},
		"Title":      {got.Title, "Fix login"},
		"Status":     {string(got.Status), string(model.StatusInProgress)},
		"Priority":   {string(got.Priority), string(model.PriorityUrgent)},
		"Team":       {got.Team, "Engineering"},
		"Project":    {got.Project, "ENG"},
		"Assignee":   {got.Assignee, "DJ T"},
		"Creator":    {got.Creator, "alice"},
		"Parent":     {got.Parent, "ENG-10"},
	}
	for field, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", field, c.got, c.want)
		}
	}
	if len(got.Labels) != 2 || got.Labels[0] != "bug" {
		t.Errorf("labels = %v, want [bug auth]", got.Labels)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps not parsed: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestLinearUpdateStatus(t *testing.T) {
	var sentStateID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(req.Query, "issueUpdate") {
			sentStateID, _ = req.Variables["stateId"].(string)
			io.WriteString(w, `{"data":{"issueUpdate":{"success":true}}}`)
			return
		}
		// states lookup
		io.WriteString(w, `{"data":{"issue":{"team":{"states":{"nodes":[
			{"id":"s-backlog","name":"Backlog","type":"backlog"},
			{"id":"s-started","name":"In Progress","type":"started"},
			{"id":"s-done","name":"Done","type":"completed"}
		]}}}}}`)
	}))
	defer srv.Close()
	orig := linearAPIURL
	linearAPIURL = srv.URL
	defer func() { linearAPIURL = orig }()

	l := &Linear{}
	if err := l.UpdateStatus(context.Background(), "tok", "uuid-1", model.StatusInProgress); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if sentStateID != "s-started" {
		t.Errorf("updated to stateId %q, want s-started", sentStateID)
	}

	// A status with no matching state type should error clearly.
	if err := l.UpdateStatus(context.Background(), "tok", "uuid-1", model.StatusCancelled); err == nil {
		t.Error("expected error when no workflow state matches the target status")
	}
}
