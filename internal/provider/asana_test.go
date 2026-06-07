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

func TestAsanaStatus(t *testing.T) {
	cases := []struct {
		section   string
		completed bool
		want      model.Status
	}{
		{"", false, model.StatusTodo},
		{"Backlog", false, model.StatusBacklog},
		{"In Progress", false, model.StatusInProgress},
		{"Code Review", false, model.StatusInReview},
		{"Done", false, model.StatusDone},
		{"To Do", true, model.StatusDone}, // completed overrides section
	}
	for _, c := range cases {
		if got := asanaStatus(c.section, c.completed); got != c.want {
			t.Errorf("asanaStatus(%q, %v) = %q, want %q", c.section, c.completed, got, c.want)
		}
	}
}

func TestAsanaFetchAssigned(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/users/me"):
			io.WriteString(w, `{"data":{"gid":"u1","workspaces":[{"gid":"ws1","name":"Acme"}]}}`)
		case strings.HasPrefix(r.URL.Path, "/tasks"):
			if r.URL.Query().Get("workspace") != "ws1" {
				t.Errorf("workspace = %q, want ws1", r.URL.Query().Get("workspace"))
			}
			io.WriteString(w, `{"data":[
				{"gid":"t1","name":"Ship feature","notes":"do it","completed":false,
				 "permalink_url":"https://app.asana.com/0/0/t1","created_at":"2026-01-02T03:04:05.000Z",
				 "modified_at":"2026-01-03T03:04:05.000Z","due_on":"2026-02-01",
				 "assignee":{"name":"DJ"},"projects":[{"name":"Roadmap"}],
				 "memberships":[{"section":{"name":"In Progress"}}],
				 "tags":[{"name":"backend"}]}
			],"next_page":null}`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()
	orig := asanaAPIURL
	asanaAPIURL = srv.URL
	defer func() { asanaAPIURL = orig }()

	a := &Asana{}
	tickets, err := a.FetchAssigned(context.Background(), Credentials{Method: AuthAPIKey, Token: "pat"}, FetchOpts{})
	if err != nil {
		t.Fatalf("FetchAssigned: %v", err)
	}
	if sawAuth != "Bearer pat" {
		t.Errorf("Authorization = %q, want Bearer pat", sawAuth)
	}
	if len(tickets) != 1 {
		t.Fatalf("got %d tickets, want 1", len(tickets))
	}
	got := tickets[0]
	if got.ID != "t1" || got.Provider != "asana" || got.Title != "Ship feature" {
		t.Errorf("id/provider/title = %q/%q/%q", got.ID, got.Provider, got.Title)
	}
	if got.Status != model.StatusInProgress {
		t.Errorf("status = %q, want in_progress (from section)", got.Status)
	}
	if got.Assignee != "DJ" || got.Project != "Roadmap" {
		t.Errorf("assignee/project = %q/%q", got.Assignee, got.Project)
	}
	if len(got.Labels) != 1 || got.Labels[0] != "backend" {
		t.Errorf("labels = %v, want [backend]", got.Labels)
	}
	if got.DueDate == nil {
		t.Error("due date not parsed")
	}
}

func TestAsanaUpdateStatus(t *testing.T) {
	var sawMethod, sawPath string
	var sawCompleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawMethod = r.Method
		sawPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Data struct {
				Completed bool `json:"completed"`
			} `json:"data"`
		}
		json.Unmarshal(body, &req)
		sawCompleted = req.Data.Completed
		io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()
	orig := asanaAPIURL
	asanaAPIURL = srv.URL
	defer func() { asanaAPIURL = orig }()

	a := &Asana{}
	if err := a.UpdateStatus(context.Background(), Credentials{Token: "pat"}, "t1", model.StatusDone); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if sawMethod != "PUT" || !strings.HasSuffix(sawPath, "/tasks/t1") {
		t.Errorf("method/path = %s %q", sawMethod, sawPath)
	}
	if !sawCompleted {
		t.Error("done should set completed=true (wrapped in data envelope)")
	}

	// A non-terminal status sets completed=false.
	_ = a.UpdateStatus(context.Background(), Credentials{Token: "pat"}, "t1", model.StatusInProgress)
	if sawCompleted {
		t.Error("in_progress should set completed=false")
	}
}
