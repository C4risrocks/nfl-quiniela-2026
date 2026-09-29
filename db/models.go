package db

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
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
	NotifyKickoff       bool       `json:"notify_kickoff"`
	NotifyRecap         bool       `json:"notify_recap"`
	IsBetaTester        bool       `json:"is_beta_tester"`
	Bio                 string           `json:"bio"`
	FeaturedBadgeCode   string           `json:"featured_badge_code"`
	FavoriteTeam        *Team            `json:"favorite_team,omitempty"`
	FeaturedBadgeTitle  string           `json:"featured_badge_title,omitempty"`
	FeaturedBadgeIcon   string           `json:"featured_badge_icon,omitempty"`
	FeaturedBadge       *BadgeDefinition `json:"featured_badge,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
}

func (u *User) HasCustomAvatar() bool {
	return u.AvatarURL != ""
}

func (u *User) GetFavoriteTeamID() int64 {
	if u != nil && u.FavoriteTeamID != nil {
		return *u.FavoriteTeamID
	}
	return 0
}

func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}

func (u *User) CanAccessBeta() bool {
	return u != nil && (u.IsAdmin() || u.IsBetaTester)
}

func (u *User) IsBot() bool {
	return u != nil && (u.Username == "ia_quiniela" || u.Role == "bot")
}

// UserStats represents aggregated user performance metrics
type UserStats struct {
	TotalPicks   int     `json:"total_picks"`
	CorrectPicks int     `json:"correct_picks"`
	TotalPoints  int     `json:"total_points"`
	AccuracyRate float64 `json:"accuracy_rate"`
	CurrentRank  int     `json:"current_rank"`
}

// ConferenceStat holds pick stats broken down by NFL Conference
type ConferenceStat struct {
	Conference   string  `json:"conference"` // "AFC", "NFC", "Interconferencia"
	TotalPicks   int     `json:"total_picks"`
	CorrectPicks int     `json:"correct_picks"`
	Accuracy     float64 `json:"accuracy"`
}

// TeamAffinityStat tracks a user's performance when picking a specific team
type TeamAffinityStat struct {
	TeamCode     string  `json:"team_code"`
	TeamName     string  `json:"team_name"`
	LogoURL      string  `json:"logo_url"`
	TotalPicked  int     `json:"total_picked"`
	CorrectCount int     `json:"correct_count"`
	Accuracy     float64 `json:"accuracy"`
}

// AdvancedUserStats provides deeper analytical metrics for a user's quiniela history
type AdvancedUserStats struct {
	UserStats
	AFCStats         ConferenceStat    `json:"afc_stats"`
	NFCStats         ConferenceStat    `json:"nfc_stats"`
	InterconfStats   ConferenceStat    `json:"interconf_stats"`
	TalismanTeam     *TeamAffinityStat `json:"talisman_team"` // Most successful team picked
	NemesisTeam      *TeamAffinityStat `json:"nemesis_team"`  // Most failed team picked
	HomePicksTotal   int               `json:"home_picks_total"`
	HomePicksCorrect int               `json:"home_picks_correct"`
	HomeAccuracy     float64           `json:"home_accuracy"`
	AwayPicksTotal   int               `json:"away_picks_total"`
	AwayPicksCorrect int               `json:"away_picks_correct"`
	AwayAccuracy     float64           `json:"away_accuracy"`
	CurrentStreak    int               `json:"current_streak"`
	MaxStreak        int               `json:"max_streak"`
}

// SystemSetting stores key-value configuration such as scoring rules
type SystemSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FeatureFlag represents an individual feature toggle and rollout configuration
type FeatureFlag struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	AccessLevel string    `json:"access_level"` // "all", "beta", "admin", "disabled"
	IsBeta      bool      `json:"is_beta"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (f *FeatureFlag) IsAccessibleTo(u *User) bool {
	if f == nil {
		return true
	}
	switch f.AccessLevel {
	case "disabled":
		return false
	case "admin":
		return u != nil && u.IsAdmin()
	case "beta":
		return u != nil && (u.IsAdmin() || u.IsBetaTester)
	case "all":
		return true
	default:
		return true
	}
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
	Broadcast     string    `json:"broadcast,omitempty"`
	Situation     string    `json:"situation,omitempty"`
	Linescores    string    `json:"linescores,omitempty"`
	StatsJSON     string    `json:"stats_json,omitempty"`
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

	// AI forecast and odds consensus
	Forecast *GameForecast `json:"forecast,omitempty"`
}

// LinescoreMatrix represents quarters breakdown for matchcast
type LinescoreMatrix struct {
	AwayScores []string `json:"away"`
	HomeScores []string `json:"home"`
}

func (g *Game) LinescoreData() *LinescoreMatrix {
	if g == nil || g.Linescores == "" {
		return nil
	}
	var matrix LinescoreMatrix
	if err := json.Unmarshal([]byte(g.Linescores), &matrix); err != nil {
		return nil
	}
	return &matrix
}

func (g *Game) HomePickPct() int {
	if g == nil || g.TotalPicks == 0 {
		return 50
	}
	return int(math.Round(float64(g.HomePickCount) / float64(g.TotalPicks) * 100))
}

func (g *Game) AwayPickPct() int {
	if g == nil || g.TotalPicks == 0 {
		return 50
	}
	return 100 - g.HomePickPct()
}

func (g *Game) CommunityFavoriteTeamID() int64 {
	if g == nil {
		return 0
	}
	if g.HomePickCount > g.AwayPickCount {
		return g.HomeTeamID
	}
	if g.AwayPickCount > g.HomePickCount {
		return g.AwayTeamID
	}
	return 0
}

// GameSituation represents live on-field drive status and situation
type GameSituation struct {
	DownDistanceText string `json:"down_distance"`
	LastPlay         string `json:"last_play"`
	PossessionCode   string `json:"possession_code"`
	IsRedZone        bool   `json:"is_red_zone"`
}

func (g *Game) SituationData() *GameSituation {
	if g == nil || g.Situation == "" {
		return nil
	}
	trimmed := strings.TrimSpace(g.Situation)
	if strings.HasPrefix(trimmed, "{") {
		var sit GameSituation
		if err := json.Unmarshal([]byte(trimmed), &sit); err == nil {
			return &sit
		}
	}
	return &GameSituation{
		DownDistanceText: g.Situation,
	}
}

func (g *Game) DownDistance() string {
	sit := g.SituationData()
	if sit != nil {
		return sit.DownDistanceText
	}
	return ""
}

func (g *Game) LastPlay() string {
	sit := g.SituationData()
	if sit != nil {
		return sit.LastPlay
	}
	return ""
}

func (g *Game) IsRedZone() bool {
	sit := g.SituationData()
	return sit != nil && sit.IsRedZone
}

func (g *Game) IsAwayPossession() bool {
	sit := g.SituationData()
	if sit != nil && sit.PossessionCode != "" && g.AwayTeam != nil {
		return strings.EqualFold(sit.PossessionCode, g.AwayTeam.Code)
	}
	return false
}

func (g *Game) IsHomePossession() bool {
	sit := g.SituationData()
	if sit != nil && sit.PossessionCode != "" && g.HomeTeam != nil {
		return strings.EqualFold(sit.PossessionCode, g.HomeTeam.Code)
	}
	return false
}

// DetailedSummary deserializes the cached stats_json if available and valid
func (g *Game) DetailedSummary() *GameDetailedSummary {
	if g == nil || strings.TrimSpace(g.StatsJSON) == "" {
		return nil
	}
	var s GameDetailedSummary
	if err := json.Unmarshal([]byte(g.StatsJSON), &s); err != nil {
		return nil
	}
	return &s
}

// TeamBoxscoreStats holds head-to-head match stats
type TeamBoxscoreStats struct {
	TeamCode        string `json:"team_code"`
	TeamName        string `json:"team_name"`
	TeamLogoURL     string `json:"team_logo_url"`
	FirstDowns      string `json:"first_downs"`
	ThirdDownEff    string `json:"third_down_eff"`
	FourthDownEff   string `json:"fourth_down_eff"`
	TotalPlays      string `json:"total_plays"`
	TotalYards      string `json:"total_yards"`
	YardsPerPlay    string `json:"yards_per_play"`
	PassingYards    string `json:"passing_yards"`
	CompAtt         string `json:"comp_att"`
	RushingYards    string `json:"rushing_yards"`
	RushingAttempts string `json:"rushing_attempts"`
	Turnovers       string `json:"turnovers"`
	Penalties       string `json:"penalties"`
	PossessionTime  string `json:"possession_time"`
}

// ScoringPlayItem represents a single score event in the game
type ScoringPlayItem struct {
	Quarter     int    `json:"quarter"`
	Clock       string `json:"clock"`
	Text        string `json:"text"`
	AwayScore   int    `json:"away_score"`
	HomeScore   int    `json:"home_score"`
	TeamCode    string `json:"team_code"`
	TeamLogoURL string `json:"team_logo_url"`
}

// DrivePlayItem represents a single play inside an offensive drive
type DrivePlayItem struct {
	PlayID           string `json:"play_id,omitempty"`
	Quarter          int    `json:"quarter"`
	Clock            string `json:"clock"`
	Text             string `json:"text"`
	Type             string `json:"type"`
	StatYardage      int    `json:"stat_yardage"`
	DownDistanceText string `json:"down_distance,omitempty"`
	IsScoringPlay    bool   `json:"is_scoring_play,omitempty"`
}

