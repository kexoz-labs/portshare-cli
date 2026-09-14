package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"portshare/tunnel"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect [tunnel-id]",
	Short: "Connect to a remote raw TCP tunnel (Client-to-Client)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tunnelID := args[0]
		port, _ := cmd.Flags().GetInt("port")
		
		serverURL := NormalizedServerURL()
		parsedURL, err := url.Parse(serverURL)
		if err != nil {
			return err
		}
		
		scheme := "ws"
		if parsedURL.Scheme == "https" {
			scheme = "wss"
		}
		wsURL := fmt.Sprintf("%s://%s/tunnel/join?tunnelId=%s", scheme, parsedURL.Host, url.QueryEscape(tunnelID))

		fmt.Printf("Connecting to tunnel %s...\n", tunnelID)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return fmt.Errorf("failed to bind local port %d: %w", port, err)
		}
		defer listener.Close()
		fmt.Printf("Listening on localhost:%d and routing to %s\n", port, tunnelID)

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Dial WS
				dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
				conn, _, err := dialer.DialContext(ctx, wsURL, nil)
				if err != nil {
					fmt.Printf("Failed to connect to gateway: %v. Retrying in 5s...\n", err)
					time.Sleep(5 * time.Second)
					continue
				}

				wsconn := &tunnel.WSNetConn{Conn: conn}
				
				// Initialize Yamux Client on this connection
				session, err := yamux.Client(wsconn, yamux.DefaultConfig())
				if err != nil {
					conn.Close()
					time.Sleep(2 * time.Second)
					continue
				}

				fmt.Println("Tunnel connection established.")

				// Now accept local connections
				for {
					localConn, err := listener.Accept()
					if err != nil {
						break
					}
					
					go func(lConn net.Conn) {
						defer lConn.Close()
						remoteStream, err := session.Open()
						if err != nil {
							fmt.Printf("Failed to open multiplexed stream: %v\n", err)
							return
						}
						defer remoteStream.Close()

						errc := make(chan error, 2)
						go func() {
							_, err := io.Copy(remoteStream, lConn)
							errc <- err
						}()
						go func() {
							_, err := io.Copy(lConn, remoteStream)
							errc <- err
						}()
						<-errc
					}(localConn)
				}

				session.Close()
			}
		}()

		<-ctx.Done()
		fmt.Println("Disconnecting...")
		return nil
	},
}

func init() {
	connectCmd.Flags().IntP("port", "p", 8080, "Local port to listen on")
	rootCmd.AddCommand(connectCmd)
}

