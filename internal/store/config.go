package store

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is rally's configuration file.
type Config struct {
	Connections []Connection `yaml:"connections"`
	Secrets     []Secret     `yaml:"secrets,omitempty"`
}

// Connection describes a connected provider.
type Connection struct {
	Provider string `yaml:"provider"`
	Project  string `yaml:"project,omitempty"`
	CloudID  string `yaml:"cloud_id,omitempty"` // Jira cloud site ID
}

// Secret declares a secret that rally needs, with vaulty policy hints.
type Secret struct {
	Name        string   `yaml:"name"`                  // env var name, e.g. RALLY_JIRA_TOKEN
	Description string   `yaml:"description"`           // human-readable purpose
	Domains     []string `yaml:"domains,omitempty"`      // vaulty allowed domains
	Commands    []string `yaml:"commands,omitempty"`     // vaulty allowed commands
	Required    bool     `yaml:"required"`               // must be present for the provider to work
}

// configSearchPaths returns the locations to search for config, in priority order.
func configSearchPaths() []string {
	paths := []string{
		".rally/config.yaml",
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "rally", "config.yaml"))
	}
	return paths
}

// LoadConfig loads the rally config from the first found location.
func LoadConfig() (*Config, string, error) {
	for _, path := range configSearchPaths() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, path, fmt.Errorf("parsing %s: %w", path, err)
		}
		return &cfg, path, nil
	}
	return &Config{}, "", nil
}

// SaveConfig writes the config to the local project path (.rally/config.yaml).
func SaveConfig(cfg *Config) error {
	dir := ".rally"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// FindConnection returns the connection for a given provider, if configured.
func (c *Config) FindConnection(providerName string) *Connection {
	for i := range c.Connections {
		if c.Connections[i].Provider == providerName {
			return &c.Connections[i]
		}
	}
	return nil
}

// AddConnection adds or updates a provider connection.
func (c *Config) AddConnection(conn Connection) {
	for i, existing := range c.Connections {
		if existing.Provider == conn.Provider {
			c.Connections[i] = conn
			return
		}
	}
	c.Connections = append(c.Connections, conn)
}

// SecretsForProvider returns the declared secrets for a given provider.
func (c *Config) SecretsForProvider(providerName string) []Secret {
	prefix := "RALLY_" + upperString(providerName) + "_"
	var result []Secret
	for _, s := range c.Secrets {
		if len(s.Name) >= len(prefix) && s.Name[:len(prefix)] == prefix {
			result = append(result, s)
		}
	}
	return result
}

// MissingSecrets returns secrets that are declared as required but not in the environment.
func (c *Config) MissingSecrets() []Secret {
	var missing []Secret
	for _, s := range c.Secrets {
		if s.Required && os.Getenv(s.Name) == "" {
			missing = append(missing, s)
		}
	}
	return missing
}

func upperString(s string) string {
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

// DefaultSecretsForProvider returns the standard secret declarations for a provider.
func DefaultSecretsForProvider(providerName string) []Secret {
	switch providerName {
	case "jira":
		return []Secret{
			{
				Name:        "RALLY_JIRA_TOKEN",
				Description: "Jira OAuth access token",
				Domains:     []string{"api.atlassian.com", "auth.atlassian.com"},
				Required:    true,
			},
			{
				Name:        "RALLY_JIRA_REFRESH",
				Description: "Jira OAuth refresh token",
				Domains:     []string{"auth.atlassian.com"},
				Required:    true,
			},
			{
				Name:        "RALLY_JIRA_CLIENT_ID",
				Description: "Jira OAuth application client ID",
				Domains:     []string{"auth.atlassian.com"},
				Required:    true,
			},
			{
				Name:        "RALLY_JIRA_CLIENT_SECRET",
				Description: "Jira OAuth application client secret",
				Domains:     []string{"auth.atlassian.com"},
				Required:    true,
			},
		}
	case "linear":
		return []Secret{
			{
				Name:        "RALLY_LINEAR_TOKEN",
				Description: "Linear OAuth access token",
				Domains:     []string{"api.linear.app", "linear.app"},
				Required:    true,
			},
			{
				Name:        "RALLY_LINEAR_CLIENT_ID",
				Description: "Linear OAuth application client ID",
				Domains:     []string{"linear.app"},
				Required:    true,
			},
			{
				Name:        "RALLY_LINEAR_CLIENT_SECRET",
				Description: "Linear OAuth application client secret",
				Domains:     []string{"linear.app"},
				Required:    true,
			},
		}
	default:
		return nil
	}
}
