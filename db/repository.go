package db

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

type Repository struct {
	db *DB
}

func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// ----------------------------------------------------
// System Settings
// ----------------------------------------------------

func (r *Repository) GetSetting(key, fallback string) (string, error) {
	var val string
	err := r.db.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&val)
	if err == sql.ErrNoRows {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	return val, nil
}

func (r *Repository) SetSetting(key, value string) error {
	query := `
	INSERT INTO system_settings (key, value, updated_at)
	VALUES (?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP;`
	_, err := r.db.Exec(query, key, value)
	return err
}

func (r *Repository) GetScoringConfig() (*ScoringConfig, error) {
	mode, _ := r.GetSetting("scoring_mode", "weighted")
	winnerPtsStr, _ := r.GetSetting("winner_points", "10")
	exactBonusStr, _ := r.GetSetting("exact_score_bonus", "5")
	marginBonusStr, _ := r.GetSetting("margin_bonus", "2")
	lockMode, _ := r.GetSetting("lock_mode", "per_game")

	winnerPts, _ := strconv.Atoi(winnerPtsStr)
	if winnerPts == 0 {
		if mode == "pure_tiebreaker" {
			winnerPts = 1
		} else {
			winnerPts = 10
		}
	}
	exactBonus, _ := strconv.Atoi(exactBonusStr)
	if exactBonus == 0 && mode == "weighted" {
		exactBonus = 5
	}
	marginBonus, _ := strconv.Atoi(marginBonusStr)
	if marginBonus == 0 && mode == "weighted" {
		marginBonus = 2
	}

	if lockMode != "full_week" {
		lockMode = "per_game"
	}

	return &ScoringConfig{
		ScoringMode:      mode,
		WinnerPoints:     winnerPts,
		ExactScoreBonus:  exactBonus,
		ExactMarginBonus: marginBonus,
		LockMode:         lockMode,
	}, nil
}

func (r *Repository) SetScoringConfig(cfg *ScoringConfig) error {
	if err := r.SetSetting("scoring_mode", cfg.ScoringMode); err != nil {
		return err
	}
	if err := r.SetSetting("winner_points", strconv.Itoa(cfg.WinnerPoints)); err != nil {
		return err
	}
	if err := r.SetSetting("exact_score_bonus", strconv.Itoa(cfg.ExactScoreBonus)); err != nil {
		return err
	}
	if err := r.SetSetting("margin_bonus", strconv.Itoa(cfg.ExactMarginBonus)); err != nil {
		return err
	}
	lockMode := cfg.LockMode
	if lockMode != "full_week" {
		lockMode = "per_game"
	}
	if err := r.SetSetting("lock_mode", lockMode); err != nil {
		return err
	}
	return nil
}

// ----------------------------------------------------
// Users
// ----------------------------------------------------

const userColumns = `id, username, email, password_hash, role, avatar_url, favorite_team_id, email_verified, verification_token, verification_sent_at, reset_token, reset_token_expires_at, notify_email, created_at`

func scanUserRow(scanner interface{ Scan(dest ...any) error }) (*User, error) {
	var u User
	var createdAtStr string
	var verifSentAtStr sql.NullString
	var resetExpStr sql.NullString

	err := scanner.Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.AvatarURL, &u.FavoriteTeamID,
		&u.EmailVerified, &u.VerificationToken, &verifSentAtStr,
		&u.ResetToken, &resetExpStr, &u.NotifyEmail, &createdAtStr,
	)
	if err != nil {
		return nil, err
	}
	u.CreatedAt = parseTimeSafe(createdAtStr)
	if verifSentAtStr.Valid {
		t := parseTimeSafe(verifSentAtStr.String)
		u.VerificationSentAt = &t
	}
	if resetExpStr.Valid {
		t := parseTimeSafe(resetExpStr.String)
		u.ResetTokenExpiresAt = &t
	}
	return &u, nil
}

func (r *Repository) CreateUser(username, email, passwordHash, role string) (*User, error) {
	return r.CreateUserWithVerification(username, email, passwordHash, role, "")
}

func (r *Repository) CreateUserWithVerification(username, email, passwordHash, role, token string) (*User, error) {
	var verifToken *string
	var verifSentAt *string
	if token != "" {
		verifToken = &token
		nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")
		verifSentAt = &nowStr
	}

	query := `
	INSERT INTO users (username, email, password_hash, role, email_verified, verification_token, verification_sent_at, notify_email, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP);`
	
	emailVerified := false
	if token == "" && role == "admin" {
		emailVerified = true // Auto-verify admin
	}

	res, err := r.db.Exec(query, username, email, passwordHash, role, emailVerified, verifToken, verifSentAt)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetUserByID(id)
}

func (r *Repository) GetUserByID(id int64) (*User, error) {
	query := fmt.Sprintf(`SELECT %s FROM users WHERE id = ?`, userColumns)
	return scanUserRow(r.db.QueryRow(query, id))
}

func (r *Repository) GetUserByUsername(username string) (*User, error) {
	query := fmt.Sprintf(`SELECT %s FROM users WHERE LOWER(username) = LOWER(?)`, userColumns)
	return scanUserRow(r.db.QueryRow(query, username))
}

func (r *Repository) GetUserByEmail(email string) (*User, error) {
	query := fmt.Sprintf(`SELECT %s FROM users WHERE LOWER(email) = LOWER(?)`, userColumns)
	return scanUserRow(r.db.QueryRow(query, email))
}

