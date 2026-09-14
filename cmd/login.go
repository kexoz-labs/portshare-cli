package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"portshare/config"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Link a Google account to unlock 1 GB free bandwidth",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := NormalizedServerURL()
		cfg, err := config.EnsureIdentity(serverURL)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
		if cfg.OwnerEmail != "" {
			fmt.Printf("Already verified as %s (tier: %s).\n", cfg.OwnerEmail, cfg.Tier())
			return nil
		}

		// Fail fast when the server has no Google OAuth configured.
		statusResp, err := config.HTTPClient.Get(serverURL + "/auth/google/status")
		if err != nil {
			return fmt.Errorf("failed to reach server: %w", err)
		}
		var gauth struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(statusResp.Body).Decode(&gauth); err != nil {
			statusResp.Body.Close()
			return fmt.Errorf("failed to read server status: %w", err)
		}
		statusResp.Body.Close()
		if !gauth.Enabled {
			return fmt.Errorf("Google verification is not enabled on this server")
		}

		finish := serverURL + "/client/link-finish?clientId=" + url.QueryEscape(cfg.ClientID)
		loginURL := serverURL + "/auth/google/login?next=" + url.QueryEscape(
			base64.RawURLEncoding.EncodeToString([]byte(finish)))

		fmt.Println("Open this URL in your browser to verify with Google:")
		fmt.Println()
		fmt.Printf("  %s\n", loginURL)
		fmt.Println()
		fmt.Println("Waiting for verification (up to 5 minutes)...")

		deadline := time.Now().Add(5 * time.Minute)
		for time.Now().Before(deadline) {
			time.Sleep(3 * time.Second)
			statusURL := serverURL + "/client/link-status?clientId=" + url.QueryEscape(cfg.ClientID)
			resp, err := config.HTTPClient.Get(statusURL)
			if err != nil {
				continue // transient — keep polling
			}
			var status struct {
				Linked         bool   `json:"linked"`
				Email          string `json:"email"`
				BandwidthLimit int64  `json:"bandwidthLimit"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&status)
			resp.Body.Close()
			if decodeErr != nil || !status.Linked {
				continue
			}
			cfg.OwnerEmail = status.Email
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("verified, but failed to save config: %w", err)
			}
			fmt.Printf("Verified as %s! Bandwidth limit: %.0f MB.\n", status.Email, float64(status.BandwidthLimit)/1e6)
			return nil
		}
		return fmt.Errorf("verification timed out — run `portshare login` to try again")
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