// DriveItem represents an offensive series (possession)
type DriveItem struct {
	ID            string          `json:"id"`
	TeamCode      string          `json:"team_code"`
	TeamName      string          `json:"team_name"`
	TeamLogoURL   string          `json:"team_logo_url"`
	Description   string          `json:"description"` // e.g. "8 jugadas, 65 yds, 3:42"
	PlaysCount    int             `json:"plays_count"`
	Yards         int             `json:"yards"`
	TimeElapsed   string          `json:"time_elapsed"`
	StartPeriod   int             `json:"start_period"`
	StartClock    string          `json:"start_clock"`
	StartField    string          `json:"start_field"`
	EndField      string          `json:"end_field"`
	Result        string          `json:"result"` // TD, FG, PUNT, INT, FUMBLE, DOWNS, MISSED FG
	DisplayResult string          `json:"display_result"`
	IsScore       bool            `json:"is_score"`
	IsCurrent     bool            `json:"is_current"`
	Plays         []DrivePlayItem `json:"plays,omitempty"`
}

// ResultBadgeClass returns Tailwind badge styling depending on drive outcome
func (d DriveItem) ResultBadgeClass() string {
	switch d.Result {
	case "TD":
		return "bg-emerald-500/20 text-emerald-300 border-emerald-500/40"
	case "FG":
		return "bg-cyan-500/20 text-cyan-300 border-cyan-500/40"
	case "PUNT":
		return "bg-zinc-800 text-zinc-300 border-zinc-700"
	case "INT", "FUMBLE":
		return "bg-rose-500/20 text-rose-300 border-rose-500/40"
	case "DOWNS":
		return "bg-amber-500/20 text-amber-300 border-amber-500/40"
	case "MISSED FG":
		return "bg-orange-500/20 text-orange-300 border-orange-500/40"
	default:
		if d.IsScore {
			return "bg-emerald-500/20 text-emerald-300 border-emerald-500/40"
		}
		return "bg-zinc-800 text-zinc-400 border-zinc-700"
	}
}

// PlayerStatEntry represents a single player's boxscore stat line
type PlayerStatEntry struct {
	Name        string   `json:"name"`
	Jersey      string   `json:"jersey"`
	Position    string   `json:"position"`
	HeadshotURL string   `json:"headshot_url"`
	Stats       []string `json:"stats"`
}

// PlayerStatCategory groups athletes for a stat category (passing, rushing, receiving, defensive)
type PlayerStatCategory struct {
	Name    string            `json:"name"`  // e.g. "passing", "rushing", "receiving", "defensive"
	Title   string            `json:"title"` // e.g. "Pase", "Acarreo", "Recepción", "Defensa"
	Labels  []string          `json:"labels"`
	Players []PlayerStatEntry `json:"players"`
}

// TeamPlayerStats holds player stat categories for one team
type TeamPlayerStats struct {
	TeamCode   string               `json:"team_code"`
	TeamName   string               `json:"team_name"`
	TeamLogo   string               `json:"team_logo"`
	Categories []PlayerStatCategory `json:"categories"`
}

// WinProbabilityPoint represents a discrete win probability snapshot during a game
type WinProbabilityPoint struct {
	PlayID            string  `json:"play_id"`
	HomeWinPercentage float64 `json:"home_win_percentage"` // 0.0 to 1.0
	AwayWinPercentage float64 `json:"away_win_percentage"` // 0.0 to 1.0
	TiePercentage     float64 `json:"tie_percentage"`
	Quarter           int     `json:"quarter,omitempty"`
	Clock             string  `json:"clock,omitempty"`
	Text              string  `json:"text,omitempty"`
	HomeScore         int     `json:"home_score,omitempty"`
	AwayScore         int     `json:"away_score,omitempty"`
	SwingDelta        float64 `json:"swing_delta,omitempty"` // Change in home probability from previous play
}

// SwingDeltaPct returns absolute swing percentage as integer (e.g. 18 for +18%)
func (w WinProbabilityPoint) SwingDeltaPct() int {
	return int(math.Round(math.Abs(w.SwingDelta) * 100))
}

// GameLeaderItem represents a top individual performer in a key stat category
type GameLeaderItem struct {
	Category    string `json:"category"`     // "Pase", "Acarreo", "Recepción"
	PlayerName  string `json:"player_name"`
	TeamCode    string `json:"team_code"`
	TeamLogoURL string `json:"team_logo_url"`
	Jersey      string `json:"jersey"`
	Position    string `json:"position"`
	HeadshotURL string `json:"headshot_url"`
	DisplayStat string `json:"display_stat"` // e.g. "24/32, 285 YDS, 3 TD"
	Value       string `json:"value"`        // e.g. "285 YDS"
}

// GameVenueInfo represents stadium venue, weather, and officiating details
type GameVenueInfo struct {
	VenueName        string `json:"venue_name"`
	City             string `json:"city"`
	State            string `json:"state"`
	Surface          string `json:"surface"`           // Pasto Natural / Artificial
	Attendance       int    `json:"attendance"`
	WeatherTemp      string `json:"weather_temp"`      // e.g. "72°F"
	WeatherCondition string `json:"weather_condition"` // e.g. "Despejado"
	WeatherWind      string `json:"weather_wind"`
	Referee          string `json:"referee"`
}

func (v *GameVenueInfo) Location() string {
	if v == nil {
		return ""
	}
	if v.City != "" && v.State != "" {
		return fmt.Sprintf("%s, %s", v.City, v.State)
	}
	if v.City != "" {
		return v.City
	}
	return v.VenueName
}

func (v *GameVenueInfo) FormattedAttendance() string {
	if v == nil || v.Attendance <= 0 {
		return ""
	}
	str := fmt.Sprintf("%d", v.Attendance)
	n := len(str)
	if n <= 3 {
		return str
	}
	var res strings.Builder
	rem := n % 3
	if rem > 0 {
		res.WriteString(str[:rem])
	}
	for i := rem; i < n; i += 3 {
		if res.Len() > 0 {
			res.WriteString(",")
		}
		res.WriteString(str[i : i+3])
	}
	return res.String()
}

// GameDetailedSummary combines boxscore team statistics, player statistics, scoring plays and offensive drives
type GameDetailedSummary struct {
	AwayStats         *TeamBoxscoreStats    `json:"away_stats,omitempty"`
	HomeStats         *TeamBoxscoreStats    `json:"home_stats,omitempty"`
	AwayPlayerStats   *TeamPlayerStats      `json:"away_player_stats,omitempty"`
	HomePlayerStats   *TeamPlayerStats      `json:"home_player_stats,omitempty"`
	ScoringPlays      []ScoringPlayItem     `json:"scoring_plays,omitempty"`
	Drives            []DriveItem           `json:"drives,omitempty"`
	StatusDetail      string                `json:"status_detail,omitempty"`
	GameStatus        string                `json:"game_status,omitempty"`
	AwayScore         *int                  `json:"away_score,omitempty"`
	HomeScore         *int                  `json:"home_score,omitempty"`
	Linescores        string                `json:"linescores,omitempty"`
	WinProbability    []WinProbabilityPoint `json:"win_probability,omitempty"`
	CurrentHomeWinPct int                   `json:"current_home_win_pct"`
	CurrentAwayWinPct int                   `json:"current_away_win_pct"`
	Leaders           []GameLeaderItem      `json:"leaders,omitempty"`
	VenueInfo         *GameVenueInfo        `json:"venue_info,omitempty"`
	HasStats          bool                  `json:"has_stats"`
	HasPlayerStats    bool                  `json:"has_player_stats"`
	HasDrives         bool                  `json:"has_drives"`
	HasWinProb        bool                  `json:"has_win_prob"`
	HasLeaders        bool                  `json:"has_leaders"`
}

func (s *GameDetailedSummary) WinProbHome() int {
	if s == nil || !s.HasWinProb {
		return 50
	}
	if s.CurrentHomeWinPct > 0 || s.CurrentAwayWinPct > 0 {
		return s.CurrentHomeWinPct
	}
	if len(s.WinProbability) > 0 {
		return int(math.Round(s.WinProbability[len(s.WinProbability)-1].HomeWinPercentage * 100))
	}
	return 50
}

func (s *GameDetailedSummary) WinProbAway() int {
	if s == nil || !s.HasWinProb {
		return 50
	}
	if s.CurrentHomeWinPct > 0 || s.CurrentAwayWinPct > 0 {
		return s.CurrentAwayWinPct
	}
	return 100 - s.WinProbHome()
}

// PivotalPlays returns up to limit plays that caused the highest probability shift
func (s *GameDetailedSummary) PivotalPlays(limit int) []WinProbabilityPoint {
	if s == nil || len(s.WinProbability) < 2 {
		return nil
	}
	if limit <= 0 {
		limit = 3
	}

	type indexedPoint struct {
		pt   WinProbabilityPoint
		absD float64
	}

	candidates := make([]indexedPoint, 0, len(s.WinProbability))
	for i := 1; i < len(s.WinProbability); i++ {
		pt := s.WinProbability[i]
		absD := math.Abs(pt.SwingDelta)
		if absD >= 0.05 && pt.Text != "" {
			candidates = append(candidates, indexedPoint{pt: pt, absD: absD})
		}
	}

	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].absD > candidates[i].absD {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	result := make([]WinProbabilityPoint, len(candidates))
	for i, c := range candidates {
		result[i] = c.pt
	}
	return result
}

