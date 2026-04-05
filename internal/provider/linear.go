package provider

import (
	"context"
	"fmt"

	"github.com/djtouchette/rally/internal/model"
)

// Linear implements the Provider interface for Linear.
// This is a stub — Jira is the priority implementation.
type Linear struct{}

func (l *Linear) Name() string { return "linear" }

func (l *Linear) AuthURL(clientID, redirectURI, state string) string {
	return fmt.Sprintf("https://linear.app/oauth/authorize?client_id=%s&redirect_uri=%s&state=%s&response_type=code&scope=read,write", clientID, redirectURI, state)
}

func (l *Linear) ExchangeCode(_ context.Context, _ OAuthConfig, _, _ string) (*TokenSet, error) {
	return nil, fmt.Errorf("linear provider not yet implemented")
}

func (l *Linear) RefreshToken(_ context.Context, _ OAuthConfig, _ string) (*TokenSet, error) {
	return nil, fmt.Errorf("linear provider not yet implemented")
}

func (l *Linear) FetchAssigned(_ context.Context, _ string, _ FetchOpts) ([]model.Ticket, error) {
	return nil, fmt.Errorf("linear provider not yet implemented")
}

func (l *Linear) UpdateStatus(_ context.Context, _ string, _ string, _ model.Status) error {
	return fmt.Errorf("linear provider not yet implemented")
}
