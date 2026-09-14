package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"portshare/config"

	"github.com/spf13/cobra"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage custom domain and subdomain",
}

var claimSubdomainCmd = &cobra.Command{
	Use:   "claim [subdomain]",
	Short: "Claim a subdomain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		subdomain := strings.ToLower(strings.TrimSpace(args[0]))
		serverURL := NormalizedServerURL()
		cfg, err := config.EnsureIdentity(serverURL)
		if err != nil {
			return err
		}

		if err := claimSubdomain(serverURL, cfg.ClientID, subdomain); err != nil {
			return err
		}

		cfg.Subdomain = subdomain
		if err := config.SaveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Successfully claimed subdomain: %s\n", subdomain)
		fmt.Printf("Public URL: https://%s.%s\n", subdomain, RootDomain)
		return nil
	},
}

// reservedSubdomains mirrors the server blocklist (client.go) so users get an
// instant, friendly error without a network round trip.
var reservedSubdomains = map[string]struct{}{
	"api": {}, "admin": {}, "administrator": {}, "www": {}, "app": {},
	"dashboard": {}, "console": {}, "panel": {}, "docs": {}, "status": {},
	"blog": {}, "mail": {}, "smtp": {}, "pop": {}, "imap": {}, "ftp": {},
	"sftp": {}, "ssh": {}, "ns1": {}, "ns2": {}, "cdn": {}, "static": {},
	"assets": {}, "auth": {}, "login": {}, "signin": {}, "signup": {},
	"sso": {}, "oauth": {}, "billing": {}, "pay": {}, "payments": {},
	"checkout": {}, "support": {}, "help": {}, "abuse": {}, "security": {},
	"privacy": {}, "terms": {}, "webhook": {}, "webhooks": {}, "metrics": {},
	"monitor": {}, "grafana": {}, "prometheus": {}, "db": {}, "database": {},
	"redis": {}, "postgres": {}, "mysql": {}, "mongo": {}, "vpn": {},
	"proxy": {}, "gateway": {}, "localhost": {}, "portshare": {}, "kexoz": {},
}

func isReservedSubdomain(name string) bool {
	_, reserved := reservedSubdomains[name]
	return reserved
}

func claimSubdomain(serverURL, clientID, subdomain string) error {
	if isReservedSubdomain(subdomain) {
		return fmt.Errorf("%q is reserved for PortShare infrastructure; choose another subdomain", subdomain)
	}
	payload := map[string]string{
		"clientId":  clientID,
		"subdomain": subdomain,
	}
	payloadBytes, _ := json.Marshal(payload)

	resp, err := config.HTTPClient.Post(serverURL+"/subdomain/claim", "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("failed to claim subdomain: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

var setCustomDomainCmd = &cobra.Command{
	Use:   "custom [domain]",
	Short: "Set a custom domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := strings.ToLower(strings.TrimSpace(args[0]))
		serverURL := NormalizedServerURL()
		cfg, err := config.EnsureIdentity(serverURL)
		if err != nil {
			return err
		}

		payload := map[string]string{
			"clientId": cfg.ClientID,
			"domain":   domain,
		}
		payloadBytes, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPut, serverURL+"/client/domain", bytes.NewReader(payloadBytes))
		req.Header.Set("Content-Type", "application/json")

		resp, err := config.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to set custom domain: %s", strings.TrimSpace(string(body)))
		}

		cfg.CustomDomain = domain
		if err := config.SaveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Successfully set custom domain: %s\n", domain)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(domainCmd)
	domainCmd.AddCommand(claimSubdomainCmd)
	domainCmd.AddCommand(setCustomDomainCmd)
}