func (r *Repository) SetVerificationToken(userID int64, token string) error {
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")
	query := `UPDATE users SET verification_token = ?, verification_sent_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, token, nowStr, userID)
	return err
}

func (r *Repository) VerifyUserEmail(token string) (*User, error) {
	if token == "" {
		return nil, fmt.Errorf("token cannot be empty")
	}
	query := fmt.Sprintf(`SELECT %s FROM users WHERE verification_token = ?`, userColumns)
	u, err := scanUserRow(r.db.QueryRow(query, token))
	if err != nil {
		return nil, fmt.Errorf("token de verificación no encontrado o inválido")
	}

	updateQuery := `UPDATE users SET email_verified = 1, verification_token = NULL, verification_sent_at = NULL WHERE id = ?`
	if _, err := r.db.Exec(updateQuery, u.ID); err != nil {
		return nil, err
	}
	u.EmailVerified = true
	u.VerificationToken = nil
	u.VerificationSentAt = nil
	return u, nil
}

func (r *Repository) SetPasswordResetToken(email, token string, expiresAt time.Time) (*User, error) {
	u, err := r.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	expStr := expiresAt.UTC().Format("2006-01-02 15:04:05")
	query := `UPDATE users SET reset_token = ?, reset_token_expires_at = ? WHERE id = ?`
	if _, err := r.db.Exec(query, token, expStr, u.ID); err != nil {
		return nil, err
	}
	u.ResetToken = &token
	u.ResetTokenExpiresAt = &expiresAt
	return u, nil
}

func (r *Repository) GetUserByResetToken(token string) (*User, error) {
	if token == "" {
		return nil, fmt.Errorf("token cannot be empty")
	}
	query := fmt.Sprintf(`SELECT %s FROM users WHERE reset_token = ?`, userColumns)
	u, err := scanUserRow(r.db.QueryRow(query, token))
	if err != nil {
		return nil, fmt.Errorf("el enlace de restablecimiento es inválido")
	}
	if u.ResetTokenExpiresAt != nil && time.Now().UTC().After(u.ResetTokenExpiresAt.UTC()) {
		return nil, fmt.Errorf("el enlace de restablecimiento ha expirado. Por favor solicita uno nuevo")
	}
	return u, nil
}

func (r *Repository) ResetPasswordWithToken(token, newPasswordHash string) error {
	u, err := r.GetUserByResetToken(token)
	if err != nil {
		return err
	}
	query := `UPDATE users SET password_hash = ?, reset_token = NULL, reset_token_expires_at = NULL WHERE id = ?`
	_, err = r.db.Exec(query, newPasswordHash, u.ID)
	return err
}

func (r *Repository) UpdateUserPassword(userID int64, newPasswordHash string) error {
	query := `UPDATE users SET password_hash = ? WHERE id = ?`
	_, err := r.db.Exec(query, newPasswordHash, userID)
	return err
}

func (r *Repository) UpdateUserPreferences(userID int64, favoriteTeamID *int64, notifyEmail bool) error {
	query := `UPDATE users SET favorite_team_id = ?, notify_email = ? WHERE id = ?`
	_, err := r.db.Exec(query, favoriteTeamID, notifyEmail, userID)
	return err
}

func (r *Repository) GetUserStats(userID int64) (*UserStats, error) {
	var totalPicks, correctPicks, totalPoints int

	statsQuery := `
	SELECT 
		COUNT(p.id),
		COALESCE(SUM(CASE WHEN p.is_correct = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(p.points_earned + p.bonus_points), 0)
	FROM picks p
	JOIN games g ON p.game_id = g.id
	WHERE p.user_id = ? AND g.status = 'final'`

	err := r.db.QueryRow(statsQuery, userID).Scan(&totalPicks, &correctPicks, &totalPoints)
	if err != nil {
		return nil, err
	}

	accuracy := 0.0
	if totalPicks > 0 {
		accuracy = (float64(correctPicks) / float64(totalPicks)) * 100.0
	}

	return &UserStats{
		TotalPicks:   totalPicks,
		CorrectPicks: correctPicks,
		TotalPoints:  totalPoints,
		AccuracyRate: accuracy,
	}, nil
}

func (r *Repository) GetAdvancedUserStats(userID int64) (*AdvancedUserStats, error) {
	baseStats, err := r.GetUserStats(userID)
	if err != nil {
		return nil, err
	}

	adv := &AdvancedUserStats{
		UserStats: *baseStats,
		AFCStats: ConferenceStat{
			Conference: "AFC",
		},
		NFCStats: ConferenceStat{
			Conference: "NFC",
		},
		InterconfStats: ConferenceStat{
			Conference: "Interconferencia",
		},
	}

	query := `
	SELECT 
		p.is_correct,
		g.home_team_id, g.away_team_id, p.picked_team_id,
		ht.conference, at.conference,
		COALESCE(pt.code, ''), COALESCE(pt.name, ''), COALESCE(pt.logo_url, '')
	FROM picks p
	JOIN games g ON p.game_id = g.id
	JOIN teams ht ON g.home_team_id = ht.id
	JOIN teams at ON g.away_team_id = at.id
	LEFT JOIN teams pt ON p.picked_team_id = pt.id
	WHERE p.user_id = ? AND g.status = 'final'
	ORDER BY g.kickoff_time ASC, g.id ASC`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return adv, nil
	}
	defer rows.Close()

	type teamCounter struct {
		code    string
		name    string
		logo    string
		total   int
		correct int
	}
	teamMap := make(map[string]*teamCounter)

	currentStreak := 0
	maxStreak := 0

	for rows.Next() {
		var isCorrectNullable sql.NullBool
		var homeID, awayID int64
		var pickedID sql.NullInt64
		var homeConf, awayConf string
		var ptCode, ptName, ptLogo string

		if err := rows.Scan(&isCorrectNullable, &homeID, &awayID, &pickedID, &homeConf, &awayConf, &ptCode, &ptName, &ptLogo); err != nil {
			continue
		}

		isCorrect := isCorrectNullable.Valid && isCorrectNullable.Bool

		// Streaks
		if isCorrect {
			currentStreak++
			if currentStreak > maxStreak {
				maxStreak = currentStreak
			}
		} else {
			currentStreak = 0
		}

		// Conference breakdown
		if homeConf == "AFC" && awayConf == "AFC" {
			adv.AFCStats.TotalPicks++
			if isCorrect {
				adv.AFCStats.CorrectPicks++
			}
		} else if homeConf == "NFC" && awayConf == "NFC" {
			adv.NFCStats.TotalPicks++
			if isCorrect {
				adv.NFCStats.CorrectPicks++
			}
		} else {
			adv.InterconfStats.TotalPicks++
			if isCorrect {
				adv.InterconfStats.CorrectPicks++
			}
		}

		// Home vs Away pick tendency
		if pickedID.Valid {
			if pickedID.Int64 == homeID {
				adv.HomePicksTotal++
				if isCorrect {
					adv.HomePicksCorrect++
				}
			} else if pickedID.Int64 == awayID {
				adv.AwayPicksTotal++
				if isCorrect {
					adv.AwayPicksCorrect++
				}
			}

			// Team Affinity
			if ptCode != "" {
				tc, exists := teamMap[ptCode]
				if !exists {
					tc = &teamCounter{code: ptCode, name: ptName, logo: ptLogo}
					teamMap[ptCode] = tc
				}
				tc.total++
				if isCorrect {
					tc.correct++
				}
			}
		}
	}

	adv.CurrentStreak = currentStreak
	adv.MaxStreak = maxStreak

	// Compute conference accuracies
	if adv.AFCStats.TotalPicks > 0 {
		adv.AFCStats.Accuracy = (float64(adv.AFCStats.CorrectPicks) / float64(adv.AFCStats.TotalPicks)) * 100.0
	}
	if adv.NFCStats.TotalPicks > 0 {
		adv.NFCStats.Accuracy = (float64(adv.NFCStats.CorrectPicks) / float64(adv.NFCStats.TotalPicks)) * 100.0
	}
	if adv.InterconfStats.TotalPicks > 0 {
		adv.InterconfStats.Accuracy = (float64(adv.InterconfStats.CorrectPicks) / float64(adv.InterconfStats.TotalPicks)) * 100.0
	}

	// Compute home/away accuracies
	if adv.HomePicksTotal > 0 {
		adv.HomeAccuracy = (float64(adv.HomePicksCorrect) / float64(adv.HomePicksTotal)) * 100.0
	}
	if adv.AwayPicksTotal > 0 {
		adv.AwayAccuracy = (float64(adv.AwayPicksCorrect) / float64(adv.AwayPicksTotal)) * 100.0
	}

	// Identify Talisman (best team) and Nemesis (worst team)
	var bestTeam *teamCounter
	var worstTeam *teamCounter

	for _, tc := range teamMap {
		if tc.total == 0 {
			continue
		}
		acc := float64(tc.correct) / float64(tc.total)

		// Talisman: high accuracy and at least 1 correct
		if tc.correct > 0 {
			if bestTeam == nil {
				bestTeam = tc
			} else {
				bestAcc := float64(bestTeam.correct) / float64(bestTeam.total)
				if acc > bestAcc || (acc == bestAcc && tc.correct > bestTeam.correct) {
					bestTeam = tc
				}
			}
		}

		// Nemesis: failures > 0 and low accuracy
		failed := tc.total - tc.correct
		if failed > 0 {
			if worstTeam == nil {
				worstTeam = tc
			} else {
				worstAcc := float64(worstTeam.correct) / float64(worstTeam.total)
				if acc < worstAcc || (acc == worstAcc && failed > (worstTeam.total-worstTeam.correct)) {
					worstTeam = tc
				}
			}
		}
	}

	if bestTeam != nil {
		adv.TalismanTeam = &TeamAffinityStat{
			TeamCode:     bestTeam.code,
			TeamName:     bestTeam.name,
			LogoURL:      bestTeam.logo,
			TotalPicked:  bestTeam.total,
			CorrectCount: bestTeam.correct,
			Accuracy:     (float64(bestTeam.correct) / float64(bestTeam.total)) * 100.0,
		}
	}
	if worstTeam != nil {
		adv.NemesisTeam = &TeamAffinityStat{
			TeamCode:     worstTeam.code,
			TeamName:     worstTeam.name,
			LogoURL:      worstTeam.logo,
			TotalPicked:  worstTeam.total,
			CorrectCount: worstTeam.correct,
			Accuracy:     (float64(worstTeam.correct) / float64(worstTeam.total)) * 100.0,
		}
	}

	return adv, nil
}

func (r *Repository) ListUsers() ([]*User, error) {
	query := fmt.Sprintf(`SELECT %s FROM users ORDER BY username ASC`, userColumns)
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// ----------------------------------------------------
// Seasons & Weeks
// ----------------------------------------------------

func (r *Repository) GetActiveSeason(year int) (*Season, error) {
	query := `SELECT id, year, name, is_active FROM seasons WHERE year = ? LIMIT 1`
	var s Season
	err := r.db.QueryRow(query, year).Scan(&s.ID, &s.Year, &s.Name, &s.IsActive)
	if err == sql.ErrNoRows {
		// Auto-create season if missing
		ins := `INSERT INTO seasons (year, name, is_active) VALUES (?, ?, 1)`
		res, err := r.db.Exec(ins, year, fmt.Sprintf("%d NFL Season", year))
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		return &Season{ID: id, Year: year, Name: fmt.Sprintf("%d NFL Season", year), IsActive: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) ListWeeks(seasonID int64) ([]*Week, error) {
	query := `SELECT id, season_id, week_number, name, status, lock_type, created_at FROM weeks WHERE season_id = ? ORDER BY week_number ASC`
	rows, err := r.db.Query(query, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var weeks []*Week
	for rows.Next() {
		var w Week
		if err := rows.Scan(&w.ID, &w.SeasonID, &w.WeekNumber, &w.Name, &w.Status, &w.LockType, &w.CreatedAt); err != nil {
			return nil, err
		}
		weeks = append(weeks, &w)
	}
	return weeks, nil
}

func (r *Repository) GetWeekByID(id int64) (*Week, error) {
	query := `SELECT id, season_id, week_number, name, status, lock_type, created_at FROM weeks WHERE id = ?`
	var w Week
	err := r.db.QueryRow(query, id).Scan(&w.ID, &w.SeasonID, &w.WeekNumber, &w.Name, &w.Status, &w.LockType, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *Repository) GetWeekByNumber(seasonID int64, weekNumber int) (*Week, error) {
	query := `SELECT id, season_id, week_number, name, status, lock_type, created_at FROM weeks WHERE season_id = ? AND week_number = ?`
	var w Week
	err := r.db.QueryRow(query, seasonID, weekNumber).Scan(&w.ID, &w.SeasonID, &w.WeekNumber, &w.Name, &w.Status, &w.LockType, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *Repository) CreateWeek(seasonID int64, weekNum int, name string) (*Week, error) {
	query := `INSERT INTO weeks (season_id, week_number, name, status, lock_type) VALUES (?, ?, ?, 'scheduled', 'per_game')`
	res, err := r.db.Exec(query, seasonID, weekNum, name)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return r.GetWeekByID(id)
}

// ----------------------------------------------------
// Teams
// ----------------------------------------------------

func (r *Repository) ListTeams() ([]*Team, error) {
	query := `SELECT id, code, name, city, logo_url, primary_color, secondary_color, conference, division FROM teams ORDER BY city ASC, name ASC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.City, &t.LogoURL, &t.PrimaryColor, &t.SecondaryColor, &t.Conference, &t.Division); err != nil {
			return nil, err
		}
		teams = append(teams, &t)
	}
	return teams, nil
}

