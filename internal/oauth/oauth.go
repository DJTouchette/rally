package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
)

// CallbackResult holds the result of an OAuth callback.
type CallbackResult struct {
	Code  string
	State string
	Error string
}

// RunCallbackServer starts a localhost HTTP server, opens the browser to authURL,
// and waits for the OAuth callback. Returns the authorization code.
func RunCallbackServer(ctx context.Context, authURL string) (*CallbackResult, error) {
	resultCh := make(chan CallbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		result := CallbackResult{
			Code:  r.URL.Query().Get("code"),
			State: r.URL.Query().Get("state"),
			Error: r.URL.Query().Get("error"),
		}

		if result.Error != "" {
			fmt.Fprintf(w, "<html><body><h2>Authorization failed</h2><p>%s</p><p>You can close this tab.</p></body></html>", result.Error)
		} else {
			fmt.Fprint(w, "<html><body><h2>Authorization successful</h2><p>You can close this tab and return to the terminal.</p></body></html>")
		}
		resultCh <- result
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback server: %w", err)
	}

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Shutdown(ctx)

	port := listener.Addr().(*net.TCPAddr).Port
	_ = port // port is embedded in the redirectURI passed to authURL

	if err := OpenBrowser(authURL); err != nil {
		fmt.Printf("Could not open browser automatically.\nPlease visit:\n\n  %s\n\n", authURL)
	}

	select {
	case result := <-resultCh:
		if result.Error != "" {
			return nil, fmt.Errorf("authorization failed: %s", result.Error)
		}
		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// RedirectURI returns the localhost callback URI for a given port.
func RedirectURI(port int) string {
	return fmt.Sprintf("http://localhost:%d/callback", port)
}

// ListenOnFreePort starts a TCP listener on a free port and returns the listener and port.
func ListenOnFreePort() (net.Listener, int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, fmt.Errorf("listening on free port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	return listener, port, nil
}

// RandomState generates a random state parameter for OAuth.
func RandomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// OpenBrowser opens the given URL in the default browser.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
	return cmd.Start()
}
