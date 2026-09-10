package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Set test environment variables
	os.Setenv("PORT", "9999")
	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_PATH", "custom_test.db")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DB_TYPE")
		os.Unsetenv("DB_PATH")
	}()

	cfg := LoadConfig()
	if cfg.Port != "9999" {
		t.Errorf("Expected port 9999, got %s", cfg.Port)
	}
	if cfg.DBPath != "custom_test.db" {
		t.Errorf("Expected DBPath custom_test.db, got %s", cfg.DBPath)
	}
}

func TestLoadConfigAppDataFallback(t *testing.T) {
	// Create a temporary directory structure mimicking /app/data
	tmpDir := t.TempDir()
	appDataDir := filepath.Join(tmpDir, "app", "data")
	if err := os.MkdirAll(appDataDir, 0755); err != nil {
		t.Fatalf("Failed to create mock /app/data: %v", err)
	}

	// Unset DB_PATH to check default fallback
	os.Unsetenv("DB_PATH")
	cfg := LoadConfig()

	// In non-container local environments, defaults to "quiniela.db"
	if _, err := os.Stat("/app/data"); os.IsNotExist(err) {
		if cfg.DBPath != "quiniela.db" {
			t.Errorf("Expected quiniela.db in non-container local env, got %s", cfg.DBPath)
		}
	}
}
