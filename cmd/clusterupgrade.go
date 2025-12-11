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
	upgradeVersion     string
	controlPlaneFirst  bool
	maxConcurrent      int
	preserve           bool
	stage              bool
	waitBetweenSeconds int
	dryRun             bool
	updateSecrets      bool
)

// clusterupgradeCmd represents the clusterupgrade command.
var clusterupgradeCmd = &cobra.Command{
	Use:   "upgrade [<cluster name>]",
	Short: "Upgrade cluster to specified Talos version",
	Long: `
Orchestrates a rolling upgrade of all cluster nodes to the specified Talos version.

This command will:
- Discover the appropriate Talos AMI for the target version
- Upgrade control plane nodes sequentially (one at a time for etcd quorum safety)
- Upgrade worker nodes with configurable concurrency
- Optionally update Vault secrets after successful upgrade

Example:
  k8sctl cluster upgrade cluster1 --version v1.10.8
  k8sctl cluster upgrade cluster1 --version v1.10.8 --control-plane-first --max-concurrent 3
  k8sctl cluster upgrade cluster1 --version v1.10.8 --dry-run
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

		if upgradeVersion == "" {
			log.Fatalf("Version is required. Use --version flag.")
		}

		baseURL := getServerBaseURL(cluster)
		serverURL := fmt.Sprintf("%s/%s/cluster/%s/upgrade", baseURL, apiVersion, cluster)

		if verbose {
			fmt.Printf("Target URL: %s\n", serverURL)
			fmt.Printf("Cluster: %s\n", cluster)
			fmt.Printf("Target Version: %s\n", upgradeVersion)
		}

		data := map[string]interface{}{
			"version":             upgradeVersion,
			"control_plane_first": controlPlaneFirst,
			"max_concurrent":      maxConcurrent,
			"preserve":            preserve,
			"stage":               stage,
			"wait_between":        waitBetweenSeconds,
			"dry_run":             dryRun,
			"update_secrets":      updateSecrets,
			"verbose":             verbose,
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
	clusterCmd.AddCommand(clusterupgradeCmd)
	clusterupgradeCmd.Flags().StringVar(&upgradeVersion, "version", "", "Target Talos version (e.g., v1.10.8)")
	clusterupgradeCmd.Flags().BoolVar(&controlPlaneFirst, "control-plane-first", true, "Upgrade control plane nodes before workers")
	clusterupgradeCmd.Flags().IntVar(&maxConcurrent, "max-concurrent", 1, "Maximum concurrent node upgrades (control plane always 1)")
	clusterupgradeCmd.Flags().BoolVar(&preserve, "preserve", true, "Preserve ephemeral data during upgrade")
	clusterupgradeCmd.Flags().BoolVar(&stage, "stage", false, "Stage upgrade and reboot later")
	clusterupgradeCmd.Flags().IntVar(&waitBetweenSeconds, "wait-between", 30, "Wait duration in seconds between node upgrades")
	clusterupgradeCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Simulate the upgrade without executing")
	clusterupgradeCmd.Flags().BoolVar(&updateSecrets, "update-secrets", true, "Update Vault secrets after successful upgrade")

	err := clusterupgradeCmd.MarkFlagRequired("version")
	if err != nil {
		log.Fatalf("failed to mark version flag as required: %s", err)
	}
}
