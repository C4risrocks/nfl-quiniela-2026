package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

type LiveHandler struct {
	repo       *db.Repository
	renderer   *Renderer
	seasonYear int
}

func NewLiveHandler(repo *db.Repository, renderer *Renderer, seasonYear int) *LiveHandler {
	return &LiveHandler{
		repo:       repo,
		renderer:   renderer,
		seasonYear: seasonYear,
	}
}

type LiveViewData struct {
	ActiveNav           string
	User                *db.User
	Season              *db.Season
	Weeks               []*db.Week
	SelectedWeek        *db.Week
	Games               []*db.Game
	FeaturedGame        *db.Game
	FeaturedStats       *db.GameCommunityStats
	FeaturedPicks       []*db.Pick
	LiveGamesCount      int
	FinalGamesCount     int
	UpcomingGamesCount  int
	UserConfirmedPoints int
	UserProvisionalPts  int
	CurrentTime         time.Time
	LockMode            string
}

func (h *LiveHandler) buildLiveData(r *http.Request) (*LiveViewData, error) {
	currentUser := auth.GetUserFromContext(r.Context())
	now := time.Now()

	season, err := h.repo.GetActiveSeason(h.seasonYear)
	if err != nil {
		return nil, err
	}

	weeks, err := h.repo.ListWeeks(season.ID)
	if err != nil {
		return nil, err
	}

	// Select requested week or default to first active / scheduled week
	var selectedWeek *db.Week
	weekParam := r.URL.Query().Get("week")
	if weekParam != "" {
		if wNum, err := strconv.Atoi(weekParam); err == nil {
			for _, w := range weeks {
				if w.WeekNumber == wNum {
					selectedWeek = w
					break
				}
			}
		}
	}

	if selectedWeek == nil {
		for _, w := range weeks {
			if w.Status == "active" {
				selectedWeek = w
				break
			}
		}
	}
	if selectedWeek == nil && len(weeks) > 0 {
		selectedWeek = weeks[0]
	}

	scoringCfg, _ := h.repo.GetScoringConfig()
	lockMode := "per_game"
	if scoringCfg != nil {
		lockMode = scoringCfg.LockMode
	}

	var games []*db.Game
	if selectedWeek != nil {
		games, _ = h.repo.ListGamesByWeek(selectedWeek.ID)
	}

	// Load user picks if logged in
	userPicksMap := make(map[int64]*db.Pick)
	if currentUser != nil && selectedWeek != nil {
		picks, _ := h.repo.GetUserPicksForWeek(currentUser.ID, selectedWeek.ID)
		for _, p := range picks {
			userPicksMap[p.GameID] = p
		}
	}

	var liveCount, finalCount, upcomingCount int
	var confirmedPoints, provisionalPoints int

	for _, g := range games {
		if p, ok := userPicksMap[g.ID]; ok {
			g.UserPick = p
		}

		// Community pick counts
		if stats, err := h.repo.GetGameCommunityStats(g.ID); err == nil && stats != nil {
			g.HomePickCount = stats.HomePicksCount
			g.AwayPickCount = stats.AwayPicksCount
			g.TotalPicks = stats.TotalPicks
		}

		switch g.Status {
		case "in_progress":
			liveCount++
			if g.UserPick != nil && g.UserPick.IsProvisionalWinner(g) {
				provisionalPoints++
			}
		case "final":
			finalCount++
			if g.UserPick != nil && g.UserPick.HasCorrectWinner() {
				confirmedPoints += g.UserPick.TotalPoints()
			}
		default:
			upcomingCount++
		}
	}

	// Select featured game for Matchcast
	var featuredGame *db.Game
	// 1. First in_progress game
	for _, g := range games {
		if g.Status == "in_progress" {
			featuredGame = g
			break
		}
	}
	// 2. Otherwise, first tiebreaker game
	if featuredGame == nil {
		for _, g := range games {
			if g.IsTiebreaker {
				featuredGame = g
				break
			}
		}
	}
	// 3. Otherwise, upcoming game closest to current time
	if featuredGame == nil {
		for _, g := range games {
			if g.Status != "final" {
				featuredGame = g
				break
			}
		}
	}
	// 4. Default to first game
	if featuredGame == nil && len(games) > 0 {
		featuredGame = games[0]
	}

	var featuredStats *db.GameCommunityStats
	var featuredPicks []*db.Pick
	if featuredGame != nil {
		featuredStats, _ = h.repo.GetGameCommunityStats(featuredGame.ID)
		featuredPicks, _ = h.repo.ListPicksForGame(featuredGame.ID)
		// Sort picks alphabetically by username
		sort.Slice(featuredPicks, func(i, j int) bool {
			if featuredPicks[i].User == nil || featuredPicks[j].User == nil {
				return false
			}
			return featuredPicks[i].User.Username < featuredPicks[j].User.Username
		})
	}

	return &LiveViewData{
		ActiveNav:           "live",
		User:                currentUser,
		Season:              season,
		Weeks:               weeks,
		SelectedWeek:        selectedWeek,
		Games:               games,
		FeaturedGame:        featuredGame,
		FeaturedStats:       featuredStats,
		FeaturedPicks:       featuredPicks,
		LiveGamesCount:      liveCount,
		FinalGamesCount:     finalCount,
		UpcomingGamesCount:  upcomingCount,
		UserConfirmedPoints: confirmedPoints,
		UserProvisionalPts:  provisionalPoints,
		CurrentTime:         now,
		LockMode:            lockMode,
	}, nil
}

func (h *LiveHandler) ShowLive(w http.ResponseWriter, r *http.Request) {
	data, err := h.buildLiveData(r)
	if err != nil {
		http.Error(w, "Error cargando Game Center en vivo", http.StatusInternalServerError)
		return
	}

	h.renderer.RenderPage(w, "live.html", data)
}

func (h *LiveHandler) LiveContent(w http.ResponseWriter, r *http.Request) {
	data, err := h.buildLiveData(r)
	if err != nil {
		http.Error(w, "Error actualizando Game Center", http.StatusInternalServerError)
		return
	}

	h.renderer.RenderPartial(w, "live_content.html", data)
}
