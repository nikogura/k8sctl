/*
Copyright © 2025 Nik Ogura
*/
package cmd

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nikogura/k8sctl/pkg/k8sctl"
	"github.com/nikogura/k8sctl/pkg/oidc"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var address string

var logLevel string

// serverCmd represents the server command.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Server that listens for OIDC-authenticated commands and executes them",
	Long: `
Server that listens for OIDC-authenticated commands and executes them.

Uses environment variables for configuration:
- OIDC_ISSUER_URL: Dex issuer URL (required, e.g., https://dex.example.com)
- OIDC_AUDIENCE: The URL of this k8sctl server (required, e.g., https://k8sctl-dev.example.com)
- OIDC_ALLOWED_GROUPS: Comma-separated list of allowed groups (optional, defaults to engineering)
- CLOUDFLARE_API_TOKEN: Cloudflare API token for DNS management (required)
- CLOUDFLARE_ZONE_ID: Cloudflare zone ID (required)

Example:
  export OIDC_ISSUER_URL="https://dex.example.com"
  export OIDC_AUDIENCE="https://k8sctl-dev.example.com"
  export CLOUDFLARE_API_TOKEN="your-token"
  export CLOUDFLARE_ZONE_ID="your-zone-id"
  k8sctl server
`,
	Run: func(cmd *cobra.Command, args []string) {
		viper.AutomaticEnv()
		gin.SetMode(gin.ReleaseMode)

		// Load OIDC configuration from environment
		oidcConfig := oidc.LoadConfigFromEnv()
		if oidcConfig.IssuerURL == "" {
			log.Fatalf("OIDC_ISSUER_URL environment variable is required")
		}
		if oidcConfig.Audience == "" {
			log.Fatalf("OIDC_AUDIENCE environment variable is required")
		}

		// Set default allowed groups if not specified
		if len(oidcConfig.AllowedGroups) == 0 {
			oidcConfig.AllowedGroups = []string{"engineering"}
		}

		fmt.Printf("OIDC Issuer: %s\n", oidcConfig.IssuerURL)
		fmt.Printf("OIDC Audience: %s\n", oidcConfig.Audience)
		fmt.Printf("OIDC Allowed Groups: %v\n", oidcConfig.AllowedGroups)

		// Load Cloudflare configuration
		cfAPIToken := viper.GetString("CLOUDFLARE_API_TOKEN")
		cfZoneID := viper.GetString("CLOUDFLARE_ZONE_ID")

		if cfAPIToken == "" {
			log.Fatalf("CLOUDFLARE_API_TOKEN environment variable is required")
		}
		if cfZoneID == "" {
			log.Fatalf("CLOUDFLARE_ZONE_ID environment variable is required")
		}

		fmt.Printf("Cloudflare Zone ID: %s\n", cfZoneID)

		// Create logger for OIDC middleware
		logger, err := zap.NewProduction()
		if err != nil {
			log.Fatalf("failed to create logger: %s", err)
		}

		// Create OIDC validator
		oidcValidator := oidc.NewValidator(oidcConfig, logger)

		// Initialize k8sctl commands
		commands := &k8sctl.K8sCtlCommands{}

		// Inject CF credentials into package
		k8sctl.SetCloudflareCredentials(cfAPIToken, cfZoneID)

		// Create Gin router
		router := gin.Default()

		// Add status endpoint (unauthenticated)
		router.GET("/status", func(ctx *gin.Context) {
			ctx.JSON(http.StatusOK, gin.H{
				"status": "ok",
			})
		})

		// Create API group with OIDC authentication
		apiGroup := router.Group("/v1")
		apiGroup.Use(oidc.Middleware(oidcValidator))

		// Add API handlers
		apiGroup.POST("/cluster/describe/:cluster", commands.DescribeClusterHandler)
		apiGroup.POST("/cluster/:cluster/node/create", commands.CreateNodeHandler)
		apiGroup.POST("/cluster/:cluster/node/delete/:name", commands.DeleteNodeHandler)
		apiGroup.POST("/cluster/:cluster/node/glass/:name", commands.GlassNodeHandler)
		apiGroup.POST("/cluster/:cluster/node/describe/:name", commands.DescribeNodeHandler)
		apiGroup.POST("/cluster/:cluster/node/upgrade/:node", commands.UpgradeNodeHandler)
		apiGroup.POST("/cluster/:cluster/reconcile", commands.ReconcileClusterHandler)
		apiGroup.POST("/cluster/:cluster/upgrade", commands.UpgradeClusterHandler)
		apiGroup.POST("/cluster/:cluster/secrets/sync", commands.SecretsSyncHandler)
		apiGroup.POST("/monitor/:cluster", commands.MonitorClusterHandler)
		apiGroup.POST("/auth-check", commands.AuthCheckHandler)

		fmt.Printf("Starting k8sctl server with OIDC authentication via %s\n", oidcConfig.IssuerURL)
		fmt.Printf("Server starting on address: %s\n", address)

		err = router.Run(address)
		if err != nil {
			log.Fatalf("Failed to start server: %s", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVarP(&address, "bind-address", "b", "0.0.0.0:9999", "Address (host and port) on which to listen")
	serverCmd.Flags().StringVarP(&logLevel, "log-level", "l", "Info", "Log Level.  One of (Trace, Debug, Info, Warn, Error).")
}
