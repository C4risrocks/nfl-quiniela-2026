package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
	"nfl-quiniela-2026/services/espn"
)

type PicksHandler struct {
	repo       *db.Repository
	renderer   *Renderer
	syncer     *espn.Syncer
	seasonYear int
}

func NewPicksHandler(repo *db.Repository, renderer *Renderer, syncer *espn.Syncer, seasonYear int) *PicksHandler {
	return &PicksHandler{
		repo:       repo,
		renderer:   renderer,
		syncer:     syncer,
		seasonYear: seasonYear,
	}
}

func (h *PicksHandler) ShowPicks(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	season, err := h.repo.GetActiveSeason(h.seasonYear)
	if err != nil {
		http.Error(w, "Season not found", http.StatusInternalServerError)
		return
	}

	weeks, err := h.repo.ListWeeks(season.ID)
	if err != nil || len(weeks) == 0 {
		http.Error(w, "Weeks not found", http.StatusInternalServerError)
		return
	}

	// Determine selected week
	selectedWeekNum := 1
	if weekParam := r.URL.Query().Get("week"); weekParam != "" {
		if wNum, err := strconv.Atoi(weekParam); err == nil && wNum >= 1 && wNum <= len(weeks) {
			selectedWeekNum = wNum
		}
	}

	var selectedWeek *db.Week
	for _, wk := range weeks {
		if wk.WeekNumber == selectedWeekNum {
			selectedWeek = wk
			break
		}
	}
	if selectedWeek == nil {
		selectedWeek = weeks[0]
	}

	games, err := h.repo.ListGamesByWeek(selectedWeek.ID)
	if err != nil {
		http.Error(w, "Error loading games", http.StatusInternalServerError)
		return
	}

	// If no games found for this week, attempt auto-sync from ESPN on demand
	if len(games) == 0 && h.syncer != nil {
		if count, _ := h.syncer.SyncWeek(selectedWeek.WeekNumber); count > 0 {
			games, _ = h.repo.ListGamesByWeek(selectedWeek.ID)
		}
	}

	// Fetch scoring config and calculate earliest kickoff for full-week lock
	scoringCfg, _ := h.repo.GetScoringConfig()
	firstKickoff := findFirstKickoff(games)
	isFullWeekLocked := false
	if scoringCfg.LockMode == "full_week" && firstKickoff != nil {
		now := time.Now()
		isFullWeekLocked = now.After(*firstKickoff) || now.Equal(*firstKickoff)
	}

	// Fetch user's picks
	var userPicks map[int64]*db.Pick
	if user != nil {
		userPicks, _ = h.repo.GetUserPicksForWeek(user.ID, selectedWeek.ID)
	}

	picksCount := 0
	userWeeklyPts := 0
	userCorrectPicks := 0

	for _, g := range games {
		if userPicks != nil {
			if pick, exists := userPicks[g.ID]; exists {
				g.UserPick = pick
				if pick.PickedTeamID != nil {
					picksCount++
				}
				userWeeklyPts += pick.PointsEarned + pick.BonusPoints
				if pick.IsCorrect != nil && *pick.IsCorrect {
					userCorrectPicks++
				}
			}
		}
	}

	h.renderer.RenderPage(w, "picks.html", map[string]interface{}{
		"ActiveNav":        "picks",
		"User":             user,
		"Weeks":            weeks,
		"SelectedWeek":     selectedWeek,
		"Games":            games,
		"PicksCount":       picksCount,
		"UserWeeklyPoints": userWeeklyPts,
		"UserCorrectPicks": userCorrectPicks,
		"CurrentTime":      time.Now(),
		"ScoringConfig":    scoringCfg,
		"LockMode":         scoringCfg.LockMode,
		"FirstKickoff":     firstKickoff,
		"IsFullWeekLocked": isFullWeekLocked,
		"JustSaved":        r.URL.Query().Get("saved") == "1",
	})
}

