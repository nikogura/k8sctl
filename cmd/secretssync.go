/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/spf13/cobra"
)

var (
	syncRole string
)

// secretssyncCmd represents the secretssync command.
var secretssyncCmd = &cobra.Command{
	Use:   "sync [<cluster name>]",
	Short: "Sync Vault secrets with running cluster versions",
	Long: `
Sync Vault secrets to match the actual versions running in the cluster.

This command will:
- Query the cluster nodes to determine their current Talos version
- Discover the appropriate AMI for that version in the region
- Update the Vault secrets (config.yaml installer image and node-aws.yaml AMI)

This is useful when clusters have been upgraded but Vault secrets were not updated,
preventing the secrets from becoming stale.

Example:
  k8sctl secrets sync cluster1
  k8sctl secrets sync cluster1 --role controlplane
  k8sctl secrets sync cluster1 --role worker --dry-run
`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			if cluster == "" {
				cluster = args[0]
			}

		}

		if cluster == "" {
			log.Fatalf("Cluster name is required. Use -c flag or provide as argument.")
		}

		// Get OIDC token
		token, err := getOIDCToken()
		if err != nil {
			log.Fatalf("Failed to get OIDC token: %v", err)
		}

		if showToken {
			fmt.Printf("OIDC Token:\n\n%s\n\n", token)
		}

		baseURL := getServerBaseURL(cluster)
		serverURL := fmt.Sprintf("%s/%s/cluster/%s/secrets/sync", baseURL, apiVersion, cluster)

		if verbose {
			fmt.Printf("Target URL: %s\n", serverURL)
			fmt.Printf("Cluster: %s\n", cluster)
			if syncRole != "" {
				fmt.Printf("Role: %s\n", syncRole)
			}
		}

		data := map[string]interface{}{
			"verbose": verbose,
			"dry_run": dryRun,
		}

		if syncRole != "" {
			data["role"] = syncRole
		}

		dataBytes, err := json.Marshal(data)
		if err != nil {
			log.Fatalf("unable to marshal post data: %s", err)
		}

		resp, err := makeAuthenticatedRequest("POST", serverURL, string(dataBytes), token)
		if err != nil {
			log.Fatalf("failed making authenticated request: %s", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("failed reading response body: %s", err)
		}

		if resp.StatusCode != http.StatusOK {
			log.Fatalf("request failed with status %d: %s", resp.StatusCode, body)
		}

		fmt.Printf("%s\n", body)
	},
}

func init() {
	rootCmd.AddCommand(secretssyncCmd)
	secretssyncCmd.Use = "secrets"
	secretssyncCmd.Short = "Manage cluster secrets"

	secretssyncCmd.Flags().StringVar(&syncRole, "role", "", "Specific role to sync (controlplane or worker). If not specified, syncs all roles.")
	secretssyncCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be updated without making changes")
}
