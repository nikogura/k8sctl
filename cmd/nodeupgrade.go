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

// nodeupgradeCmd represents the nodeupgrade command.
var nodeupgradeCmd = &cobra.Command{
	Use:   "upgrade [<node name>]",
	Short: "Upgrade a specific node to specified Talos version",
	Long: `
Upgrade a specific K8s node to the specified Talos version.

This command will:
- Discover the appropriate Talos AMI for the target version
- Upgrade the specified node
- Optionally update Vault secrets after successful upgrade

Example:
  k8sctl node upgrade cluster1-cp-0 --version v1.10.8
  k8sctl node upgrade cluster1-worker-2 --version v1.10.8 --dry-run
`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			if nodeName == "" {
				nodeName = args[0]
			}
		}

		if cluster == "" {
			log.Fatalf("Cluster name is required. Use -c flag.")
		}

		// Get OIDC token
		token, err := getOIDCToken()
		if err != nil {
			log.Fatalf("Failed to get OIDC token: %v", err)
		}

		if showToken {
			fmt.Printf("OIDC Token:\n\n%s\n\n", token)
		}

		if nodeName == "" {
			log.Fatalf("Node name is required. Use -n flag or provide as argument.")
		}

		if upgradeVersion == "" {
			log.Fatalf("Version is required. Use --version flag.")
		}

		baseURL := getServerBaseURL(cluster)
		serverURL := fmt.Sprintf("%s/%s/cluster/%s/node/upgrade/%s", baseURL, apiVersion, cluster, nodeName)

		if verbose {
			fmt.Printf("Target URL: %s\n", serverURL)
			fmt.Printf("Cluster: %s\n", cluster)
			fmt.Printf("Node: %s\n", nodeName)
			fmt.Printf("Target Version: %s\n", upgradeVersion)
		}

		data := map[string]interface{}{
			"version":        upgradeVersion,
			"preserve":       preserve,
			"stage":          stage,
			"dry_run":        dryRun,
			"update_secrets": updateSecrets,
			"verbose":        verbose,
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
	nodeCmd.AddCommand(nodeupgradeCmd)
	nodeupgradeCmd.Flags().StringVar(&upgradeVersion, "version", "", "Target Talos version (e.g., v1.10.8)")
	nodeupgradeCmd.Flags().BoolVar(&preserve, "preserve", true, "Preserve ephemeral data during upgrade")
	nodeupgradeCmd.Flags().BoolVar(&stage, "stage", false, "Stage upgrade and reboot later")
	nodeupgradeCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Simulate the upgrade without executing")
	nodeupgradeCmd.Flags().BoolVar(&updateSecrets, "update-secrets", false, "Update Vault secrets after successful upgrade")

	err := nodeupgradeCmd.MarkFlagRequired("version")
	if err != nil {
		log.Fatalf("failed to mark version flag as required: %s", err)
	}
}
