package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var verbose bool

var username string

var timeoutSeconds int

var apiVersion string

var showToken bool

var cluster string

var dexURL string

var clientID string

var clientSecret string

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "k8sctl",
	Short: "Manage Talos Kubernetes Clusters",
	Long: `k8sctl is a command-line tool for managing Talos Kubernetes Clusters behind Cloud Load Balancers.

Authentication is performed via Dex OIDC provider using SSH key-based JWT tokens.
Users must be members of authorized groups configured on the server.

Example usage:
  # Describe a cluster
  k8sctl -c cluster1 cluster describe

  # Create a new node
  k8sctl -c cluster1 node create --name cluster1-cp-4 --role controlplane

  # Check authentication
  k8sctl -c cluster1 auth-check

  # With full Dex configuration
  k8sctl -d https://dex.example.com --client-id client-id --client-secret secret -c cluster1 cluster describe

Environment variables:
  DEX_URL, K8SCTL_CLIENT_ID, K8SCTL_CLIENT_SECRET, KUBECTL_SSH_USER can be used instead of flags
  (CLIENT_ID and CLIENT_SECRET have built-in defaults for internal use)`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "", false, "verbose output")
	rootCmd.PersistentFlags().StringVarP(&username, "username", "u", "", "Username for authentication")
	rootCmd.PersistentFlags().IntVarP(&timeoutSeconds, "timeout-seconds", "", 300, "Timeout")
	rootCmd.PersistentFlags().StringVarP(&apiVersion, "version", "v", "v1", "API version")
	rootCmd.PersistentFlags().BoolVarP(&showToken, "show-token", "", false, "Dump OIDC token to stdout")
	rootCmd.PersistentFlags().StringVarP(&cluster, "cluster", "c", "", "Cluster name (required)")
	rootCmd.PersistentFlags().StringVarP(&dexURL, "dex-url", "d", "", "Dex issuer URL for OIDC authentication")
	rootCmd.PersistentFlags().StringVar(&clientID, "client-id", "", "OAuth2 client ID for Dex (default: built-in)")
	rootCmd.PersistentFlags().StringVar(&clientSecret, "client-secret", "", "OAuth2 client secret for Dex (default: built-in)")
}