// WinProbQuarterMarker marks quarter transitions on the SVG chart
type WinProbQuarterMarker struct {
	Label string  `json:"label"` // "Q1", "Q2", "Q3", "Q4", "OT"
	X     float64 `json:"x"`
}

// WinProbInteractivePoint holds data for interactive hover inspection on the chart
type WinProbInteractivePoint struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	HomePct  int     `json:"home_pct"`
	AwayPct  int     `json:"away_pct"`
	Quarter  int     `json:"quarter"`
	Clock    string  `json:"clock"`
	Text     string  `json:"text"`
	Score    string  `json:"score"`
	SwingPct int     `json:"swing_pct"`
}

// WinProbChartData contains precalculated SVG coordinates and JSON for reactive rendering
type WinProbChartData struct {
	Width             float64                   `json:"width"`
	Height            float64                   `json:"height"`
	MidY              float64                   `json:"mid_y"`
	LinePath          string                    `json:"line_path"`
	HomeAreaPath      string                    `json:"home_area_path"`
	QuarterMarkers    []WinProbQuarterMarker   `json:"quarter_markers"`
	InteractivePoints []WinProbInteractivePoint `json:"interactive_points"`
	PointsJSON        string                    `json:"points_json"`
}

// WinProbChart computes SVG coordinates and paths for the win probability line and area graph
func (s *GameDetailedSummary) WinProbChart(w, h float64) *WinProbChartData {
	if s == nil || len(s.WinProbability) == 0 {
		return nil
	}
	if w <= 0 {
		w = 600
	}
	if h <= 0 {
		h = 200
	}

	leftPad := 40.0
	rightPad := 20.0
	topPad := 15.0
	bottomPad := 25.0

	plotW := w - leftPad - rightPad
	plotH := h - topPad - bottomPad
	midY := topPad + (plotH / 2.0)

	n := len(s.WinProbability)
	if n == 1 {
		pt := s.WinProbability[0]
		y := topPad + (1.0-pt.HomeWinPercentage)*plotH
		line := fmt.Sprintf("M %.1f %.1f L %.1f %.1f", leftPad, y, leftPad+plotW, y)
		return &WinProbChartData{
			Width:      w,
			Height:     h,
			MidY:       midY,
			LinePath:   line,
			PointsJSON: "[]",
		}
	}

	type ptCoords struct {
		x, y float64
	}
	coords := make([]ptCoords, n)
	quarterSeen := make(map[int]bool)
	var markers []WinProbQuarterMarker
	var interPoints []WinProbInteractivePoint

	var lineSb strings.Builder
	for i, pt := range s.WinProbability {
		x := leftPad + (float64(i)/float64(n-1))*plotW
		homeProb := pt.HomeWinPercentage
		if homeProb < 0 {
			homeProb = 0
		}
		if homeProb > 1 {
			homeProb = 1
		}
		y := topPad + (1.0-homeProb)*plotH
		coords[i] = ptCoords{x: x, y: y}

		if i == 0 {
			lineSb.WriteString(fmt.Sprintf("M %.1f %.1f", x, y))
		} else {
			lineSb.WriteString(fmt.Sprintf(" L %.1f %.1f", x, y))
		}

		if pt.Quarter > 0 && !quarterSeen[pt.Quarter] {
			quarterSeen[pt.Quarter] = true
			lbl := fmt.Sprintf("Q%d", pt.Quarter)
			if pt.Quarter > 4 {
				lbl = "OT"
			}
			markers = append(markers, WinProbQuarterMarker{
				Label: lbl,
				X:     math.Round(x*10) / 10,
			})
		}

		interPoints = append(interPoints, WinProbInteractivePoint{
			X:        math.Round(x*10) / 10,
			Y:        math.Round(y*10) / 10,
			HomePct:  int(math.Round(pt.HomeWinPercentage * 100)),
			AwayPct:  int(math.Round(pt.AwayWinPercentage * 100)),
			Quarter:  pt.Quarter,
			Clock:    pt.Clock,
			Text:     pt.Text,
			Score:    fmt.Sprintf("%d - %d", pt.AwayScore, pt.HomeScore),
			SwingPct: pt.SwingDeltaPct(),
		})
	}

	var homeAreaSb strings.Builder
	homeAreaSb.WriteString(fmt.Sprintf("M %.1f %.1f", coords[0].x, midY))
	for _, c := range coords {
		homeAreaSb.WriteString(fmt.Sprintf(" L %.1f %.1f", c.x, c.y))
	}
	homeAreaSb.WriteString(fmt.Sprintf(" L %.1f %.1f Z", coords[n-1].x, midY))

	pointsJSON := "[]"
	if b, err := json.Marshal(interPoints); err == nil {
		pointsJSON = string(b)
	}

	return &WinProbChartData{
		Width:             w,
		Height:            h,
		MidY:              midY,
		LinePath:          lineSb.String(),
		HomeAreaPath:      homeAreaSb.String(),
		QuarterMarkers:    markers,
		InteractivePoints: interPoints,
		PointsJSON:        pointsJSON,
	}
}


func (g *Game) HomeScoreVal() int {
	if g != nil && g.HomeScore != nil {
		return *g.HomeScore
	}
	return 0
}

func (g *Game) AwayScoreVal() int {
	if g != nil && g.AwayScore != nil {
		return *g.AwayScore
	}
	return 0
}

func (g *Game) IsHomeLeading() bool {
	if g != nil && g.HomeScore != nil && g.AwayScore != nil {
		return *g.HomeScore > *g.AwayScore
	}
	return false
}

func (g *Game) IsAwayLeading() bool {
	if g != nil && g.HomeScore != nil && g.AwayScore != nil {
		return *g.AwayScore > *g.HomeScore
	}
	return false
}

func (g *Game) HomePickPercent() int {
	if g == nil {
		return 0
	}
	valid := g.HomePickCount + g.AwayPickCount
	if valid == 0 {
		return 0
	}
	return int(math.Round(float64(g.HomePickCount) / float64(valid) * 100.0))
}

func (g *Game) AwayPickPercent() int {
	if g == nil {
		return 0
	}
	valid := g.HomePickCount + g.AwayPickCount
	if valid == 0 {
		return 0
	}
	return 100 - g.HomePickPercent()
}

func (g *Game) FormattedKickoff() string {
	if g == nil || g.KickoffTime.IsZero() {
		return "--"
	}
	t := g.KickoffTime
	loc, err := time.LoadLocation("America/Mexico_City")
	if err == nil {
		t = t.In(loc)
	} else {
		t = t.In(time.FixedZone("CST", -6*3600))
	}
	spanishDays := map[string]string{
		"Mon": "Lun", "Tue": "Mar", "Wed": "Mié", "Thu": "Jue", "Fri": "Vie", "Sat": "Sáb", "Sun": "Dom",
	}
	spanishMonths := map[string]string{
		"Jan": "Ene", "Feb": "Feb", "Mar": "Mar", "Apr": "Abr", "May": "May", "Jun": "Jun",
		"Jul": "Jul", "Aug": "Ago", "Sep": "Sep", "Oct": "Oct", "Nov": "Nov", "Dec": "Dic",
	}
	dayAbbr := spanishDays[t.Format("Mon")]
	monthAbbr := spanishMonths[t.Format("Jan")]
	dayNum := t.Format("2")
	hourMin := t.Format("3:04 PM")
	return fmt.Sprintf("%s, %s %s - %s", dayAbbr, dayNum, monthAbbr, hourMin)
}

// BroadcastOption represents a viewing option (TV channel or streaming platform) in Mexico
type BroadcastOption struct {
	Name     string `json:"name"`
	Type     string `json:"type"`      // "tv" (TV Abierta/Paga) or "streaming" (Plataforma Digital)
	BadgeCSS string `json:"badge_css"` // Tailwind badge classes
	IconCSS  string `json:"icon_css"`  // FontAwesome icon class
	WatchURL string `json:"watch_url"` // Direct official streaming/viewing URL
}

