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
	// OfficialDomain: dominio oficial de la instancia (§0.2 CSRF, §2.3 [C3] anti-quishing).
	// Vacío (dev): se acepta Origin igual al Host de la request.
	OfficialDomain string
	// Presupuesto de sandboxes por escuela (§0.3/§7): contenedores prendidos
	// simultáneos y cola dura. Lleno el presupuesto → 202+queue_position;
	// llena la cola → 503 capacity_unavailable.
	SandboxMaxContainers int
	SandboxQueueLimit    int
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
		OfficialDomain:   getEnv("OFFICIAL_DOMAIN", ""),
		SandboxMaxContainers: getEnvInt("SANDBOX_MAX_CONTAINERS", 4),
		SandboxQueueLimit:    getEnvInt("SANDBOX_QUEUE_LIMIT", 8),
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
