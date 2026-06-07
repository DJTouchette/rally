package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/djtouchette/rally/internal/model"
)

// Linear implements the Provider interface for Linear (https://linear.app).
// Linear uses OAuth2 for auth and a GraphQL API for data.
type Linear struct{}

// Endpoints are vars (not consts) so tests can point them at a mock server.
var (
	linearTokenURL = "https://api.linear.app/oauth/token"
	linearAPIURL   = "https://api.linear.app/graphql"
)

func (l *Linear) Name() string { return "linear" }

func (l *Linear) AuthURL(clientID, redirectURI, state string) string {
	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"state":         {state},
		"response_type": {"code"},
		"scope":         {"read,write"},
	}
	return "https://linear.app/oauth/authorize?" + params.Encode()
}

func (l *Linear) ExchangeCode(ctx context.Context, cfg OAuthConfig, code, redirectURI string) (*TokenSet, error) {
	return l.tokenRequest(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	})
}

func (l *Linear) RefreshToken(ctx context.Context, cfg OAuthConfig, refreshToken string) (*TokenSet, error) {
	// Since Linear's April 2026 migration, access tokens expire in 24h and the
	// token endpoint issues refresh tokens. A successful refresh returns a new
	// access token AND a new refresh token (the old pair is invalidated).
	return l.tokenRequest(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"refresh_token": {refreshToken},
	})
}

// tokenRequest performs an OAuth token request (code exchange or refresh) and
// parses the response.
func (l *Linear) tokenRequest(ctx context.Context, body url.Values) (*TokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", linearTokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token request failed (%d): %s", resp.StatusCode, respBody)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour) // Linear default
	if tokenResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return &TokenSet{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		Scope:        tokenResp.Scope,
	}, nil
}

// linearAuthHeader builds the Authorization header value. Linear OAuth tokens
// use the Bearer scheme; personal API keys are sent as the raw key value with
// NO "Bearer" prefix — sending Bearer with an API key fails authentication.
func linearAuthHeader(creds Credentials) string {
	if creds.IsAPIKey() {
		return creds.Token
	}
	return "Bearer " + creds.Token
}

// --- GraphQL types ---

type linearState struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type linearIssue struct {
	ID          string      `json:"id"`
	Identifier  string      `json:"identifier"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	URL         string      `json:"url"`
	Priority    int         `json:"priority"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	State       linearState `json:"state"`
	Labels      struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Team struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"team"`
	Assignee struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"assignee"`
	Creator struct {
		Name string `json:"name"`
	} `json:"creator"`
	Parent struct {
		Identifier string `json:"identifier"`
	} `json:"parent"`
}