var knownBroadcastMap = map[string]BroadcastOption{
	"espn": {
		Name:     "ESPN",
		Type:     "tv",
		BadgeCSS: "bg-red-600/15 text-red-400 border-red-500/30 hover:bg-red-600/25 hover:border-red-500/50",
		IconCSS:  "fa-solid fa-tv",
		WatchURL: "https://www.espn.com.mx/watch/",
	},
	"disney+": {
		Name:     "Disney+",
		Type:     "streaming",
		BadgeCSS: "bg-indigo-600/15 text-indigo-300 border-indigo-500/30 hover:bg-indigo-600/25 hover:border-indigo-500/50",
		IconCSS:  "fa-solid fa-play",
		WatchURL: "https://www.disneyplus.com/es-419/brand/espn",
	},
	"fox sports": {
		Name:     "Fox Sports",
		Type:     "tv",
		BadgeCSS: "bg-blue-600/15 text-blue-300 border-blue-500/30 hover:bg-blue-600/25 hover:border-blue-500/50",
		IconCSS:  "fa-solid fa-tv",
		WatchURL: "https://www.foxsports.com.mx/en-vivo/",
	},
	"fox sports premium": {
		Name:     "Fox Sports Premium",
		Type:     "streaming",
		BadgeCSS: "bg-cyan-600/15 text-cyan-300 border-cyan-500/30 hover:bg-cyan-600/25 hover:border-cyan-500/50",
		IconCSS:  "fa-solid fa-play",
		WatchURL: "https://www.foxsports.com.mx/fox-sports-premium/",
	},
	"prime video": {
		Name:     "Prime Video",
		Type:     "streaming",
		BadgeCSS: "bg-sky-500/15 text-sky-300 border-sky-500/30 hover:bg-sky-500/25 hover:border-sky-500/50",
		IconCSS:  "fa-brands fa-amazon",
		WatchURL: "https://www.primevideo.com/storefront/sports",
	},
	"netflix": {
		Name:     "Netflix",
		Type:     "streaming",
		BadgeCSS: "bg-rose-600/20 text-rose-300 border-rose-500/40 hover:bg-rose-600/30 hover:border-rose-500/60",
		IconCSS:  "fa-solid fa-play",
		WatchURL: "https://www.netflix.com",
	},
	"canal 5": {
		Name:     "Canal 5",
		Type:     "tv",
		BadgeCSS: "bg-amber-500/15 text-amber-300 border-amber-500/30 hover:bg-amber-500/25 hover:border-amber-500/50",
		IconCSS:  "fa-solid fa-tower-broadcast",
		WatchURL: "https://www.televisa.com/canal5/en-vivo",
	},
	"vix": {
		Name:     "ViX",
		Type:     "streaming",
		BadgeCSS: "bg-orange-500/15 text-orange-300 border-orange-500/30 hover:bg-orange-500/25 hover:border-orange-500/50",
		IconCSS:  "fa-solid fa-play",
		WatchURL: "https://vix.com/es-es/deportes",
	},
	"dazn": {
		Name:     "DAZN (Game Pass)",
		Type:     "streaming",
		BadgeCSS: "bg-zinc-800 text-zinc-300 border-zinc-700 hover:bg-zinc-700 hover:border-zinc-500",
		IconCSS:  "fa-solid fa-play",
		WatchURL: "https://www.dazn.com/es-MX/welcome/nfl",
	},
}

func (g *Game) MexicoBroadcastOptions() []BroadcastOption {
	if g == nil {
		return nil
	}

	var results []BroadcastOption
	seen := make(map[string]bool)

	addOption := func(key string) {
		keyLower := strings.ToLower(strings.TrimSpace(key))
		if opt, ok := knownBroadcastMap[keyLower]; ok {
			if !seen[opt.Name] {
				seen[opt.Name] = true
				results = append(results, opt)
			}
		} else if !seen[key] && key != "" {
			seen[key] = true
			results = append(results, BroadcastOption{
				Name:     key,
				Type:     "tv",
				BadgeCSS: "bg-zinc-800 text-zinc-300 border-zinc-700 hover:bg-zinc-700 hover:border-zinc-500",
				IconCSS:  "fa-solid fa-tv",
				WatchURL: "https://www.nfl.com/scores",
			})
		}
	}

	bRaw := strings.ToLower(strings.TrimSpace(g.Broadcast))

	// 1. Check if specific platforms are explicitly mentioned in g.Broadcast
	hasNetflix := strings.Contains(bRaw, "netflix")
	hasPrime := strings.Contains(bRaw, "prime") || strings.Contains(bRaw, "amazon")
	hasESPN := strings.Contains(bRaw, "espn") || strings.Contains(bRaw, "abc")
	hasDisney := strings.Contains(bRaw, "disney")
	hasFox := strings.Contains(bRaw, "fox")
	hasCanal5 := strings.Contains(bRaw, "canal 5") || strings.Contains(bRaw, "televisa")
	hasViX := strings.Contains(bRaw, "vix")
	hasNBC := strings.Contains(bRaw, "nbc") || strings.Contains(bRaw, "peacock")
	hasCBS := strings.Contains(bRaw, "cbs")
	hasDAZN := strings.Contains(bRaw, "dazn") || strings.Contains(bRaw, "game pass")

	// 2. Kickoff Time context in Mexico City
	loc, err := time.LoadLocation("America/Mexico_City")
	tCDMX := g.KickoffTime
	if err == nil {
		tCDMX = tCDMX.In(loc)
	} else {
		tCDMX = tCDMX.In(time.FixedZone("CST", -6*3600))
	}
	weekday := tCDMX.Weekday() // Sunday=0, Monday=1, Thursday=4, Friday=5, Saturday=6
	hour := tCDMX.Hour()
	month := tCDMX.Month()
	day := tCDMX.Day()

	// Special holiday: Christmas (Dec 25/26) -> Netflix global NFL deal
	isChristmas := month == time.December && (day == 25 || day == 26)

	if hasNetflix || isChristmas {
		addOption("netflix")
		addOption("dazn")
		return results
	}

	// Thursday Night Football
	if hasPrime || (weekday == time.Thursday && hour >= 17) {
		addOption("prime video")
		addOption("fox sports")
		addOption("dazn")
		return results
	}

	// Monday Night Football
	if g.IsTiebreaker || (weekday == time.Monday && hour >= 17) || (hasESPN && weekday == time.Monday) {
		addOption("espn")
		addOption("disney+")
		addOption("canal 5")
		addOption("vix")
		addOption("dazn")
		return results
	}

	// Sunday Night Football (SNF on NBC in US -> ESPN / Disney+ in Mexico)
	if hasNBC || (weekday == time.Sunday && hour >= 18) {
		addOption("espn")
		addOption("disney+")
		addOption("dazn")
		return results
	}

	// If explicit tags were provided (e.g. from admin or ESPN)
	if hasFox {
		addOption("fox sports")
	}
	if hasESPN {
		addOption("espn")
		addOption("disney+")
	}
	if hasDisney {
		addOption("disney+")
	}
	if hasCBS {
		addOption("fox sports")
		addOption("vix")
	}
	if hasCanal5 {
		addOption("canal 5")
	}
	if hasViX {
		addOption("vix")
	}
	if hasDAZN {
		addOption("dazn")
	}

	// If by here we already have matched options, add DAZN and return
	if len(results) > 0 {
		addOption("dazn")
		return results
	}

	// Contextual Fallbacks by Sunday kickoff windows
	if weekday == time.Sunday {
		if hour <= 14 { // Sunday Early Window (11:00 AM / 12:00 PM CDMX)
			addOption("fox sports")
			addOption("canal 5")
			addOption("vix")
			addOption("dazn")
		} else { // Sunday Late Window (15:05 / 15:25 PM CDMX)
			addOption("fox sports")
			addOption("fox sports premium")
			addOption("espn")
			addOption("disney+")
			addOption("dazn")
		}
	} else if weekday == time.Friday || weekday == time.Saturday {
		addOption("espn")
		addOption("disney+")
		addOption("dazn")
	} else {
		// General fallback
		addOption("fox sports")
		addOption("espn")
		addOption("disney+")
		addOption("dazn")
	}

	return results
}

