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

// Jira implements the Provider interface for Atlassian Jira Cloud.
type Jira struct{}

func (j *Jira) Name() string { return "jira" }

func (j *Jira) AuthURL(clientID, redirectURI, state string) string {
	params := url.Values{
		"audience":      {"api.atlassian.com"},
		"client_id":     {clientID},
		"scope":         {"read:jira-work write:jira-work offline_access"},
		"redirect_uri":  {redirectURI},
		"state":         {state},
		"response_type": {"code"},
		"prompt":        {"consent"},
	}
	return "https://auth.atlassian.com/authorize?" + params.Encode()
}

func (j *Jira) ExchangeCode(ctx context.Context, cfg OAuthConfig, code, redirectURI string) (*TokenSet, error) {
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://auth.atlassian.com/oauth/token", strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	ts := &TokenSet{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}

	// Fetch the cloud ID for API requests
	cloudID, err := j.fetchCloudID(ctx, ts.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("fetching cloud ID: %w", err)
	}
	ts.CloudID = cloudID

	return ts, nil
}

func (j *Jira) RefreshToken(ctx context.Context, cfg OAuthConfig, refreshToken string) (*TokenSet, error) {
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"refresh_token": {refreshToken},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://auth.atlassian.com/oauth/token", strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, respBody)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding refresh response: %w", err)
	}

	return &TokenSet{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}, nil
}

func (j *Jira) FetchAssigned(ctx context.Context, token string, opts FetchOpts) ([]model.Ticket, error) {
	cloudID, err := j.fetchCloudID(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("fetching cloud ID: %w", err)
	}

	jql := "assignee = currentUser() AND statusCategory != Done ORDER BY priority ASC, created ASC"
	if opts.Project != "" {
		jql = fmt.Sprintf("project = %s AND %s", opts.Project, jql)
	}

	maxResults := 50
	if opts.MaxResults > 0 {
		maxResults = opts.MaxResults
	}

	apiURL := fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/search?jql=%s&maxResults=%d&fields=summary,description,status,priority,issuetype,project,labels,creator,created,updated,duedate,parent",
		cloudID, url.QueryEscape(jql), maxResults)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating search request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed (%d): %s", resp.StatusCode, respBody)
	}

	var searchResult jiraSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}

	tickets := make([]model.Ticket, 0, len(searchResult.Issues))
	for _, issue := range searchResult.Issues {
		tickets = append(tickets, j.normalizeIssue(issue))
	}

	return tickets, nil
}

func (j *Jira) UpdateStatus(ctx context.Context, token string, providerID string, status model.Status) error {
	// providerID format: "cloudID:issueID"
	parts := strings.SplitN(providerID, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid provider ID %q — expected cloudID:issueID", providerID)
	}
	cloudID, issueID := parts[0], parts[1]

	// Get available transitions for this issue
	transURL := fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/issue/%s/transitions", cloudID, issueID)
	req, err := http.NewRequestWithContext(ctx, "GET", transURL, nil)
	if err != nil {
		return fmt.Errorf("creating transitions request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("transitions request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("transitions request failed (%d): %s", resp.StatusCode, respBody)
	}

	var transResult struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&transResult); err != nil {
		return fmt.Errorf("decoding transitions: %w", err)
	}

	// Find the transition that matches our target status category
	targetCategory := statusToJiraCategory(status)
	var transitionID string
	for _, tr := range transResult.Transitions {
		if tr.To.StatusCategory.Key == targetCategory {
			transitionID = tr.ID
			break
		}
	}
	if transitionID == "" {
		return fmt.Errorf("no transition found for status %q (target category: %s)", status, targetCategory)
	}

	// Execute the transition
	transBody := fmt.Sprintf(`{"transition":{"id":"%s"}}`, transitionID)
	req, err = http.NewRequestWithContext(ctx, "POST", transURL, strings.NewReader(transBody))
	if err != nil {
		return fmt.Errorf("creating transition request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("transition request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("transition failed (%d): %s", resp.StatusCode, respBody)
	}

	return nil
}

// fetchCloudID retrieves the Atlassian cloud site ID for API calls.
func (j *Jira) fetchCloudID(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.atlassian.com/oauth/token/accessible-resources", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("accessible-resources failed (%d): %s", resp.StatusCode, respBody)
	}

	var resources []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resources); err != nil {
		return "", err
	}
	if len(resources) == 0 {
		return "", fmt.Errorf("no accessible Jira sites found — check your OAuth scopes")
	}

	return resources[0].ID, nil
}

