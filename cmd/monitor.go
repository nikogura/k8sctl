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

var monitorInterval int

// monitorCmd represents the monitor command.
var monitorCmd = &cobra.Command{
	Use:   "monitor [<cluster name>]",
	Short: "Continuously monitor cluster health",
	Long: `
Continuously monitor cluster health by periodically checking:
- EC2 instances with Cluster tag
- Kubernetes node status
- Load balancer target health
- Discrepancies between systems

The monitor will run indefinitely, checking every interval (default 60 seconds).
Press Ctrl+C to stop monitoring.
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
		serverURL := fmt.Sprintf("%s/%s/monitor/%s", baseURL, apiVersion, cluster)

		if verbose {
			fmt.Printf("Target URL: %s\n", serverURL)
			fmt.Printf("Cluster: %s\n", cluster)
		}

		data := map[string]interface{}{
			"verbose":  verbose,
			"interval": monitorInterval,
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
	rootCmd.AddCommand(monitorCmd)
	monitorCmd.Flags().IntVarP(&monitorInterval, "interval", "i", 60, "Monitoring interval in seconds")
}
