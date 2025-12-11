/*
Copyright © 2025 Nik Ogura
*/
package cmd

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/spf13/cobra"
)

// authCheckCmd represents the auth-check command.
var authCheckCmd = &cobra.Command{
	Use:   "auth-check",
	Short: "Check authentication with the server without performing any operations",
	Long: `
Check authentication with the server by sending a test request that verifies
your credentials without performing any actual operations. This is useful for
testing that your SSH keys and server configuration are working correctly.

Returns success (exit code 0) if authentication works, failure (exit code 1) otherwise.
`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get OIDC token using kubectl-ssh-oidc pattern
		token, err := getOIDCToken()
		if err != nil {
			log.Fatalf("Failed to get OIDC token: %v", err)
		}

		if showToken {
			fmt.Printf("OIDC Token:\n\n%s\n\n", token)
		}

		// Build server URL based on cluster
		baseURL := getServerBaseURL(cluster)
		serverURL := fmt.Sprintf("%s/%s/auth-check", baseURL, apiVersion)

		if verbose {
			fmt.Printf("Target URL: %s\n", serverURL)
			fmt.Printf("Cluster: %s\n", cluster)
		}

		resp, err := makeAuthenticatedRequest("POST", serverURL, "", token)
		if err != nil {
			log.Fatalf("failed making authenticated request: %s", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("failed reading response body: %s", err)
		}

		if resp.StatusCode == http.StatusOK {
			fmt.Printf("✓ Authentication successful: %s\n", body)
		} else {
			log.Fatalf("✗ Authentication failed with status %d: %s", resp.StatusCode, body)
		}
	},
}

func init() {
	rootCmd.AddCommand(authCheckCmd)
}