func (r *Repository) GetTeamByID(id int64) (*Team, error) {
	query := `SELECT id, code, name, city, logo_url, primary_color, secondary_color, conference, division FROM teams WHERE id = ?`
	var t Team
	err := r.db.QueryRow(query, id).Scan(&t.ID, &t.Code, &t.Name, &t.City, &t.LogoURL, &t.PrimaryColor, &t.SecondaryColor, &t.Conference, &t.Division)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) GetTeamByCode(code string) (*Team, error) {
	query := `SELECT id, code, name, city, logo_url, primary_color, secondary_color, conference, division FROM teams WHERE code = ?`
	var t Team
	err := r.db.QueryRow(query, code).Scan(&t.ID, &t.Code, &t.Name, &t.City, &t.LogoURL, &t.PrimaryColor, &t.SecondaryColor, &t.Conference, &t.Division)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) UpsertTeam(t *Team) error {
	query := `
	INSERT INTO teams (code, name, city, logo_url, primary_color, secondary_color, conference, division)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(code) DO UPDATE SET
		name = excluded.name,
		city = excluded.city,
		logo_url = excluded.logo_url,
		primary_color = excluded.primary_color,
		secondary_color = excluded.secondary_color,
		conference = excluded.conference,
		division = excluded.division;`
	_, err := r.db.Exec(query, t.Code, t.Name, t.City, t.LogoURL, t.PrimaryColor, t.SecondaryColor, t.Conference, t.Division)
	return err
}

// ----------------------------------------------------
// Games
// ----------------------------------------------------

func (r *Repository) ListGamesByWeek(weekID int64) ([]*Game, error) {
	week, _ := r.GetWeekByID(weekID)
	weekNum := 0
	if week != nil {
		weekNum = week.WeekNumber
	}

	query := `
	SELECT g.id, g.week_id, g.espn_game_id, g.home_team_id, g.away_team_id, g.kickoff_time,
	       g.home_score, g.away_score, g.status, g.status_detail,
	       COALESCE(g.broadcast, ''), COALESCE(g.situation, ''), COALESCE(g.linescores, ''),
	       g.is_tiebreaker, g.is_locked, g.created_at,
	       ht.id, ht.code, ht.name, ht.city, ht.logo_url, ht.primary_color, ht.secondary_color, ht.conference, ht.division,
	       at.id, at.code, at.name, at.city, at.logo_url, at.primary_color, at.secondary_color, at.conference, at.division
	FROM games g
	JOIN teams ht ON g.home_team_id = ht.id
	JOIN teams at ON g.away_team_id = at.id
	WHERE g.week_id = ?
	ORDER BY g.kickoff_time ASC, g.id ASC`

	rows, err := r.db.Query(query, weekID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		var g Game
		var ht Team
		var at Team
		var kickoffStr string

		if err := rows.Scan(
			&g.ID, &g.WeekID, &g.ESPNGameID, &g.HomeTeamID, &g.AwayTeamID, &kickoffStr,
			&g.HomeScore, &g.AwayScore, &g.Status, &g.StatusDetail,
			&g.Broadcast, &g.Situation, &g.Linescores,
			&g.IsTiebreaker, &g.IsLocked, &g.CreatedAt,
			&ht.ID, &ht.Code, &ht.Name, &ht.City, &ht.LogoURL, &ht.PrimaryColor, &ht.SecondaryColor, &ht.Conference, &ht.Division,
			&at.ID, &at.Code, &at.Name, &at.City, &at.LogoURL, &at.PrimaryColor, &at.SecondaryColor, &at.Conference, &at.Division,
		); err != nil {
			return nil, err
		}

		g.WeekNumber = weekNum
		g.HomeTeam = &ht
		g.AwayTeam = &at
		g.KickoffTime = parseTimeSafe(kickoffStr)

		games = append(games, &g)
	}

	return games, nil
}