const linearAssignedQuery = `query($after: String, $first: Int!, $filter: IssueFilter) {
  viewer {
    assignedIssues(first: $first, after: $after, filter: $filter) {
      nodes {
        id identifier title description url priority createdAt updatedAt
        state { name type }
        labels { nodes { name } }
        team { key name }
        assignee { name displayName }
        creator { name }
        parent { identifier }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

func (l *Linear) FetchAssigned(ctx context.Context, creds Credentials, opts FetchOpts) ([]model.Ticket, error) {
	max := opts.MaxResults
	if max <= 0 {
		max = 200
	}

	// Open (not completed/canceled) issues, optionally scoped to one team.
	filter := map[string]any{
		"state": map[string]any{"type": map[string]any{"nin": []string{"completed", "canceled"}}},
	}
	if opts.Project != "" {
		filter["team"] = map[string]any{"key": map[string]any{"eq": opts.Project}}
	}

	var tickets []model.Ticket
	var after *string
	for {
		pageSize := max - len(tickets)
		if pageSize > 100 {
			pageSize = 100
		}
		if pageSize <= 0 {
			break
		}

		var resp struct {
			Viewer struct {
				AssignedIssues struct {
					Nodes    []linearIssue `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"assignedIssues"`
			} `json:"viewer"`
		}
		vars := map[string]any{"first": pageSize, "filter": filter}
		if after != nil {
			vars["after"] = *after
		}
		if err := l.graphql(ctx, creds, linearAssignedQuery, vars, &resp); err != nil {
			return nil, err
		}

		for _, issue := range resp.Viewer.AssignedIssues.Nodes {
			tickets = append(tickets, l.normalizeIssue(issue))
		}

		page := resp.Viewer.AssignedIssues.PageInfo
		if !page.HasNextPage || len(tickets) >= max {
			break
		}
		cursor := page.EndCursor
		after = &cursor
	}

	return tickets, nil
}

func (l *Linear) normalizeIssue(issue linearIssue) model.Ticket {
	t := model.Ticket{
		ID:          issue.Identifier,
		ProviderID:  issue.ID,
		Provider:    "linear",
		URL:         issue.URL,
		Title:       issue.Title,
		Description: issue.Description,
		Status:      normalizeLinearStatus(issue.State),
		Priority:    normalizeLinearPriority(issue.Priority),
		Team:        issue.Team.Name,
		Project:     issue.Team.Key,
		CreatedAt:   issue.CreatedAt,
		UpdatedAt:   issue.UpdatedAt,
	}
	for _, lbl := range issue.Labels.Nodes {
		t.Labels = append(t.Labels, lbl.Name)
	}
	if issue.Assignee.DisplayName != "" {
		t.Assignee = issue.Assignee.DisplayName
	} else {
		t.Assignee = issue.Assignee.Name
	}
	t.Creator = issue.Creator.Name
	if issue.Parent.Identifier != "" {
		t.Parent = issue.Parent.Identifier
	}
	return t
}

const (
	linearIssueStatesQuery = `query($id: String!) {
  issue(id: $id) {
    team {
      states { nodes { id name type } }
    }
  }
}`
	linearIssueUpdateMutation = `mutation($id: String!, $stateId: String!) {
  issueUpdate(id: $id, input: { stateId: $stateId }) { success }
}`
)

func (l *Linear) UpdateStatus(ctx context.Context, creds Credentials, providerID string, status model.Status) error {
	// Linear workflow states are per-team and identified by UUID, so we look up
	// the issue's team states and pick one whose type matches the target status.
	var statesResp struct {
		Issue struct {
			Team struct {
				States struct {
					Nodes []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
						Type string `json:"type"`
					} `json:"nodes"`
				} `json:"states"`
			} `json:"team"`
		} `json:"issue"`
	}
	if err := l.graphql(ctx, creds, linearIssueStatesQuery, map[string]any{"id": providerID}, &statesResp); err != nil {
		return err
	}

	wantType := linearStateTypeFor(status)
	stateID := ""
	for _, s := range statesResp.Issue.Team.States.Nodes {
		// Prefer a state literally named "review" for the in_review status.
		if status == model.StatusInReview && strings.Contains(strings.ToLower(s.Name), "review") {
			stateID = s.ID
			break
		}
		if s.Type == wantType && stateID == "" {
			stateID = s.ID
		}
	}
	if stateID == "" {
		return fmt.Errorf("no Linear workflow state matching %q for this issue's team", status)
	}

	var updateResp struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}
	if err := l.graphql(ctx, creds, linearIssueUpdateMutation, map[string]any{"id": providerID, "stateId": stateID}, &updateResp); err != nil {
		return err
	}
	if !updateResp.IssueUpdate.Success {
		return fmt.Errorf("linear issueUpdate reported failure for %s", providerID)
	}
	return nil
}

// graphql executes a Linear GraphQL request and unmarshals the "data" field
// into out.
func (l *Linear) graphql(ctx context.Context, creds Credentials, query string, vars map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return fmt.Errorf("marshaling graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", linearAPIURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating graphql request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", linearAuthHeader(creds))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("graphql request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("linear graphql failed (%d): %s", resp.StatusCode, body)
	}

	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("decoding graphql response: %w", err)
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("linear graphql error: %s", env.Errors[0].Message)
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decoding graphql data: %w", err)
		}
	}
	return nil
}

// normalizeLinearStatus maps a Linear workflow state to a rally status. Linear
// state types are: triage, backlog, unstarted, started, completed, canceled.
func normalizeLinearStatus(s linearState) model.Status {
	// A custom state named "review" maps to in_review regardless of its type.
	if strings.Contains(strings.ToLower(s.Name), "review") {
		return model.StatusInReview
	}
	switch s.Type {
	case "backlog":
		return model.StatusBacklog
	case "triage", "unstarted":
		return model.StatusTodo
	case "started":
		return model.StatusInProgress
	case "completed":
		return model.StatusDone
	case "canceled":
		return model.StatusCancelled
	default:
		return model.StatusTodo
	}
}

// linearStateTypeFor maps a rally status to the Linear state type to target when
// pushing a status change.
func linearStateTypeFor(status model.Status) string {
	switch status {
	case model.StatusBacklog:
		return "backlog"
	case model.StatusTodo:
		return "unstarted"
	case model.StatusInProgress, model.StatusInReview:
		return "started"
	case model.StatusDone:
		return "completed"
	case model.StatusCancelled:
		return "canceled"
	default:
		return "unstarted"
	}
}

// normalizeLinearPriority maps Linear's 0–4 priority to a rally priority.
// Linear: 0 = none, 1 = urgent, 2 = high, 3 = medium, 4 = low.
func normalizeLinearPriority(p int) model.Priority {
	switch p {
	case 1:
		return model.PriorityUrgent
	case 2:
		return model.PriorityHigh
	case 3:
		return model.PriorityMedium
	case 4:
		return model.PriorityLow
	default:
		return model.PriorityNone
	}
}
