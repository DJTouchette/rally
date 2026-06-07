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

// Asana implements the Provider interface for Asana. Both OAuth tokens and
// personal access tokens authenticate with a Bearer header. All Asana request
// and response bodies are wrapped in a top-level "data" object.
type Asana struct{}

// Endpoints are vars so tests can point them at a mock server.
var (
	asanaAPIURL   = "https://app.asana.com/api/1.0"
	asanaTokenURL = "https://app.asana.com/-/oauth_token"
)

const asanaTaskFields = "name,notes,completed,completed_at,due_on,permalink_url,created_at,modified_at,assignee.name,projects.name,memberships.section.name,tags.name"

func (a *Asana) Name() string { return "asana" }

func (a *Asana) AuthURL(clientID, redirectURI, state string) string {
	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"state":         {state},
		"response_type": {"code"},
	}
	return "https://app.asana.com/-/oauth_authorize?" + params.Encode()
}

func (a *Asana) ExchangeCode(ctx context.Context, cfg OAuthConfig, code, redirectURI string) (*TokenSet, error) {
	return a.tokenRequest(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"redirect_uri":  {redirectURI},
		"code":          {code},
	})
}

func (a *Asana) RefreshToken(ctx context.Context, cfg OAuthConfig, refreshToken string) (*TokenSet, error) {
	// Asana OAuth access tokens expire after 1 hour; refresh tokens are durable.
	return a.tokenRequest(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"refresh_token": {refreshToken},
	})
}

func (a *Asana) tokenRequest(ctx context.Context, body url.Values) (*TokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", asanaTokenURL, strings.NewReader(body.Encode()))
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

	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	expiresAt := time.Now().Add(time.Hour)
	if tr.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return &TokenSet{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

type asanaTask struct {
	GID          string `json:"gid"`
	Name         string `json:"name"`
	Notes        string `json:"notes"`
	Completed    bool   `json:"completed"`
	DueOn        string `json:"due_on"`
	PermalinkURL string `json:"permalink_url"`
	CreatedAt    string `json:"created_at"`
	ModifiedAt   string `json:"modified_at"`
	Assignee     *struct {
		Name string `json:"name"`
	} `json:"assignee"`
	Projects []struct {
		Name string `json:"name"`
	} `json:"projects"`
	Memberships []struct {
		Section *struct {
			Name string `json:"name"`
		} `json:"section"`
	} `json:"memberships"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

func (a *Asana) FetchAssigned(ctx context.Context, creds Credentials, opts FetchOpts) ([]model.Ticket, error) {
	workspaces, err := a.workspaces(ctx, creds)
	if err != nil {
		return nil, err
	}

	max := opts.MaxResults
	var tickets []model.Ticket

	for _, ws := range workspaces {
		offset := ""
		for {
			q := url.Values{
				"assignee":        {"me"},
				"workspace":       {ws},
				"completed_since": {"now"}, // only incomplete tasks
				"opt_fields":      {asanaTaskFields},
				"limit":           {"100"},
			}
			if offset != "" {
				q.Set("offset", offset)
			}

			var resp struct {
				Data     []asanaTask `json:"data"`
				NextPage *struct {
					Offset string `json:"offset"`
				} `json:"next_page"`
			}
			if err := a.get(ctx, creds, "/tasks?"+q.Encode(), &resp); err != nil {
				return nil, err
			}

			for i := range resp.Data {
				tickets = append(tickets, a.normalizeTask(&resp.Data[i]))
				if max > 0 && len(tickets) >= max {
					return tickets, nil
				}
			}

			if resp.NextPage == nil || resp.NextPage.Offset == "" {
				break
			}
			offset = resp.NextPage.Offset
		}
	}
	return tickets, nil
}

// workspaces returns the gids of the authenticated user's workspaces.
func (a *Asana) workspaces(ctx context.Context, creds Credentials) ([]string, error) {
	var resp struct {
		Data struct {
			Workspaces []struct {
				GID string `json:"gid"`
			} `json:"workspaces"`
		} `json:"data"`
	}
	if err := a.get(ctx, creds, "/users/me?opt_fields=workspaces", &resp); err != nil {
		return nil, err
	}
	var gids []string
	for _, w := range resp.Data.Workspaces {
		gids = append(gids, w.GID)
	}
	if len(gids) == 0 {
		return nil, fmt.Errorf("no Asana workspaces found for this token")
	}
	return gids, nil
}

func (a *Asana) normalizeTask(task *asanaTask) model.Ticket {
	t := model.Ticket{
		ID:          task.GID,
		ProviderID:  task.GID,
		Provider:    "asana",
		URL:         task.PermalinkURL,
		Title:       task.Name,
		Description: task.Notes,
		Priority:    model.PriorityNone, // Asana has no native priority
	}
	if task.Assignee != nil {
		t.Assignee = task.Assignee.Name
	}
	if len(task.Projects) > 0 {
		t.Project = task.Projects[0].Name
	}
	for _, tag := range task.Tags {
		t.Labels = append(t.Labels, tag.Name)
	}

	// Status: prefer the board section name (teams use sections as columns);
	// fall back to the completed flag.
	section := ""
	for _, m := range task.Memberships {
		if m.Section != nil && m.Section.Name != "" {
			section = m.Section.Name
			break
		}
	}
	t.Status = asanaStatus(section, task.Completed)

	if task.DueOn != "" {
		if parsed, err := time.Parse("2006-01-02", task.DueOn); err == nil {
			t.DueDate = &parsed
		}
	}
	if parsed, err := time.Parse(time.RFC3339, task.CreatedAt); err == nil {
		t.CreatedAt = parsed
	}
	if parsed, err := time.Parse(time.RFC3339, task.ModifiedAt); err == nil {
		t.UpdatedAt = parsed
	}
	return t
}

func (a *Asana) UpdateStatus(ctx context.Context, creds Credentials, providerID string, status model.Status) error {
	// Asana tasks are complete or incomplete; section/column moves aren't done
	// via this field, so we map done/cancelled to completed and the rest to
	// incomplete.
	completed := status == model.StatusDone || status == model.StatusCancelled
	payload, _ := json.Marshal(map[string]any{"data": map[string]any{"completed": completed}})

	apiURL := asanaAPIURL + "/tasks/" + providerID
	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating update request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)
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

// get performs an authenticated GET against the Asana API and decodes the JSON
// envelope into out.
func (a *Asana) get(ctx context.Context, creds Credentials, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", asanaAPIURL+path, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("asana request failed (%d): %s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// asanaStatus maps an Asana board section name (and completed flag) to a rally
// status.
func asanaStatus(section string, completed bool) model.Status {
	if completed {
		return model.StatusDone
	}
	switch s := strings.ToLower(section); {
	case s == "":
		return model.StatusTodo
	case strings.Contains(s, "backlog"):
		return model.StatusBacklog
	case strings.Contains(s, "review"):
		return model.StatusInReview
	case strings.Contains(s, "progress"), strings.Contains(s, "doing"), strings.Contains(s, "in dev"):
		return model.StatusInProgress
	case strings.Contains(s, "done"), strings.Contains(s, "complete"), strings.Contains(s, "shipped"):
		return model.StatusDone
	case strings.Contains(s, "cancel"):
		return model.StatusCancelled
	default:
		return model.StatusTodo
	}
}
