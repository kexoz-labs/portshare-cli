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

var tcpCmd = &cobra.Command{
	Use:   "tcp [port]",
	Short: "Start a raw TCP tunnel to a local port (e.g. 22 for SSH, 5432 for Postgres)",
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

		// Create TCP tunnel
		payload := map[string]interface{}{
			"clientId":   cfg.ClientID,
			"port":       port,
			"tunnelType": "tcp",
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
			return fmt.Errorf("failed to create tcp tunnel, status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		
		var tunnelResponse struct {
			PublicTCPPort int `json:"publicTcpPort"`
		}
		if err := json.Unmarshal(body, &tunnelResponse); err != nil {
			return fmt.Errorf("failed to parse tunnel response: %w", err)
		}

		fmt.Printf("Client ID: %s\n", cfg.ClientID)
		fmt.Printf("Tunnel ID: tnl_%s\n", cfg.ClientID) // Just a mock for now, the user uses Connect
		fmt.Printf("To connect to this tunnel from another machine, run:\n")
		fmt.Printf("  portshare connect %s --port %d\n", cfg.ClientID, port)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		t := &tunnel.Tunnel{
			ServerURL: serverURL,
			ClientID:  cfg.ClientID,
			LocalPort: port,
			IsTCP:     true,
		}
		if err := t.StartMultiplexedWS(ctx); err != nil {
			return err
		}

		fmt.Println("Tunnel stopped.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tcpCmd)
}
