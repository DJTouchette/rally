package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/djtouchette/rally/internal/model"
)

// Provider abstracts a project management service (Jira, Linear, etc.).
type Provider interface {
	// Name returns the provider identifier ("jira", "linear").
	Name() string

	// AuthURL returns the OAuth authorization URL.
	AuthURL(clientID, redirectURI, state string) string

	// ExchangeCode exchanges an OAuth authorization code for tokens.
	ExchangeCode(ctx context.Context, cfg OAuthConfig, code, redirectURI string) (*TokenSet, error)

	// RefreshToken refreshes an expired access token.
	RefreshToken(ctx context.Context, cfg OAuthConfig, refreshToken string) (*TokenSet, error)

	// FetchAssigned returns tickets assigned to the authenticated user.
	FetchAssigned(ctx context.Context, creds Credentials, opts FetchOpts) ([]model.Ticket, error)

	// UpdateStatus pushes a status change back to the provider.
	UpdateStatus(ctx context.Context, creds Credentials, providerID string, status model.Status) error
}

// AuthMethod values identify how a connection authenticates.
const (
	AuthOAuth  = "oauth"   // OAuth 2.0 access token (default)
	AuthAPIKey = "api_key" // personal API key / token
)

// Credentials carry how to authenticate a single provider request. OAuth is the
// default; API-key mode uses a personal token (Linear) or an API token with
// Basic auth (Jira, which then also needs Email + Site).
type Credentials struct {
	Method string // AuthOAuth (default) or AuthAPIKey
	Token  string // OAuth access token, or the API key/token
	Email  string // Jira API-token Basic-auth email (api_key mode)
	Site   string // Jira site host, e.g. acme.atlassian.net (api_key mode)
}

// IsAPIKey reports whether these credentials use a static API key/token.
func (c Credentials) IsAPIKey() bool { return c.Method == AuthAPIKey }

// OAuthConfig holds the client credentials for a provider.
type OAuthConfig struct {
	ClientID     string `yaml:"client_id" json:"client_id"`
	ClientSecret string `yaml:"client_secret" json:"client_secret"`
}

// TokenSet holds OAuth tokens.
type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	CloudID      string    `json:"cloud_id,omitempty"` // Jira-specific: the Atlassian cloud site ID
	Scope        string    `json:"scope,omitempty"`
}

// FetchOpts holds optional parameters for fetching tickets.
type FetchOpts struct {
	Project    string // filter to a specific project key
	MaxResults int    // limit results (0 = provider default)
}

// New creates a Provider by name.
func New(name string) (Provider, error) {
	switch name {
	case "jira":
		return &Jira{}, nil
	case "linear":
		return &Linear{}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q — supported: jira, linear", name)
	}
}
