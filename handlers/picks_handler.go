package handlers

import (
	"fmt"
	"math"
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
	if activeWeek, _ := h.repo.GetActiveWeek(season.ID); activeWeek != nil {
		selectedWeekNum = activeWeek.WeekNumber
	}
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
	now := time.Now()
	effectiveFirstKickoff := firstKickoff
	if selectedWeek != nil && selectedWeek.WeekNumber == 1 && effectiveFirstKickoff != nil && effectiveFirstKickoff.Before(db.Week1GraceDeadline) && now.Before(db.Week1GraceDeadline) {
		effectiveFirstKickoff = &db.Week1GraceDeadline
	}
	if scoringCfg.LockMode == "full_week" && effectiveFirstKickoff != nil {
		isFullWeekLocked = now.After(*effectiveFirstKickoff) || now.Equal(*effectiveFirstKickoff)
	}

	// Fetch user's picks
	var userPicks map[int64]*db.Pick
	if user != nil {
		userPicks, _ = h.repo.GetUserPicksForWeek(user.ID, selectedWeek.ID)
	}

	picksCount := 0
	userWeeklyPts := 0
	userCorrectPicks := 0

	var weekCommunityStats map[int64]*db.GameCommunityStats
	var weekForecasts map[int64]*db.GameForecast
	if selectedWeek != nil {
		weekCommunityStats, _ = h.repo.GetWeekCommunityStats(selectedWeek.ID)
		weekForecasts, _ = h.repo.GetWeekForecasts(selectedWeek.ID)
	}

	for _, g := range games {
		if weekForecasts != nil {
			g.Forecast = weekForecasts[g.ID]
		}
		if weekCommunityStats != nil {
			if cs, exists := weekCommunityStats[g.ID]; exists && cs != nil {
				g.TotalPicks = cs.TotalPicks
				g.HomePickCount = cs.HomePicksCount
				g.AwayPickCount = cs.AwayPicksCount
			}
		}
		if userPicks != nil {
			if pick, exists := userPicks[g.ID]; exists {
				pick.InferWinnerFromScores(g)
				g.UserPick = pick
				if g.HasPickCompleted() {
					picksCount++
				}
				userWeeklyPts += pick.PointsEarned + pick.BonusPoints
				if pick.IsCorrect != nil && *pick.IsCorrect {
					userCorrectPicks++
				}
			}
		}
	}

	// Upcoming kickoff & 24h deadline countdown calculation
	upcomingKickoff, upcomingGame, _ := h.repo.GetUpcomingKickoffForWeek(selectedWeek.ID, now)
	hasUpcomingDeadline := false
	var timeUntilKickoff time.Duration
	var nextKickoffISO string
	var formattedNextKickoff string
	var closingTargetDeadline *time.Time

	if scoringCfg.LockMode == "full_week" && effectiveFirstKickoff != nil {
		closingTargetDeadline = effectiveFirstKickoff
	} else if upcomingKickoff != nil {
		closingTargetDeadline = upcomingKickoff
	}

	if closingTargetDeadline != nil {
		nextKickoffISO = closingTargetDeadline.Format(time.RFC3339)
		timeUntilKickoff = closingTargetDeadline.Sub(now)
		if timeUntilKickoff > 0 && timeUntilKickoff <= 24*time.Hour {
			hasUpcomingDeadline = true
		}
		formattedNextKickoff = h.formatKickoff(*closingTargetDeadline)
	}

	missingPicksCount := 0
	for _, g := range games {
		isLocked := g.IsGameOrWeekLocked(now, scoringCfg.LockMode, firstKickoff)
		if !isLocked && !g.HasPickCompleted() {
			missingPicksCount++
		}
	}

	featureFlags, _ := h.repo.GetFeatureFlagsMap()

	totalUsersCount := 0
	readyUsersCount := 0
	readyPercent := 0
	if summaries, err := h.repo.GetUserWeeklySummaries(selectedWeek.ID); err == nil && len(summaries) > 0 {
		totalUsersCount = len(summaries)
		for _, s := range summaries {
			if s.TotalGames > 0 && s.CompletedPicks == s.TotalGames {
				readyUsersCount++
			}
		}
		if totalUsersCount > 0 {
			readyPercent = int(math.Round(float64(readyUsersCount) * 100.0 / float64(totalUsersCount)))
		}
	}

	h.renderer.RenderPage(w, "picks.html", map[string]interface{}{
		"ActiveNav":               "picks",
		"User":                    user,
		"Weeks":                   weeks,
		"SelectedWeek":            selectedWeek,
		"Games":                   games,
		"PicksCount":              picksCount,
		"UserWeeklyPoints":        userWeeklyPts,
		"UserCorrectPicks":        userCorrectPicks,
		"CurrentTime":             time.Now(),
		"ScoringConfig":           scoringCfg,
		"LockMode":                scoringCfg.LockMode,
		"FirstKickoff":            firstKickoff,
		"IsFullWeekLocked":        isFullWeekLocked,
		"JustSaved":               r.URL.Query().Get("saved") == "1",
		"MissingCount":            func() int { m, _ := strconv.Atoi(r.URL.Query().Get("missing")); return m }(),
		"IsWeek1GraceActive":      selectedWeek.WeekNumber == 1 && time.Now().Before(db.Week1GraceDeadline),
		"Week1GraceDeadline":      db.Week1GraceDeadline,
		"HasUpcomingDeadline":     hasUpcomingDeadline,
		"UpcomingKickoff":         closingTargetDeadline,
		"UpcomingGame":            upcomingGame,
		"NextKickoffISO":          nextKickoffISO,
		"FormattedNextKickoff":    formattedNextKickoff,
		"TimeUntilKickoffSeconds": int64(timeUntilKickoff.Seconds()),
		"MissingPicksCount":       missingPicksCount,
		"FeatureFlags":            featureFlags,
		"TotalUsersCount":         totalUsersCount,
		"ReadyUsersCount":         readyUsersCount,
		"ReadyPercent":            readyPercent,
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

	if pickedTeamID == nil && homeScore != nil && awayScore != nil {
		if *homeScore > *awayScore {
			pickedTeamID = &game.HomeTeamID
		} else if *awayScore > *homeScore {
			pickedTeamID = &game.AwayTeamID
		}
	}

	pick, err := h.repo.SavePick(user.ID, gameID, pickedTeamID, homeScore, awayScore)
	if err != nil {
		http.Error(w, "Error saving pick", http.StatusInternalServerError)
		return
	}

	pick.InferWinnerFromScores(game)
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

	var pickedTeamID *int64
	if homeScore != nil && awayScore != nil {
		if *homeScore > *awayScore {
			pickedTeamID = &game.HomeTeamID
		} else if *awayScore > *homeScore {
			pickedTeamID = &game.AwayTeamID
		}
	}

	pick, err := h.repo.SavePick(user.ID, gameID, pickedTeamID, homeScore, awayScore)
	if err != nil {
		http.Error(w, "Error saving score", http.StatusInternalServerError)
		return
	}

	pick.InferWinnerFromScores(game)
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

	openGamesCount := 0
	savedCount := 0
	for _, game := range weekGames {
		if game.IsGameOrWeekLocked(now, scoringCfg.LockMode, firstKickoff) {
			continue
		}
		openGamesCount++

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

		// Infer winner from scores if user entered scores but didn't explicitly pick team
		if pickedTeamID == nil && homeScore != nil && awayScore != nil {
			if *homeScore > *awayScore {
				pickedTeamID = &game.HomeTeamID
			} else if *awayScore > *homeScore {
				pickedTeamID = &game.AwayTeamID
			}
		}

		if pickedTeamID != nil || homeScore != nil || awayScore != nil {
			pick, err := h.repo.SavePick(user.ID, game.ID, pickedTeamID, homeScore, awayScore)
			if err == nil {
				pick.InferWinnerFromScores(game)
				if pick.IsCompleteForGame(game) {
					savedCount++
				}
			}
		}
	}

	missingCount := openGamesCount - savedCount
	if missingCount < 0 {
		missingCount = 0
	}

	redirectURL := fmt.Sprintf("/picks?week=%d&saved=1&missing=%d", week.WeekNumber, missingCount)
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
		h.renderer.RenderPartial(w, "community_picks.html", map[string]interface{}{
			"LockedFairPlay": true,
			"Game":           game,
		})
		return
	}

	picks, err := h.repo.ListPicksForGame(gameID)
	if err != nil {
		http.Error(w, "Error loading community picks", http.StatusInternalServerError)
		return
	}

	stats, _ := h.repo.GetGameCommunityStats(gameID)

	h.renderer.RenderPartial(w, "community_picks.html", map[string]interface{}{
		"Picks": picks,
		"Game":  game,
		"Stats": stats,
	})
}

// ComparePicks handles the head-to-head comparison modal between two players
func (h *PicksHandler) ComparePicks(w http.ResponseWriter, r *http.Request) {
	currentUser := auth.GetUserFromContext(r.Context())
	if currentUser == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !h.repo.IsFeatureAccessible("picks_compare", currentUser) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<div class="p-6 rounded-2xl bg-zinc-950 border border-purple-500/30 text-center space-y-3"><div class="w-12 h-12 rounded-2xl bg-purple-500/20 text-purple-400 border border-purple-500/40 flex items-center justify-center mx-auto text-xl"><i class="fa-solid fa-flask"></i></div><h3 class="text-sm font-bold text-white">Duelo Cara a Cara (Beta Privada)</h3><p class="text-xs text-zinc-400">Esta función está reservada temporalmente para Beta Testers y Administradores.</p></div>`)
		return
	}

	rivalIDStr := r.URL.Query().Get("rival_id")
	rivalID, err := strconv.ParseInt(rivalIDStr, 10, 64)
	if err != nil || rivalID <= 0 {
		http.Error(w, "Invalid rival_id", http.StatusBadRequest)
		return
	}

	rival, err := h.repo.GetUserByID(rivalID)
	if err != nil || rival == nil {
		http.Error(w, "Rival not found", http.StatusNotFound)
		return
	}
	if rival.IsAdmin() {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<div class="p-6 rounded-2xl bg-zinc-950 border border-zinc-800 text-center space-y-3"><div class="w-12 h-12 rounded-2xl bg-zinc-900 text-zinc-400 border border-zinc-800 flex items-center justify-center mx-auto text-xl"><i class="fa-solid fa-shield-halved"></i></div><h3 class="text-sm font-bold text-white">Cuenta de Administración</h3><p class="text-xs text-zinc-400">Las cuentas de administración no participan en competencias ni duelos directos.</p></div>`)
		return
	}

	weekIDStr := r.URL.Query().Get("week_id")
	var weekID int64
	if weekIDStr != "" {
		weekID, _ = strconv.ParseInt(weekIDStr, 10, 64)
	}

	if weekID <= 0 {
		season, err := h.repo.GetActiveSeason(h.seasonYear)
		if err != nil {
			http.Error(w, "Active season not found", http.StatusInternalServerError)
			return
		}
		weeks, _ := h.repo.ListWeeks(season.ID)
		if len(weeks) > 0 {
			weekID = weeks[0].ID
			for _, wk := range weeks {
				if wk.Status == "active" {
					weekID = wk.ID
					break
				}
			}
		}
	}

	comparison, err := h.repo.GetHeadToHeadComparison(weekID, currentUser.ID, rivalID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating comparison: %v", err), http.StatusInternalServerError)
		return
	}

	var seasonID int64
	season, err := h.repo.GetActiveSeason(h.seasonYear)
	if err == nil && season != nil {
		seasonID = season.ID
	} else if comparison.Week != nil {
		seasonID = comparison.Week.SeasonID
	}

	seasonHistory, _ := h.repo.GetHeadToHeadSeasonHistory(currentUser.ID, rivalID, seasonID)

	h.renderer.RenderPartial(w, "head_to_head_modal.html", map[string]interface{}{
		"Comparison":    comparison,
		"CurrentUser":   currentUser,
		"SeasonHistory": seasonHistory,
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

// ShowPicksMatrix displays the full community picks matrix (sábana)
func (h *PicksHandler) ShowPicksMatrix(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if !h.repo.IsFeatureAccessible("picks_matrix", user) {
		h.renderer.RenderPage(w, "beta_locked.html", map[string]interface{}{
			"ActiveNav":   "picks",
			"User":        user,
			"FeatureName": "Matriz de Pronósticos",
			"FeatureDesc": "La sábana comparativa comunitaria permite ver en una sola grilla en tiempo real los pronósticos de todos los competidores semana a semana.",
		})
		return
	}

	season, err := h.repo.GetActiveSeason(2026)
	if err != nil {
		http.Error(w, "Error loading season", http.StatusInternalServerError)
		return
	}

	weeks, err := h.repo.ListWeeks(season.ID)
	if err != nil || len(weeks) == 0 {
		http.Error(w, "No weeks found", http.StatusNotFound)
		return
	}

	weekNumStr := r.URL.Query().Get("week")
	var selectedWeek *db.Week
	if weekNumStr != "" {
		wn, _ := strconv.Atoi(weekNumStr)
		for _, wk := range weeks {
			if wk.WeekNumber == wn {
				selectedWeek = wk
				break
			}
		}
	}
	if selectedWeek == nil {
		for _, wk := range weeks {
			if wk.Status == "active" {
				selectedWeek = wk
				break
			}
		}
		if selectedWeek == nil {
			selectedWeek = weeks[0]
		}
	}

	var currentUserID int64 = 0
	if user != nil {
		currentUserID = user.ID
	}

	matrixData, err := h.repo.GetPicksMatrixForWeek(selectedWeek.ID, currentUserID)
	if err != nil {
		http.Error(w, "Error loading picks matrix: "+err.Error(), http.StatusInternalServerError)
		return
	}

	matrixData.User = user

	if r.Header.Get("HX-Request") == "true" || r.URL.Query().Get("partial") == "1" {
		h.renderer.RenderPartial(w, "picks_matrix_table.html", map[string]interface{}{
			"Matrix":       matrixData,
			"User":         user,
			"SelectedWeek": selectedWeek,
			"Weeks":        weeks,
		})
		return
	}

	h.renderer.RenderPage(w, "picks_matrix.html", map[string]interface{}{
		"ActiveNav":    "picks",
		"User":         user,
		"Matrix":       matrixData,
		"SelectedWeek": selectedWeek,
		"Weeks":        weeks,
	})
}

func (h *PicksHandler) PicksReadinessModal(w http.ResponseWriter, r *http.Request) {
	currentUser := auth.GetUserFromContext(r.Context())
	if currentUser == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	weekIDStr := r.URL.Query().Get("week_id")
	var weekID int64
	if weekIDStr != "" {
		weekID, _ = strconv.ParseInt(weekIDStr, 10, 64)
	}

	var selectedWeek *db.Week
	if weekID > 0 {
		selectedWeek, _ = h.repo.GetWeekByID(weekID)
	}

	if selectedWeek == nil {
		season, err := h.repo.GetActiveSeason(h.seasonYear)
		if err != nil {
			http.Error(w, "Active season not found", http.StatusInternalServerError)
			return
		}
		weeks, _ := h.repo.ListWeeks(season.ID)
		if len(weeks) > 0 {
			selectedWeek = weeks[0]
			for _, wk := range weeks {
				if wk.Status == "active" {
					selectedWeek = wk
					break
				}
			}
		}
	}

	if selectedWeek == nil {
		http.Error(w, "Week not found", http.StatusNotFound)
		return
	}

	summaries, err := h.repo.GetUserWeeklySummaries(selectedWeek.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching summaries: %v", err), http.StatusInternalServerError)
		return
	}

	readyCount := 0
	inProgressCount := 0
	pendingCount := 0
	totalCount := len(summaries)

	for _, s := range summaries {
		if s.TotalGames > 0 && s.CompletedPicks == s.TotalGames {
			readyCount++
		} else if s.CompletedPicks > 0 {
			inProgressCount++
		} else {
			pendingCount++
		}
	}

	readyPercent := 0
	if totalCount > 0 {
		readyPercent = int(math.Round(float64(readyCount) * 100.0 / float64(totalCount)))
	}

	h.renderer.RenderPartial(w, "picks_readiness_modal.html", map[string]interface{}{
		"Week":            selectedWeek,
		"Summaries":       summaries,
		"CurrentUser":     currentUser,
		"TotalCount":      totalCount,
		"ReadyCount":      readyCount,
		"InProgressCount": inProgressCount,
		"PendingCount":    pendingCount,
		"ReadyPercent":    readyPercent,
	})
}
