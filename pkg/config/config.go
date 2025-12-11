package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the k8sctl configuration.
type Config struct {
	// Clusters maps cluster names to their configuration
	Clusters map[string]ClusterConfig `yaml:"clusters"`

	// DefaultEnvironment is used when cluster is not found in mappings
	DefaultEnvironment string `yaml:"default_environment,omitempty"`
}

// ClusterConfig represents configuration for a single cluster.
type ClusterConfig struct {
	// Environment suffix for this cluster (e.g., "dev", "staging", "prod")
	Environment string `yaml:"environment"`

	// ServerURL overrides the default server URL for this cluster
	ServerURL string `yaml:"server_url,omitempty"`
}

// Load loads configuration from a file.
func Load(path string) (cfg *Config, err error) {
	var data []byte
	data, err = os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("failed to read config file %s: %w", path, err)
		return cfg, err
	}

	cfg = &Config{}
	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		err = fmt.Errorf("failed to parse config file %s: %w", path, err)
		return cfg, err
	}

	return cfg, err
}

// LoadDefault attempts to load configuration from default locations.
// It searches in order:
// 1. K8SCTL_CONFIG environment variable
// 2. ./k8sctl.yaml
// 3. ~/.config/k8sctl/config.yaml
// 4. /etc/k8sctl/config.yaml
// Returns nil config if no file found (not an error).
func LoadDefault() (cfg *Config, err error) {
	// Check environment variable first
	if configPath := os.Getenv("K8SCTL_CONFIG"); configPath != "" {
		cfg, err = Load(configPath)
		if err != nil {
			return cfg, err
		}
		return cfg, err
	}

	// Try default locations
	locations := []string{
		"./k8sctl.yaml",
		filepath.Join(os.Getenv("HOME"), ".config", "k8sctl", "config.yaml"),
		"/etc/k8sctl/config.yaml",
	}

	for _, loc := range locations {
		var statErr error
		_, statErr = os.Stat(loc)
		if statErr == nil {
			cfg, err = Load(loc)
			if err != nil {
				return cfg, err
			}
			return cfg, err
		}
	}

	// No config found - return empty config (not an error)
	cfg = &Config{
		Clusters:           make(map[string]ClusterConfig),
		DefaultEnvironment: "dev",
	}

	return cfg, err
}

// GetClusterEnvironment returns the environment suffix for a cluster.
func (c *Config) GetClusterEnvironment(clusterName string) (environment string) {
	if clusterCfg, ok := c.Clusters[clusterName]; ok {
		environment = clusterCfg.Environment
		return environment
	}

	// Fall back to default or cluster name itself
	if c.DefaultEnvironment != "" {
		environment = c.DefaultEnvironment
		return environment
	}

	environment = clusterName
	return environment
}

// GetClusterServerURL returns the server URL for a cluster.
// If not configured, returns empty string (caller should construct default).
func (c *Config) GetClusterServerURL(clusterName string) (serverURL string) {
	if clusterCfg, ok := c.Clusters[clusterName]; ok {
		serverURL = clusterCfg.ServerURL
		return serverURL
	}

	serverURL = ""
	return serverURL
}
