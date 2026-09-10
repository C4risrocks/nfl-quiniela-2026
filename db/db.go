package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type DB struct {
	*sql.DB
	DriverName string
	DBPath     string
}

// InitDB initializes database connection and migrates schema
func InitDB(driverName, dsn string) (*DB, error) {
	var sqlDriver string
	var connectionString string
	var resolvedPath string

	switch strings.ToLower(driverName) {
	case "postgres", "postgresql", "pgx":
		sqlDriver = "pgx"
		connectionString = dsn
		resolvedPath = dsn
		log.Println("[DB] Initializing PostgreSQL database connection...")
	default:
		sqlDriver = "sqlite"
		// Strip query parameters for directory creation
		dbFilePath := dsn
		if idx := strings.Index(dsn, "?"); idx != -1 {
			dbFilePath = dsn[:idx]
		}
		if dir := filepath.Dir(dbFilePath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0775); err != nil {
				log.Printf("[DB] Warning creating database directory %s: %v", dir, err)
			}
		}

		absPath, _ := filepath.Abs(dbFilePath)
		resolvedPath = absPath

		// Verify directory is writable
		testFile := filepath.Join(filepath.Dir(absPath), fmt.Sprintf(".perm_test_%d", time.Now().UnixNano()))
		if err := os.WriteFile(testFile, []byte("ok"), 0660); err != nil {
			log.Printf("[DB] CRITICAL WARNING: Database directory %s is NOT writable: %v", filepath.Dir(absPath), err)
		} else {
			_ = os.Remove(testFile)
			log.Printf("[DB] Storage directory verified writable: %s", filepath.Dir(absPath))
		}

		if !strings.Contains(dsn, "?") {
			connectionString = fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dsn)
		} else {
			connectionString = dsn
		}
		log.Printf("[DB] Initializing SQLite database at %s (path: %s)...", dsn, absPath)
	}

	dbConn, err := sql.Open(sqlDriver, connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Connection pool settings
	if sqlDriver == "sqlite" {
		dbConn.SetMaxOpenConns(1) // SQLite single writer for optimal WAL safety
		dbConn.SetMaxIdleConns(1)
		dbConn.SetConnMaxLifetime(0)
	} else {
		dbConn.SetMaxOpenConns(25)
		dbConn.SetMaxIdleConns(5)
		dbConn.SetConnMaxLifetime(5 * time.Minute)
	}

	if err := dbConn.Ping(); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	database := &DB{
		DB:         dbConn,
		DriverName: sqlDriver,
		DBPath:     resolvedPath,
	}

	if err := database.migrate(); err != nil {
		return nil, fmt.Errorf("database migration failed: %w", err)
	}

	log.Println("[DB] Database initialized and schema migrated successfully.")
	return database, nil
}

func (d *DB) migrate() error {
	ddl := schemaSQL
	if d.DriverName == "pgx" {
		// Postgres adaptations
		ddl = strings.ReplaceAll(ddl, "INTEGER PRIMARY KEY AUTOINCREMENT", "SERIAL PRIMARY KEY")
		ddl = strings.ReplaceAll(ddl, "BOOLEAN NOT NULL DEFAULT 1", "BOOLEAN NOT NULL DEFAULT TRUE")
		ddl = strings.ReplaceAll(ddl, "BOOLEAN NOT NULL DEFAULT 0", "BOOLEAN NOT NULL DEFAULT FALSE")
	}

	// Execute statements
	statements := strings.Split(ddl, ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := d.Exec(stmt); err != nil {
			return fmt.Errorf("executing migration query [%s]: %w", stmt, err)
		}
	}

	// Safe incremental column additions for existing databases
	userCols := []string{
		"ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT 0",
		"ALTER TABLE users ADD COLUMN verification_token TEXT DEFAULT NULL",
		"ALTER TABLE users ADD COLUMN verification_sent_at TIMESTAMP DEFAULT NULL",
		"ALTER TABLE users ADD COLUMN reset_token TEXT DEFAULT NULL",
		"ALTER TABLE users ADD COLUMN reset_token_expires_at TIMESTAMP DEFAULT NULL",
		"ALTER TABLE users ADD COLUMN notify_email BOOLEAN NOT NULL DEFAULT 1",
	}
	if d.DriverName == "pgx" {
		userCols = []string{
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT FALSE",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_token TEXT DEFAULT NULL",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_sent_at TIMESTAMP DEFAULT NULL",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS reset_token TEXT DEFAULT NULL",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS reset_token_expires_at TIMESTAMP DEFAULT NULL",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS notify_email BOOLEAN NOT NULL DEFAULT TRUE",
		}
	}
	for _, colStmt := range userCols {
		_, _ = d.Exec(colStmt) // Safe ignore if column already exists
	}

	gameCols := []string{
		"ALTER TABLE games ADD COLUMN broadcast TEXT DEFAULT ''",
		"ALTER TABLE games ADD COLUMN situation TEXT DEFAULT ''",
		"ALTER TABLE games ADD COLUMN linescores TEXT DEFAULT ''",
	}
	if d.DriverName == "pgx" {
		gameCols = []string{
			"ALTER TABLE games ADD COLUMN IF NOT EXISTS broadcast TEXT DEFAULT ''",
			"ALTER TABLE games ADD COLUMN IF NOT EXISTS situation TEXT DEFAULT ''",
			"ALTER TABLE games ADD COLUMN IF NOT EXISTS linescores TEXT DEFAULT ''",
		}
	}
	for _, colStmt := range gameCols {
		_, _ = d.Exec(colStmt) // Safe ignore if column already exists
	}

	return nil
}

// CheckpointWAL forces SQLite to merge and truncate the WAL file back into the main database
func (d *DB) CheckpointWAL() error {
	if d.DriverName == "sqlite" {
		_, err := d.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
		return err
	}
	return nil
}

// IsStorageWritable checks if the database directory is writable
func (d *DB) IsStorageWritable() bool {
	if d.DriverName == "sqlite" && d.DBPath != "" {
		dir := filepath.Dir(d.DBPath)
		testFile := filepath.Join(dir, fmt.Sprintf(".perm_check_%d", time.Now().UnixNano()))
		if err := os.WriteFile(testFile, []byte("ok"), 0660); err != nil {
			return false
		}
		_ = os.Remove(testFile)
		return true
	}
	return true
}
