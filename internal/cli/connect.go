package cli

import (
	"bufio"
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
	"golang.org/x/term"
)

// promptSecret reads a secret from the terminal without echoing it. When stdin
// is not a terminal (e.g. piped: `echo $KEY | rally connect ... --api-key`) it
// reads a single line instead.
func promptSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return line, err
}

func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <provider>",
		Short: "Connect to a provider via OAuth, or --api-key for a personal token",
		Args:  cobra.ExactArgs(1),
		RunE:  runConnect,
	}
	cmd.Flags().Bool("api-key", false, "Connect with a personal API key/token instead of OAuth (no app registration needed)")
	cmd.Flags().String("email", "", "Jira account email (required for Jira --api-key)")
	cmd.Flags().String("site", "", "Jira site host, e.g. co.atlassian.net (required for Jira --api-key)")
	return cmd
}

func runConnect(cmd *cobra.Command, args []string) error {
	providerName := args[0]

	prov, err := provider.New(providerName)
	if err != nil {
		return err
	}

	if apiKey, _ := cmd.Flags().GetBool("api-key"); apiKey {
		return runConnectAPIKey(cmd, providerName, prov)
	}

	// Read OAuth client credentials from environment (injected by vaulty exec)
	clientID := os.Getenv("RALLY_" + upperName(providerName) + "_CLIENT_ID")
	clientSecret := os.Getenv("RALLY_" + upperName(providerName) + "_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		// Use the provider's own OAuth domain in the help text.
		authDomain := "the provider's auth domain"
		for _, s := range store.DefaultSecretsForProvider(providerName) {
			if strings.HasSuffix(s.Name, "_CLIENT_ID") && len(s.Domains) > 0 {
				authDomain = s.Domains[0]
				break
			}
		}
		fmt.Println("Missing OAuth client credentials.")
		fmt.Println("")
		fmt.Println("First, store them in vaulty:")
		fmt.Printf("  vaulty set RALLY_%s_CLIENT_ID --value <your-client-id> --domains %s\n", upperName(providerName), authDomain)
		fmt.Printf("  vaulty set RALLY_%s_CLIENT_SECRET --value <your-client-secret> --domains %s\n", upperName(providerName), authDomain)
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

	// Start callback server on a FIXED port. Jira and Linear require the
	// redirect_uri (including the port) to match the OAuth app's registered
	// callback URL exactly, so the port can't be random.
	port := oauth.CallbackPort()
	redirectURI := oauth.RedirectURI(port)

	listener, err := oauth.ListenOnPort(port)
	if err != nil {
		return fmt.Errorf("%w\n\nThe OAuth callback needs the fixed port %d. Free it (or set RALLY_OAUTH_PORT to a port registered as the redirect URI in your %s app).", err, port, providerName)
	}

	state := oauth.RandomState()
	authURL := prov.AuthURL(clientID, redirectURI, state)

	fmt.Printf("Make sure this exact callback URL is registered in your %s OAuth app:\n  %s\n\n", providerName, redirectURI)
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

// runConnectAPIKey connects using a personal API key/token (no OAuth app). The
// key is expected to already be in vaulty as RALLY_<PROVIDER>_TOKEN; this
// verifies it works and records the connection.
func runConnectAPIKey(cmd *cobra.Command, providerName string, prov provider.Provider) error {
	email, _ := cmd.Flags().GetString("email")
	site, _ := cmd.Flags().GetString("site")

	if providerName == "jira" && (email == "" || site == "") {
		return fmt.Errorf("jira --api-key needs --email and --site (e.g. --email you@co.com --site co.atlassian.net)")
	}

	tokenDomain := "api.linear.app"
	if providerName == "jira" {
		tokenDomain = site
	}

	secretName := "RALLY_" + upperName(providerName) + "_TOKEN"

	// The key may already be in the environment (when run via `vaulty exec`);
	// otherwise prompt for it and we'll store it in vaulty ourselves.
	token := os.Getenv(secretName)
	fromEnv := token != ""
	if token == "" {
		if !hasVaulty() {
			fmt.Println("vaulty not found — it's required to store the API key securely.")
			fmt.Printf("Install vaulty, or store the key yourself and run via exec:\n")
			fmt.Printf("  vaulty set %s --value <key> --domains %s\n", secretName, tokenDomain)
			fmt.Printf("  vaulty exec --secrets %s -- rally connect %s --api-key\n", secretName, providerName)
			return fmt.Errorf("vaulty required for API-key storage")
		}
		entered, err := promptSecret(fmt.Sprintf("Paste your %s API key: ", providerName))
		if err != nil {
			return fmt.Errorf("reading API key: %w", err)
		}
		token = strings.TrimSpace(entered)
		if token == "" {
			return fmt.Errorf("no API key entered")
		}
	}

	creds := provider.Credentials{Method: provider.AuthAPIKey, Token: token, Email: email, Site: site}

	// Verify the key works by fetching a single ticket before storing anything.
	fmt.Printf("Verifying %s API key...\n", providerName)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := prov.FetchAssigned(ctx, creds, provider.FetchOpts{MaxResults: 1}); err != nil {
		return fmt.Errorf("API key verification failed: %w", err)
	}

	// Store the verified key in vaulty (only when we prompted for it — if it
	// came from the environment it is already managed by vaulty).
	if !fromEnv {
		if err := vaultySet(secretName, token, tokenDomain); err != nil {
			return fmt.Errorf("storing %s in vaulty: %w", secretName, err)
		}
		fmt.Printf("Stored %s in vaulty.\n", secretName)
	}

	// Record the connection (no secrets — safe to commit).
	cfg, _, err := store.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg.AddConnection(store.Connection{
		Provider: providerName,
		Auth:     provider.AuthAPIKey,
		Email:    email,
		Site:     site,
	})
	secret := store.Secret{
		Name:        "RALLY_" + upperName(providerName) + "_TOKEN",
		Description: providerName + " API token",
		Domains:     []string{tokenDomain},
		Required:    true,
	}
	exists := false
	for _, s := range cfg.Secrets {
		if s.Name == secret.Name {
			exists = true
			break
		}
	}
	if !exists {
		cfg.Secrets = append(cfg.Secrets, secret)
	}
	if err := store.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nConnected to %s via API key.\n", providerName)
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