// UserWeeklyPerformance tracks a user's points and rank for a single week
type UserWeeklyPerformance struct {
	WeekID       int64   `json:"week_id"`
	WeekNumber   int     `json:"week_number"`
	WeekName     string  `json:"week_name"`
	Points       int     `json:"points"`
	CorrectPicks int     `json:"correct_picks"`
	TotalGames   int     `json:"total_games"`
	AccuracyRate float64 `json:"accuracy_rate"`
	Rank         int     `json:"rank"`
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

// ProvisionalWinnerCorrect evaluates whether the pick is winning during an in_progress game
func (p *Pick) ProvisionalWinnerCorrect(game *Game) *bool {
	if p == nil || p.PickedTeamID == nil || game == nil || game.Status != "in_progress" || game.HomeScore == nil || game.AwayScore == nil {
		return nil
	}
	if *game.HomeScore == *game.AwayScore {
		return nil // Currently tied
	}
	var currentLeadingTeamID int64
	if *game.HomeScore > *game.AwayScore {
		currentLeadingTeamID = game.HomeTeamID
	} else {
		currentLeadingTeamID = game.AwayTeamID
	}
	isLeading := (*p.PickedTeamID == currentLeadingTeamID)
	return &isLeading
}

func (p *Pick) IsProvisionalWinner(game *Game) bool {
	res := p.ProvisionalWinnerCorrect(game)
	return res != nil && *res
}

func (p *Pick) IsProvisionalLoser(game *Game) bool {
	res := p.ProvisionalWinnerCorrect(game)
	return res != nil && !*res
}

// InferWinnerFromScores sets PickedTeamID based on score comparison if PickedTeamID is nil
func (p *Pick) InferWinnerFromScores(game *Game) {
	if p == nil || game == nil || p.PickedTeamID != nil {
		return
	}
	if p.PredictedHomeScore != nil && p.PredictedAwayScore != nil {
		if *p.PredictedHomeScore > *p.PredictedAwayScore {
			id := game.HomeTeamID
			p.PickedTeamID = &id
		} else if *p.PredictedAwayScore > *p.PredictedHomeScore {
			id := game.AwayTeamID
			p.PickedTeamID = &id
		}
	}
}

// HasMissingTiebreakerScores returns true if the game is a tiebreaker and the user has picked a winner but not both scores
func (p *Pick) HasMissingTiebreakerScores(game *Game) bool {
	if p == nil || game == nil || !game.IsTiebreaker {
		return false
	}
	hasWinner := p.PickedTeamID != nil
	hasScores := p.PredictedHomeScore != nil && p.PredictedAwayScore != nil
	return hasWinner && !hasScores
}

// IsCompleteForGame checks if the pick has all required information for the given game
func (p *Pick) IsCompleteForGame(game *Game) bool {
	if p == nil || game == nil {
		return false
	}
	hasWinner := p.PickedTeamID != nil
	if !hasWinner && p.PredictedHomeScore != nil && p.PredictedAwayScore != nil {
		hasWinner = *p.PredictedHomeScore != *p.PredictedAwayScore
	}
	if !hasWinner {
		return false
	}
	if game.IsTiebreaker {
		return p.PredictedHomeScore != nil && p.PredictedAwayScore != nil
	}
	return true
}

// HasPickCompleted checks if this game has a completed pick
func (g *Game) HasPickCompleted() bool {
	if g == nil || g.UserPick == nil {
		return false
	}
	return g.UserPick.IsCompleteForGame(g)
}


// LeaderboardEntry represents a user's standings in a week or season
type LeaderboardEntry struct {
	Rank                    int                `json:"rank"`
	UserID                  int64              `json:"user_id"`
	Username                string             `json:"username"`
	AvatarURL               string             `json:"avatar_url"`
	FavoriteTeamID          *int64             `json:"favorite_team_id,omitempty"`
	FavoriteTeam            *Team              `json:"favorite_team,omitempty"`
	Bio                     string             `json:"bio,omitempty"`
	FeaturedBadgeCode       string             `json:"featured_badge_code,omitempty"`
	FeaturedBadgeTitle      string             `json:"featured_badge_title,omitempty"`
	FeaturedBadgeIcon       string             `json:"featured_badge_icon,omitempty"`
	FeaturedBadge           *BadgeDefinition   `json:"featured_badge,omitempty"`
	TotalPoints             int                `json:"total_points"`
	LiveProjectedPoints     int                `json:"live_projected_points"`
	CorrectPicks            int                `json:"correct_picks"`
	TotalPicks              int                `json:"total_picks"`
	TiebreakerError         int                `json:"tiebreaker_error"`
	TiebreakerWinnerCorrect bool               `json:"tiebreaker_winner_correct"`
	WinPercentage           float64            `json:"win_percentage"`
	HasTiebreaker           bool               `json:"has_tiebreaker"`
	HasLiveGames            bool               `json:"has_live_games"`
	IsBot                   bool               `json:"is_bot"`
	Achievements            []*UserAchievement `json:"achievements,omitempty"`
}

func (e *LeaderboardEntry) IsAI() bool {
	return e != nil && (e.IsBot || e.Username == "ia_quiniela")
}

func (e *LeaderboardEntry) HasCustomAvatar() bool {
	return e != nil && e.AvatarURL != ""
}

func (e *LeaderboardEntry) HasBio() bool {
	return e != nil && strings.TrimSpace(e.Bio) != ""
}

func (e *LeaderboardEntry) HasFeaturedBadge() bool {
	return e != nil && (e.FeaturedBadge != nil || e.FeaturedBadgeCode != "")
}

// DistinctAchievements returns achievements deduplicated by BadgeCode
func (e *LeaderboardEntry) DistinctAchievements() []*UserAchievement {
	seen := make(map[string]bool)
	var list []*UserAchievement
	for _, a := range e.Achievements {
		if !seen[a.BadgeCode] {
			seen[a.BadgeCode] = true
			list = append(list, a)
		}
	}
	return list
}

func (e *LeaderboardEntry) TopAchievements(limit int) []*UserAchievement {
	distinct := e.DistinctAchievements()
	if len(distinct) <= limit {
		return distinct
	}
	return distinct[:limit]
}

func (e *LeaderboardEntry) ExtraAchievementsCount(limit int) int {
	distinct := e.DistinctAchievements()
	if len(distinct) > limit {
		return len(distinct) - limit
	}
	return 0
}

func (e *LeaderboardEntry) UnlockedAchievementsCount() int {
	return len(e.DistinctAchievements())
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

// GameCommunityStats aggregates community picking patterns for a single game
type GameCommunityStats struct {
	TotalPicks     int      `json:"total_picks"`
	HomePicksCount int      `json:"home_picks_count"`
	AwayPicksCount int      `json:"away_picks_count"`
	HomePct        int      `json:"home_pct"` // 0 - 100 rounded
	AwayPct        int      `json:"away_pct"` // 0 - 100 rounded
	AvgHomeScore   *float64 `json:"avg_home_score"`
	AvgAwayScore   *float64 `json:"avg_away_score"`
}

// HeadToHeadMatchup compares two users' picks on a single game
type HeadToHeadMatchup struct {
	Game            *Game `json:"game"`
	UserAPick       *Pick `json:"user_a_pick"`
	UserBPick       *Pick `json:"user_b_pick"`
	UserAPickedTeam *Team `json:"user_a_picked_team"`
	UserBPickedTeam *Team `json:"user_b_picked_team"`
	IsDivergent     bool  `json:"is_divergent"`
	UserAPoints     int   `json:"user_a_points"`
	UserBPoints     int   `json:"user_b_points"`
	IsLive               bool  `json:"is_live"`
	IsFinal              bool  `json:"is_final"`
	IsMaskedForFairPlay  bool  `json:"is_masked_for_fair_play"`
}

// HeadToHeadComparison aggregates a direct rivalry matchup between two users for a week
type HeadToHeadComparison struct {
	UserA           *User                `json:"user_a"`
	UserB           *User                `json:"user_b"`
	Week            *Week                `json:"week"`
	Matchups        []*HeadToHeadMatchup `json:"matchups"`
	TotalGames      int                  `json:"total_games"`
	AgreementsCount int                  `json:"agreements_count"`
	DivergenceCount int                  `json:"divergence_count"`
	UserATotalPts   int                  `json:"user_a_total_pts"`
	UserBTotalPts   int                  `json:"user_b_total_pts"`
	PointsAtStake   int                  `json:"points_at_stake"`
}

// UserWeekSimulationData represents a participant's picks in a week for the What-If Simulator
type UserWeekSimulationData struct {
	UserID    int64           `json:"user_id"`
	Username  string          `json:"username"`
	AvatarURL string          `json:"avatar_url"`
	Picks     map[int64]int64 `json:"picks"` // game_id -> picked_team_id
}

// PicksMatrixCell represents a single cell in the Picks Matrix
type PicksMatrixCell struct {
	GameID         int64  `json:"game_id"`
	HasPick        bool   `json:"has_pick"`
	IsRevealed     bool   `json:"is_revealed"`
	PickedTeamID   *int64 `json:"picked_team_id"`
	PickedTeamCode string `json:"picked_team_code"`
	PickedTeamLogo string `json:"picked_team_logo"`
	AwayScore      *int   `json:"away_score"`
	HomeScore      *int   `json:"home_score"`
	IsCorrect      *bool  `json:"is_correct"`
	IsTiebreaker   bool   `json:"is_tiebreaker"`
}

func (c *PicksMatrixCell) HasResult() bool {
	return c != nil && c.IsCorrect != nil
}

func (c *PicksMatrixCell) IsWon() bool {
	return c != nil && c.IsCorrect != nil && *c.IsCorrect
}

// PicksMatrixRow represents a player's row in the matrix
type PicksMatrixRow struct {
	User         *User              `json:"user"`
	Cells        []*PicksMatrixCell `json:"cells"`
	TotalCorrect int                `json:"total_correct"`
	TotalPoints  int                `json:"total_points"`
	Rank         int                `json:"rank"`
	IsCurrent    bool               `json:"is_current"`
}

// PicksMatrixData represents the entire matrix view data
// MatrixGameConsensus holds majority pick percentages for a single game in the matrix
type MatrixGameConsensus struct {
	GameID          int64  `json:"game_id"`
	AwayPicks       int    `json:"away_picks"`
	HomePicks       int    `json:"home_picks"`
	TotalPicks      int    `json:"total_picks"`
	AwayPct         int    `json:"away_pct"`
	HomePct         int    `json:"home_pct"`
	LeadingTeamCode string `json:"leading_team_code"`
	LeadingPct      int    `json:"leading_pct"`
}

type PicksMatrixData struct {
	Week             *Week                  `json:"week"`
	Weeks            []*Week                `json:"weeks"`
	Games            []*Game                `json:"games"`
	Rows             []*PicksMatrixRow      `json:"rows"`
	Consensus        []*MatrixGameConsensus `json:"consensus"`
	TotalPlayers     int                    `json:"total_players"`
	IsFullWeekLocked bool                   `json:"is_full_week_locked"`
	User             *User                  `json:"user"`
	CurrentUserPts   int                    `json:"current_user_pts"`
}

// UserAchievement represents an unlocked badge by a player
type UserAchievement struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	BadgeCode  string    `json:"badge_code"`
	BadgeName  string    `json:"badge_name"`
	BadgeDesc  string    `json:"badge_desc"`
	Icon       string    `json:"icon"`
	WeekNumber *int      `json:"week_number"`
	UnlockedAt time.Time `json:"unlocked_at"`
}

// H2HWeekResult represents a single completed week matchup between two players
type H2HWeekResult struct {
	WeekNumber  int    `json:"week_number"`
	WeekName    string `json:"week_name"`
	UserAPoints int    `json:"user_a_points"`
	UserBPoints int    `json:"user_b_points"`
	Winner      string `json:"winner"` // "user_a", "user_b", or "tie"
}

// H2HSeasonHistory aggregates all finished head-to-head weeks in the season
type H2HSeasonHistory struct {
	UserA               *User            `json:"user_a"`
	UserB               *User            `json:"user_b"`
	UserAWins           int              `json:"user_a_wins"`
	UserBWins           int              `json:"user_b_wins"`
	Ties                int              `json:"ties"`
	UserATotalPoints    int              `json:"user_a_total_points"`
	UserBTotalPoints    int              `json:"user_b_total_points"`
	WeekResults         []*H2HWeekResult `json:"week_results"`
	LeaderStatus        string           `json:"leader_status"` // "a_leads", "b_leads", or "tied"
	CurrentStreakWinner string           `json:"current_streak_winner"` // "user_a", "user_b", or ""
	CurrentStreakCount  int              `json:"current_streak_count"`
	MaxMargin           int              `json:"max_margin"`
	MaxMarginWeek       int              `json:"max_margin_week"`
	MaxMarginWinner     string           `json:"max_margin_winner"`
}

// BadgeDefinition represents the catalog definition of an achievement
type BadgeDefinition struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Rarity      string `json:"rarity"` // "legendary", "epic", "rare", "common"
}

