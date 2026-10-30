package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

type StatsHandler struct {
	repo       *db.Repository
	renderer   *Renderer
	seasonYear int
}

func NewStatsHandler(repo *db.Repository, renderer *Renderer, seasonYear int) *StatsHandler {
	return &StatsHandler{
		repo:       repo,
		renderer:   renderer,
		seasonYear: seasonYear,
	}
}

type StatsDashboardViewData struct {
	SeasonYear           int
	Users                []*db.User
	TargetUser           *db.User
	TargetUserAnalytics  *db.UserAnalytics
	LeaderboardEvolution []*db.LeaderboardTrajectory
	TeamATSTrends        []*db.TeamATSRecord
	ActiveTab            string // "users" or "teams_ats"
}

func (h *StatsHandler) ShowStats(w http.ResponseWriter, r *http.Request) {
	currentUser := auth.GetUserFromContext(r.Context())

	users, _ := h.repo.ListUsers()
	var targetUser *db.User

	// Determine target user to display (from ?user=id, or current user, or first user)
	if uStr := r.URL.Query().Get("user"); uStr != "" {
		if uid, err := strconv.ParseInt(uStr, 10, 64); err == nil {
			targetUser, _ = h.repo.GetUserByID(uid)
		}
	}
	if targetUser == nil && currentUser != nil {
		targetUser = currentUser
	}
	if targetUser == nil && len(users) > 0 {
		targetUser = users[0]
	}

	var targetAnalytics *db.UserAnalytics
	if targetUser != nil {
		targetAnalytics, _ = h.repo.GetUserAnalytics(targetUser.ID, h.seasonYear)
	}

	evolution, _ := h.repo.GetLeaderboardEvolution(h.seasonYear)
	atsTrends, _ := h.repo.GetTeamATSTrends(h.seasonYear)

	activeTab := r.URL.Query().Get("tab")
	if activeTab != "teams_ats" {
		activeTab = "users"
	}

	viewData := &StatsDashboardViewData{
		SeasonYear:           h.seasonYear,
		Users:                users,
		TargetUser:           targetUser,
		TargetUserAnalytics:  targetAnalytics,
		LeaderboardEvolution: evolution,
		TeamATSTrends:        atsTrends,
		ActiveTab:            activeTab,
	}

	h.renderer.RenderPage(w, "stats.html", map[string]interface{}{
		"User":      currentUser,
		"ActiveNav": "stats",
		"Data":      viewData,
	})
}

func (h *StatsHandler) UserStatsPartial(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	userID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	analytics, err := h.repo.GetUserAnalytics(userID, h.seasonYear)
	if err != nil || analytics == nil {
		http.Error(w, "User analytics not found", http.StatusNotFound)
		return
	}

	h.renderer.RenderPartial(w, "stats_user_tab.html", map[string]interface{}{
		"Analytics": analytics,
	})
}

func (h *StatsHandler) MatchupIntelModal(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameId")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	intel, err := h.repo.GetMatchupIntel(gameID)
	if err != nil || intel == nil {
		http.Error(w, fmt.Sprintf("Matchup intel not found: %v", err), http.StatusNotFound)
		return
	}

	h.renderer.RenderPartial(w, "game_matchup_intel_modal.html", map[string]interface{}{
		"Intel": intel,
	})
}
