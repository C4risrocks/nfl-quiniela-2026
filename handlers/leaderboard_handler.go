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

	var leaderboard []*db.LeaderboardEntry
	if viewMode == "weekly" {
		weekNum := 1
		if wStr := r.URL.Query().Get("week"); wStr != "" {
			if wn, err := strconv.Atoi(wStr); err == nil {
				weekNum = wn
			}
		}
		week, _ := h.repo.GetWeekByNumber(season.ID, weekNum)
		if week != nil {
			leaderboard, _ = h.repo.GetWeeklyLeaderboard(week.ID)
		}
	} else {
		leaderboard, _ = h.repo.GetSeasonLeaderboard(season.ID)
	}

	h.renderer.RenderPartial(w, "leaderboard_table.html", map[string]interface{}{
		"Leaderboard": leaderboard,
		"CurrentUser": user,
	})
}