// GetAllBadgeDefinitions returns the full catalog of achievements
func GetAllBadgeDefinitions() []BadgeDefinition {
	return []BadgeDefinition{
		{
			Code:        "perfect_week",
			Name:        "Pleno Perfecto",
			Description: "Acertaste el 100% de los partidos en una jornada completa (mín. 10 partidos).",
			Icon:        "🎯",
			Rarity:      "legendary",
		},
		{
			Code:        "comeback_kid",
			Name:        "Remontada Legendaria",
			Description: "Escalaste 4 o más posiciones en la tabla general dentro de una sola jornada.",
			Icon:        "🚀",
			Rarity:      "legendary",
		},
		{
			Code:        "week_champion",
			Name:        "Campeón de Jornada",
			Description: "Conquistaste el 1er lugar de la tabla semanal en una jornada finalizada.",
			Icon:        "👑",
			Rarity:      "epic",
		},
		{
			Code:        "century_club",
			Name:        "Club de los 100",
			Description: "Acumulaste 100 o más puntos totales a lo largo de la temporada.",
			Icon:        "💯",
			Rarity:      "epic",
		},
		{
			Code:        "rival_slayer",
			Name:        "Verdugo de Rivales",
			Description: "Conquistaste 3 o más duelos cara a cara directos en la temporada.",
			Icon:        "🥊",
			Rarity:      "epic",
		},
		{
			Code:        "sniper_mnf",
			Name:        "Francotirador MNF",
			Description: "Acertaste con exactitud la suma total de puntos en el partido de desempate.",
			Icon:        "🎯",
			Rarity:      "epic",
		},
		{
			Code:        "tnf_master",
			Name:        "Maestro del Kickoff",
			Description: "Acertaste el partido inaugural de Thursday Night Football de la jornada.",
			Icon:        "⚡",
			Rarity:      "rare",
		},
		{
			Code:        "hawk_eye",
			Name:        "Ojo de Halcón",
			Description: "Acertaste 10 o más ganadores en una sola jornada.",
			Icon:        "🦅",
			Rarity:      "rare",
		},
		{
			Code:        "underdog_king",
			Name:        "Rey Underdog",
			Description: "Acertaste 2 o más victorias sorpresa (< 35% de selecciones comunitarias).",
			Icon:        "🐺",
			Rarity:      "rare",
		},
		{
			Code:        "fire_streak",
			Name:        "Racha de Fuego",
			Description: "Hilvanaste 5 o más aciertos consecutivos dentro de una misma semana.",
			Icon:        "🔥",
			Rarity:      "rare",
		},
		{
			Code:        "elite_accuracy",
			Name:        "Efectividad Élite",
			Description: "Superaste el 75% de efectividad en pronósticos de una jornada (mín. 12 partidos).",
			Icon:        "📊",
			Rarity:      "rare",
		},
		{
			Code:        "iron_streak",
			Name:        "Veterano de Acero",
			Description: "Completaste puntualmente tus pronósticos durante 4 jornadas consecutivas.",
			Icon:        "🛡️",
			Rarity:      "common",
		},
	}
}

// GetBadgeDefinitionByCode returns the catalog badge definition for a given code, or nil if not found
func GetBadgeDefinitionByCode(code string) *BadgeDefinition {
	if code == "" {
		return nil
	}
	for _, b := range GetAllBadgeDefinitions() {
		if b.Code == code {
			return &b
		}
	}
	return nil
}

// UserAchievementDisplay models a badge slot for the profile showcase
type UserAchievementDisplay struct {
	Definition BadgeDefinition
	IsUnlocked bool
	UnlockedAt *time.Time
	WeekNumber *int
}

// GameForecast represents the AI model projection and odds consensus for a game
type GameForecast struct {
	GameID            int64      `json:"game_id"`
	EloHomeProb       float64    `json:"elo_home_prob"`
	EloAwayProb       float64    `json:"elo_away_prob"`
	EloSpread         float64    `json:"elo_spread"`
	ProjHomeScore     int        `json:"proj_home_score"`
	ProjAwayScore     int        `json:"proj_away_score"`
	PredictedWinnerID int64      `json:"predicted_winner_id"`
	VegasFavoriteID   *int64     `json:"vegas_favorite_id,omitempty"`
	VegasSpread       *float64   `json:"vegas_spread,omitempty"`
	ConsensusLevel    string     `json:"consensus_level"` // "high", "moderate", "upset_alert"
	ESPNAvailable     bool       `json:"espn_available"`
	SourcesSummary    string     `json:"sources_summary"`
	AuditNotes        string     `json:"audit_notes"`
	CalculatedAt      time.Time  `json:"calculated_at"`

	// Relational helpers
	PredictedWinner   *Team      `json:"predicted_winner,omitempty"`
}

func (f *GameForecast) WinProbabilityPct() int {
	if f == nil {
		return 50
	}
	maxProb := f.EloHomeProb
	if f.EloAwayProb > maxProb {
		maxProb = f.EloAwayProb
	}
	return int(math.Round(maxProb * 100))
}

func (f *GameForecast) HomeProbPct() int {
	if f == nil {
		return 50
	}
	return int(math.Round(f.EloHomeProb * 100))
}

func (f *GameForecast) AwayProbPct() int {
	if f == nil {
		return 50
	}
	return int(math.Round(f.EloAwayProb * 100))
}

func (f *GameForecast) BadgeClass() string {
	if f == nil {
		return "bg-zinc-800 text-zinc-300 border-zinc-700"
	}
	switch f.ConsensusLevel {
	case "high":
		return "bg-emerald-500/15 text-emerald-400 border-emerald-500/30"
	case "upset_alert":
		return "bg-amber-500/15 text-amber-300 border-amber-500/30"
	default:
		return "bg-indigo-500/15 text-indigo-300 border-indigo-500/30"
	}
}

func (f *GameForecast) ConsensusLabel() string {
	if f == nil {
		return "Sin pronóstico"
	}
	switch f.ConsensusLevel {
	case "high":
		return "Consenso Fuerte"
	case "upset_alert":
		return "Alerta Sorpresa"
	case "moderate":
		return "Consenso Moderado"
	default:
		if !f.ESPNAvailable {
			return "Modelo Elo Puro"
		}
		return "Consenso IA"
	}
}

// InAppNotification represents an in-app alert for a user
type InAppNotification struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Link      string    `json:"link"`
	Type      string    `json:"type"` // "kickoff_reminder", "weekly_recap", "achievement", "system"
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

func (n *InAppNotification) TimeAgo() string {
	diff := time.Since(n.CreatedAt)
	if diff < time.Minute {
		return "Hace un momento"
	} else if diff < time.Hour {
		mins := int(diff.Minutes())
		if mins == 1 {
			return "Hace 1 minuto"
		}
		return fmt.Sprintf("Hace %d minutos", mins)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "Hace 1 hora"
		}
		return fmt.Sprintf("Hace %d horas", hours)
	} else {
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "Hace 1 día"
		}
		return fmt.Sprintf("Hace %d días", days)
	}
}

func (n *InAppNotification) Icon() string {
	switch n.Type {
	case "kickoff_reminder":
		return "fa-solid fa-clock text-amber-400"
	case "weekly_recap":
		return "fa-solid fa-trophy text-yellow-400"
	case "achievement":
		return "fa-solid fa-medal text-emerald-400"
	default:
		return "fa-solid fa-bell text-blue-400"
	}
}

// PodiumEntry represents a top player in the weekly or season rankings
type PodiumEntry struct {
	Rank            int    `json:"rank"`
	UserID          int64  `json:"user_id"`
	Username        string `json:"username"`
	AvatarURL       string `json:"avatar_url"`
	FavoriteTeamCode string `json:"favorite_team_code"`
	TotalPoints     int    `json:"total_points"`
	CorrectPicks    int    `json:"correct_picks"`
	TotalPicks      int    `json:"total_picks"`
	TiebreakerError int    `json:"tiebreaker_error"`
	IsBot           bool   `json:"is_bot"`
}

// UserRecapStats holds the user's personal summary for a completed week
type UserRecapStats struct {
	WeeklyRank       int `json:"weekly_rank"`
	TotalPoints      int `json:"total_points"`
	CorrectPicks     int `json:"correct_picks"`
	TotalGames       int `json:"total_games"`
	AccuracyPercent  int `json:"accuracy_percent"`
	TiebreakerPoints int `json:"tiebreaker_points"`
	HasTiebreaker    bool `json:"has_tiebreaker"`
	SeasonRank       int `json:"season_rank"`
	SeasonTotalPts   int `json:"season_total_pts"`
}

