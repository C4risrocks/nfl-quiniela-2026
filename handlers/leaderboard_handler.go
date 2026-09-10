package handlers

import (
	"net/http"
	"strconv"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

type LeaderboardHandler struct {
	repo       *db.Repository
	renderer   *Renderer
	seasonYear int
}

func NewLeaderboardHandler(repo *db.Repository, renderer *Renderer, seasonYear int) *LeaderboardHandler {
	return &LeaderboardHandler{
		repo:       repo,
		renderer:   renderer,
		seasonYear: seasonYear,
	}
}

func (h *LeaderboardHandler) ShowLeaderboard(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	season, err := h.repo.GetActiveSeason(h.seasonYear)
	if err != nil {
		http.Error(w, "Season not found", http.StatusInternalServerError)
		return
	}

	weeks, _ := h.repo.ListWeeks(season.ID)
	viewMode := r.URL.Query().Get("mode")
	if viewMode != "weekly" {
		viewMode = "season"
	}

	selectedWeekNum := 1
	if wStr := r.URL.Query().Get("week"); wStr != "" {
		if wn, err := strconv.Atoi(wStr); err == nil && wn >= 1 && wn <= len(weeks) {
			selectedWeekNum = wn
		}
	}

	var selectedWeek *db.Week
	for _, wk := range weeks {
		if wk.WeekNumber == selectedWeekNum {
			selectedWeek = wk
			break
		}
	}
	if selectedWeek == nil && len(weeks) > 0 {
		selectedWeek = weeks[0]
	}

	var leaderboard []*db.LeaderboardEntry
	if viewMode == "weekly" && selectedWeek != nil {
		leaderboard, _ = h.repo.GetWeeklyLeaderboard(selectedWeek.ID)
		h.enrichWithLiveProjected(selectedWeek.ID, leaderboard)
	} else {
		leaderboard, _ = h.repo.GetSeasonLeaderboard(season.ID)
	}

	scoringCfg, _ := h.repo.GetScoringConfig()

	h.renderer.RenderPage(w, "leaderboard.html", map[string]interface{}{
		"ActiveNav":     "leaderboard",
		"User":          user,
		"Weeks":         weeks,
		"SelectedWeek":  selectedWeek,
		"ViewMode":      viewMode,
		"Leaderboard":   leaderboard,
		"ScoringConfig": scoringCfg,
	})
}

func (h *LeaderboardHandler) LeaderboardTable(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	season, err := h.repo.GetActiveSeason(h.seasonYear)
	if err != nil {
		http.Error(w, "Season not found", http.StatusInternalServerError)
		return
	}

	viewMode := r.URL.Query().Get("mode")
	if viewMode != "weekly" {
		viewMode = "season"
	}

	var selectedWeek *db.Week
	var leaderboard []*db.LeaderboardEntry
	if viewMode == "weekly" {
		weekNum := 1
		if wStr := r.URL.Query().Get("week"); wStr != "" {
			if wn, err := strconv.Atoi(wStr); err == nil {
				weekNum = wn
			}
		}
		selectedWeek, _ = h.repo.GetWeekByNumber(season.ID, weekNum)
		if selectedWeek != nil {
			leaderboard, _ = h.repo.GetWeeklyLeaderboard(selectedWeek.ID)
			h.enrichWithLiveProjected(selectedWeek.ID, leaderboard)
		}
	} else {
		leaderboard, _ = h.repo.GetSeasonLeaderboard(season.ID)
	}

	h.renderer.RenderPartial(w, "leaderboard_table.html", map[string]interface{}{
		"Leaderboard":  leaderboard,
		"CurrentUser":  user,
		"SelectedWeek": selectedWeek,
	})
}

func (h *LeaderboardHandler) enrichWithLiveProjected(weekID int64, entries []*db.LeaderboardEntry) {
	games, err := h.repo.ListGamesByWeek(weekID)
	if err != nil || len(games) == 0 {
		return
	}

	var liveGames []*db.Game
	for _, g := range games {
		if g.Status == "in_progress" {
			liveGames = append(liveGames, g)
		}
	}
	if len(liveGames) == 0 {
		return
	}

	scoringCfg, _ := h.repo.GetScoringConfig()
	winnerPts := 10
	if scoringCfg != nil && scoringCfg.WinnerPoints > 0 {
		winnerPts = scoringCfg.WinnerPoints
	}

	allPicks, err := h.repo.ListAllPicksForWeek(weekID)
	if err != nil {
		return
	}

	userPickMap := make(map[int64]map[int64]*db.Pick)
	for _, p := range allPicks {
		if userPickMap[p.UserID] == nil {
			userPickMap[p.UserID] = make(map[int64]*db.Pick)
		}
		userPickMap[p.UserID][p.GameID] = p
	}

	for _, entry := range entries {
		entry.HasLiveGames = true
		liveBonus := 0
		userPicks := userPickMap[entry.UserID]
		for _, lg := range liveGames {
			p := userPicks[lg.ID]
			if p != nil {
				prov := p.ProvisionalWinnerCorrect(lg)
				if prov != nil && *prov {
					liveBonus += winnerPts
				}
			}
		}
		entry.LiveProjectedPoints = entry.TotalPoints + liveBonus
	}
}
