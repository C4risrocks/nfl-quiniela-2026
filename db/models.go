package db

import (
	"fmt"
	"time"
)

// User represents a player or administrator
type User struct {
	ID             int64     `json:"id"`
	Username       string    `json:"username"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"`
	Role           string    `json:"role"` // "admin" or "player"
	AvatarURL      string    `json:"avatar_url"`
	FavoriteTeamID *int64    `json:"favorite_team_id"`
	CreatedAt      time.Time `json:"created_at"`
}

func (u *User) IsAdmin() bool {
	return u.Role == "admin"
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

// IsGameLocked checks if the game cannot be picked anymore (individual game kickoff lock)
func (g *Game) IsEffectivelyLocked(now time.Time) bool {
	if g.IsLocked {
		return true
	}
	if g.Status == "in_progress" || g.Status == "final" {
		return true
	}
	return now.After(g.KickoffTime) || now.Equal(g.KickoffTime)
}

// IsGameOrWeekLocked checks if the game is locked under the given lock mode
func (g *Game) IsGameOrWeekLocked(now time.Time, lockMode string, firstKickoffInWeek *time.Time) bool {
	if g.IsLocked {
		return true
	}
	if g.Status == "in_progress" || g.Status == "final" {
		return true
	}
	if lockMode == "full_week" && firstKickoffInWeek != nil {
		return now.After(*firstKickoffInWeek) || now.Equal(*firstKickoffInWeek)
	}
	return now.After(g.KickoffTime) || now.Equal(g.KickoffTime)
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
}

// ScoringConfig holds active pool scoring settings and lock timing
type ScoringConfig struct {
	ScoringMode      string // "weighted" or "pure_tiebreaker"
	WinnerPoints     int    // e.g. 10 (weighted) or 1 (pure)
	ExactScoreBonus  int    // e.g. 5
	ExactMarginBonus int    // e.g. 2
	LockMode         string // "per_game" or "full_week"
}
