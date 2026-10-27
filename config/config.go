package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"

	webpush "github.com/SherClockHolmes/webpush-go"
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
	SMTPHost             string
	SMTPPort             int
	SMTPUser             string
	SMTPPass             string
	SMTPFrom             string
	AppBaseURL           string
	EnableReminders      bool
	UploadDir            string
	VAPIDPublicKey       string
	VAPIDPrivateKey      string
	VAPIDSubject         string
}

func LoadConfig() *Config {
	_ = godotenv.Load() // Loads .env if present, ignore error if missing

	port := getEnv("PORT", "8080")
	dbType := getEnv("DB_TYPE", "sqlite")

	defaultDBPath := "quiniela.db"
	if fi, err := os.Stat("/app/data"); err == nil && fi.IsDir() {
		defaultDBPath = "/app/data/quiniela.db"
	}
	dbPath := getEnv("DB_PATH", defaultDBPath)
	// If relative path given but running in container with /app/data volume, ensure data persistence
	if (dbPath == "quiniela.db" || dbPath == "./quiniela.db") && defaultDBPath == "/app/data/quiniela.db" {
		dbPath = "/app/data/quiniela.db"
	}

	defaultUploadDir := "uploads"
	if fi, err := os.Stat("/app/data"); err == nil && fi.IsDir() {
		defaultUploadDir = "/app/data/uploads"
	}
	uploadDir := getEnv("UPLOAD_DIR", defaultUploadDir)

	databaseURL := getEnv("DATABASE_URL", "")
	sessionSecret := getEnv("SESSION_SECRET", "super-secret-session-key-change-in-prod-2026")
	adminUser := getEnv("ADMIN_USERNAME", "admin")
	adminPass := getEnv("ADMIN_PASSWORD", "admin123")
	adminEmail := getEnv("ADMIN_EMAIL", "admin@quiniela.com")

	syncInterval := getEnvAsInt("ESPN_SYNC_INTERVAL_MINS", 5)
	seasonYear := getEnvAsInt("CURRENT_SEASON_YEAR", 2026)
	bgSync := getEnvAsBool("ENABLE_BACKGROUND_SYNC", true)

	smtpHost := getEnv("SMTP_HOST", "")
	smtpPort := getEnvAsInt("SMTP_PORT", 587)
	smtpUser := getEnv("SMTP_USER", "")
	smtpPass := getEnv("SMTP_PASS", "")
	smtpFrom := getEnv("SMTP_FROM", "quiniela@nfl2026.app")
	appBaseURL := getEnv("APP_BASE_URL", "http://localhost:8080")
	enableReminders := getEnvAsBool("ENABLE_REMINDERS", true)

	// Web Push VAPID Keys (RFC 8292)
	vapidPublicKey := getEnv("VAPID_PUBLIC_KEY", "")
	vapidPrivateKey := getEnv("VAPID_PRIVATE_KEY", "")
	vapidSubject := getEnv("VAPID_SUBJECT", "")
	if vapidSubject == "" {
		if adminEmail != "" {
			vapidSubject = "mailto:" + adminEmail
		} else {
			vapidSubject = "mailto:admin@quiniela.com"
		}
	}

	// If VAPID keys are not provided in environment, maintain persistent keypair in storage
	if vapidPublicKey == "" || vapidPrivateKey == "" {
		keyFile := ".vapid_keys.json"
		if fi, err := os.Stat("/app/data"); err == nil && fi.IsDir() {
			keyFile = "/app/data/.vapid_keys.json"
		} else if dbDir := filepath.Dir(dbPath); dbDir != "" && dbDir != "." {
			keyFile = filepath.Join(dbDir, ".vapid_keys.json")
		}

		type StoredKeys struct {
			PublicKey  string `json:"public_key"`
			PrivateKey string `json:"private_key"`
		}

		loaded := false
		if data, err := os.ReadFile(keyFile); err == nil {
			var sk StoredKeys
			if err := json.Unmarshal(data, &sk); err == nil && sk.PublicKey != "" && sk.PrivateKey != "" {
				vapidPublicKey = sk.PublicKey
				vapidPrivateKey = sk.PrivateKey
				loaded = true
				log.Printf("[Config] Loaded persistent VAPID keys from %s", keyFile)
			}
		}

		if !loaded {
			priv, pub, err := webpush.GenerateVAPIDKeys()
			if err == nil {
				vapidPublicKey = pub
				vapidPrivateKey = priv
				sk := StoredKeys{PublicKey: pub, PrivateKey: priv}
				if bytes, err := json.MarshalIndent(sk, "", "  "); err == nil {
					_ = os.WriteFile(keyFile, bytes, 0600)
					log.Printf("[Config] Generated new VAPID keypair and persisted to %s", keyFile)
				}
			} else {
				log.Printf("[Config] Warning: Failed to auto-generate VAPID keys: %v", err)
			}
		}
	}

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
		SMTPHost:             smtpHost,
		SMTPPort:             smtpPort,
		SMTPUser:             smtpUser,
		SMTPPass:             smtpPass,
		SMTPFrom:             smtpFrom,
		AppBaseURL:           appBaseURL,
		EnableReminders:      enableReminders,
		UploadDir:            uploadDir,
		VAPIDPublicKey:       vapidPublicKey,
		VAPIDPrivateKey:      vapidPrivateKey,
		VAPIDSubject:         vapidSubject,
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