func (r *Repository) GetGameByID(id int64) (*Game, error) {
	query := `
	SELECT g.id, g.week_id, g.espn_game_id, g.home_team_id, g.away_team_id, g.kickoff_time,
	       g.home_score, g.away_score, g.status, g.status_detail,
	       COALESCE(g.broadcast, ''), COALESCE(g.situation, ''), COALESCE(g.linescores, ''),
	       g.is_tiebreaker, g.is_locked, g.created_at,
	       ht.id, ht.code, ht.name, ht.city, ht.logo_url, ht.primary_color, ht.secondary_color, ht.conference, ht.division,
	       at.id, at.code, at.name, at.city, at.logo_url, at.primary_color, at.secondary_color, at.conference, at.division
	FROM games g
	JOIN teams ht ON g.home_team_id = ht.id
	JOIN teams at ON g.away_team_id = at.id
	WHERE g.id = ?`

	var g Game
	var ht Team
	var at Team
	var kickoffStr string

	err := r.db.QueryRow(query, id).Scan(
		&g.ID, &g.WeekID, &g.ESPNGameID, &g.HomeTeamID, &g.AwayTeamID, &kickoffStr,
		&g.HomeScore, &g.AwayScore, &g.Status, &g.StatusDetail,
		&g.Broadcast, &g.Situation, &g.Linescores,
		&g.IsTiebreaker, &g.IsLocked, &g.CreatedAt,
		&ht.ID, &ht.Code, &ht.Name, &ht.City, &ht.LogoURL, &ht.PrimaryColor, &ht.SecondaryColor, &ht.Conference, &ht.Division,
		&at.ID, &at.Code, &at.Name, &at.City, &at.LogoURL, &at.PrimaryColor, &at.SecondaryColor, &at.Conference, &at.Division,
	)
	if err != nil {
		return nil, err
	}
	g.HomeTeam = &ht
	g.AwayTeam = &at
	g.KickoffTime = parseTimeSafe(kickoffStr)
	if w, _ := r.GetWeekByID(g.WeekID); w != nil {
		g.WeekNumber = w.WeekNumber
	}
	return &g, nil
}

