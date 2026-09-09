-- Schema for NFL Quiniela 2026
-- Compatible with SQLite and PostgreSQL

CREATE TABLE IF NOT EXISTS system_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'player', -- 'admin' or 'player'
    avatar_url TEXT DEFAULT '',
    favorite_team_id INTEGER DEFAULT NULL,
    email_verified BOOLEAN NOT NULL DEFAULT 0,
    verification_token TEXT DEFAULT NULL,
    verification_sent_at TIMESTAMP DEFAULT NULL,
    reset_token TEXT DEFAULT NULL,
    reset_token_expires_at TIMESTAMP DEFAULT NULL,
    notify_email BOOLEAN NOT NULL DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS seasons (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    year INTEGER NOT NULL UNIQUE,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS weeks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id INTEGER NOT NULL,
    week_number INTEGER NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'scheduled', -- 'scheduled', 'active', 'completed'
    lock_type TEXT NOT NULL DEFAULT 'per_game', -- 'per_game' or 'week_start'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (season_id) REFERENCES seasons(id) ON DELETE CASCADE,
    UNIQUE(season_id, week_number)
);

CREATE TABLE IF NOT EXISTS teams (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE, -- e.g. 'KC', 'SF', 'PHI'
    name TEXT NOT NULL,        -- e.g. 'Chiefs'
    city TEXT NOT NULL,        -- e.g. 'Kansas City'
    logo_url TEXT NOT NULL,
    primary_color TEXT NOT NULL DEFAULT '#000000',
    secondary_color TEXT NOT NULL DEFAULT '#FFFFFF',
    conference TEXT NOT NULL,   -- 'AFC' or 'NFC'
    division TEXT NOT NULL     -- 'East', 'North', 'South', 'West'
);

CREATE TABLE IF NOT EXISTS games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    week_id INTEGER NOT NULL,
    espn_game_id TEXT DEFAULT '',
    home_team_id INTEGER NOT NULL,
    away_team_id INTEGER NOT NULL,
    kickoff_time TIMESTAMP NOT NULL,
    home_score INTEGER DEFAULT NULL,
    away_score INTEGER DEFAULT NULL,
    status TEXT NOT NULL DEFAULT 'scheduled', -- 'scheduled', 'in_progress', 'final'
    status_detail TEXT DEFAULT '',             -- e.g. 'Final', 'Q4 02:15', 'Halftime'
    is_tiebreaker BOOLEAN NOT NULL DEFAULT 0, -- 1 for designated tiebreaker game (e.g. Monday Night)
    is_locked BOOLEAN NOT NULL DEFAULT 0,     -- Manual override lock
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (week_id) REFERENCES weeks(id) ON DELETE CASCADE,
    FOREIGN KEY (home_team_id) REFERENCES teams(id),
    FOREIGN KEY (away_team_id) REFERENCES teams(id)
);

CREATE TABLE IF NOT EXISTS picks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    game_id INTEGER NOT NULL,
    picked_team_id INTEGER DEFAULT NULL,
    predicted_home_score INTEGER DEFAULT NULL,
    predicted_away_score INTEGER DEFAULT NULL,
    points_earned INTEGER DEFAULT 0,
    bonus_points INTEGER DEFAULT 0,
    is_correct BOOLEAN DEFAULT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE,
    UNIQUE(user_id, game_id)
);

CREATE TABLE IF NOT EXISTS weekly_leaderboard (
    week_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    total_points INTEGER NOT NULL DEFAULT 0,
    correct_picks INTEGER NOT NULL DEFAULT 0,
    total_picks INTEGER NOT NULL DEFAULT 0,
    tiebreaker_error INTEGER NOT NULL DEFAULT 999,
    rank INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (week_id, user_id),
    FOREIGN KEY (week_id) REFERENCES weeks(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_games_week_id ON games(week_id);
CREATE INDEX IF NOT EXISTS idx_games_kickoff ON games(kickoff_time);
CREATE INDEX IF NOT EXISTS idx_picks_user_id ON picks(user_id);
CREATE INDEX IF NOT EXISTS idx_picks_game_id ON picks(game_id);
CREATE INDEX IF NOT EXISTS idx_leaderboard_week ON weekly_leaderboard(week_id, rank);

CREATE TABLE IF NOT EXISTS notification_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    week_id INTEGER NOT NULL REFERENCES weeks(id) ON DELETE CASCADE,
    reminder_type TEXT NOT NULL,
    sent_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, week_id, reminder_type)
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_week ON notification_logs(user_id, week_id);
