package config

import (
	"os"
)

type Config struct {
	Port               string
	DatabaseURL        string
	GitHubClientID     string
	GitHubClientSecret string
	SessionSecret      string
	DeploymentPath     string
	BaseDomain         string
	Environment        string
}

func Load() *Config {
	return &Config{
		Port:               getEnv("PORT", "8080"),
		DatabaseURL:        getEnv("DATABASE_URL", "./data/app.db"),
		GitHubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
		SessionSecret:      getEnv("SESSION_SECRET", "your-secret-key"),
		DeploymentPath:     getEnv("DEPLOYMENT_PATH", "./deployments"),
		BaseDomain:         getEnv("BASE_DOMAIN", "localhost:8080"),
		Environment:        getEnv("ENVIRONMENT", "development"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
