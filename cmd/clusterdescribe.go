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

	"github.com/nikogura/k8s-cluster-manager/pkg/manager"
	"github.com/nikogura/k8sctl/pkg/k8sctl"
	"github.com/spf13/cobra"
)

// clusterDescribeCmd represents the clusterlist command.
var clusterDescribeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Describe a cluster",
	Long: `
List Information about a cluster.
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
		serverURL := fmt.Sprintf("%s/%s/cluster/describe/%s", baseURL, apiVersion, cluster)

		if verbose {
			fmt.Printf("Target URL: %s\n", serverURL)
			fmt.Printf("Cluster: %s\n", cluster)
		}

		data := k8sctl.DescribeClusterBody{
			Verbose: verbose,
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

		var info manager.ClusterInfo
		err = json.Unmarshal(body, &info)
		if err != nil {
			log.Fatalf("Failed unmarshalling cluster info: %s", err)
		}

		info.ConsolePrint()
	},
}

func init() {
	clusterCmd.AddCommand(clusterDescribeCmd)
}
