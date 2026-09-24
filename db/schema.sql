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
    notify_kickoff BOOLEAN NOT NULL DEFAULT 1,
    notify_recap BOOLEAN NOT NULL DEFAULT 1,
    is_beta_tester BOOLEAN NOT NULL DEFAULT 0,
    bio TEXT DEFAULT '',
    featured_badge_code TEXT DEFAULT '',
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
    broadcast TEXT DEFAULT '',                 -- e.g. 'ESPN', 'FOX', 'NBC'
    situation TEXT DEFAULT '',                 -- e.g. '3rd & 4 at KC 42'
    linescores TEXT DEFAULT '',                -- JSON matrix of quarters
    stats_json TEXT DEFAULT '',                -- Detailed GameDetailedSummary JSON (boxscore, player stats, scoring, drives)
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
    tiebreaker_winner_correct BOOLEAN NOT NULL DEFAULT 0,
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

CREATE TABLE IF NOT EXISTS user_achievements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    badge_code TEXT NOT NULL,
    badge_name TEXT NOT NULL,
    badge_desc TEXT NOT NULL,
    icon TEXT NOT NULL,
    week_number INTEGER DEFAULT NULL,
    unlocked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, badge_code, week_number)
);

CREATE INDEX IF NOT EXISTS idx_achievements_user ON user_achievements(user_id);
CREATE INDEX IF NOT EXISTS idx_achievements_badge ON user_achievements(badge_code);

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
    consensus_level TEXT NOT NULL DEFAULT 'neutral', -- 'high', 'moderate', 'upset_alert'
    espn_available BOOLEAN NOT NULL DEFAULT 1,
    sources_summary TEXT NOT NULL DEFAULT '',
    audit_notes TEXT NOT NULL DEFAULT '',
    calculated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_forecasts_game_id ON game_forecasts(game_id);

CREATE TABLE IF NOT EXISTS in_app_notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    link TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL DEFAULT 'info', -- 'kickoff_reminder', 'weekly_recap', 'achievement', 'system'
    is_read BOOLEAN NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_in_app_notifs_user_read ON in_app_notifications(user_id, is_read, created_at DESC);

CREATE TABLE IF NOT EXISTS feature_flags (
    key TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    access_level TEXT NOT NULL DEFAULT 'all', -- 'all', 'beta', 'admin', 'disabled'
    is_beta BOOLEAN NOT NULL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS team_season_standings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    season_year INTEGER NOT NULL,
    team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL,
    team_code TEXT NOT NULL,
    team_name TEXT NOT NULL DEFAULT '',
    team_city TEXT NOT NULL DEFAULT '',
    logo_url TEXT NOT NULL DEFAULT '',
    primary_color TEXT NOT NULL DEFAULT '#000000',
    secondary_color TEXT NOT NULL DEFAULT '#FFFFFF',
    conference TEXT NOT NULL DEFAULT '',
    division TEXT NOT NULL DEFAULT '',
    wins INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    ties INTEGER NOT NULL DEFAULT 0,
    win_percent REAL NOT NULL DEFAULT 0.0,
    win_percent_formatted TEXT NOT NULL DEFAULT '.000',
    games_played INTEGER NOT NULL DEFAULT 0,
    points_for INTEGER NOT NULL DEFAULT 0,
    points_against INTEGER NOT NULL DEFAULT 0,
    point_diff INTEGER NOT NULL DEFAULT 0,
    offensive_ppg REAL NOT NULL DEFAULT 0.0,
    defensive_ppg REAL NOT NULL DEFAULT 0.0,
    streak TEXT NOT NULL DEFAULT '-',
    home_record TEXT NOT NULL DEFAULT '0-0',
    away_record TEXT NOT NULL DEFAULT '0-0',
    division_record TEXT NOT NULL DEFAULT '0-0',
    conf_record TEXT NOT NULL DEFAULT '0-0',
    games_behind TEXT NOT NULL DEFAULT '-',
    division_rank INTEGER NOT NULL DEFAULT 0,
    conference_seed INTEGER NOT NULL DEFAULT 0,
    is_super_bowl_champion BOOLEAN NOT NULL DEFAULT 0,
    super_bowl_title TEXT NOT NULL DEFAULT '',
    is_final BOOLEAN NOT NULL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(season_year, team_code)
);

CREATE INDEX IF NOT EXISTS idx_team_standings_season_seed ON team_season_standings(season_year, conference_seed);
CREATE INDEX IF NOT EXISTS idx_team_standings_team ON team_season_standings(team_id);

CREATE TABLE IF NOT EXISTS team_season_schedules (
    season_year INTEGER NOT NULL,
    team_code TEXT NOT NULL,
    schedule_json TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(season_year, team_code)
);

