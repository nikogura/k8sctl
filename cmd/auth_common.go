package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nikogura/k8sctl/pkg/config"
	"github.com/nikogura/kubectl-ssh-oidc/pkg/kubectl"
)

var (
	// cachedConfig holds the loaded configuration.
	cachedConfig *config.Config
)

const debugEnvValue = "true"

// getOIDCToken gets an OIDC token from Dex using kubectl-ssh-oidc library, exactly like tdoctl.
func getOIDCToken() (token string, err error) {
	// Create config using kubectl-ssh-oidc's LoadConfig and override with our values
	config := kubectl.LoadConfig()

	// Override with our specific values (flags or env vars take precedence)
	if usernameValue := getConfigValue(username, "KUBECTL_SSH_USER"); usernameValue != "" {
		config.Username = usernameValue
	}

	// Set client credentials: use flag/env var if provided, otherwise use hardcoded defaults
	clientIDValue := getConfigValue(clientID, "K8SCTL_CLIENT_ID")
	if clientIDValue != "" {
		config.ClientID = clientIDValue
	} else {
		config.ClientID = "dc49b1cda0ee88b545e1f71c7460bf" // Default for internal VPN-only tool
	}

	clientSecretValue := getConfigValue(clientSecret, "K8SCTL_CLIENT_SECRET")
	if clientSecretValue != "" {
		config.ClientSecret = clientSecretValue
	} else {
		config.ClientSecret = "ecab4d07b4c83e779d9484acdc80e6" // Default for internal VPN-only tool
	}

	// Configure dual audience model exactly like imgctl
	// DexInstanceID: The audience for the SSH-signed JWT (must match Dex URL)
	// TargetAudience: The final audience for the OIDC token (should match server's expected audience)
	if dexURLValue := getConfigValue(dexURL, "DEX_URL"); dexURLValue != "" {
		config.DexURL = dexURLValue
		config.DexInstanceID = dexURLValue // SSH JWT audience = Dex instance URL
	}

	// Set TargetAudience for token generation (must match server's expected audience)
	// Use environment variable if set, otherwise construct from cluster suffix
	var targetAudience string
	if envAudience := os.Getenv("K8SCTL_AUDIENCE"); envAudience != "" {
		targetAudience = envAudience
	} else if cluster != "" {
		// Determine suffix from cluster mapping or environment override
		suffix := getClusterSuffix(cluster)
		targetAudience = fmt.Sprintf("https://k8sctl-%s.example.com", suffix)
	} else {
		// Default to dev environment if no cluster specified
		targetAudience = "https://k8sctl-dev.example.com"
	}
	config.TargetAudience = targetAudience

	// CRITICAL: Ensure SSHKeyPaths is nil (not empty slice) to allow kubectl-ssh-oidc
	// to properly detect SSH keys from agent and default locations, just like tdoctl
	config.SSHKeyPaths = nil

	// Debug: log configuration being used
	if os.Getenv("DEBUG") == debugEnvValue {
		fmt.Fprintf(os.Stderr, "DEBUG: Using config - DexURL: %s, ClientID: %s, DexInstanceID: %s, TargetAudience: %s, Username: %s\n",
			config.DexURL, config.ClientID, config.DexInstanceID, config.TargetAudience, config.Username)
		fmt.Fprintf(os.Stderr, "DEBUG: Config SSH key paths: %v\n", config.SSHKeyPaths)
		fmt.Fprintf(os.Stderr, "DEBUG: Config ClientSecret set: %t\n", config.ClientSecret != "")
	}

	// Create SSH-signed JWT using kubectl-ssh-oidc's function
	var sshJWT string
	sshJWT, err = kubectl.CreateSSHSignedJWT(config)
	if err != nil {
		err = fmt.Errorf("failed to create SSH-signed JWT: %w", err)
		return token, err
	}

	// Exchange with Dex for OIDC token using custom function that supports server URL audiences
	var tokenResp *kubectl.DexTokenResponse
	tokenResp, err = exchangeJWTForOIDC(config, sshJWT)
	if err != nil {
		err = fmt.Errorf("failed to exchange JWT with Dex: %w", err)
		return token, err
	}

	// Prefer ID token for OIDC, fallback to access token
	token = tokenResp.IDToken
	if token == "" {
		token = tokenResp.AccessToken
	}

	if token == "" {
		err = errors.New("no token received from Dex")
		return token, err
	}

	// Debug: decode token to see what we got
	if os.Getenv("DEBUG") == debugEnvValue {
		fmt.Fprintf(os.Stderr, "DEBUG: Received OIDC token: %s\n", token)
	}

	return token, err
}

// getConfigValue returns flag value if set, otherwise environment variable.
func getConfigValue(flagValue, envVar string) (value string) {
	if flagValue != "" {
		value = flagValue
		return value
	}
	value = os.Getenv(envVar)
	return value
}

