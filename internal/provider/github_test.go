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

func TestGithubStatusAndStateChange(t *testing.T) {
	if got := githubStatus("open", ""); got != model.StatusTodo {
		t.Errorf("open -> %q, want todo", got)
	}
	if got := githubStatus("closed", "completed"); got != model.StatusDone {
		t.Errorf("closed/completed -> %q, want done", got)
	}
	if got := githubStatus("closed", "not_planned"); got != model.StatusCancelled {
		t.Errorf("closed/not_planned -> %q, want cancelled", got)
	}
	if s, r := githubStateChange(model.StatusDone); s != "closed" || r != "completed" {
		t.Errorf("done -> %s/%s, want closed/completed", s, r)
	}
	if s, r := githubStateChange(model.StatusCancelled); s != "closed" || r != "not_planned" {
		t.Errorf("cancelled -> %s/%s, want closed/not_planned", s, r)
	}
	if s, _ := githubStateChange(model.StatusInProgress); s != "open" {
		t.Errorf("in_progress -> %s, want open", s)
	}
}

func TestGithubPriorityFromLabels(t *testing.T) {
	cases := []struct {
		labels []string
		want   model.Priority
	}{
		{[]string{"bug", "P0"}, model.PriorityUrgent},
		{[]string{"priority: high"}, model.PriorityHigh},
		{[]string{"P2"}, model.PriorityMedium},
		{[]string{"low-priority"}, model.PriorityLow},
		{[]string{"bug", "docs"}, model.PriorityNone},
	}
	for _, c := range cases {
		if got := githubPriority(c.labels); got != c.want {
			t.Errorf("githubPriority(%v) = %q, want %q", c.labels, got, c.want)
		}
	}
}

func TestGithubFetchAssigned(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		// One real issue, one PR (must be skipped).
		io.WriteString(w, `[
			{"number":42,"title":"Fix login","body":"desc","state":"open",
			 "html_url":"https://github.com/acme/web/issues/42",
			 "created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-03T03:04:05Z",
			 "labels":[{"name":"bug"},{"name":"P1"}],
			 "user":{"login":"alice"},"assignee":{"login":"dj"},
			 "repository":{"full_name":"acme/web"}},
			{"number":43,"title":"A PR","state":"open","pull_request":{},
			 "repository":{"full_name":"acme/web"}}
		]`)
	}))
	defer srv.Close()
	orig := githubAPIURL
	githubAPIURL = srv.URL
	defer func() { githubAPIURL = orig }()

	g := &GitHub{}
	tickets, err := g.FetchAssigned(context.Background(), Credentials{Method: AuthAPIKey, Token: "ghp_x"}, FetchOpts{})
	if err != nil {
		t.Fatalf("FetchAssigned: %v", err)
	}
	if sawAuth != "Bearer ghp_x" {
		t.Errorf("Authorization = %q, want Bearer ghp_x", sawAuth)
	}
	if len(tickets) != 1 {
		t.Fatalf("got %d tickets, want 1 (PR should be skipped)", len(tickets))
	}
	got := tickets[0]
	if got.ID != "acme/web#42" || got.ProviderID != "acme/web#42" {
		t.Errorf("ID/ProviderID = %q/%q, want acme/web#42", got.ID, got.ProviderID)
	}
	if got.Title != "Fix login" || got.Status != model.StatusTodo {
		t.Errorf("title/status = %q/%q", got.Title, got.Status)
	}
	if got.Priority != model.PriorityHigh {
		t.Errorf("priority = %q, want high (from P1 label)", got.Priority)
	}
	if got.Assignee != "dj" || got.Creator != "alice" || got.Project != "acme/web" {
		t.Errorf("assignee/creator/project = %q/%q/%q", got.Assignee, got.Creator, got.Project)
	}
}

func TestGithubUpdateStatus(t *testing.T) {
	var sawPath, sawState, sawReason string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		sawState, _ = req["state"].(string)
		sawReason, _ = req["state_reason"].(string)
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	orig := githubAPIURL
	githubAPIURL = srv.URL
	defer func() { githubAPIURL = orig }()

	g := &GitHub{}
	if err := g.UpdateStatus(context.Background(), Credentials{Token: "t"}, "acme/web#42", model.StatusDone); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if !strings.HasSuffix(sawPath, "/repos/acme/web/issues/42") {
		t.Errorf("path = %q, want .../repos/acme/web/issues/42", sawPath)
	}
	if sawState != "closed" || sawReason != "completed" {
		t.Errorf("state/reason = %q/%q, want closed/completed", sawState, sawReason)
	}

	// A malformed provider ID errors.
	if err := g.UpdateStatus(context.Background(), Credentials{Token: "t"}, "no-hash", model.StatusDone); err == nil {
		t.Error("expected error for provider ID without #number")
	}
}
