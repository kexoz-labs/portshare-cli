package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"

	"portshare/config"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show client identity, plan and tunnel stats",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := NormalizedServerURL()
		cfg, err := config.EnsureIdentity(serverURL)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		statsURL := serverURL + "/client/stats?clientId=" + url.QueryEscape(cfg.ClientID)
		resp, err := config.HTTPClient.Get(statsURL)
		if err != nil {
			return fmt.Errorf("failed to fetch stats: %w", err)
		}
		defer resp.Body.Close()
		var stats struct {
			TotalRequests  int64            `json:"totalRequests"`
			StatusCounts   map[string]int64 `json:"statusCounts"`
			BytesIn        int64            `json:"bytesIn"`
			BytesOut       int64            `json:"bytesOut"`
			BandwidthUsed  int64            `json:"bandwidthUsed"`
			BandwidthLimit int64            `json:"bandwidthLimit"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
			return fmt.Errorf("failed to decode stats: %w", err)
		}
		if JSONOutput {
			fmt.Printf("%s\n", mustJSON(map[string]interface{}{
				"clientId": cfg.ClientID, "tier": cfg.Tier(), "ownerEmail": cfg.OwnerEmail,
				"subdomain": cfg.Subdomain, "customDomain": cfg.CustomDomain, "stats": stats,
			}))
			return nil
		}
		fmt.Printf("Client ID:    %s\n", cfg.ClientID)
		fmt.Printf("Tier:         %s\n", cfg.Tier())
		if cfg.OwnerEmail != "" {
			fmt.Printf("Google:       %s\n", cfg.OwnerEmail)
		} else {
			fmt.Printf("Google:       (not linked — run `portshare login` for 1 GB free)\n")
		}
		if cfg.Subdomain != "" {
			fmt.Printf("Public URL:   https://%s.%s\n", cfg.Subdomain, RootDomain)
		} else {
			fmt.Printf("Public URL:   (no subdomain claimed yet)\n")
		}
		if cfg.CustomDomain != "" {
			fmt.Printf("Custom domain: https://%s\n", cfg.CustomDomain)
		}
		fmt.Printf("Config file:  %s\n", config.ConfigPath())
		fmt.Printf("Requests:     %d\n", stats.TotalRequests)
		fmt.Printf("Bandwidth:    %.1f MB / %.1f MB\n",
			float64(stats.BandwidthUsed)/1e6, float64(stats.BandwidthLimit)/1e6)
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out of Google without deleting the local client identity",
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := config.HTTPClient.Get(NormalizedServerURL() + "/auth/google/logout")
		if err == nil {
			_ = resp.Body.Close()
		}
		fmt.Println("Logged out of the Google session. Local client identity retained for recovery.")
		return nil
	},
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("portshare %s\n", Version)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(versionCmd)
}
