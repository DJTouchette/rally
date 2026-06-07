package provider

import (
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

// GitHub implements the Provider interface for GitHub Issues. Both OAuth tokens
// and personal access tokens authenticate the same way (Bearer), so the request
// path is identical for either credential method.
type GitHub struct{}

// Endpoints are vars so tests can point them at a mock server.
var (
	githubAPIURL   = "https://api.github.com"
	githubTokenURL = "https://github.com/login/oauth/access_token"
)

func (g *GitHub) Name() string { return "github" }

func (g *GitHub) AuthURL(clientID, redirectURI, state string) string {
	params := url.Values{
		"client_id":    {clientID},
		"redirect_uri": {redirectURI},
		"state":        {state},
		"scope":        {"repo"},
	}
	return "https://github.com/login/oauth/authorize?" + params.Encode()
}

func (g *GitHub) ExchangeCode(ctx context.Context, cfg OAuthConfig, code, redirectURI string) (*TokenSet, error) {
	body := url.Values{
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", githubTokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, respBody)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
		Error       string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("github token exchange returned no token: %s", tokenResp.Error)
	}

	// Classic OAuth tokens don't expire and there's no refresh token.
	return &TokenSet{
		AccessToken: tokenResp.AccessToken,
		ExpiresAt:   time.Now().Add(10 * 365 * 24 * time.Hour),
		Scope:       tokenResp.Scope,
	}, nil
}

func (g *GitHub) RefreshToken(_ context.Context, _ OAuthConfig, _ string) (*TokenSet, error) {
	return nil, fmt.Errorf("github OAuth tokens do not expire; run `rally connect github` again if access was revoked")
}

type githubIssue struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	State       string `json:"state"`
	StateReason string `json:"state_reason"`
	HTMLURL     string `json:"html_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Assignee struct {
		Login string `json:"login"`
	} `json:"assignee"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest *struct{} `json:"pull_request"`
}

func (g *GitHub) FetchAssigned(ctx context.Context, creds Credentials, opts FetchOpts) ([]model.Ticket, error) {
	max := opts.MaxResults
	var tickets []model.Ticket
	page := 1
	const perPage = 100

	for {
		apiURL := fmt.Sprintf("%s/user/issues?filter=assigned&state=open&per_page=%d&page=%d", githubAPIURL, perPage, page)
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating issues request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+creds.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("issues request: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("issues request failed (%d): %s", resp.StatusCode, body)
		}

		var issues []githubIssue
		if err := json.Unmarshal(body, &issues); err != nil {
			return nil, fmt.Errorf("decoding issues: %w", err)
		}
		if len(issues) == 0 {
			break
		}

		for i := range issues {
			issue := &issues[i]
			if issue.PullRequest != nil {
				continue // /user/issues includes PRs; skip them
			}
			if opts.Project != "" && !strings.EqualFold(issue.Repository.FullName, opts.Project) {
				continue
			}
			tickets = append(tickets, g.normalizeIssue(issue))
			if max > 0 && len(tickets) >= max {
				return tickets, nil
			}
		}

		if len(issues) < perPage {
			break
		}
		page++
	}
	return tickets, nil
}

func (g *GitHub) normalizeIssue(issue *githubIssue) model.Ticket {
	t := model.Ticket{
		ID:          fmt.Sprintf("%s#%d", issue.Repository.FullName, issue.Number),
		ProviderID:  fmt.Sprintf("%s#%d", issue.Repository.FullName, issue.Number),
		Provider:    "github",
		URL:         issue.HTMLURL,
		Title:       issue.Title,
		Description: issue.Body,
		Status:      githubStatus(issue.State, issue.StateReason),
		Project:     issue.Repository.FullName,
		Assignee:    issue.Assignee.Login,
		Creator:     issue.User.Login,
	}
	for _, l := range issue.Labels {
		t.Labels = append(t.Labels, l.Name)
	}
	t.Priority = githubPriority(t.Labels)
	if parsed, err := time.Parse(time.RFC3339, issue.CreatedAt); err == nil {
		t.CreatedAt = parsed
	}
	if parsed, err := time.Parse(time.RFC3339, issue.UpdatedAt); err == nil {
		t.UpdatedAt = parsed
	}
	return t
}

func (g *GitHub) UpdateStatus(ctx context.Context, creds Credentials, providerID string, status model.Status) error {
	// providerID format: "owner/repo#number"
	hash := strings.LastIndex(providerID, "#")
	if hash < 0 {
		return fmt.Errorf("invalid provider ID %q — expected owner/repo#number", providerID)
	}
	repo, number := providerID[:hash], providerID[hash+1:]
	state, reason := githubStateChange(status)

	bodyMap := map[string]any{"state": state}
	if reason != "" {
		bodyMap["state_reason"] = reason
	}
	payload, _ := json.Marshal(bodyMap)

	apiURL := fmt.Sprintf("%s/repos/%s/issues/%s", githubAPIURL, repo, number)
	req, err := http.NewRequestWithContext(ctx, "PATCH", apiURL, strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("creating update request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("update request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed (%d): %s", resp.StatusCode, respBody)
	}
	return nil
}

// githubStatus maps GitHub's open/closed (+ close reason) to a rally status.
func githubStatus(state, stateReason string) model.Status {
	if state == "closed" {
		if stateReason == "not_planned" {
			return model.StatusCancelled
		}
		return model.StatusDone
	}
	return model.StatusTodo
}

// githubStateChange maps a rally status to a GitHub issue state change.
func githubStateChange(status model.Status) (state, reason string) {
	switch status {
	case model.StatusDone:
		return "closed", "completed"
	case model.StatusCancelled:
		return "closed", "not_planned"
	default:
		return "open", "reopened"
	}
}

// githubPriority derives a priority from issue labels, since GitHub issues have
// no native priority. Recognizes P0–P3 and urgent/high/medium/low conventions.
func githubPriority(labels []string) model.Priority {
	for _, l := range labels {
		ll := strings.ToLower(l)
		switch {
		case strings.Contains(ll, "p0"), strings.Contains(ll, "urgent"), strings.Contains(ll, "critical"):
			return model.PriorityUrgent
		case strings.Contains(ll, "p1"), strings.Contains(ll, "high"):
			return model.PriorityHigh
		case strings.Contains(ll, "p2"), strings.Contains(ll, "medium"):
			return model.PriorityMedium
		case strings.Contains(ll, "p3"), strings.Contains(ll, "low"):
			return model.PriorityLow
		}
	}
	return model.PriorityNone
}
