package db

import (
	"testing"
)

func TestRebindPostgres(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single placeholder",
			input:    "SELECT * FROM users WHERE id = ?",
			expected: "SELECT * FROM users WHERE id = $1",
		},
		{
			name:     "multiple placeholders",
			input:    "INSERT INTO picks (user_id, game_id, points) VALUES (?, ?, ?)",
			expected: "INSERT INTO picks (user_id, game_id, points) VALUES ($1, $2, $3)",
		},
		{
			name:     "literal question mark inside single quotes",
			input:    "SELECT * FROM games WHERE comment = 'Is it ready?' AND id = ?",
			expected: "SELECT * FROM games WHERE comment = 'Is it ready?' AND id = $1",
		},
		{
			name:     "escaped quote inside string with question mark",
			input:    "SELECT * FROM logs WHERE msg = 'It''s true?' AND user_id = ? AND code = ?",
			expected: "SELECT * FROM logs WHERE msg = 'It''s true?' AND user_id = $1 AND code = $2",
		},
		{
			name:     "no placeholders",
			input:    "SELECT count(*) FROM teams",
			expected: "SELECT count(*) FROM teams",
		},
		{
			name:     "ON CONFLICT upsert",
			input:    "INSERT INTO teams (code, name) VALUES (?, ?) ON CONFLICT(code) DO UPDATE SET name = excluded.name",
			expected: "INSERT INTO teams (code, name) VALUES ($1, $2) ON CONFLICT(code) DO UPDATE SET name = excluded.name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := RebindPostgres(tc.input)
			if actual != tc.expected {
				t.Errorf("\nInput:    %s\nExpected: %s\nActual:   %s", tc.input, tc.expected, actual)
			}
		})
	}
}

func TestDBRebindMethod(t *testing.T) {
	sqliteDB := &DB{DriverName: "sqlite"}
	pgDB := &DB{DriverName: "pgx"}

	query := "SELECT * FROM users WHERE id = ? AND email = ?"

	// SQLite must leave placeholders as ?
	if sqliteDB.Rebind(query) != query {
		t.Errorf("Expected SQLite rebind to leave query unchanged, got: %s", sqliteDB.Rebind(query))
	}

	// PostgreSQL must rebind to $1, $2
	expectedPG := "SELECT * FROM users WHERE id = $1 AND email = $2"
	if pgDB.Rebind(query) != expectedPG {
		t.Errorf("Expected PostgreSQL rebind to '%s', got: '%s'", expectedPG, pgDB.Rebind(query))
	}
}
