package db

import (
	"fmt"
	"time"
)

// User represents a player or administrator
type User struct {
	ID                  int64      `json:"id"`
	Username            string     `json:"username"`
	Email               string     `json:"email"`
	PasswordHash        string     `json:"-"`
	Role                string     `json:"role"` // "admin" or "player"
	AvatarURL           string     `json:"avatar_url"`
	FavoriteTeamID      *int64     `json:"favorite_team_id"`
	EmailVerified       bool       `json:"email_verified"`
	VerificationToken   *string    `json:"-"`
	VerificationSentAt  *time.Time `json:"-"`
	ResetToken          *string    `json:"-"`
	ResetTokenExpiresAt *time.Time `json:"-"`
	NotifyEmail         bool       `json:"notify_email"`
	CreatedAt           time.Time  `json:"created_at"`
}

func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}

// UserStats represents aggregated user performance metrics
type UserStats struct {
	TotalPicks   int     `json:"total_picks"`
	CorrectPicks int     `json:"correct_picks"`
	TotalPoints  int     `json:"total_points"`
	AccuracyRate float64 `json:"accuracy_rate"`
	CurrentRank  int     `json:"current_rank"`
}

// SystemSetting stores key-value configuration such as scoring rules
type SystemSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Season represents an NFL season
type Season struct {
	ID       int64  `json:"id"`
	Year     int    `json:"year"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

// Week represents a week in the NFL season (e.g. Week 1, Wildcard, Super Bowl)
type Week struct {
	ID         int64     `json:"id"`
	SeasonID   int64     `json:"season_id"`
	WeekNumber int       `json:"week_number"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`    // "scheduled", "active", "completed"
	LockType   string    `json:"lock_type"` // "per_game" or "week_start"
	CreatedAt  time.Time `json:"created_at"`
}

// Team represents an NFL franchise
type Team struct {
	ID             int64  `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	City           string `json:"city"`
	LogoURL        string `json:"logo_url"`
	PrimaryColor   string `json:"primary_color"`
	SecondaryColor string `json:"secondary_color"`
	Conference     string `json:"conference"`
	Division       string `json:"division"`
}

func (t *Team) FullName() string {
	return t.City + " " + t.Name
}

// Game represents an NFL matchup
type Game struct {
	ID            int64     `json:"id"`
	WeekID        int64     `json:"week_id"`
	WeekNumber    int       `json:"week_number,omitempty"`
	ESPNGameID    string    `json:"espn_game_id"`
	HomeTeamID    int64     `json:"home_team_id"`
	AwayTeamID    int64     `json:"away_team_id"`
	KickoffTime   time.Time `json:"kickoff_time"`
	HomeScore     *int      `json:"home_score"`
	AwayScore     *int      `json:"away_score"`
	Status        string    `json:"status"` // "scheduled", "in_progress", "final"
	StatusDetail  string    `json:"status_detail"`
	IsTiebreaker  bool      `json:"is_tiebreaker"`
	IsLocked      bool      `json:"is_locked"`
	CreatedAt     time.Time `json:"created_at"`

	// Enriched fields for view rendering
	HomeTeam *Team `json:"home_team,omitempty"`
	AwayTeam *Team `json:"away_team,omitempty"`
	UserPick *Pick `json:"user_pick,omitempty"`

	// Aggregated community stats for locked games
	HomePickCount int `json:"home_pick_count,omitempty"`
	AwayPickCount int `json:"away_pick_count,omitempty"`
	TotalPicks    int `json:"total_picks,omitempty"`
}

// Week1GraceDeadline defines the extended deadline for Week 1 picks
// (kickoff of SF vs LAR on Thursday September 10, 2026 at 18:35 UTC-6 / 2026-09-11 00:35 UTC)
var Week1GraceDeadline = time.Date(2026, 9, 11, 0, 35, 0, 0, time.UTC)

// isWeek1GraceGame checks if a game qualifies for the Week 1 grace period extension
func (g *Game) isWeek1GraceGame(now time.Time) bool {
	if !now.Before(Week1GraceDeadline) {
		return false
	}
	if g.WeekNumber == 1 {
		return true
	}
	if g.WeekNumber > 1 {
		return false
	}
	week1Start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return g.KickoffTime.After(week1Start) && g.KickoffTime.Before(Week1GraceDeadline)
}

// IsEffectivelyLocked checks if the game cannot be picked anymore (individual game kickoff lock)
func (g *Game) IsEffectivelyLocked(now time.Time) bool {
	if g.IsLocked {
		return true
	}
	// Week 1 Special Grace Period:
	// Includes today's game and all Week 1 games before Thursday's kickoff.
	// They remain completely open and pickable until Week1GraceDeadline, even if in_progress or final.
	if g.isWeek1GraceGame(now) {
		return false
	}
	if g.Status == "in_progress" || g.Status == "final" {
		return true
	}
	effectiveKickoff := g.KickoffTime
	if g.WeekNumber == 1 && effectiveKickoff.Before(Week1GraceDeadline) {
		effectiveKickoff = Week1GraceDeadline
	}
	return now.After(effectiveKickoff) || now.Equal(effectiveKickoff)
}

// IsGracePeriodActive checks if the game's original kickoff has passed but is still open due to Week 1 extension
func (g *Game) IsGracePeriodActive(now time.Time) bool {
	return g.isWeek1GraceGame(now) && (now.After(g.KickoffTime) || now.Equal(g.KickoffTime))
}

