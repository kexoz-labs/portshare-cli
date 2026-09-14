package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"portshare/config"
	"portshare/tunnel"

	"github.com/spf13/cobra"
)

var e2eSubdomain string

var e2eCmd = &cobra.Command{
	Use:   "e2e [port]",
	Short: "Start an End-to-End Encrypted (Private) tunnel",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, err := strconv.Atoi(args[0])
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("invalid port number: %s", args[0])
		}
		serverURL := NormalizedServerURL()

		fmt.Printf("Authenticating with %s...\n", serverURL)
		cfg, err := config.EnsureIdentity(serverURL)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		if strings.TrimSpace(e2eSubdomain) != "" {
			if err := claimSubdomain(serverURL, cfg.ClientID, strings.ToLower(strings.TrimSpace(e2eSubdomain))); err != nil {
				return err
			}
			cfg.Subdomain = strings.ToLower(strings.TrimSpace(e2eSubdomain))
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
		}

		payload := map[string]interface{}{
			"clientId":   cfg.ClientID,
			"port":       port,
			"tunnelType": "e2e",
		}
		if cfg.Subdomain != "" {
			payload["subdomain"] = cfg.Subdomain
		}
		
		payloadBytes, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, serverURL+"/client/tunnels", bytes.NewReader(payloadBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, err := config.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to create e2e tunnel on server: %w", err)
		}
		defer resp.Body.Close()
		
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to create e2e tunnel, status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		fmt.Printf("Client ID: %s\n", cfg.ClientID)
		if cfg.Subdomain != "" {
			fmt.Printf("Private E2E URL: https://%s.%s:8443\n", cfg.Subdomain, RootDomain)
			fmt.Printf("(Ignore browser warnings about self-signed certs)\n")
		} else {
			return fmt.Errorf("E2E tunnels require a claimed subdomain (e.g. portshare domain claim <name>)")
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		t := &tunnel.Tunnel{
			ServerURL: serverURL,
			ClientID:  cfg.ClientID,
			LocalPort: port,
			IsTCP:     false,
			IsE2E:     true,
		}
		if err := t.StartMultiplexedWS(ctx); err != nil {
			return err
		}

		fmt.Println("Tunnel stopped.")
		return nil
	},
}

func init() {
	e2eCmd.Flags().StringVar(&e2eSubdomain, "subdomain", "", "Claim this subdomain before starting the tunnel")
	rootCmd.AddCommand(e2eCmd)
}