func (j *Jira) normalizeIssue(issue jiraIssue) model.Ticket {
	t := model.Ticket{
		ID:         issue.Key,
		ProviderID: fmt.Sprintf("%s", issue.ID), // will be prefixed with cloudID at sync time
		Provider:   "jira",
		URL:        issue.Self,
		Title:      issue.Fields.Summary,
		Labels:     issue.Fields.Labels,
		Status:     normalizeJiraStatus(issue.Fields.Status.StatusCategory.Key),
		Priority:   normalizeJiraPriority(issue.Fields.Priority.Name),
		Assignee:   issue.Fields.Assignee.DisplayName,
	}

	if issue.Fields.IssueType.Name != "" {
		t.Type = strings.ToLower(issue.Fields.IssueType.Name)
	}
	if issue.Fields.Project.Key != "" {
		t.Project = issue.Fields.Project.Key
	}
	if issue.Fields.Creator.DisplayName != "" {
		t.Creator = issue.Fields.Creator.DisplayName
	}
	if issue.Fields.Parent != nil {
		t.Parent = issue.Fields.Parent.Key
	}

	if issue.Fields.Description != nil {
		t.Description = extractTextFromADF(issue.Fields.Description)
	}

	if issue.Fields.Created != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05.000-0700", issue.Fields.Created); err == nil {
			t.CreatedAt = parsed
		}
	}
	if issue.Fields.Updated != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05.000-0700", issue.Fields.Updated); err == nil {
			t.UpdatedAt = parsed
		}
	}
	if issue.Fields.DueDate != "" {
		if parsed, err := time.Parse("2006-01-02", issue.Fields.DueDate); err == nil {
			t.DueDate = &parsed
		}
	}

	return t
}

// extractTextFromADF extracts plain text from Jira's Atlassian Document Format.
// ADF is a nested JSON structure; we walk it and concatenate text nodes.
func extractTextFromADF(adf json.RawMessage) string {
	var doc struct {
		Content []adfNode `json:"content"`
	}
	if err := json.Unmarshal(adf, &doc); err != nil {
		return ""
	}

	var b strings.Builder
	for _, node := range doc.Content {
		extractADFText(&b, node)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

type adfNode struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Content json.RawMessage `json:"content"`
}

func extractADFText(b *strings.Builder, node adfNode) {
	if node.Text != "" {
		b.WriteString(node.Text)
	}
	if node.Content != nil {
		var children []adfNode
		if err := json.Unmarshal(node.Content, &children); err == nil {
			for _, child := range children {
				extractADFText(b, child)
			}
		}
	}
}

func normalizeJiraStatus(categoryKey string) model.Status {
	switch categoryKey {
	case "new":
		return model.StatusTodo
	case "indeterminate":
		return model.StatusInProgress
	case "done":
		return model.StatusDone
	default:
		return model.StatusTodo
	}
}

func normalizeJiraPriority(name string) model.Priority {
	switch strings.ToLower(name) {
	case "highest", "blocker":
		return model.PriorityUrgent
	case "high":
		return model.PriorityHigh
	case "medium":
		return model.PriorityMedium
	case "low":
		return model.PriorityLow
	case "lowest", "trivial":
		return model.PriorityNone
	default:
		return model.PriorityMedium
	}
}

func statusToJiraCategory(s model.Status) string {
	switch s {
	case model.StatusTodo, model.StatusBacklog:
		return "new"
	case model.StatusInProgress, model.StatusInReview:
		return "indeterminate"
	case model.StatusDone:
		return "done"
	default:
		return "new"
	}
}

// Jira API response types

type jiraSearchResult struct {
	Issues []jiraIssue `json:"issues"`
	Total  int         `json:"total"`
}

type jiraIssue struct {
	ID     string    `json:"id"`
	Key    string    `json:"key"`
	Self   string    `json:"self"`
	Fields jiraFields `json:"fields"`
}

type jiraFields struct {
	Summary   string          `json:"summary"`
	Description json.RawMessage `json:"description"` // ADF format
	Status    jiraStatus      `json:"status"`
	Priority  jiraPriority    `json:"priority"`
	IssueType jiraIssueType   `json:"issuetype"`
	Project   jiraProject     `json:"project"`
	Labels    []string        `json:"labels"`
	Assignee  jiraPerson      `json:"assignee"`
	Creator   jiraPerson      `json:"creator"`
	Parent    *jiraParent     `json:"parent"`
	Created   string          `json:"created"`
	Updated   string          `json:"updated"`
	DueDate   string          `json:"duedate"`
}

type jiraStatus struct {
	Name           string `json:"name"`
	StatusCategory struct {
		Key string `json:"key"`
	} `json:"statusCategory"`
}

type jiraPriority struct {
	Name string `json:"name"`
}

type jiraIssueType struct {
	Name string `json:"name"`
}

type jiraProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type jiraPerson struct {
	DisplayName string `json:"displayName"`
	AccountID   string `json:"accountId"`
}

type jiraParent struct {
	Key string `json:"key"`
}
