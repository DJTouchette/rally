package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/djtouchette/rally/internal/model"
)

// In API-token mode Jira uses Basic auth against the site host directly (no
// cloud-ID lookup). This exercises the full FetchAssigned HTTP path.
func TestJiraAPIKeyFetchAssigned(t *testing.T) {
	var sawAuth, sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(jiraSearchResult{
			Issues: []jiraIssue{{
				ID:  "10001",
				Key: "TEST-1",
				Fields: jiraFields{
					Summary: "API key issue",
					Status: jiraStatus{Name: "To Do", StatusCategory: struct {
						Key string `json:"key"`
					}{Key: "new"}},
					Priority:  jiraPriority{Name: "High"},
					IssueType: jiraIssueType{Name: "Task"},
					Project:   jiraProject{Key: "TEST"},
					Assignee:  jiraPerson{DisplayName: "Tester"},
					Created:   "2026-04-01T10:00:00.000+0000",
					Updated:   "2026-04-01T10:00:00.000+0000",
				},
			}},
			Total: 1,
		})
	}))
	defer srv.Close()

	j := &Jira{}
	creds := Credentials{
		Method: AuthAPIKey,
		Token:  "api-tok",
		Email:  "me@co.com",
		Site:   srv.URL, // full URL with scheme — used as-is
	}
	tickets, err := j.FetchAssigned(context.Background(), creds, FetchOpts{})
	if err != nil {
		t.Fatalf("FetchAssigned: %v", err)
	}
	if len(tickets) != 1 || tickets[0].ID != "TEST-1" {
		t.Fatalf("got %d tickets (%v), want 1 TEST-1", len(tickets), tickets)
	}
	if tickets[0].Priority != model.PriorityHigh {
		t.Errorf("priority = %q, want high", tickets[0].Priority)
	}

	// No cloud-ID indirection: path hits /rest/api/3/search directly.
	if !strings.HasSuffix(sawPath, "/rest/api/3/search") {
		t.Errorf("request path = %q, want .../rest/api/3/search", sawPath)
	}
	// Basic auth header = base64(email:token).
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("me@co.com:api-tok"))
	if sawAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", sawAuth, wantAuth)
	}
}

// API-token mode requires email and site.
func TestJiraAPIKeyRequiresEmailAndSite(t *testing.T) {
	j := &Jira{}
	_, _, err := j.jiraBase(context.Background(), Credentials{Method: AuthAPIKey, Token: "t"})
	if err == nil {
		t.Error("expected error when email/site missing for api-key auth")
	}
}
