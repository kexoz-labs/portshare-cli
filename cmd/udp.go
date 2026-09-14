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

var udpCmd = &cobra.Command{
	Use:   "udp [port]",
	Short: "Start a UDP tunnel to a local port (e.g. 53 for DNS)",
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

		// Create UDP tunnel
		payload := map[string]interface{}{
			"clientId":   cfg.ClientID,
			"port":       port,
			"tunnelType": "udp",
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
			return fmt.Errorf("failed to create udp tunnel, status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		
		var tunnelResponse struct {
			PublicTCPPort int `json:"publicTcpPort"`
		}
		if err := json.Unmarshal(body, &tunnelResponse); err != nil {
			return fmt.Errorf("failed to parse tunnel response: %w", err)
		}

		fmt.Printf("Client ID: %s\n", cfg.ClientID)
		fmt.Printf("Forwarding udp://%s:%d -> localhost:%d\n", strings.TrimPrefix(strings.TrimPrefix(serverURL, "https://"), "http://"), tunnelResponse.PublicTCPPort, port)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		t := &tunnel.Tunnel{
			ServerURL: serverURL,
			ClientID:  cfg.ClientID,
			LocalPort: port,
			IsTCP:     false,
			IsUDP:     true,
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

		fmt.Println("Tunnel stopped.")
		return nil
	},
}

func init() {
	udpCmd.Flags().String("tcp", "", "Use raw TCP transport (specify server address, e.g. localhost:8081)")
	rootCmd.AddCommand(udpCmd)
}
