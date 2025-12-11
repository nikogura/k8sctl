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

	"github.com/nikogura/k8sctl/pkg/k8sctl"

	"github.com/spf13/cobra"
)

var roleName string

// nodecreateCmd represents the nodecreate command.
var nodecreateCmd = &cobra.Command{
	Use:   "create [<node name>]",
	Short: "Create a new K8s node",
	Long: `
Create a new K8s node
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

		baseURL := getServerBaseURL(cluster)
		serverURL := fmt.Sprintf("%s/%s/cluster/%s/node/create", baseURL, apiVersion, cluster)

		if verbose {
			fmt.Printf("Target URL: %s\n", serverURL)
			fmt.Printf("Cluster: %s\n", cluster)
			fmt.Printf("Node: %s\n", nodeName)
			fmt.Printf("Role: %s\n", roleName)
		}

		data := k8sctl.NodeCreateBody{
			Name:          nodeName,
			Role:          roleName,
			Verbose:       verbose,
			CloudProvider: "aws",
			Type:          nodeType,
			Purpose:       purpose,
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
	nodeCmd.AddCommand(nodecreateCmd)
	nodecreateCmd.Flags().StringVarP(&roleName, "role", "r", "worker", "Node role")

}
