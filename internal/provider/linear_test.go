package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestLinearTokenExchangeAndRefresh(t *testing.T) {
	var lastGrant, lastCode, lastRefresh string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		lastGrant = r.Form.Get("grant_type")
		lastCode = r.Form.Get("code")
		lastRefresh = r.Form.Get("refresh_token")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"at-new","refresh_token":"rt-new","token_type":"Bearer","expires_in":86399,"scope":"read,write"}`)
	}))
	defer srv.Close()
	orig := linearTokenURL
	linearTokenURL = srv.URL
	defer func() { linearTokenURL = orig }()

	l := &Linear{}
	cfg := OAuthConfig{ClientID: "cid", ClientSecret: "csec"}

	// Code exchange captures both tokens and a ~24h expiry.
	ts, err := l.ExchangeCode(context.Background(), cfg, "the-code", "http://localhost/cb")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if lastGrant != "authorization_code" || lastCode != "the-code" {
		t.Errorf("exchange sent grant=%q code=%q", lastGrant, lastCode)
	}
	if ts.AccessToken != "at-new" || ts.RefreshToken != "rt-new" {
		t.Errorf("exchange tokens: access=%q refresh=%q", ts.AccessToken, ts.RefreshToken)
	}
	if d := time.Until(ts.ExpiresAt); d < 23*time.Hour || d > 25*time.Hour {
		t.Errorf("expiry = %v, want ~24h", d)
	}

	// Refresh uses the refresh_token grant and returns the rotated pair.
	ts2, err := l.RefreshToken(context.Background(), cfg, "rt-old")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if lastGrant != "refresh_token" || lastRefresh != "rt-old" {
		t.Errorf("refresh sent grant=%q refresh=%q", lastGrant, lastRefresh)
	}
	if ts2.AccessToken != "at-new" || ts2.RefreshToken != "rt-new" {
		t.Errorf("refresh tokens: access=%q refresh=%q", ts2.AccessToken, ts2.RefreshToken)
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
