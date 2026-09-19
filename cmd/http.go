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

	"github.com/mdp/qrterminal/v3"
	"github.com/spf13/cobra"
)

var httpSubdomain string
var httpRoutes []string
var tunnelDuration string
var tunnelOneTime bool
var tunnelPassword string

var httpCmd = &cobra.Command{
	Use:   "http [port]",
	Short: "Start an HTTP tunnel to a local port",
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

		payload := map[string]interface{}{
			"clientId":   cfg.ClientID,
			"port":       port,
			"subdomain":  httpSubdomain,
			"tunnelType": "http",
			"duration":   tunnelDuration,
			"oneTime":    tunnelOneTime,
			"password":   tunnelPassword,
		}

		if len(httpRoutes) > 0 {
			routes, err := parseRoutes(httpRoutes, port)
			if err != nil {
				return err
			}
			payload["pathRoutes"] = routes
		}

		payloadBytes, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, serverURL+"/client/tunnels", bytes.NewReader(payloadBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, err := config.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to create tunnel on server: %w", err)
		}
		defer resp.Body.Close()
		
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to create tunnel on server, status: %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var tunnelData struct {
			ID        string `json:"id"`
			Subdomain string `json:"subdomain"`
		}
		if err := json.Unmarshal(body, &tunnelData); err != nil {
			return fmt.Errorf("failed to parse tunnel response: %w", err)
		}

		if tunnelData.Subdomain != "" {
			cfg.Subdomain = tunnelData.Subdomain
			_ = config.SaveConfig(cfg)
		}

		localPortStr := fmt.Sprintf("%d", port)
		publicURL := ""
		if cfg.Subdomain != "" {
			publicURL = fmt.Sprintf("https://%s.%s", cfg.Subdomain, RootDomain)
		}

		if !JSONOutput {
			fmt.Printf("%sAuthenticating...%s done\n", ansiGray, ansiReset)
		}
		fmt.Printf("Client ID: %s\n", cfg.ClientID)

		if publicURL != "" {
			if JSONOutput {
				fmt.Printf("{\"url\":%q,\"port\":%d,\"client_id\":%q}\n", publicURL, port, cfg.ClientID)
			} else {
				printBanner(publicURL, localPortStr, cfg.ClientID, "")
				qrterminal.GenerateHalfBlock(publicURL, qrterminal.L, os.Stdout)
				fmt.Println()
			}
		} else {
			fmt.Printf("Tip: claim a subdomain for a stable URL: portshare domain claim <name>\n")
		}
		if cfg.CustomDomain != "" {
			fmt.Printf("Custom Domain URL: https://%s\n", cfg.CustomDomain)
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		t := &tunnel.Tunnel{
			ServerURL: serverURL,
			ClientID:  cfg.ClientID,
			LocalPort: port,
			OnRequest: func(method, path string, status int, durationMs int64) {
				if !JSONOutput {
					PrintRequest(method, path, status, durationMs)
				}
			},
		}
		
		tcpAddr, _ := cmd.Flags().GetString("tcp")
		if tcpAddr != "" {
			if err := t.StartTCP(ctx, tcpAddr); err != nil {
				return err
			}
		} else {
			if err := t.Start(ctx); err != nil {
				return err
			}
		}

		fmt.Println("Tunnel stopped.")
		return nil
	},
}

func init() {
	httpCmd.Flags().StringVar(&httpSubdomain, "subdomain", "", "Claim this subdomain before starting the tunnel")
	httpCmd.Flags().StringSliceVar(&httpRoutes, "route", nil, "Path mapping such as /api=7000")
	httpCmd.Flags().String("tcp", "", "Use raw TCP transport (specify server address, e.g. localhost:8081)")
	httpCmd.Flags().StringVar(&tunnelDuration, "duration", "", "Duration after which the tunnel expires (e.g. 1h, 30m)")
	httpCmd.Flags().BoolVar(&tunnelOneTime, "one-time", false, "Destroy the tunnel after a single use")
	httpCmd.Flags().StringVar(&tunnelPassword, "password", "", "Basic Auth password for the tunnel")
	rootCmd.AddCommand(httpCmd)
}
