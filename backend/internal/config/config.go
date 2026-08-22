package config

import (
	"os"
	"strconv"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	Port             string
	TursoDatabaseURL string
	TursoAuthToken   string
	SessionSecret    string
	CookieDomain     string
	RateLimitRPH     int
	Env              string
}

// Load loads configuration from environment variables with sensible defaults for dev.
func Load() *Config {
	return &Config{
		Port:             getEnv("PORT", "8080"),
		TursoDatabaseURL: getEnv("TURSO_DATABASE_URL", ""),
		TursoAuthToken:   getEnv("TURSO_AUTH_TOKEN", ""),
		SessionSecret:    getEnv("SESSION_SECRET", "dev-secret-change-in-production"),
		CookieDomain:     getEnv("COOKIE_DOMAIN", ""),
		RateLimitRPH:     getEnvInt("RATE_LIMIT_RPH", 60),
		Env:              getEnv("ENV", "development"),
	}
}

// IsProduction returns true if running in production mode.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