// WeeklyRecapData holds all aggregated data for the weekly digest
type WeeklyRecapData struct {
	WeekID            int64           `json:"week_id"`
	WeekNumber        int             `json:"week_number"`
	WeekName          string          `json:"week_name"`
	Podium            []*PodiumEntry  `json:"podium"`
	TotalParticipants int             `json:"total_participants"`
	UserRecap         *UserRecapStats `json:"user_recap"`
	NextWeekNumber    int             `json:"next_week_number"`
	NextWeekName      string          `json:"next_week_name"`
	NextWeekKickoff   string          `json:"next_week_kickoff"`
}

// ----------------------------------------------------
// Team Statistics & Standings Models
// ----------------------------------------------------

// TeamStanding holds the complete seasonal statistics for an NFL franchise
type TeamStanding struct {
	TeamID              int64   `json:"team_id"`
	TeamCode            string  `json:"team_code"`
	TeamName            string  `json:"team_name"`
	TeamCity            string  `json:"team_city"`
	LogoURL             string  `json:"logo_url"`
	PrimaryColor        string  `json:"primary_color"`
	SecondaryColor      string  `json:"secondary_color"`
	Conference          string  `json:"conference"` // AFC or NFC
	Division            string  `json:"division"`   // East, North, South, West
	Rank                int     `json:"rank"`       // Division rank (1-4)
	ConferenceSeed      int     `json:"seed"`       // Conference playoff seed (1-16)
	Wins                int     `json:"wins"`
	Losses              int     `json:"losses"`
	Ties                int     `json:"ties"`
	WinPercent          float64 `json:"win_percent"`
	WinPercentFormatted string  `json:"win_percent_formatted"` // e.g. ".800" or "1.000"
	GamesPlayed         int     `json:"games_played"`
	PointsFor           int     `json:"points_for"`
	PointsAgainst       int     `json:"points_against"`
	PointDiff           int     `json:"point_diff"`
	OffensivePPG        float64 `json:"offensive_ppg"`
	DefensivePPG        float64 `json:"defensive_ppg"`
	Streak              string  `json:"streak"`          // e.g. "W3", "L1"
	HomeRecord          string  `json:"home_record"`     // e.g. "2-0"
	AwayRecord          string  `json:"away_record"`     // e.g. "1-1"
	DivisionRecord      string  `json:"division_record"` // e.g. "1-0"
	ConfRecord          string  `json:"conf_record"`     // e.g. "2-1"
	GamesBehind         string  `json:"games_behind"`    // e.g. "-", "1.5"
	IsSuperBowlChampion bool    `json:"is_super_bowl_champion"`
	SuperBowlTitle      string  `json:"super_bowl_title"`

	// Community quiniela metrics (for active season)
	FavoriteFansCount int `json:"favorite_fans_count"`
	QuinielaPickCount int `json:"quiniela_pick_count"`
	QuinielaWinCount  int `json:"quiniela_win_count"`
	QuinielaWinRate   int `json:"quiniela_win_rate"` // e.g. 75%

	// Advanced Analytics & Sabermetrics
	PythagoreanWins   float64        `json:"pythagorean_wins"`
	PythagoreanDiff   float64        `json:"pythagorean_diff"`
	PythagoreanStatus string         `json:"pythagorean_status"` // "overperforming", "underperforming", "balanced"
	OneScoreRecord    string         `json:"one_score_record"`   // e.g. "4-1"
	LastFiveResults   []TeamFormItem `json:"last_five_results"`  // L5 form guide
	PlayoffStatus     string         `json:"playoff_status"`     // "clinched_bye", "clinched_division", "clinched_playoff", "in_hunt", "eliminated", ""
	PlayoffGB         string         `json:"playoff_gb"`         // Games Behind 7th seed

	// ESPN Power Rankings & FPI Playoff Picture
	PowerRank           int     `json:"power_rank"`            // e.g. 1-32
	PowerRankPrev       int     `json:"power_rank_prev"`       // e.g. 4
	PowerRankChange     int     `json:"power_rank_change"`     // e.g. +3
	PowerRankBlurb      string  `json:"power_rank_blurb"`      // Editorial analysis from ESPN
	FPIPlayoffPct       float64 `json:"fpi_playoff_pct"`       // e.g. 0.967
	FPIDivisionPct      float64 `json:"fpi_division_pct"`      // e.g. 0.898
	FPIFirstSeedPct     float64 `json:"fpi_first_seed_pct"`    // e.g. 0.331
	FPIWildCardPct      float64 `json:"fpi_wild_card_pct"`     // e.g. 0.069
	FPIWinProjPct       float64 `json:"fpi_win_proj_pct"`      // Next game win prob e.g. 0.55
	FPIPlayoffWithWin   float64 `json:"fpi_playoff_with_win"`  // e.g. 0.43
	FPIPlayoffWithLoss  float64 `json:"fpi_playoff_with_loss"` // e.g. 0.22
	FPINextOpponentCode string  `json:"fpi_next_opponent_code"`
}

// TeamFormItem represents a single match result in a team's recent form guide
type TeamFormItem struct {
	Result       string `json:"result"` // "W", "L", "T"
	Score        string `json:"score"`  // "28-24"
	OpponentCode string `json:"opponent_code"`
	IsHome       bool   `json:"is_home"`
	WeekNumber   int    `json:"week_number"`
}

func (s *TeamStanding) FullName() string {
	return s.TeamCity + " " + s.TeamName
}

// DivisionStandings groups standings for a 4-team NFL division
type DivisionStandings struct {
	Name       string          `json:"name"`       // e.g. "AFC Este", "NFC Norte"
	Conference string          `json:"conference"` // AFC / NFC
	Division   string          `json:"division"`   // East, North, South, West
	Teams      []*TeamStanding `json:"teams"`
}

// ConferenceStandings groups 16 teams sorted by playoff seed
type ConferenceStandings struct {
	Conference string          `json:"conference"` // AFC / NFC
	Name       string          `json:"name"`       // "American Football Conference"
	Teams      []*TeamStanding `json:"teams"`
}

// PlayoffMatchupMockup represents a projected Wild Card playoff clash
type PlayoffMatchupMockup struct {
	HighSeed *TeamStanding `json:"high_seed"`
	LowSeed  *TeamStanding `json:"low_seed"`
	Label    string        `json:"label"` // e.g. "Duelo #2 vs #7"
}

// ConferencePlayoffPicture models seed hierarchy, bye team, wild card matchups and bubble teams
type ConferencePlayoffPicture struct {
	Conference string                 `json:"conference"`
	Name       string                 `json:"name"`
	ByeTeam    *TeamStanding          `json:"bye_team"`    // Seed #1
	Matchups   []PlayoffMatchupMockup `json:"matchups"`    // 2v7, 3v6, 4v5
	InTheHunt  []*TeamStanding        `json:"in_the_hunt"` // Seeds 8-11
	Eliminated []*TeamStanding        `json:"eliminated"`  // Mathematically eliminated
}

// TeamH2HComparison holds direct comparison data between two NFL franchises
type TeamH2HComparison struct {
	TeamA              *TeamStanding       `json:"team_a"`
	TeamB              *TeamStanding       `json:"team_b"`
	HistoricalMatchups []*TeamScheduleItem `json:"historical_matchups"`
	TeamAWins          int                 `json:"team_a_wins"`
	TeamBWins          int                 `json:"team_b_wins"`
	Ties               int                 `json:"ties"`
	CommunityAdvantage string              `json:"community_advantage"`
	VerdictHeadline    string              `json:"verdict_headline"`
	VerdictDetail      string              `json:"verdict_detail"`
}

// SeasonDashboardSummary provides quick high-level league stats
type SeasonDashboardSummary struct {
	SuperBowlChampion   *TeamStanding `json:"super_bowl_champion"`
	TopRecordTeam       *TeamStanding `json:"top_record_team"`
	TopOffenseTeam      *TeamStanding `json:"top_offense_team"`
	TopDefenseTeam      *TeamStanding `json:"top_defense_team"`
	BestStreakTeam      *TeamStanding `json:"best_streak_team"`
	QuinielaMostPopular *TeamStanding `json:"quiniela_most_popular"`
}

// SeasonStandings holds full standings and groupings for a single season
type SeasonStandings struct {
	Year            int                         `json:"year"`
	IsCurrent       bool                        `json:"is_current"`
	Summary         *SeasonDashboardSummary     `json:"summary"`
	Divisions       []*DivisionStandings        `json:"divisions"`
	Conferences     []*ConferenceStandings      `json:"conferences"`
	League               []*TeamStanding             `json:"league"` // All 32 sorted by record
	PlayoffPictures      []*ConferencePlayoffPicture `json:"playoff_pictures,omitempty"`
	PowerRankings        []*TeamPowerRanking         `json:"power_rankings,omitempty"`
	PlayoffProbabilities []*TeamPlayoffProbability   `json:"playoff_probabilities,omitempty"`
	MatchupImpacts       []*PlayoffMatchupImpact     `json:"matchup_impacts,omitempty"`
	LatestRankingsWeek   int                         `json:"latest_rankings_week,omitempty"`
	LatestFPIWeek        int                         `json:"latest_fpi_week,omitempty"`
}

