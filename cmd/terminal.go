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
	"strings"
	"syscall"

	"portshare/config"
	"portshare/tunnel"

	"github.com/spf13/cobra"
)

var terminalSubdomain string

var terminalCmd = &cobra.Command{
	Use:   "terminal",
	Short: "Start a Browser Terminal (Public PTY)",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := NormalizedServerURL()

		fmt.Printf("Authenticating with %s...\n", serverURL)
		cfg, err := config.EnsureIdentity(serverURL)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		if strings.TrimSpace(terminalSubdomain) != "" {
			if err := claimSubdomain(serverURL, cfg.ClientID, strings.ToLower(strings.TrimSpace(terminalSubdomain))); err != nil {
				return err
			}
			cfg.Subdomain = strings.ToLower(strings.TrimSpace(terminalSubdomain))
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
		}

		payload := map[string]interface{}{
			"clientId":   cfg.ClientID,
			"tunnelType": "pty",
		}
		if cfg.Subdomain != "" {
			payload["subdomain"] = cfg.Subdomain
		}
		
		payloadBytes, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, serverURL+"/client/tunnels", bytes.NewReader(payloadBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, err := config.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to create terminal tunnel on server: %w", err)
		}
		defer resp.Body.Close()
		
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to create terminal tunnel, status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		fmt.Printf("Client ID: %s\n", cfg.ClientID)
		if cfg.Subdomain != "" {
			fmt.Printf("Terminal URL: https://%s.%s\n", cfg.Subdomain, RootDomain)
			fmt.Printf("(Authentication is strictly required)\n")
		} else {
			return fmt.Errorf("Terminal tunnels require a claimed subdomain (e.g. --subdomain <name>)")
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		t := &tunnel.Tunnel{
			ServerURL: serverURL,
			ClientID:  cfg.ClientID,
			IsTCP:     false,
			IsPTY:     true,
		}
		
		tcpAddr, _ := cmd.Flags().GetString("tcp")
		if tcpAddr != "" {
			if err := t.StartTCP(ctx, tcpAddr); err != nil {
				return err
			}
		} else {
			host := strings.TrimPrefix(strings.TrimPrefix(serverURL, "https://"), "http://")
			if strings.Contains(host, ":") {
				host = strings.Split(host, ":")[0]
			}
			tcpAddr = host + ":8081"
			if err := t.StartTCP(ctx, tcpAddr); err != nil {
				return err
			}
		}

		fmt.Println("Terminal stopped.")
		return nil
	},
}

func init() {
	terminalCmd.Flags().StringVar(&terminalSubdomain, "subdomain", "", "Claim this subdomain before starting the tunnel")
	terminalCmd.Flags().String("tcp", "", "Use raw TCP transport (specify server address, e.g. localhost:8081)")
	rootCmd.AddCommand(terminalCmd)
}
