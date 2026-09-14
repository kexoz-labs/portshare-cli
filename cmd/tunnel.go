package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"portshare/config"
)

type cliPathRoute struct {
	Path string `json:"path"`
	Port int    `json:"port"`
}

type cliTunnel struct {
	ID           string         `json:"id"`
	Subdomain    string         `json:"subdomain"`
	CustomDomain string         `json:"customDomain"`
	Port         int            `json:"port"`
	RequireAuth  bool           `json:"requireAuth"`
	PathRoutes   []cliPathRoute `json:"pathRoutes"`
	Active       bool           `json:"active"`
}

var tunnelCmd = &cobra.Command{Use: "tunnel", Short: "Manage persistent tunnel configurations"}

func clientConfig() (*config.Config, error) {
	cfg, err := config.EnsureIdentity(NormalizedServerURL())
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	return cfg, nil
}

func tunnelURL(cfg *config.Config) string {
	return NormalizedServerURL() + "/client/tunnels?clientId=" + url.QueryEscape(cfg.ClientID)
}

func decodeAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	return fmt.Errorf("server returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
}

func fetchTunnels(cfg *config.Config) ([]cliTunnel, error) {
	resp, err := config.HTTPClient.Get(tunnelURL(cfg))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}
	var result struct {
		Tunnels []cliTunnel `json:"tunnels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Tunnels, nil
}

var tunnelListCmd = &cobra.Command{
	Use: "list", Short: "List saved tunnels",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := clientConfig()
		if err != nil {
			return err
		}
		tunnels, err := fetchTunnels(cfg)
		if err != nil {
			return err
		}
		if JSONOutput {
			fmt.Println(mustJSON(map[string]any{"tunnels": tunnels}))
			return nil
		}
		if len(tunnels) == 0 {
			fmt.Println("No saved tunnels.")
			return nil
		}
		for _, tunnel := range tunnels {
			fmt.Printf("%s\t%s.%s\tlocalhost:%d\t%s\n", tunnel.ID, tunnel.Subdomain, RootDomain, tunnel.Port, map[bool]string{true: "running", false: "stopped"}[tunnel.Active])
		}
		return nil
	},
}

var tunnelCreateSubdomain string
var tunnelCreatePort int
var tunnelCreateRoutes []string

var tunnelCreateCmd = &cobra.Command{
	Use: "create", Short: "Create a persistent tunnel",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := clientConfig()
		if err != nil {
			return err
		}
		if tunnelCreateSubdomain == "" || tunnelCreatePort < 1 || tunnelCreatePort > 65535 {
			return fmt.Errorf("--subdomain and a valid --port are required")
		}
		routes, err := parseRoutes(tunnelCreateRoutes, tunnelCreatePort)
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"clientId": cfg.ClientID, "subdomain": strings.ToLower(tunnelCreateSubdomain), "port": tunnelCreatePort, "customDomain": "", "requireAuth": false, "pathRoutes": routes})
		resp, err := config.HTTPClient.Post(NormalizedServerURL()+"/client/tunnels", "application/json", bytes.NewReader(payload))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			return decodeAPIError(resp)
		}
		var tunnel cliTunnel
		if err := json.NewDecoder(resp.Body).Decode(&tunnel); err != nil {
			return err
		}
		if JSONOutput {
			fmt.Println(mustJSON(tunnel))
			return nil
		}
		fmt.Printf("Created %s at https://%s.%s\n", tunnel.ID, tunnel.Subdomain, RootDomain)
		return nil
	},
}

var tunnelDeleteCmd = &cobra.Command{
	Use: "delete [tunnel-id]", Short: "Delete a persistent tunnel", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := clientConfig()
		if err != nil {
			return err
		}
		req, _ := http.NewRequest(http.MethodDelete, NormalizedServerURL()+"/client/tunnels/"+url.PathEscape(args[0])+"?clientId="+url.QueryEscape(cfg.ClientID), nil)
		resp, err := config.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			return decodeAPIError(resp)
		}
		if JSONOutput {
			fmt.Println(`{"deleted":true}`)
		} else {
			fmt.Println("Tunnel deleted.")
		}
		return nil
	},
}

var tunnelRoutesCmd = &cobra.Command{
	Use: "routes [tunnel-id] [path=port...]", Short: "View or replace tunnel path routes", Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := clientConfig()
		if err != nil {
			return err
		}
		tunnels, err := fetchTunnels(cfg)
		if err != nil {
			return err
		}
		var selected *cliTunnel
		for index := range tunnels {
			if tunnels[index].ID == args[0] {
				selected = &tunnels[index]
				break
			}
		}
		if selected == nil {
			return fmt.Errorf("tunnel %q was not found", args[0])
		}
		if len(args) == 1 {
			if JSONOutput {
				fmt.Println(mustJSON(selected.PathRoutes))
			} else {
				for _, route := range selected.PathRoutes {
					fmt.Printf("%s=%d\n", route.Path, route.Port)
				}
			}
			return nil
		}
		routes, err := parseRoutes(args[1:], selected.Port)
		if err != nil {
			return err
		}
		selected.PathRoutes = routes
		payload, _ := json.Marshal(map[string]any{"clientId": cfg.ClientID, "subdomain": selected.Subdomain, "customDomain": selected.CustomDomain, "port": selected.Port, "requireAuth": selected.RequireAuth, "pathRoutes": routes, "active": selected.Active})
		req, _ := http.NewRequest(http.MethodPut, NormalizedServerURL()+"/client/tunnels/"+url.PathEscape(selected.ID)+"?clientId="+url.QueryEscape(cfg.ClientID), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := config.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return decodeAPIError(resp)
		}
		if JSONOutput {
			var updated cliTunnel
			if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
				return err
			}
			fmt.Println(mustJSON(updated))
		} else {
			fmt.Println("Routes updated.")
		}
		return nil
	},
}

func parseRoutes(values []string, fallbackPort int) ([]cliPathRoute, error) {
	routes := make([]cliPathRoute, 0, len(values)+1)
	hasRoot := false
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("route %q must use /path=port", value)
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid route port in %q", value)
		}
		path := strings.TrimRight(strings.TrimSpace(parts[0]), "/")
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("route path %q must begin with /", path)
		}
		if path == "/" {
			hasRoot = true
		}
		routes = append(routes, cliPathRoute{Path: path, Port: port})
	}
	if !hasRoot {
		routes = append(routes, cliPathRoute{Path: "/", Port: fallbackPort})
	}
	return routes, nil
}

func init() {
	tunnelCreateCmd.Flags().StringVar(&tunnelCreateSubdomain, "subdomain", "", "Public subdomain")
	tunnelCreateCmd.Flags().IntVar(&tunnelCreatePort, "port", 0, "Local port")
	tunnelCreateCmd.Flags().StringSliceVar(&tunnelCreateRoutes, "route", nil, "Path mapping such as /api=7000")
	tunnelCmd.AddCommand(tunnelCreateCmd, tunnelListCmd, tunnelDeleteCmd, tunnelRoutesCmd)
	rootCmd.AddCommand(tunnelCmd)
}