// TeamPowerRanking represents a single franchise's placement in ESPN's weekly Power Rankings
type TeamPowerRanking struct {
	ID           int64     `json:"id"`
	SeasonYear   int       `json:"season_year"`
	WeekNumber   int       `json:"week_number"`
	TeamID       int64     `json:"team_id"`
	TeamCode     string    `json:"team_code"`
	TeamName     string    `json:"team_name"`
	TeamCity     string    `json:"team_city"`
	LogoURL      string    `json:"logo_url"`
	PrimaryColor string    `json:"primary_color"`
	Conference   string    `json:"conference"`
	Division     string    `json:"division"`
	Rank         int       `json:"rank"`
	PreviousRank int       `json:"previous_rank"`
	RankChange   int       `json:"rank_change"` // PreviousRank - Rank: +3 climbed, -2 dropped, 0 unchanged
	Record       string    `json:"record"`      // e.g. "3-0"
	Analysis     string    `json:"analysis"`    // Editorial text by ESPN analyst
	Author       string    `json:"author"`      // e.g. "Eric Gómez", "Fernando Villa"
	UpdatedAt    time.Time `json:"updated_at"`
}

// TeamPlayoffProbability represents ESPN FPI projections and next-matchup leverage
type TeamPlayoffProbability struct {
	ID                 int64     `json:"id"`
	SeasonYear         int       `json:"season_year"`
	WeekNumber         int       `json:"week_number"`
	TeamID             int64     `json:"team_id"`
	TeamCode           string    `json:"team_code"`
	TeamName           string    `json:"team_name"`
	TeamCity           string    `json:"team_city"`
	LogoURL            string    `json:"logo_url"`
	Conference         string    `json:"conference"` // "AFC" or "NFC"
	Division           string    `json:"division"`
	MakePlayoffsPct    float64   `json:"make_playoffs_pct"`    // 0.0 - 1.0 (e.g. 0.967 = 96.7%)
	ClinchDivisionPct  float64   `json:"clinch_division_pct"`  // 0.0 - 1.0 (e.g. 0.898 = 89.8%)
	ClinchFirstSeedPct float64   `json:"clinch_first_seed_pct"`// 0.0 - 1.0 (e.g. 0.331 = 33.1%)
	WildCardPct        float64   `json:"wild_card_pct"`        // 0.0 - 1.0 (e.g. 0.069 = 6.9%)
	NextOpponentCode   string    `json:"next_opponent_code"`
	NextOpponentLogo   string    `json:"next_opponent_logo"`
	WinProjPct         float64   `json:"win_proj_pct"`         // Probability to win next game
	PlayoffPctWithWin  float64   `json:"playoff_pct_with_win"`
	PlayoffPctWithLoss float64   `json:"playoff_pct_with_loss"`
	PlayoffLeverage    float64   `json:"playoff_leverage"`     // PlayoffPctWithWin - PlayoffPctWithLoss (volatility/stakes)
	IsFavorite         bool      `json:"is_favorite"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// PlayoffMatchupImpact represents one of the weekly games with FPI win probabilities and playoff stakes for both sides
type PlayoffMatchupImpact struct {
	WeekNumber      int       `json:"week_number"`
	GameDateTime    string    `json:"game_datetime"`
	AwayTeamCode    string    `json:"away_team_code"`
	AwayTeamName    string    `json:"away_team_name"`
	AwayLogoURL     string    `json:"away_logo_url"`
	AwayWinProb     float64   `json:"away_win_prob"`
	AwayPlayoffCur  float64   `json:"away_playoff_cur"`
	AwayPlayoffWin  float64   `json:"away_playoff_win"`
	AwayPlayoffLoss float64   `json:"away_playoff_loss"`
	AwayIsFavorite  bool      `json:"away_is_favorite"`
	HomeTeamCode    string    `json:"home_team_code"`
	HomeTeamName    string    `json:"home_team_name"`
	HomeLogoURL     string    `json:"home_logo_url"`
	HomeWinProb     float64   `json:"home_win_prob"`
	HomePlayoffCur  float64   `json:"home_playoff_cur"`
	HomePlayoffWin  float64   `json:"home_playoff_win"`
	HomePlayoffLoss float64   `json:"home_playoff_loss"`
	HomeIsFavorite  bool      `json:"home_is_favorite"`
	StakesLevel     string    `json:"stakes_level"` // "critical", "high", "moderate"
}

// TeamScheduleItem represents a single matchup in a team's schedule
type TeamScheduleItem struct {
	WeekNumber       int       `json:"week_number"`
	KickoffTime      time.Time `json:"kickoff_time"`
	KickoffFormatted string    `json:"kickoff_formatted"`
	OpponentCode     string    `json:"opponent_code"`
	OpponentName     string    `json:"opponent_name"`
	OpponentCity     string    `json:"opponent_city"`
	OpponentLogo     string    `json:"opponent_logo"`
	IsHome           bool      `json:"is_home"`
	HomeScore        *int      `json:"home_score"`
	AwayScore        *int      `json:"away_score"`
	TeamScore        *int      `json:"team_score"`
	OpponentScore    *int      `json:"opponent_score"`
	Result           string    `json:"result"` // "W", "L", "T", "scheduled", "in_progress"
	StatusDetail     string    `json:"status_detail"`
	Broadcast        string    `json:"broadcast"`
}

// TeamCommunityStats holds quiniela player affinity and pick performance
type TeamCommunityStats struct {
	TeamID         int64   `json:"team_id"`
	TeamCode       string  `json:"team_code"`
	FavoriteUsers  []*User `json:"favorite_users"`
	TotalPicksMade int     `json:"total_picks_made"`
	WinningPicks   int     `json:"winning_picks"`
	PickWinRate    int     `json:"pick_win_rate"`
}

// ----------------------------------------------------
// AI Picks Advisor & Risk Matrix Models
// ----------------------------------------------------

type TacticalQuadrant string

const (
	QuadrantAnchor   TacticalQuadrant = "anchor"     // Ancla de Seguridad (High prob, High consensus)
	QuadrantValueGem TacticalQuadrant = "value_gem"  // Gema de Valor +EV (Viable prob, Low/Mid community pick)
	QuadrantUpset    TacticalQuadrant = "upset_alert"// Alerta de Sorpresa (Viable underdog, Heavy favorite bias)
	QuadrantCoinToss TacticalQuadrant = "coin_toss"  // Moneda al Aire (50-50 high variance)
)

type AdvisorMatchupRecommendation struct {
	Game                 *Game               `json:"game"`
	Forecast             *GameForecast       `json:"forecast"`
	CommunityStats       *GameCommunityStats `json:"community_stats"`
	RecommendedWinner    *Team               `json:"recommended_winner"`
	RecommendedHomeScore int                 `json:"recommended_home_score"`
	RecommendedAwayScore int                 `json:"recommended_away_score"`
	WinProbability       int                 `json:"win_probability"` // 0-100%
	OpponentWinProb      int                 `json:"opponent_win_prob"`
	CommunityPickPct     int                 `json:"community_pick_pct"` // % on recommended team
	OpponentPickPct      int                 `json:"opponent_pick_pct"`
	ExpectedValueScore   float64             `json:"expected_value_score"`
	ConfidenceScore      int                 `json:"confidence_score"` // 1-100
	Quadrant             TacticalQuadrant    `json:"quadrant"`
	QuadrantLabel        string              `json:"quadrant_label"`
	QuadrantBadgeClass   string              `json:"quadrant_badge_class"`
	TacticalHeadline     string              `json:"tactical_headline"`
	TacticalReasoning    string              `json:"tactical_reasoning"`
	UserCurrentPickID    *int64              `json:"user_current_pick_id"`
	MatchesUserPick      bool                `json:"matches_user_pick"`
	IsLocked             bool                `json:"is_locked"`
}

type AdvisorStrategyPreset struct {
	ID              string                          `json:"id"` // "conservative", "balanced", "aggressive"
	Name            string                          `json:"name"`
	Icon            string                          `json:"icon"`
	BadgeColor      string                          `json:"badge_color"`
	Description     string                          `json:"description"`
	TargetAudience  string                          `json:"target_audience"`
	ProjectedPoints float64                         `json:"projected_points"`
	DivergenceCount int                             `json:"divergence_count"`
	AverageWinProb  int                             `json:"average_win_prob"`
	RiskLevel       string                          `json:"risk_level"` // "Bajo", "Moderado", "Alto"
	RiskBadgeClass  string                          `json:"risk_badge_class"`
	Recommendations []*AdvisorMatchupRecommendation `json:"recommendations"`
}

type AdvisorWeeklyOverview struct {
	Week             *Week                    `json:"week"`
	Weeks            []*Week                  `json:"weeks"`
	ActivePresetID   string                   `json:"active_preset_id"`
	ActivePreset     *AdvisorStrategyPreset   `json:"active_preset"`
	AllPresets       []*AdvisorStrategyPreset `json:"all_presets"`
	TotalGames       int                      `json:"total_games"`
	OpenGamesCount   int                      `json:"open_games_count"`
	LockedGamesCount int                      `json:"locked_games_count"`
	AnchorsCount     int                      `json:"anchors_count"`
	GemsCount        int                      `json:"gems_count"`
	UpsetsCount      int                      `json:"upsets_count"`
	CoinTossesCount  int                      `json:"coin_tosses_count"`
}



