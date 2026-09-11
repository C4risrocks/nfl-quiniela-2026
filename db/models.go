package db

import (
	"encoding/json"
	"fmt"
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
	if g == nil || g.TotalPicks == 0 {
		return 50
	}
	return int(float64(g.HomePickCount) / float64(g.TotalPicks) * 100.0)
}

func (g *Game) AwayPickPercent() int {
	if g == nil || g.TotalPicks == 0 {
		return 50
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
	Rank                int                `json:"rank"`
	UserID              int64              `json:"user_id"`
	Username            string             `json:"username"`
	AvatarURL           string             `json:"avatar_url"`
	TotalPoints         int                `json:"total_points"`
	LiveProjectedPoints int                `json:"live_projected_points"`
	CorrectPicks        int                `json:"correct_picks"`
	TotalPicks          int                `json:"total_picks"`
	TiebreakerError     int                `json:"tiebreaker_error"`
	WinPercentage       float64            `json:"win_percentage"`
	HasTiebreaker       bool               `json:"has_tiebreaker"`
	HasLiveGames        bool               `json:"has_live_games"`
	Achievements        []*UserAchievement `json:"achievements,omitempty"`
}

func (e *LeaderboardEntry) TopAchievements(limit int) []*UserAchievement {
	if len(e.Achievements) <= limit {
		return e.Achievements
	}
	return e.Achievements[:limit]
}

func (e *LeaderboardEntry) ExtraAchievementsCount(limit int) int {
	if len(e.Achievements) > limit {
		return len(e.Achievements) - limit
	}
	return 0
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
	IsLive          bool  `json:"is_live"`
	IsFinal         bool  `json:"is_final"`
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
type PicksMatrixData struct {
	Week             *Week             `json:"week"`
	Weeks            []*Week           `json:"weeks"`
	Games            []*Game           `json:"games"`
	Rows             []*PicksMatrixRow `json:"rows"`
	TotalPlayers     int               `json:"total_players"`
	IsFullWeekLocked bool              `json:"is_full_week_locked"`
	User             *User             `json:"user"`
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
	UserA            *User            `json:"user_a"`
	UserB            *User            `json:"user_b"`
	UserAWins        int              `json:"user_a_wins"`
	UserBWins        int              `json:"user_b_wins"`
	Ties             int              `json:"ties"`
	UserATotalPoints int              `json:"user_a_total_points"`
	UserBTotalPoints int              `json:"user_b_total_points"`
	WeekResults      []*H2HWeekResult `json:"week_results"`
	LeaderStatus     string           `json:"leader_status"` // "a_leads", "b_leads", or "tied"
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
			Code:        "week_champion",
			Name:        "Campeón de Jornada",
			Description: "Conquistaste el 1er lugar de la tabla semanal en una jornada finalizada.",
			Icon:        "👑",
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
			Icon:        "⚡",
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

// UserAchievementDisplay models a badge slot for the profile showcase
type UserAchievementDisplay struct {
	Definition BadgeDefinition
	IsUnlocked bool
	UnlockedAt *time.Time
	WeekNumber *int
}
