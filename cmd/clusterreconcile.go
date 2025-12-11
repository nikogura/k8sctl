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

var fixTags bool

// clusterreconcileCmd represents the clusterreconcile command.
var clusterreconcileCmd = &cobra.Command{
	Use:   "reconcile [<cluster name>]",
	Short: "Reconcile cluster state and fix discrepancies",
	Long: `
Reconcile cluster state by comparing EC2 instances, Kubernetes nodes, and load balancer targets.

This command will:
- List all EC2 instances (with and without Cluster tag)
- List all Kubernetes nodes
- List all load balancer targets
- Report any discrepancies
- Optionally fix missing Cluster tags with --fix-tags
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
		serverURL := fmt.Sprintf("%s/%s/cluster/%s/reconcile", baseURL, apiVersion, cluster)

		if verbose {
			fmt.Printf("Target URL: %s\n", serverURL)
			fmt.Printf("Cluster: %s\n", cluster)
		}

		data := map[string]interface{}{
			"verbose":  verbose,
			"fix_tags": fixTags,
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
	clusterCmd.AddCommand(clusterreconcileCmd)
	clusterreconcileCmd.Flags().BoolVar(&fixTags, "fix-tags", false, "Automatically fix missing Cluster tags")
}