// IsGameOrWeekLocked checks if the game is locked under the given lock mode
func (g *Game) IsGameOrWeekLocked(now time.Time, lockMode string, firstKickoffInWeek *time.Time) bool {
	if g.IsLocked {
		return true
	}

	// Week 1 Special Grace Period:
	// If now is before Thursday's grace deadline, games in Week 1 (including today's game)
	// remain open and pickable regardless of status (in_progress/final) or lockMode.
	if g.isWeek1GraceGame(now) {
		return false
	}

	if g.Status == "in_progress" || g.Status == "final" {
		return true
	}

	effectiveKickoff := g.KickoffTime
	var effectiveFirstKickoff *time.Time
	if firstKickoffInWeek != nil {
		t := *firstKickoffInWeek
		effectiveFirstKickoff = &t
	}

	if g.WeekNumber == 1 && effectiveKickoff.Before(Week1GraceDeadline) {
		effectiveKickoff = Week1GraceDeadline
	}
	if g.WeekNumber == 1 && effectiveFirstKickoff != nil && effectiveFirstKickoff.Before(Week1GraceDeadline) {
		effectiveFirstKickoff = &Week1GraceDeadline
	}

	if lockMode == "full_week" && effectiveFirstKickoff != nil {
		return now.After(*effectiveFirstKickoff) || now.Equal(*effectiveFirstKickoff)
	}
	return now.After(effectiveKickoff) || now.Equal(effectiveKickoff)
}

// WinningTeamID returns pointer to winning team ID if final and not a tie
func (g *Game) WinningTeamID() *int64 {
	if g.Status != "final" || g.HomeScore == nil || g.AwayScore == nil {
		return nil
	}
	if *g.HomeScore > *g.AwayScore {
		id := g.HomeTeamID
		return &id
	}
	if *g.AwayScore > *g.HomeScore {
		id := g.AwayTeamID
		return &id
	}
	return nil // Tie
}

// Pick represents a user's prediction for a game
type Pick struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id"`
	GameID             int64     `json:"game_id"`
	PickedTeamID       *int64    `json:"picked_team_id"`
	PredictedHomeScore *int      `json:"predicted_home_score"`
	PredictedAwayScore *int      `json:"predicted_away_score"`
	PointsEarned       int       `json:"points_earned"`
	BonusPoints        int       `json:"bonus_points"`
	IsCorrect          *bool     `json:"is_correct"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Enriched fields
	User     *User `json:"user,omitempty"`
	Game     *Game `json:"game,omitempty"`
	TeamCode string `json:"team_code,omitempty"`
}

func (p *Pick) TotalPoints() int {
	return p.PointsEarned + p.BonusPoints
}

func (p *Pick) IsPickedTeam(teamID int64) bool {
	if p == nil || p.PickedTeamID == nil {
		return false
	}
	return *p.PickedTeamID == teamID
}

func (p *Pick) HomeScoreStr() string {
	if p == nil || p.PredictedHomeScore == nil {
		return ""
	}
	return fmt.Sprintf("%d", *p.PredictedHomeScore)
}

func (p *Pick) AwayScoreStr() string {
	if p == nil || p.PredictedAwayScore == nil {
		return ""
	}
	return fmt.Sprintf("%d", *p.PredictedAwayScore)
}

func (p *Pick) HasCorrectWinner() bool {
	return p != nil && p.IsCorrect != nil && *p.IsCorrect
}

func (p *Pick) HasIncorrectWinner() bool {
	return p != nil && p.IsCorrect != nil && !*p.IsCorrect
}

// LeaderboardEntry represents a user's standings in a week or season
type LeaderboardEntry struct {
	Rank            int     `json:"rank"`
	UserID          int64   `json:"user_id"`
	Username        string  `json:"username"`
	AvatarURL       string  `json:"avatar_url"`
	TotalPoints     int     `json:"total_points"`
	CorrectPicks    int     `json:"correct_picks"`
	TotalPicks      int     `json:"total_picks"`
	TiebreakerError int     `json:"tiebreaker_error"`
	WinPercentage   float64 `json:"win_percentage"`
	HasTiebreaker   bool    `json:"has_tiebreaker"`
}

// ScoringConfig holds active pool scoring settings and lock timing
type ScoringConfig struct {
	ScoringMode      string // "weighted" or "pure_tiebreaker"
	WinnerPoints     int    // e.g. 10 (weighted) or 1 (pure)
	ExactScoreBonus  int    // e.g. 5
	ExactMarginBonus int    // e.g. 2
	LockMode         string // "per_game" or "full_week"
}

// UserWeeklySummary represents a player's pick completion metrics for an admin view
type UserWeeklySummary struct {
	User           *User `json:"user"`
	CompletedPicks int   `json:"completed_picks"`
	TotalGames     int   `json:"total_games"`
	HasTiebreaker  bool  `json:"has_tiebreaker"`
	TotalPoints    int   `json:"total_points"`
}

// PickExportRow represents a flattened pick record suitable for CSV export
type PickExportRow struct {
	Username           string    `json:"username"`
	Email              string    `json:"email"`
	WeekNumber         int       `json:"week_number"`
	AwayTeamCode       string    `json:"away_team_code"`
	HomeTeamCode       string    `json:"home_team_code"`
	PickedTeamCode     string    `json:"picked_team_code"`
	PredictedAwayScore *int      `json:"predicted_away_score"`
	PredictedHomeScore *int      `json:"predicted_home_score"`
	ActualAwayScore    *int      `json:"actual_away_score"`
	ActualHomeScore    *int      `json:"actual_home_score"`
	PointsEarned       int       `json:"points_earned"`
	BonusPoints        int       `json:"bonus_points"`
	GameStatus         string    `json:"game_status"`
	UpdatedAt          time.Time `json:"updated_at"`
}