// exchangeJWTForOIDC exchanges SSH-signed JWT for OIDC token with proper audience support.
func exchangeJWTForOIDC(config *kubectl.Config, sshJWT string) (tokenResp *kubectl.DexTokenResponse, err error) {
	baseURL := strings.TrimSuffix(config.DexURL, "/")
	tokenURL := baseURL + "/token"

	// Prepare OAuth2 Token Exchange request with audience parameter
	formData := url.Values{
		"grant_type":           {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token_type":   {"urn:ietf:params:oauth:token-type:access_token"},
		"subject_token":        {sshJWT},
		"requested_token_type": {"urn:ietf:params:oauth:token-type:id_token"},
		"scope":                {"openid email groups profile"},
		"connector_id":         {"ssh"},
		"client_id":            {config.ClientID},
	}

	// Add the crucial audience parameter for server URL audiences
	if config.TargetAudience != "" {
		formData.Set("audience", config.TargetAudience)
		if os.Getenv("DEBUG") == debugEnvValue {
			fmt.Fprintf(os.Stderr, "DEBUG: Setting audience parameter to: %s\n", config.TargetAudience)
		}
	}

	if config.ClientSecret != "" {
		formData.Set("client_secret", config.ClientSecret)
	}

	var req *http.Request
	req, err = http.NewRequestWithContext(context.Background(), http.MethodPost, tokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		err = fmt.Errorf("failed to create token request: %w", err)
		return tokenResp, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	var resp *http.Response
	resp, err = client.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to exchange with Dex: %w", err)
		return tokenResp, err
	}
	defer resp.Body.Close()

	var respBody []byte
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("failed to read token response: %w", err)
		return tokenResp, err
	}

	if os.Getenv("DEBUG") == debugEnvValue {
		fmt.Fprintf(os.Stderr, "DEBUG: Dex response: %s\n", string(respBody))
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("SSH authentication failed (%d): %s", resp.StatusCode, string(respBody))
		return tokenResp, err
	}

	var token kubectl.DexTokenResponse
	unmarshalErr := json.Unmarshal(respBody, &token)
	if unmarshalErr != nil {
		err = fmt.Errorf("failed to parse token response: %w", unmarshalErr)
		return tokenResp, err
	}

	tokenResp = &token
	return tokenResp, err
}

// loadConfig loads the k8sctl configuration if not already loaded.
func loadConfig() (cfg *config.Config, err error) {
	if cachedConfig != nil {
		cfg = cachedConfig
		return cfg, err
	}

	cfg, err = config.LoadDefault()
	if err != nil {
		return cfg, err
	}

	cachedConfig = cfg
	return cfg, err
}

// getClusterSuffix returns the environment suffix for a cluster.
// Priority order:
// 1. K8SCTL_CLUSTER_SUFFIX environment variable (overrides everything)
// 2. Configuration file cluster mapping
// 3. Default environment from config
// 4. Cluster name itself.
func getClusterSuffix(clusterName string) (suffix string) {
	// Check for environment override first
	if envSuffix := os.Getenv("K8SCTL_CLUSTER_SUFFIX"); envSuffix != "" {
		suffix = envSuffix
		return suffix
	}

	// Load config and check for cluster mapping
	cfg, err := loadConfig()
	if err == nil {
		suffix = cfg.GetClusterEnvironment(clusterName)
		return suffix
	}

	// Fall back to cluster name
	suffix = clusterName
	return suffix
}

// getServerBaseURL returns the base URL for the k8sctl server based on cluster name.
// Priority order:
// 1. K8SCTL_SERVER_URL environment variable (overrides everything)
// 2. Configuration file cluster-specific server URL
// 3. Constructed URL from environment suffix.
func getServerBaseURL(clusterName string) (baseURL string) {
	// Check for environment override first
	if envURL := os.Getenv("K8SCTL_SERVER_URL"); envURL != "" {
		baseURL = strings.TrimSuffix(envURL, "/")
		return baseURL
	}

	// Load config and check for cluster-specific URL
	cfg, err := loadConfig()
	if err == nil {
		if clusterURL := cfg.GetClusterServerURL(clusterName); clusterURL != "" {
			baseURL = strings.TrimSuffix(clusterURL, "/")
			return baseURL
		}
	}

	// Construct URL from environment suffix
	suffix := getClusterSuffix(clusterName)
	baseURL = fmt.Sprintf("https://k8sctl-%s.example.com", suffix)
	return baseURL
}

// makeAuthenticatedRequest makes an HTTP request with Bearer token authentication.
func makeAuthenticatedRequest(method, urlStr, body, token string) (resp *http.Response, err error) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	var req *http.Request
	req, err = http.NewRequestWithContext(context.Background(), method, urlStr, bodyReader)
	if err != nil {
		err = fmt.Errorf("failed to create request: %w", err)
		return resp, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	// Use configurable timeout from --timeout-seconds flag (default 300s)
	timeout := time.Duration(timeoutSeconds) * time.Second
	httpClient := &http.Client{Timeout: timeout}
	resp, err = httpClient.Do(req)
	return resp, err
}
