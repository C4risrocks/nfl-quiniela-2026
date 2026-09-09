package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                 string
	DBType               string // "sqlite" or "postgres"
	DBPath               string
	DatabaseURL          string
	SessionSecret        string
	AdminUsername        string
	AdminPassword        string
	AdminEmail           string
	ESPNSyncIntervalMins int
	CurrentSeasonYear    int
	EnableBackgroundSync bool
}

func LoadConfig() *Config {
	_ = godotenv.Load() // Loads .env if present, ignore error if missing

	port := getEnv("PORT", "8080")
	dbType := getEnv("DB_TYPE", "sqlite")
	dbPath := getEnv("DB_PATH", "quiniela.db")
	databaseURL := getEnv("DATABASE_URL", "")
	sessionSecret := getEnv("SESSION_SECRET", "super-secret-session-key-change-in-prod-2026")
	adminUser := getEnv("ADMIN_USERNAME", "admin")
	adminPass := getEnv("ADMIN_PASSWORD", "admin123")
	adminEmail := getEnv("ADMIN_EMAIL", "admin@quiniela.com")

	syncInterval := getEnvAsInt("ESPN_SYNC_INTERVAL_MINS", 5)
	seasonYear := getEnvAsInt("CURRENT_SEASON_YEAR", 2026)
	bgSync := getEnvAsBool("ENABLE_BACKGROUND_SYNC", true)

	return &Config{
		Port:                 port,
		DBType:               dbType,
		DBPath:               dbPath,
		DatabaseURL:          databaseURL,
		SessionSecret:        sessionSecret,
		AdminUsername:        adminUser,
		AdminPassword:        adminPass,
		AdminEmail:           adminEmail,
		ESPNSyncIntervalMins: syncInterval,
		CurrentSeasonYear:    seasonYear,
		EnableBackgroundSync: bgSync,
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvAsBool(key string, fallback bool) bool {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return fallback
	}
	return val
}
