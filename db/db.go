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
		"ALTER TABLE users ADD COLUMN notify_kickoff BOOLEAN NOT NULL DEFAULT 1",
		"ALTER TABLE users ADD COLUMN notify_recap BOOLEAN NOT NULL DEFAULT 1",
		"ALTER TABLE users ADD COLUMN bio TEXT DEFAULT ''",
		"ALTER TABLE users ADD COLUMN featured_badge_code TEXT DEFAULT ''",
		"ALTER TABLE users ADD COLUMN is_beta_tester BOOLEAN NOT NULL DEFAULT 0",
	}
	if d.DriverName == "pgx" {
		userCols = []string{
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT FALSE",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_token TEXT DEFAULT NULL",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_sent_at TIMESTAMP DEFAULT NULL",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS reset_token TEXT DEFAULT NULL",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS reset_token_expires_at TIMESTAMP DEFAULT NULL",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS notify_email BOOLEAN NOT NULL DEFAULT TRUE",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS notify_kickoff BOOLEAN NOT NULL DEFAULT TRUE",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS notify_recap BOOLEAN NOT NULL DEFAULT TRUE",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT DEFAULT ''",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS featured_badge_code TEXT DEFAULT ''",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS is_beta_tester BOOLEAN NOT NULL DEFAULT FALSE",
		}
	}
	for _, colStmt := range userCols {
		_, _ = d.Exec(colStmt) // Safe ignore if column already exists
	}

	gameCols := []string{
		"ALTER TABLE games ADD COLUMN broadcast TEXT DEFAULT ''",
		"ALTER TABLE games ADD COLUMN situation TEXT DEFAULT ''",
		"ALTER TABLE games ADD COLUMN linescores TEXT DEFAULT ''",
		"ALTER TABLE games ADD COLUMN stats_json TEXT DEFAULT ''",
	}
	if d.DriverName == "pgx" {
		gameCols = []string{
			"ALTER TABLE games ADD COLUMN IF NOT EXISTS broadcast TEXT DEFAULT ''",
			"ALTER TABLE games ADD COLUMN IF NOT EXISTS situation TEXT DEFAULT ''",
			"ALTER TABLE games ADD COLUMN IF NOT EXISTS linescores TEXT DEFAULT ''",
			"ALTER TABLE games ADD COLUMN IF NOT EXISTS stats_json TEXT DEFAULT ''",
		}
	}
	for _, colStmt := range gameCols {
		_, _ = d.Exec(colStmt) // Safe ignore if column already exists
	}

	lbCols := []string{
		"ALTER TABLE weekly_leaderboard ADD COLUMN tiebreaker_winner_correct BOOLEAN NOT NULL DEFAULT 0",
	}
	if d.DriverName == "pgx" {
		lbCols = []string{
			"ALTER TABLE weekly_leaderboard ADD COLUMN IF NOT EXISTS tiebreaker_winner_correct BOOLEAN NOT NULL DEFAULT FALSE",
		}
	}
	for _, colStmt := range lbCols {
		_, _ = d.Exec(colStmt) // Safe ignore if column already exists
	}

	// Clean up duplicate user achievements and enforce partial unique indexes
	cleanupAchievements := `
	DELETE FROM user_achievements 
	WHERE id NOT IN (
		SELECT MIN(id) 
		FROM user_achievements 
		GROUP BY user_id, badge_code, COALESCE(week_number, -1)
	)`
	if res, err := d.Exec(cleanupAchievements); err == nil {
		if affected, _ := res.RowsAffected(); affected > 0 {
			log.Printf("[DB] Cleaned up %d duplicate user achievement records", affected)
		}
	}

	achievementIndexes := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_achievements_unique_week ON user_achievements(user_id, badge_code, week_number) WHERE week_number IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_achievements_unique_season ON user_achievements(user_id, badge_code) WHERE week_number IS NULL`,
	}
	for _, idxStmt := range achievementIndexes {
		if _, err := d.Exec(idxStmt); err != nil {
			log.Printf("[DB] Warning: could not create achievement index (%s): %v", idxStmt, err)
		}
	}

	// Create game_forecasts table if not exists
	createForecastsTable := `
	CREATE TABLE IF NOT EXISTS game_forecasts (
		game_id INTEGER PRIMARY KEY REFERENCES games(id) ON DELETE CASCADE,
		elo_home_prob REAL NOT NULL DEFAULT 0.5,
		elo_away_prob REAL NOT NULL DEFAULT 0.5,
		elo_spread REAL NOT NULL DEFAULT 0.0,
		proj_home_score INTEGER NOT NULL DEFAULT 21,
		proj_away_score INTEGER NOT NULL DEFAULT 20,
		predicted_winner_id INTEGER NOT NULL REFERENCES teams(id),
		vegas_favorite_id INTEGER DEFAULT NULL REFERENCES teams(id),
		vegas_spread REAL DEFAULT NULL,
		consensus_level TEXT NOT NULL DEFAULT 'neutral',
		espn_available BOOLEAN NOT NULL DEFAULT 1,
		sources_summary TEXT NOT NULL DEFAULT '',
		audit_notes TEXT NOT NULL DEFAULT '',
		calculated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`
	if d.DriverName == "pgx" {
		createForecastsTable = strings.ReplaceAll(createForecastsTable, "BOOLEAN NOT NULL DEFAULT 1", "BOOLEAN NOT NULL DEFAULT TRUE")
	}
	if _, err := d.Exec(createForecastsTable); err != nil {
		log.Printf("[DB] Warning creating game_forecasts table: %v", err)
	}
	_, _ = d.Exec(`CREATE INDEX IF NOT EXISTS idx_forecasts_game_id ON game_forecasts(game_id)`)

	// Seed official AI bot user (@ia_quiniela) if not exists
	seedBotUser := `
	INSERT INTO users (username, email, password_hash, role, email_verified, bio, avatar_url, notify_email)
	SELECT 'ia_quiniela', 'ia@quiniela.internal', 'NOPASSWORD_BOT_ACCOUNT', 'player', 1, '🤖 Bot Oficial de Inteligencia Artificial de la Quiniela. Pronósticos generados con FiveThirtyEight Elo y consenso de Las Vegas.', '/static/icons/bot_avatar.svg', 0
	WHERE NOT EXISTS (SELECT 1 FROM users WHERE username = 'ia_quiniela');
	`
	if d.DriverName == "pgx" {
		seedBotUser = strings.ReplaceAll(seedBotUser, "1, '🤖", "TRUE, '🤖")
		seedBotUser = strings.ReplaceAll(seedBotUser, ", 0\n", ", FALSE\n")
	}
	if _, err := d.Exec(seedBotUser); err != nil {
		log.Printf("[DB] Warning seeding bot user: %v", err)
	}

	// Create feature_flags table if not exists
	createFeatureFlagsTable := `
	CREATE TABLE IF NOT EXISTS feature_flags (
		key TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		access_level TEXT NOT NULL DEFAULT 'all',
		is_beta BOOLEAN NOT NULL DEFAULT 0,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	if d.DriverName == "pgx" {
		createFeatureFlagsTable = strings.ReplaceAll(createFeatureFlagsTable, "BOOLEAN NOT NULL DEFAULT 0", "BOOLEAN NOT NULL DEFAULT FALSE")
	}
	if _, err := d.Exec(createFeatureFlagsTable); err != nil {
		log.Printf("[DB] Warning creating feature_flags table: %v", err)
	}

	// Seed default feature flags
	seedFeatures := []struct {
		key         string
		name        string
		description string
		accessLevel string
		isBeta      int
	}{
		{"picks_matrix", "Matriz de Pronósticos", "Grilla interactiva comparativa de pronósticos de toda la liga semana a semana.", "beta", 1},
		{"picks_compare", "Duelo Cara a Cara", "Comparador de pronósticos y divergencias entre rivales directos.", "all", 1},
		{"ai_forecast", "Pronósticos con IA", "Probabilidades estimadas por machine learning y marcadores proyectados.", "all", 1},
		{"live_gamecenter", "Game Center en Vivo", "Transmisión en directo minuto a minuto vía Server-Sent Events.", "all", 0},
		{"community_picks", "Tendencias Comunitarias", "Porcentaje de selección comunitaria de cada equipo por partido.", "all", 0},
		{"live_whatif", "Simulador What-If", "Simulador interactivo de escenarios y proyección de tabla de posiciones en tiempo real.", "all", 1},
		{"ai_autofill", "Llenado Asistido por IA", "Autocompletar partidos y marcadores proyectados con un solo clic usando el modelo predictivo oficial.", "beta", 1},
	}
	for _, f := range seedFeatures {
		query := `INSERT OR IGNORE INTO feature_flags (key, name, description, access_level, is_beta) VALUES (?, ?, ?, ?, ?)`
		if d.DriverName == "pgx" {
			query = `INSERT INTO feature_flags (key, name, description, access_level, is_beta) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (key) DO NOTHING`
		}
		_, _ = d.Exec(query, f.key, f.name, f.description, f.accessLevel, f.isBeta)
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