func (h *PicksHandler) SavePick(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	gameID, err := strconv.ParseInt(r.FormValue("game_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid game_id", http.StatusBadRequest)
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	scoringCfg, _ := h.repo.GetScoringConfig()
	weekGames, _ := h.repo.ListGamesByWeek(game.WeekID)
	firstKickoff := findFirstKickoff(weekGames)

	// Enforce lock check (per-game or full-week)
	if game.IsGameOrWeekLocked(time.Now(), scoringCfg.LockMode, firstKickoff) {
		if scoringCfg.LockMode == "full_week" {
			http.Error(w, "La jornada completa se encuentra bloqueada porque el primer partido ya inició.", http.StatusForbidden)
		} else {
			http.Error(w, "Este partido ya se encuentra bloqueado.", http.StatusForbidden)
		}
		return
	}

	var pickedTeamID *int64
	if teamStr := r.FormValue("picked_team_id"); teamStr != "" {
		if tid, err := strconv.ParseInt(teamStr, 10, 64); err == nil {
			pickedTeamID = &tid
		}
	}

	pick, err := h.repo.SavePick(user.ID, gameID, pickedTeamID, nil, nil)
	if err != nil {
		http.Error(w, "Error saving pick", http.StatusInternalServerError)
		return
	}

	game.UserPick = pick

	w.Header().Set("HX-Trigger", `{"show-toast": {"message": "¡Pronóstico guardado exitosamente!", "type": "success"}}`)

	// Render updated single game card partial
	h.renderer.RenderPartial(w, "game_card.html", map[string]interface{}{
		"Game":             game,
		"CurrentTime":      time.Now(),
		"FormattedKickoff": h.formatKickoff(game.KickoffTime),
		"LockMode":         scoringCfg.LockMode,
		"FirstKickoff":     firstKickoff,
	})
}

func (h *PicksHandler) SaveScore(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	gameID, err := strconv.ParseInt(r.FormValue("game_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid game_id", http.StatusBadRequest)
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	scoringCfg, _ := h.repo.GetScoringConfig()
	weekGames, _ := h.repo.ListGamesByWeek(game.WeekID)
	firstKickoff := findFirstKickoff(weekGames)

	// Enforce lock check
	if game.IsGameOrWeekLocked(time.Now(), scoringCfg.LockMode, firstKickoff) {
		if scoringCfg.LockMode == "full_week" {
			http.Error(w, "La jornada completa se encuentra bloqueada porque el primer partido ya inició.", http.StatusForbidden)
		} else {
			http.Error(w, "Este partido ya se encuentra bloqueado.", http.StatusForbidden)
		}
		return
	}

	var homeScore, awayScore *int
	if hsStr := r.FormValue("home_score"); hsStr != "" {
		if hs, err := strconv.Atoi(hsStr); err == nil && hs >= 0 {
			homeScore = &hs
		}
	}
	if asStr := r.FormValue("away_score"); asStr != "" {
		if as, err := strconv.Atoi(asStr); err == nil && as >= 0 {
			awayScore = &as
		}
	}

	pick, err := h.repo.SavePick(user.ID, gameID, nil, homeScore, awayScore)
	if err != nil {
		http.Error(w, "Error saving score", http.StatusInternalServerError)
		return
	}

	game.UserPick = pick

	w.Header().Set("HX-Trigger", `{"show-toast": {"message": "¡Marcador guardado exitosamente!", "type": "success"}}`)

	h.renderer.RenderPartial(w, "game_card.html", map[string]interface{}{
		"Game":             game,
		"CurrentTime":      time.Now(),
		"FormattedKickoff": h.formatKickoff(game.KickoffTime),
		"LockMode":         scoringCfg.LockMode,
		"FirstKickoff":     firstKickoff,
	})
}

// SaveAll saves all submitted picks and predicted scores for a week at once
func (h *PicksHandler) SaveAll(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	weekID, err := strconv.ParseInt(r.FormValue("week_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid week_id", http.StatusBadRequest)
		return
	}

	week, err := h.repo.GetWeekByID(weekID)
	if err != nil || week == nil {
		http.Error(w, "Week not found", http.StatusNotFound)
		return
	}

	scoringCfg, _ := h.repo.GetScoringConfig()
	weekGames, err := h.repo.ListGamesByWeek(weekID)
	if err != nil {
		http.Error(w, "Error listing games", http.StatusInternalServerError)
		return
	}
	firstKickoff := findFirstKickoff(weekGames)
	now := time.Now()

	savedCount := 0
	for _, game := range weekGames {
		if game.IsGameOrWeekLocked(now, scoringCfg.LockMode, firstKickoff) {
			continue
		}

		var pickedTeamID *int64
		if teamStr := strings.TrimSpace(r.FormValue(fmt.Sprintf("picked_team_%d", game.ID))); teamStr != "" {
			if tid, err := strconv.ParseInt(teamStr, 10, 64); err == nil {
				pickedTeamID = &tid
			}
		}

		var homeScore, awayScore *int
		if hsStr := strings.TrimSpace(r.FormValue(fmt.Sprintf("home_score_%d", game.ID))); hsStr != "" {
			if hs, err := strconv.Atoi(hsStr); err == nil && hs >= 0 {
				homeScore = &hs
			}
		}
		if asStr := strings.TrimSpace(r.FormValue(fmt.Sprintf("away_score_%d", game.ID))); asStr != "" {
			if as, err := strconv.Atoi(asStr); err == nil && as >= 0 {
				awayScore = &as
			}
		}

		if pickedTeamID != nil || homeScore != nil || awayScore != nil {
			_, err := h.repo.SavePick(user.ID, game.ID, pickedTeamID, homeScore, awayScore)
			if err == nil {
				savedCount++
			}
		}
	}

	redirectURL := fmt.Sprintf("/picks?week=%d&saved=1", week.WeekNumber)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirectURL)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (h *PicksHandler) CommunityPicks(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameId")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid game id", http.StatusBadRequest)
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	scoringCfg, _ := h.repo.GetScoringConfig()
	weekGames, _ := h.repo.ListGamesByWeek(game.WeekID)
	firstKickoff := findFirstKickoff(weekGames)

	// Fair play check: only reveal picks once game/week is locked or live/final
	if !game.IsGameOrWeekLocked(time.Now(), scoringCfg.LockMode, firstKickoff) {
		http.Error(w, "Los pronósticos se revelarán cuando inicie el partido.", http.StatusForbidden)
		return
	}

	picks, err := h.repo.ListPicksForGame(gameID)
	if err != nil {
		http.Error(w, "Error loading community picks", http.StatusInternalServerError)
		return
	}

	h.renderer.RenderPartial(w, "community_picks.html", map[string]interface{}{
		"Picks": picks,
		"Game":  game,
	})
}

func findFirstKickoff(games []*db.Game) *time.Time {
	if len(games) == 0 {
		return nil
	}
	earliest := games[0].KickoffTime
	for _, g := range games[1:] {
		if g.KickoffTime.Before(earliest) {
			earliest = g.KickoffTime
		}
	}
	return &earliest
}

func (h *PicksHandler) formatKickoff(t time.Time) string {
	if t.IsZero() {
		return "--"
	}
	if CDMXLocation != nil {
		t = t.In(CDMXLocation)
	}
	dayAbbr := map[string]string{"Mon": "Lun", "Tue": "Mar", "Wed": "Mié", "Thu": "Jue", "Fri": "Vie", "Sat": "Sáb", "Sun": "Dom"}[t.Format("Mon")]
	monthAbbr := map[string]string{"Jan": "Ene", "Feb": "Feb", "Mar": "Mar", "Apr": "Abr", "May": "May", "Jun": "Jun", "Jul": "Jul", "Aug": "Ago", "Sep": "Sep", "Oct": "Oct", "Nov": "Nov", "Dec": "Dic"}[t.Format("Jan")]
	return dayAbbr + ", " + t.Format("2") + " " + monthAbbr + " - " + t.Format("3:04 PM")
}
