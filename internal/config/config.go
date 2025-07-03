package config

import (
	"os"
)

// Config holds all configuration for the application
type Config struct {
	Port                string
	DatabaseURL         string
	SessionSecret       string
	GitHubClientID      string
	GitHubClientSecret  string
	GitHubRedirectURL   string
	DeploymentRoot      string
	BaseDomain          string
	EnableHTTPS         bool
	GitHubWebhookSecret string
}

// New creates a new configuration instance with values from environment variables
func New() *Config {
	return &Config{
		Port:                getEnv("PORT", "8080"),
		DatabaseURL:         getEnv("DATABASE_URL", "./data/app.db"),
		SessionSecret:       getEnv("SESSION_SECRET", "change-this-secret-key"),
		GitHubClientID:      getEnv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret:  getEnv("GITHUB_CLIENT_SECRET", ""),
		GitHubRedirectURL:   getEnv("GITHUB_REDIRECT_URL", "http://localhost:8080/auth/github/callback"),
		DeploymentRoot:      getEnv("DEPLOYMENT_ROOT", "./deployments"),
		BaseDomain:          getEnv("BASE_DOMAIN", "localhost:8080"),
		EnableHTTPS:         getEnv("ENABLE_HTTPS", "false") == "true",
		GitHubWebhookSecret: getEnv("GITHUB_WEBHOOK_SECRET", ""),
	}
}

// getEnv gets an environment variable with a fallback default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
