package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Version is injected at release time via:
// go build -ldflags "-X portshare/cmd.Version=v1.2.3"
var Version = "dev"

var ServerURL string
var RootDomain string
var JSONOutput bool

var rootCmd = &cobra.Command{
	Use:   "portshare",
	Short: "PortShare CLI to expose local ports to the internet",
	Long:  `PortShare CLI allows you to expose local web servers to the internet using PortShare tunnels.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// NormalizedServerURL returns ServerURL without a trailing slash.
func NormalizedServerURL() string {
	return strings.TrimSuffix(strings.TrimSpace(ServerURL), "/")
}

func defaultServerURL() string {
	if v := strings.TrimSpace(os.Getenv("PORTSHARE_SERVER_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return "https://api.portshare.kexoz.dev"
}

func defaultRootDomain() string {
	if v := strings.TrimSpace(os.Getenv("PORTSHARE_ROOT_DOMAIN")); v != "" {
		return v
	}
	return "portshare.kexoz.dev"
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&ServerURL, "server", "s", defaultServerURL(), "PortShare server URL (or PORTSHARE_SERVER_URL)")
	rootCmd.PersistentFlags().StringVarP(&RootDomain, "root-domain", "d", defaultRootDomain(), "PortShare root domain for public URLs (or PORTSHARE_ROOT_DOMAIN)")
	rootCmd.PersistentFlags().BoolVar(&JSONOutput, "json", false, "Print machine-readable JSON output")
}
