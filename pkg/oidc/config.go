package oidc

import (
	"os"
	"strings"
)

// Config holds OIDC configuration.
type Config struct {
	IssuerURL     string
	Audience      string
	AllowedGroups []string
}

// LoadConfigFromEnv loads OIDC configuration from environment variables.
func LoadConfigFromEnv() (config *Config) {
	config = &Config{
		IssuerURL: os.Getenv("OIDC_ISSUER_URL"),
		Audience:  os.Getenv("OIDC_AUDIENCE"),
	}

	// Parse allowed groups from comma-separated list
	allowedGroupsStr := os.Getenv("OIDC_ALLOWED_GROUPS")
	if allowedGroupsStr != "" {
		groups := strings.Split(allowedGroupsStr, ",")
		for _, group := range groups {
			trimmed := strings.TrimSpace(group)
			if trimmed != "" {
				config.AllowedGroups = append(config.AllowedGroups, trimmed)
			}
		}
	}

	return config
}
