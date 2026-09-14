package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ClientID     string `json:"clientId" yaml:"clientId"`
	Subdomain    string `json:"subdomain,omitempty" yaml:"subdomain,omitempty"`
	CustomDomain string `json:"customDomain,omitempty" yaml:"customDomain,omitempty"`
	OwnerEmail   string `json:"ownerEmail,omitempty" yaml:"ownerEmail,omitempty"`
	Plan         string `json:"plan,omitempty" yaml:"plan,omitempty"`
}

type authTransport struct{ base http.RoundTripper }

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cfg, _ := LoadConfig()
	if cfg != nil && cfg.ClientID != "" {
		clone := req.Clone(req.Context())
		clone.Header.Set("X-PortShare-Client-ID", cfg.ClientID)
		req = clone
	}
	return t.base.RoundTrip(req)
}

// serverIdentity mirrors the server's client record, which uses "id"
// (not "clientId"). It also accepts "clientId" for forward compatibility.
type serverIdentity struct {
	ID           string `json:"id"`
	ClientID     string `json:"clientId"`
	Subdomain    string `json:"subdomain"`
	CustomDomain string `json:"customDomain"`
	OwnerEmail   string `json:"ownerEmail"`
	Plan         string `json:"plan"`
}

func (s serverIdentity) resolvedID() string {
	if s.ID != "" {
		return s.ID
	}
	return s.ClientID
}

var configPath string
var legacyConfigPath string

// HTTPClient is shared by all API calls so nothing hangs forever.
var HTTPClient = &http.Client{Timeout: 15 * time.Second, Transport: authTransport{base: http.DefaultTransport}}

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	configPath = filepath.Join(home, ".config", "portshare", "config.yaml")
	legacyConfigPath = filepath.Join(home, ".portshare", "config.json")
}

// ConfigPath returns the resolved config file location (for status/logout).
func ConfigPath() string { return configPath }

func LoadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			data, err = os.ReadFile(legacyConfigPath)
			if os.IsNotExist(err) {
				return &Config{}, nil
			}
		}
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0o644)
}

func ClearConfig() error {
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func EnsureIdentity(serverURL string) (*Config, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	payload := map[string]string{"id": cfg.ClientID}
	payloadBytes, _ := json.Marshal(payload)

	resp, err := HTTPClient.Post(serverURL+"/client/identity", "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to contact server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result serverIdentity
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	if result.resolvedID() == "" {
		return nil, fmt.Errorf("server did not return a valid client id")
	}

	if result.resolvedID() != cfg.ClientID {
		// New or replaced identity: take the whole server record.
		cfg.ClientID = result.resolvedID()
		cfg.Subdomain = result.Subdomain
		cfg.CustomDomain = result.CustomDomain
		cfg.OwnerEmail = result.OwnerEmail
		cfg.Plan = result.Plan
		if err := SaveConfig(cfg); err != nil {
			return nil, fmt.Errorf("failed to save config: %w", err)
		}
		return cfg, nil
	}

	// Same identity: adopt server-side upgrades, but never wipe local state
	// with empty values (e.g. email linked via `login` then re-synced).
	changed := false
	if result.OwnerEmail != "" && result.OwnerEmail != cfg.OwnerEmail {
		cfg.OwnerEmail = result.OwnerEmail
		changed = true
	}
	if result.Plan != "" && result.Plan != cfg.Plan {
		cfg.Plan = result.Plan
		changed = true
	}
	if result.Subdomain != "" && result.Subdomain != cfg.Subdomain {
		cfg.Subdomain = result.Subdomain
		changed = true
	}
	if result.CustomDomain != "" && result.CustomDomain != cfg.CustomDomain {
		cfg.CustomDomain = result.CustomDomain
		changed = true
	}
	if changed {
		if err := SaveConfig(cfg); err != nil {
			return nil, fmt.Errorf("failed to save config: %w", err)
		}
	}

	return cfg, nil
}

// Tier reports the bandwidth tier: pro, verified, or anonymous.
func (c *Config) Tier() string {
	if c == nil {
		return "anonymous"
	}
	if c.Plan == "pro" {
		return "pro"
	}
	if c.OwnerEmail != "" {
		return "verified"
	}
	return "anonymous"
}
