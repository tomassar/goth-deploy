package config

import (
	"os"
	"strconv"
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
	EncryptionKey      string
	IsolationConfig    IsolationConfig
}

type IsolationConfig struct {
	BaseUID      int
	BaseGID      int
	UsersDir     string
	ChrootBase   string
	EnableChroot bool
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
		EncryptionKey:      getEnv("ENCRYPTION_KEY", "default-encryption-key-change-me-in-production"),
		IsolationConfig: IsolationConfig{
			BaseUID:      getEnvInt("ISOLATION_BASE_UID", 10000),
			BaseGID:      getEnvInt("ISOLATION_BASE_GID", 10000),
			UsersDir:     getEnv("ISOLATION_USERS_DIR", "/var/lib/deployer/users"),
			ChrootBase:   getEnv("ISOLATION_CHROOT_BASE", "/var/lib/deployer/chroot"),
			EnableChroot: getEnvBool("ISOLATION_ENABLE_CHROOT", false),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