func (r *Repository) UpsertGameByESPNID(g *Game) error {
	if g.ESPNGameID == "" {
		return fmt.Errorf("espn_game_id is required for upsert")
	}

	var existingID int64
	err := r.db.QueryRow("SELECT id FROM games WHERE espn_game_id = ?", g.ESPNGameID).Scan(&existingID)
	if err == sql.ErrNoRows {
		// Insert
		query := `
		INSERT INTO games (week_id, espn_game_id, home_team_id, away_team_id, kickoff_time, home_score, away_score, status, status_detail, broadcast, situation, linescores, is_tiebreaker, is_locked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		res, err := r.db.Exec(query, g.WeekID, g.ESPNGameID, g.HomeTeamID, g.AwayTeamID, g.KickoffTime.Format("2006-01-02 15:04:05"), g.HomeScore, g.AwayScore, g.Status, g.StatusDetail, g.Broadcast, g.Situation, g.Linescores, g.IsTiebreaker, g.IsLocked)
		if err != nil {
			return err
		}
		if id, err := res.LastInsertId(); err == nil {
			g.ID = id
		}
		return nil
	}
	if err != nil {
		return err
	}

	// Update existing (preserve manual is_locked or is_tiebreaker if set)
	query := `
	UPDATE games SET
		week_id = ?,
		home_team_id = ?,
		away_team_id = ?,
		kickoff_time = ?,
		home_score = ?,
		away_score = ?,
		status = ?,
		status_detail = ?,
		broadcast = ?,
		situation = ?,
		linescores = ?
	WHERE id = ?`
	_, err = r.db.Exec(query, g.WeekID, g.HomeTeamID, g.AwayTeamID, g.KickoffTime.Format("2006-01-02 15:04:05"), g.HomeScore, g.AwayScore, g.Status, g.StatusDetail, g.Broadcast, g.Situation, g.Linescores, existingID)
	if err == nil {
		g.ID = existingID
	}
	return err
}

func (r *Repository) CreateManualGame(g *Game) (*Game, error) {
	query := `
	INSERT INTO games (week_id, espn_game_id, home_team_id, away_team_id, kickoff_time, home_score, away_score, status, status_detail, is_tiebreaker, is_locked)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.Exec(query, g.WeekID, g.ESPNGameID, g.HomeTeamID, g.AwayTeamID, g.KickoffTime.Format("2006-01-02 15:04:05"), g.HomeScore, g.AwayScore, g.Status, g.StatusDetail, g.IsTiebreaker, g.IsLocked)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return r.GetGameByID(id)
}

func (r *Repository) UpdateGameScoreAndStatus(gameID int64, homeScore, awayScore *int, status, statusDetail string) error {
	query := `UPDATE games SET home_score = ?, away_score = ?, status = ?, status_detail = ? WHERE id = ?`
	_, err := r.db.Exec(query, homeScore, awayScore, status, statusDetail, gameID)
	return err
}

func (r *Repository) ToggleGameLock(gameID int64, locked bool) error {
	query := `UPDATE games SET is_locked = ? WHERE id = ?`
	_, err := r.db.Exec(query, locked, gameID)
	return err
}

func (r *Repository) SetGameTiebreaker(gameID int64, isTiebreaker bool) error {
	query := `UPDATE games SET is_tiebreaker = ? WHERE id = ?`
	_, err := r.db.Exec(query, isTiebreaker, gameID)
	return err
}

func (r *Repository) DeletePlaceholderSeedGames(weekID int64) error {
	query := `DELETE FROM games WHERE week_id = ? AND espn_game_id LIKE 'seed-%'`
	_, err := r.db.Exec(query, weekID)
	return err
}

// ----------------------------------------------------
// Picks
// ----------------------------------------------------

func (r *Repository) GetUserPicksForWeek(userID, weekID int64) (map[int64]*Pick, error) {
	query := `
	SELECT p.id, p.user_id, p.game_id, p.picked_team_id, p.predicted_home_score, p.predicted_away_score, p.points_earned, p.bonus_points, p.is_correct, p.updated_at,
	       COALESCE(t.code, '') as team_code
	FROM picks p
	JOIN games g ON p.game_id = g.id
	LEFT JOIN teams t ON p.picked_team_id = t.id
	WHERE p.user_id = ? AND g.week_id = ?`

	rows, err := r.db.Query(query, userID, weekID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	picksMap := make(map[int64]*Pick)
	for rows.Next() {
		var p Pick
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.GameID, &p.PickedTeamID,
			&p.PredictedHomeScore, &p.PredictedAwayScore,
			&p.PointsEarned, &p.BonusPoints, &p.IsCorrect,
			&p.UpdatedAt, &p.TeamCode,
		); err != nil {
			return nil, err
		}
		picksMap[p.GameID] = &p
	}
	return picksMap, nil
}

func (r *Repository) GetUserPickForGame(userID, gameID int64) (*Pick, error) {
	query := `
	SELECT id, user_id, game_id, picked_team_id, predicted_home_score, predicted_away_score, points_earned, bonus_points, is_correct, updated_at
	FROM picks WHERE user_id = ? AND game_id = ?`
	var p Pick
	err := r.db.QueryRow(query, userID, gameID).Scan(
		&p.ID, &p.UserID, &p.GameID, &p.PickedTeamID,
		&p.PredictedHomeScore, &p.PredictedAwayScore,
		&p.PointsEarned, &p.BonusPoints, &p.IsCorrect,
		&p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) SavePick(userID, gameID int64, pickedTeamID *int64, predHome, predAway *int) (*Pick, error) {
	query := `
	INSERT INTO picks (user_id, game_id, picked_team_id, predicted_home_score, predicted_away_score, updated_at)
	VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(user_id, game_id) DO UPDATE SET
		picked_team_id = COALESCE(excluded.picked_team_id, picks.picked_team_id),
		predicted_home_score = COALESCE(excluded.predicted_home_score, picks.predicted_home_score),
		predicted_away_score = COALESCE(excluded.predicted_away_score, picks.predicted_away_score),
		updated_at = CURRENT_TIMESTAMP;`

	_, err := r.db.Exec(query, userID, gameID, pickedTeamID, predHome, predAway)
	if err != nil {
		return nil, err
	}
	return r.GetUserPickForGame(userID, gameID)
}

func (r *Repository) ListPicksForGame(gameID int64) ([]*Pick, error) {
	query := `
	SELECT p.id, p.user_id, p.game_id, p.picked_team_id, p.predicted_home_score, p.predicted_away_score, p.points_earned, p.bonus_points, p.is_correct, p.updated_at,
	       u.id, u.username, u.email, u.role, u.avatar_url,
	       COALESCE(t.code, '') as team_code
	FROM picks p
	JOIN users u ON p.user_id = u.id
	LEFT JOIN teams t ON p.picked_team_id = t.id
	WHERE p.game_id = ?
	ORDER BY u.username ASC`

	rows, err := r.db.Query(query, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var picks []*Pick
	for rows.Next() {
		var p Pick
		var u User
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.GameID, &p.PickedTeamID,
			&p.PredictedHomeScore, &p.PredictedAwayScore,
			&p.PointsEarned, &p.BonusPoints, &p.IsCorrect,
			&p.UpdatedAt,
			&u.ID, &u.Username, &u.Email, &u.Role, &u.AvatarURL,
			&p.TeamCode,
		); err != nil {
			return nil, err
		}
		p.User = &u
		picks = append(picks, &p)
	}
	return picks, nil
}

func (r *Repository) GetAllUsersPicksForWeek(weekID int64) ([]*UserWeekSimulationData, error) {
	query := `
	SELECT p.user_id, u.username, COALESCE(u.avatar_url, ''), p.game_id, COALESCE(p.picked_team_id, 0)
	FROM picks p
	JOIN users u ON p.user_id = u.id
	JOIN games g ON p.game_id = g.id
	WHERE g.week_id = ? AND COALESCE(u.role, 'player') != 'admin'
	ORDER BY u.username ASC`

	rows, err := r.db.Query(query, weekID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	userMap := make(map[int64]*UserWeekSimulationData)
	var userOrder []int64

	for rows.Next() {
		var uID, gID, teamID int64
		var uname, avatar string
		if err := rows.Scan(&uID, &uname, &avatar, &gID, &teamID); err != nil {
			return nil, err
		}
		data, ok := userMap[uID]
		if !ok {
			data = &UserWeekSimulationData{
				UserID:    uID,
				Username:  uname,
				AvatarURL: avatar,
				Picks:     make(map[int64]int64),
			}
			userMap[uID] = data
			userOrder = append(userOrder, uID)
		}
		if teamID > 0 {
			data.Picks[gID] = teamID
		}
	}

	result := make([]*UserWeekSimulationData, 0, len(userOrder))
	for _, id := range userOrder {
		result = append(result, userMap[id])
	}
	return result, nil
}

func (r *Repository) GetGameCommunityStats(gameID int64) (*GameCommunityStats, error) {
	game, err := r.GetGameByID(gameID)
	if err != nil {
		return nil, err
	}
	picks, err := r.ListPicksForGame(gameID)
	if err != nil {
		return nil, err
	}

	stats := &GameCommunityStats{
		TotalPicks: len(picks),
	}

	if len(picks) == 0 {
		return stats, nil
	}

	var homeCount, awayCount int
	var homeScoreSum, awayScoreSum int
	var scoreCount int

	for _, p := range picks {
		if p.PickedTeamID != nil {
			if *p.PickedTeamID == game.HomeTeamID {
				homeCount++
			} else if *p.PickedTeamID == game.AwayTeamID {
				awayCount++
			}
		}
		if p.PredictedHomeScore != nil && p.PredictedAwayScore != nil {
			homeScoreSum += *p.PredictedHomeScore
			awayScoreSum += *p.PredictedAwayScore
			scoreCount++
		}
	}

	stats.HomePicksCount = homeCount
	stats.AwayPicksCount = awayCount

	validPicks := homeCount + awayCount
	if validPicks > 0 {
		stats.HomePct = int(math.Round(float64(homeCount) / float64(validPicks) * 100.0))
		stats.AwayPct = 100 - stats.HomePct
	}

	if scoreCount > 0 {
		avgHome := math.Round((float64(homeScoreSum)/float64(scoreCount))*10) / 10
		avgAway := math.Round((float64(awayScoreSum)/float64(scoreCount))*10) / 10
		stats.AvgHomeScore = &avgHome
		stats.AvgAwayScore = &avgAway
	}

	return stats, nil
}

func (r *Repository) GetHeadToHeadComparison(weekID, userAID, userBID int64) (*HeadToHeadComparison, error) {
	userA, err := r.GetUserByID(userAID)
	if err != nil {
		return nil, fmt.Errorf("user A not found: %w", err)
	}
	userB, err := r.GetUserByID(userBID)
	if err != nil {
		return nil, fmt.Errorf("user B not found: %w", err)
	}
	week, err := r.GetWeekByID(weekID)
	if err != nil {
		return nil, fmt.Errorf("week not found: %w", err)
	}

	games, err := r.ListGamesByWeek(weekID)
	if err != nil {
		return nil, fmt.Errorf("listing games: %w", err)
	}

	teams, err := r.ListTeams()
	if err != nil {
		return nil, fmt.Errorf("listing teams: %w", err)
	}
	teamMap := make(map[int64]*Team)
	for _, t := range teams {
		teamMap[t.ID] = t
	}

	picksA, err := r.GetUserPicksForWeek(userAID, weekID)
	if err != nil {
		return nil, fmt.Errorf("listing user A picks: %w", err)
	}
	picksB, err := r.GetUserPicksForWeek(userBID, weekID)
	if err != nil {
		return nil, fmt.Errorf("listing user B picks: %w", err)
	}

	scoringCfg, _ := r.GetScoringConfig()
	winnerPts := 10
	if scoringCfg != nil && scoringCfg.WinnerPoints > 0 {
		winnerPts = scoringCfg.WinnerPoints
	}

	var matchups []*HeadToHeadMatchup
	agreements := 0
	divergences := 0
	userATotal := 0
	userBTotal := 0
	pointsAtStake := 0

	for _, g := range games {
		pA := picksA[g.ID]
		pB := picksB[g.ID]

		var teamA, teamB *Team
		if pA != nil && pA.PickedTeamID != nil {
			teamA = teamMap[*pA.PickedTeamID]
		}
		if pB != nil && pB.PickedTeamID != nil {
			teamB = teamMap[*pB.PickedTeamID]
		}

		ptsA := 0
		if pA != nil {
			ptsA = pA.PointsEarned + pA.BonusPoints
			userATotal += ptsA
		}
		ptsB := 0
		if pB != nil {
			ptsB = pB.PointsEarned + pB.BonusPoints
			userBTotal += ptsB
		}

		isDivergent := false
		if (pA != nil && pA.PickedTeamID != nil) || (pB != nil && pB.PickedTeamID != nil) {
			if pA == nil || pA.PickedTeamID == nil || pB == nil || pB.PickedTeamID == nil {
				isDivergent = true
			} else if *pA.PickedTeamID != *pB.PickedTeamID {
				isDivergent = true
			}
		}

		if isDivergent {
			divergences++
			if g.Status != "final" {
				pointsAtStake += winnerPts
			}
		} else if (pA != nil && pA.PickedTeamID != nil) && (pB != nil && pB.PickedTeamID != nil) {
			agreements++
		}

		m := &HeadToHeadMatchup{
			Game:            g,
			UserAPick:       pA,
			UserBPick:       pB,
			UserAPickedTeam: teamA,
			UserBPickedTeam: teamB,
			IsDivergent:     isDivergent,
			UserAPoints:     ptsA,
			UserBPoints:     ptsB,
			IsLive:          g.Status == "in_progress",
			IsFinal:         g.Status == "final",
		}
		matchups = append(matchups, m)
	}

	return &HeadToHeadComparison{
		UserA:           userA,
		UserB:           userB,
		Week:            week,
		Matchups:        matchups,
		TotalGames:      len(games),
		AgreementsCount: agreements,
		DivergenceCount: divergences,
		UserATotalPts:   userATotal,
		UserBTotalPts:   userBTotal,
		PointsAtStake:   pointsAtStake,
	}, nil
}

func (r *Repository) ListAllPicksForWeek(weekID int64) ([]*Pick, error) {
	query := `
	SELECT p.id, p.user_id, p.game_id, p.picked_team_id, p.predicted_home_score, p.predicted_away_score, p.points_earned, p.bonus_points, p.is_correct, p.updated_at,
	       u.id, u.username, u.email, u.role, u.avatar_url
	FROM picks p
	JOIN games g ON p.game_id = g.id
	JOIN users u ON p.user_id = u.id
	WHERE g.week_id = ?`

	rows, err := r.db.Query(query, weekID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var picks []*Pick
	for rows.Next() {
		var p Pick
		var u User
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.GameID, &p.PickedTeamID,
			&p.PredictedHomeScore, &p.PredictedAwayScore,
			&p.PointsEarned, &p.BonusPoints, &p.IsCorrect,
			&p.UpdatedAt,
			&u.ID, &u.Username, &u.Email, &u.Role, &u.AvatarURL,
		); err != nil {
			return nil, err
		}
		p.User = &u
		picks = append(picks, &p)
	}
	return picks, nil
}

func (r *Repository) UpdatePickPoints(pickID int64, pointsEarned, bonusPoints int, isCorrect *bool) error {
	query := `UPDATE picks SET points_earned = ?, bonus_points = ?, is_correct = ? WHERE id = ?`
	_, err := r.db.Exec(query, pointsEarned, bonusPoints, isCorrect, pickID)
	return err
}

// ----------------------------------------------------
// Leaderboards
// ----------------------------------------------------

func (r *Repository) UpsertWeeklyLeaderboard(entry *LeaderboardEntry, weekID int64) error {
	query := `
	INSERT INTO weekly_leaderboard (week_id, user_id, total_points, correct_picks, total_picks, tiebreaker_error, rank, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(week_id, user_id) DO UPDATE SET
		total_points = excluded.total_points,
		correct_picks = excluded.correct_picks,
		total_picks = excluded.total_picks,
		tiebreaker_error = excluded.tiebreaker_error,
		rank = excluded.rank,
		updated_at = CURRENT_TIMESTAMP;`

	_, err := r.db.Exec(query, weekID, entry.UserID, entry.TotalPoints, entry.CorrectPicks, entry.TotalPicks, entry.TiebreakerError, entry.Rank)
	return err
}

func (r *Repository) GetWeeklyLeaderboard(weekID int64) ([]*LeaderboardEntry, error) {
	query := `
	SELECT wl.rank, wl.user_id, u.username, u.avatar_url, wl.total_points, wl.correct_picks, wl.total_picks, wl.tiebreaker_error
	FROM weekly_leaderboard wl
	JOIN users u ON wl.user_id = u.id
	WHERE wl.week_id = ? AND COALESCE(u.role, 'player') != 'admin'
	ORDER BY wl.total_points DESC, wl.correct_picks DESC, wl.tiebreaker_error ASC, u.username ASC`

	rows, err := r.db.Query(query, weekID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*LeaderboardEntry
	rank := 1
	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(&e.Rank, &e.UserID, &e.Username, &e.AvatarURL, &e.TotalPoints, &e.CorrectPicks, &e.TotalPicks, &e.TiebreakerError); err != nil {
			return nil, err
		}
		e.Rank = rank
		if e.TotalPicks > 0 {
			e.WinPercentage = (float64(e.CorrectPicks) / float64(e.TotalPicks)) * 100.0
		}
		e.HasTiebreaker = e.TiebreakerError >= 0 && e.TiebreakerError < 999
		entries = append(entries, &e)
		rank++
	}
	return entries, nil
}

func (r *Repository) GetSeasonLeaderboard(seasonID int64) ([]*LeaderboardEntry, error) {
	query := `
	SELECT u.id, u.username, u.avatar_url,
	       COALESCE(SUM(wl.total_points), 0) as grand_total_points,
	       COALESCE(SUM(wl.correct_picks), 0) as grand_correct_picks,
	       COALESCE(SUM(wl.total_picks), 0) as grand_total_picks,
	       COALESCE(SUM(CASE WHEN wl.tiebreaker_error < 999 AND wl.tiebreaker_error >= 0 THEN wl.tiebreaker_error ELSE 0 END), 0) as grand_tiebreaker_error,
	       COALESCE(SUM(CASE WHEN wl.tiebreaker_error < 999 AND wl.tiebreaker_error >= 0 THEN 1 ELSE 0 END), 0) as evaluated_tiebreakers
	FROM users u
	LEFT JOIN weekly_leaderboard wl ON u.id = wl.user_id
	LEFT JOIN weeks w ON wl.week_id = w.id AND w.season_id = ?
	WHERE COALESCE(u.role, 'player') != 'admin'
	GROUP BY u.id, u.username, u.avatar_url
	ORDER BY grand_total_points DESC, grand_correct_picks DESC, grand_tiebreaker_error ASC, u.username ASC`

	rows, err := r.db.Query(query, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*LeaderboardEntry
	rank := 1
	for rows.Next() {
		var e LeaderboardEntry
		var evaluatedCount int
		if err := rows.Scan(&e.UserID, &e.Username, &e.AvatarURL, &e.TotalPoints, &e.CorrectPicks, &e.TotalPicks, &e.TiebreakerError, &evaluatedCount); err != nil {
			return nil, err
		}
		e.Rank = rank
		if e.TotalPicks > 0 {
			e.WinPercentage = (float64(e.CorrectPicks) / float64(e.TotalPicks)) * 100.0
		}
		e.HasTiebreaker = evaluatedCount > 0
		entries = append(entries, &e)
		rank++
	}
	return entries, nil
}

// Helpers
func parseTimeSafe(tStr string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04Z",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02",
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, tStr); err == nil {
			return t
		}
	}
	return time.Now()
}

// ----------------------------------------------------
// Notifications & Reminders
// ----------------------------------------------------

func (r *Repository) GetUsersWithPendingPicks(weekID int64) ([]*User, error) {
	query := `
	SELECT u.id, u.username, u.email, u.password_hash, u.role, u.created_at
	FROM users u
	WHERE (
		SELECT COUNT(*) FROM picks p 
		JOIN games g ON p.game_id = g.id 
		WHERE p.user_id = u.id AND g.week_id = ? AND (p.picked_team_id IS NOT NULL OR (p.predicted_home_score IS NOT NULL AND p.predicted_away_score IS NOT NULL))
	) < (
		SELECT COUNT(*) FROM games WHERE week_id = ?
	)
	AND (SELECT COUNT(*) FROM games WHERE week_id = ?) > 0
	AND u.notify_email = 1
	ORDER BY u.username ASC`

	rows, err := r.db.Query(query, weekID, weekID, weekID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		var createdAtStr string
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &createdAtStr); err != nil {
			return nil, err
		}
		u.CreatedAt = parseTimeSafe(createdAtStr)
		users = append(users, &u)
	}
	return users, nil
}

func (r *Repository) HasUserReceivedReminder(userID, weekID int64, reminderType string) (bool, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM notification_logs WHERE user_id = ? AND week_id = ? AND reminder_type = ?`, userID, weekID, reminderType).Scan(&count)
	return count > 0, err
}

func (r *Repository) LogReminderSent(userID, weekID int64, reminderType string) error {
	query := `INSERT INTO notification_logs (user_id, week_id, reminder_type, sent_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`
	_, err := r.db.Exec(query, userID, weekID, reminderType)
	return err
}

func (r *Repository) SetUserEmailVerified(userID int64, verified bool) error {
	v := 0
	if verified {
		v = 1
	}
	query := `UPDATE users SET email_verified = ? WHERE id = ?`
	_, err := r.db.Exec(query, v, userID)
	return err
}

func (r *Repository) SetUserRole(userID int64, role string) error {
	if role != "admin" && role != "player" {
		return fmt.Errorf("invalid role: %s", role)
	}
	query := `UPDATE users SET role = ? WHERE id = ?`
	_, err := r.db.Exec(query, role, userID)
	return err
}

func (r *Repository) GetUserWeeklySummaries(weekID int64) ([]*UserWeeklySummary, error) {
	users, err := r.ListUsers()
	if err != nil {
		return nil, err
	}

	games, err := r.ListGamesByWeek(weekID)
	if err != nil {
		return nil, err
	}
	totalGames := len(games)

	var tiebreakerGameID int64
	for _, g := range games {
		if g.IsTiebreaker {
			tiebreakerGameID = g.ID
			break
		}
	}

	var summaries []*UserWeeklySummary
	for _, u := range users {
		picks, _ := r.GetUserPicksForWeek(u.ID, weekID)
		completed := 0
		hasTb := false
		totalPts := 0
		for _, p := range picks {
			if p.PickedTeamID != nil {
				completed++
			}
			if tiebreakerGameID > 0 && p.GameID == tiebreakerGameID {
				if p.PredictedHomeScore != nil && p.PredictedAwayScore != nil {
					hasTb = true
				}
			}
			totalPts += p.PointsEarned + p.BonusPoints
		}

		summaries = append(summaries, &UserWeeklySummary{
			User:           u,
			CompletedPicks: completed,
			TotalGames:     totalGames,
			HasTiebreaker:  hasTb,
			TotalPoints:    totalPts,
		})
	}

	return summaries, nil
}

func (r *Repository) GetPicksExportDataForWeek(weekID int64) ([]*PickExportRow, error) {
	query := `
	SELECT u.username, u.email, w.week_number,
	       at.code as away_code, ht.code as home_code,
	       COALESCE(pt.code, '') as picked_code,
	       p.predicted_away_score, p.predicted_home_score,
	       g.away_score, g.home_score,
	       COALESCE(p.points_earned, 0), COALESCE(p.bonus_points, 0),
	       g.status, COALESCE(p.updated_at, g.created_at)
	FROM users u
	CROSS JOIN games g
	JOIN weeks w ON g.week_id = w.id
	JOIN teams ht ON g.home_team_id = ht.id
	JOIN teams at ON g.away_team_id = at.id
	LEFT JOIN picks p ON p.user_id = u.id AND p.game_id = g.id
	LEFT JOIN teams pt ON p.picked_team_id = pt.id
	WHERE g.week_id = ?
	ORDER BY u.username ASC, g.kickoff_time ASC, g.id ASC`

	rows, err := r.db.Query(query, weekID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exportRows []*PickExportRow
	for rows.Next() {
		var row PickExportRow
		var updatedAtStr string
		if err := rows.Scan(
			&row.Username, &row.Email, &row.WeekNumber,
			&row.AwayTeamCode, &row.HomeTeamCode,
			&row.PickedTeamCode,
			&row.PredictedAwayScore, &row.PredictedHomeScore,
			&row.ActualAwayScore, &row.ActualHomeScore,
			&row.PointsEarned, &row.BonusPoints,
			&row.GameStatus, &updatedAtStr,
		); err != nil {
			return nil, err
		}
		row.UpdatedAt = parseTimeSafe(updatedAtStr)
		exportRows = append(exportRows, &row)
	}
	return exportRows, nil
}

// GetFeaturedLiveGame returns the primary game to highlight in matchcast (live in_progress, tiebreaker, or upcoming)
func (r *Repository) GetFeaturedLiveGame(weekID int64) (*Game, error) {
	games, err := r.ListGamesByWeek(weekID)
	if err != nil || len(games) == 0 {
		return nil, err
	}
	// 1. Any in_progress game?
	for _, g := range games {
		if g.Status == "in_progress" {
			return g, nil
		}
	}
	// 2. Any tiebreaker game?
	for _, g := range games {
		if g.IsTiebreaker {
			return g, nil
		}
	}
	// 3. First game of week
	return games[0], nil
}

// GetUserRank returns user's rank in the overall standings and the total number of players
func (r *Repository) GetUserRank(userID int64, seasonID int64) (int, int, error) {
	entries, err := r.GetSeasonLeaderboard(seasonID)
	if err != nil {
		return 0, 0, err
	}
	total := len(entries)
	for _, e := range entries {
		if e.UserID == userID {
			return e.Rank, total, nil
		}
	}
	return 0, total, nil
}

// GetUserWeeklyBreakdown returns the historical performance per week for dashboard charts
func (r *Repository) GetUserWeeklyBreakdown(userID int64, seasonID int64) ([]*UserWeeklyPerformance, error) {
	query := `
	SELECT w.id, w.week_number, w.name,
	       COALESCE(wl.total_points, 0),
	       COALESCE(wl.correct_picks, 0),
	       COALESCE(wl.total_picks, 0),
	       COALESCE(wl.rank, 0)
	FROM weeks w
	LEFT JOIN weekly_leaderboard wl ON w.id = wl.week_id AND wl.user_id = ?
	WHERE w.season_id = ?
	ORDER BY w.week_number ASC`

	rows, err := r.db.Query(query, userID, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*UserWeeklyPerformance
	for rows.Next() {
		var p UserWeeklyPerformance
		if err := rows.Scan(&p.WeekID, &p.WeekNumber, &p.WeekName, &p.Points, &p.CorrectPicks, &p.TotalGames, &p.Rank); err != nil {
			return nil, err
		}
		if p.TotalGames > 0 {
			p.AccuracyRate = (float64(p.CorrectPicks) / float64(p.TotalGames)) * 100.0
		}
		result = append(result, &p)
	}
	return result, nil
}

// ----------------------------------------------------
// Picks Matrix
// ----------------------------------------------------

// GetPicksMatrixForWeek compiles all participants and their picks across all games of a week
func (r *Repository) GetPicksMatrixForWeek(weekID int64, currentUserID int64) (*PicksMatrixData, error) {
	week, err := r.GetWeekByID(weekID)
	if err != nil || week == nil {
		return nil, fmt.Errorf("semana no encontrada")
	}

	games, err := r.ListGamesByWeek(weekID)
	if err != nil {
		return nil, err
	}

	scoringCfg, _ := r.GetScoringConfig()
	now := time.Now()

	var firstKickoff *time.Time
	for _, g := range games {
		if !g.KickoffTime.IsZero() {
			if firstKickoff == nil || g.KickoffTime.Before(*firstKickoff) {
				t := g.KickoffTime
				firstKickoff = &t
			}
		}
	}

	effectiveFirstKickoff := firstKickoff
	if week.WeekNumber == 1 && effectiveFirstKickoff != nil && effectiveFirstKickoff.Before(Week1GraceDeadline) && now.Before(Week1GraceDeadline) {
		effectiveFirstKickoff = &Week1GraceDeadline
	}

	isFullWeekLocked := false
	if scoringCfg.LockMode == "full_week" && effectiveFirstKickoff != nil {
		isFullWeekLocked = now.After(*effectiveFirstKickoff) || now.Equal(*effectiveFirstKickoff)
	}

	// Fetch players (excluding admin accounts)
	usersQuery := `
	SELECT id, username, email, COALESCE(avatar_url, ''), role
	FROM users
	WHERE COALESCE(role, 'player') != 'admin'
	ORDER BY username ASC`
	uRows, err := r.db.Query(usersQuery)
	if err != nil {
		return nil, err
	}
	defer uRows.Close()

	var players []*User
	for uRows.Next() {
		var u User
		if err := uRows.Scan(&u.ID, &u.Username, &u.Email, &u.AvatarURL, &u.Role); err != nil {
			return nil, err
		}
		players = append(players, &u)
	}

	// Fetch all picks for games in this week
	picksQuery := `
	SELECT p.id, p.user_id, p.game_id, p.picked_team_id, p.predicted_away_score, p.predicted_home_score,
	       p.points_earned, p.bonus_points, p.is_correct
	FROM picks p
	JOIN games g ON p.game_id = g.id
	WHERE g.week_id = ?`
	pRows, err := r.db.Query(picksQuery, weekID)
	if err != nil {
		return nil, err
	}
	defer pRows.Close()

	picksMap := make(map[int64]map[int64]*Pick)
	for pRows.Next() {
		var p Pick
		if err := pRows.Scan(&p.ID, &p.UserID, &p.GameID, &p.PickedTeamID, &p.PredictedAwayScore, &p.PredictedHomeScore,
			&p.PointsEarned, &p.BonusPoints, &p.IsCorrect); err != nil {
			return nil, err
		}
		if picksMap[p.UserID] == nil {
			picksMap[p.UserID] = make(map[int64]*Pick)
		}
		picksMap[p.UserID][p.GameID] = &p
	}

	var rows []*PicksMatrixRow
	for _, player := range players {
		row := &PicksMatrixRow{
			User:      player,
			IsCurrent: currentUserID > 0 && currentUserID == player.ID,
			Cells:     make([]*PicksMatrixCell, 0, len(games)),
		}

		userPicks := picksMap[player.ID]
		for _, g := range games {
			isGameLocked := g.IsGameOrWeekLocked(now, scoringCfg.LockMode, firstKickoff)
			isRevealed := isGameLocked || isFullWeekLocked || (currentUserID > 0 && currentUserID == player.ID)

			cell := &PicksMatrixCell{
				GameID:       g.ID,
				IsTiebreaker: g.IsTiebreaker,
				IsRevealed:   isRevealed,
			}

			if userPicks != nil {
				if pick, ok := userPicks[g.ID]; ok {
					cell.HasPick = true
					pick.InferWinnerFromScores(g)

					if isRevealed {
						cell.PickedTeamID = pick.PickedTeamID
						if pick.PickedTeamID != nil {
							if *pick.PickedTeamID == g.HomeTeamID && g.HomeTeam != nil {
								cell.PickedTeamCode = g.HomeTeam.Code
								cell.PickedTeamLogo = g.HomeTeam.LogoURL
							} else if *pick.PickedTeamID == g.AwayTeamID && g.AwayTeam != nil {
								cell.PickedTeamCode = g.AwayTeam.Code
								cell.PickedTeamLogo = g.AwayTeam.LogoURL
							}
						}
						if g.IsTiebreaker {
							cell.AwayScore = pick.PredictedAwayScore
							cell.HomeScore = pick.PredictedHomeScore
						}
						cell.IsCorrect = pick.IsCorrect
						row.TotalPoints += pick.PointsEarned + pick.BonusPoints
						if pick.IsCorrect != nil && *pick.IsCorrect {
							row.TotalCorrect++
						}
					}
				}
			}
			row.Cells = append(row.Cells, cell)
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalPoints != rows[j].TotalPoints {
			return rows[i].TotalPoints > rows[j].TotalPoints
		}
		if rows[i].TotalCorrect != rows[j].TotalCorrect {
			return rows[i].TotalCorrect > rows[j].TotalCorrect
		}
		return rows[i].User.Username < rows[j].User.Username
	})

	for idx, rRow := range rows {
		rRow.Rank = idx + 1
	}

	weeks, _ := r.ListWeeks(week.SeasonID)

	return &PicksMatrixData{
		Week:             week,
		Weeks:            weeks,
		Games:            games,
		Rows:             rows,
		TotalPlayers:     len(rows),
		IsFullWeekLocked: isFullWeekLocked,
	}, nil
}

// ----------------------------------------------------
// Gamification & Achievements
// ----------------------------------------------------

// AwardAchievement grants an achievement to a user if not already earned
func (r *Repository) AwardAchievement(userID int64, badgeCode, badgeName, badgeDesc, icon string, weekNumber *int) (bool, error) {
	query := `
	INSERT INTO user_achievements (user_id, badge_code, badge_name, badge_desc, icon, week_number, unlocked_at)
	VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(user_id, badge_code, week_number) DO NOTHING`

	res, err := r.db.Exec(query, userID, badgeCode, badgeName, badgeDesc, icon, weekNumber)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// GetUserAchievements returns all unlocked achievements for a specific user
func (r *Repository) GetUserAchievements(userID int64) ([]*UserAchievement, error) {
	query := `
	SELECT id, user_id, badge_code, badge_name, badge_desc, icon, week_number, unlocked_at
	FROM user_achievements
	WHERE user_id = ?
	ORDER BY unlocked_at DESC, id DESC`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var achievements []*UserAchievement
	for rows.Next() {
		var a UserAchievement
		var unlockedStr string
		if err := rows.Scan(&a.ID, &a.UserID, &a.BadgeCode, &a.BadgeName, &a.BadgeDesc, &a.Icon, &a.WeekNumber, &unlockedStr); err != nil {
			return nil, err
		}
		a.UnlockedAt = parseTimeSafe(unlockedStr)
		achievements = append(achievements, &a)
	}
	return achievements, nil
}

// GetAllUserAchievements returns a map of user_id -> slice of achievements
func (r *Repository) GetAllUserAchievements() (map[int64][]*UserAchievement, error) {
	query := `
	SELECT id, user_id, badge_code, badge_name, badge_desc, icon, week_number, unlocked_at
	FROM user_achievements
	ORDER BY unlocked_at DESC`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64][]*UserAchievement)
	for rows.Next() {
		var a UserAchievement
		var unlockedStr string
		if err := rows.Scan(&a.ID, &a.UserID, &a.BadgeCode, &a.BadgeName, &a.BadgeDesc, &a.Icon, &a.WeekNumber, &unlockedStr); err != nil {
			return nil, err
		}
		a.UnlockedAt = parseTimeSafe(unlockedStr)
		result[a.UserID] = append(result[a.UserID], &a)
	}
	return result, nil
}

// ----------------------------------------------------
// Head-to-Head Season History
// ----------------------------------------------------

// GetHeadToHeadSeasonHistory compiles head-to-head records across all finished weeks of a season
func (r *Repository) GetHeadToHeadSeasonHistory(userAID, userBID, seasonID int64) (*H2HSeasonHistory, error) {
	userA, err := r.GetUserByID(userAID)
	if err != nil || userA == nil {
		return nil, fmt.Errorf("usuario A no encontrado")
	}
	userB, err := r.GetUserByID(userBID)
	if err != nil || userB == nil {
		return nil, fmt.Errorf("usuario B no encontrado")
	}

	query := `
	SELECT w.week_number, w.name,
	       COALESCE(wlA.total_points, 0) as a_pts,
	       COALESCE(wlB.total_points, 0) as b_pts
	FROM weeks w
	LEFT JOIN weekly_leaderboard wlA ON w.id = wlA.week_id AND wlA.user_id = ?
	LEFT JOIN weekly_leaderboard wlB ON w.id = wlB.week_id AND wlB.user_id = ?
	WHERE w.season_id = ?
	  AND (wlA.user_id IS NOT NULL OR wlB.user_id IS NOT NULL)
	  AND (
	      w.status = 'final'
	      OR NOT EXISTS (SELECT 1 FROM games g WHERE g.week_id = w.id AND g.status != 'final')
	  )
	ORDER BY w.week_number ASC`

	rows, err := r.db.Query(query, userAID, userBID, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := &H2HSeasonHistory{
		UserA: userA,
		UserB: userB,
	}

	for rows.Next() {
		var res H2HWeekResult
		if err := rows.Scan(&res.WeekNumber, &res.WeekName, &res.UserAPoints, &res.UserBPoints); err != nil {
			return nil, err
		}
		history.UserATotalPoints += res.UserAPoints
		history.UserBTotalPoints += res.UserBPoints

		if res.UserAPoints > res.UserBPoints {
			res.Winner = "user_a"
			history.UserAWins++
		} else if res.UserBPoints > res.UserAPoints {
			res.Winner = "user_b"
			history.UserBWins++
		} else {
			res.Winner = "tie"
			history.Ties++
		}
		history.WeekResults = append(history.WeekResults, &res)
	}

	if history.UserAWins > history.UserBWins {
		history.LeaderStatus = "a_leads"
	} else if history.UserBWins > history.UserAWins {
		history.LeaderStatus = "b_leads"
	} else {
		history.LeaderStatus = "tied"
	}

	return history, nil
}

