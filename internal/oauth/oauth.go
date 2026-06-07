package oauth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

// CallbackResult holds the result of an OAuth callback.
type CallbackResult struct {
	Code  string
	State string
	Error string
}

// RedirectURI returns the localhost callback URI for a given port.
func RedirectURI(port int) string {
	return fmt.Sprintf("http://localhost:%d/callback", port)
}

// DefaultCallbackPort is the fixed localhost port rally listens on for the OAuth
// callback. Providers like Jira and Linear require the redirect_uri (including
// the port) to match the callback URL registered in the OAuth app exactly, so
// the port must be stable across runs. Override with RALLY_OAUTH_PORT.
const DefaultCallbackPort = 8412

// CallbackPort returns the configured callback port — RALLY_OAUTH_PORT if set to
// a valid port, otherwise DefaultCallbackPort.
func CallbackPort() int {
	if v := os.Getenv("RALLY_OAUTH_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
			return p
		}
	}
	return DefaultCallbackPort
}

// ListenOnPort binds a localhost TCP listener on the given fixed port.
func ListenOnPort(port int) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("binding 127.0.0.1:%d: %w", port, err)
	}
	return listener, nil
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
