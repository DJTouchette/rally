package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/djtouchette/rally/internal/oauth"
	"github.com/djtouchette/rally/internal/provider"
	"github.com/djtouchette/rally/internal/store"
	"github.com/spf13/cobra"
)

func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <provider>",
		Short: "Connect to a provider via OAuth and store tokens in vaulty",
		Args:  cobra.ExactArgs(1),
		RunE:  runConnect,
	}
	return cmd
}

func runConnect(cmd *cobra.Command, args []string) error {
	providerName := args[0]

	prov, err := provider.New(providerName)
	if err != nil {
		return err
	}

	// Read OAuth client credentials from environment (injected by vaulty exec)
	clientID := os.Getenv("RALLY_" + upperName(providerName) + "_CLIENT_ID")
	clientSecret := os.Getenv("RALLY_" + upperName(providerName) + "_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		fmt.Println("Missing OAuth client credentials.")
		fmt.Println("")
		fmt.Println("First, store them in vaulty:")
		fmt.Printf("  vaulty set RALLY_%s_CLIENT_ID --value <your-client-id> --domains auth.atlassian.com\n", upperName(providerName))
		fmt.Printf("  vaulty set RALLY_%s_CLIENT_SECRET --value <your-client-secret> --domains auth.atlassian.com\n", upperName(providerName))
		fmt.Println("")
		fmt.Println("Then run connect via vaulty exec:")
		fmt.Printf("  vaulty exec --secrets RALLY_%s_CLIENT_ID,RALLY_%s_CLIENT_SECRET -- rally connect %s\n",
			upperName(providerName), upperName(providerName), providerName)
		return fmt.Errorf("RALLY_%s_CLIENT_ID and RALLY_%s_CLIENT_SECRET not in environment",
			upperName(providerName), upperName(providerName))
	}

	oauthCfg := provider.OAuthConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	// Start callback server
	listener, port, err := oauth.ListenOnFreePort()
	if err != nil {
		return err
	}

	redirectURI := oauth.RedirectURI(port)
	state := oauth.RandomState()
	authURL := prov.AuthURL(clientID, redirectURI, state)

	fmt.Printf("Opening browser for %s authorization...\n", providerName)
	fmt.Printf("If it doesn't open, visit:\n\n  %s\n\n", authURL)

	// Handle callback
	resultCh := make(chan oauth.CallbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		result := oauth.CallbackResult{
			Code:  r.URL.Query().Get("code"),
			State: r.URL.Query().Get("state"),
			Error: r.URL.Query().Get("error"),
		}
		if result.Error != "" {
			fmt.Fprintf(w, "<html><body><h2>Authorization failed</h2><p>%s</p></body></html>", result.Error)
		} else {
			fmt.Fprint(w, "<html><body><h2>Authorization successful</h2><p>You can close this tab.</p></body></html>")
		}
		resultCh <- result
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)

	if err := oauth.OpenBrowser(authURL); err != nil {
		// Already printed the URL above
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	defer server.Shutdown(ctx)

	var result oauth.CallbackResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		return fmt.Errorf("authorization timed out after 5 minutes")
	}

	if result.Error != "" {
		return fmt.Errorf("authorization failed: %s", result.Error)
	}
	if result.State != state {
		return fmt.Errorf("state mismatch — possible CSRF attack")
	}

	fmt.Println("Exchanging authorization code for tokens...")

	tokens, err := prov.ExchangeCode(ctx, oauthCfg, result.Code, redirectURI)
	if err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}

	// Store tokens in vaulty (tokens never touch disk or stdout)
	secrets := store.DefaultSecretsForProvider(providerName)
	tokenValues := map[string]string{
		"RALLY_" + upperName(providerName) + "_TOKEN":   tokens.AccessToken,
		"RALLY_" + upperName(providerName) + "_REFRESH": tokens.RefreshToken,
	}

	if hasVaulty() {
		for name, val := range tokenValues {
			if val == "" {
				continue
			}
			// Find the matching secret declaration for domain info
			var domains string
			for _, s := range secrets {
				if s.Name == name {
					domains = strings.Join(s.Domains, ",")
					break
				}
			}
			if err := vaultySet(name, val, domains); err != nil {
				return fmt.Errorf("storing %s in vaulty: %w", name, err)
			}
			fmt.Printf("Stored %s in vaulty.\n", name)
		}
	} else {
		fmt.Println("\nWARNING: vaulty not found. Tokens were obtained but NOT stored.")
		fmt.Println("Install vaulty and run connect again to store tokens securely.")
		fmt.Println("Tokens will NOT be written to disk without vaulty.")
		return fmt.Errorf("vaulty required for token storage")
	}

	// Save connection config (no secrets — safe to commit)
	cfg, _, err := store.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	conn := store.Connection{
		Provider: providerName,
		CloudID:  tokens.CloudID,
	}
	cfg.AddConnection(conn)

	// Add secret declarations to config
	for _, s := range secrets {
		found := false
		for _, existing := range cfg.Secrets {
			if existing.Name == s.Name {
				found = true
				break
			}
		}
		if !found {
			cfg.Secrets = append(cfg.Secrets, s)
		}
	}

	if err := store.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nConnected to %s successfully.\n", providerName)
	fmt.Println("\nTo sync tickets, run:")
	fmt.Printf("  vaulty exec --secrets RALLY_%s_TOKEN -- rally sync\n", upperName(providerName))
	return nil
}

func upperName(s string) string {
	if len(s) == 0 {
		return s
	}
	result := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// hasVaulty checks if the vaulty binary is available.
func hasVaulty() bool {
	_, err := exec.LookPath("vaulty")
	return err == nil
}

// vaultySet stores a secret in vaulty by piping the value to stdin.
// The value never appears in process arguments or environment.
func vaultySet(name, value, domains string) error {
	args := []string{"set", name}
	if domains != "" {
		args = append(args, "--domains", domains)
	}

	cmd := exec.Command("vaulty", args...)
	cmd.Stdin = strings.NewReader(value)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
