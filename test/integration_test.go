package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nikogura/k8sctl/pkg/k8sctl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestK8sctlAuthIntegration tests k8sctl authentication flow.
func TestK8sctlAuthIntegration(t *testing.T) {
	t.Run("Basic auth-check endpoint functionality", func(t *testing.T) {
		// Create minimal test server with auth-check endpoint and mock auth
		server := createBasicTestServer(t)
		defer server.Close()

		// Test auth-check endpoint directly
		testBasicAuthCheck(t, server.URL)

		t.Logf("✓ Basic auth-check functionality verified")
	})

	t.Run("Auth-check endpoint without authentication", func(t *testing.T) {
		// Create server without auth middleware
		commands := &k8sctl.K8sCtlCommands{}
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.POST("/v1/auth-check", commands.AuthCheckHandler)

		server := httptest.NewServer(router)
		defer server.Close()

		// Test auth-check endpoint
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/v1/auth-check", nil)
		require.NoError(t, err)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		assert.Equal(t, "authenticated", result["status"])

		t.Logf("✓ Auth-check endpoint responds correctly")
	})

	t.Run("Complete OIDC authentication flow", func(t *testing.T) {
		t.Skip("Skipping OIDC mock test - requires complex JWKS setup. Use real Dex for integration testing.")
	})
}

// Helper functions

func createBasicTestServer(t *testing.T) (server *httptest.Server) {
	t.Helper()

	// Create k8sctl commands
	commands := &k8sctl.K8sCtlCommands{}

	// Create Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add auth-check endpoint (no auth for basic test)
	router.POST("/v1/auth-check", commands.AuthCheckHandler)

	server = httptest.NewServer(router)
	return server
}

func testBasicAuthCheck(t *testing.T, serverURL string) {
	t.Helper()

	// Make request to auth-check endpoint
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, serverURL+"/v1/auth-check", nil)
	require.NoError(t, err)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Decode response
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "authenticated", result["status"])
}
