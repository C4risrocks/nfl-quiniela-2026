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
}

// InitDB initializes database connection and migrates schema
func InitDB(driverName, dsn string) (*DB, error) {
	var sqlDriver string
	var connectionString string

	switch strings.ToLower(driverName) {
	case "postgres", "postgresql", "pgx":
		sqlDriver = "pgx"
		connectionString = dsn
		log.Println("[DB] Initializing PostgreSQL database connection...")
	default:
		sqlDriver = "sqlite"
		// Strip query parameters for directory creation
		dbFilePath := dsn
		if idx := strings.Index(dsn, "?"); idx != -1 {
			dbFilePath = dsn[:idx]
		}
		if dir := filepath.Dir(dbFilePath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Printf("[DB] Warning creating database directory %s: %v", dir, err)
			}
		}

		if !strings.Contains(dsn, "?") {
			connectionString = fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dsn)
		} else {
			connectionString = dsn
		}
		log.Printf("[DB] Initializing SQLite database at %s...", dsn)
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
	return nil
}
