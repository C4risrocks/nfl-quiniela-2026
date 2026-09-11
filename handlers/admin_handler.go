package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
	"nfl-quiniela-2026/services/espn"
	"nfl-quiniela-2026/services/events"
	"nfl-quiniela-2026/services/notifications"
	"nfl-quiniela-2026/services/scoring"
)

type AdminHandler struct {
	repo           *db.Repository
	renderer       *Renderer
	syncer         *espn.Syncer
	calculator     *scoring.Calculator
	broker         *events.Broker
	reminderWorker *notifications.ReminderWorker
	seasonYear     int
}

func NewAdminHandler(
	repo *db.Repository,
	renderer *Renderer,
	syncer *espn.Syncer,
	calculator *scoring.Calculator,
	broker *events.Broker,
	reminderWorker *notifications.ReminderWorker,
	seasonYear int,
) *AdminHandler {
	return &AdminHandler{
		repo:           repo,
		renderer:       renderer,
		syncer:         syncer,
		calculator:     calculator,
		broker:         broker,
		reminderWorker: reminderWorker,
		seasonYear:     seasonYear,
	}
}

func (h *AdminHandler) ShowAdmin(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	season, err := h.repo.GetActiveSeason(h.seasonYear)
	if err != nil {
		http.Error(w, "Season not found", http.StatusInternalServerError)
		return
	}

	weeks, _ := h.repo.ListWeeks(season.ID)
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

	games, _ := h.repo.ListGamesByWeek(selectedWeek.ID)
	users, _ := h.repo.ListUsers()
	scoringCfg, _ := h.repo.GetScoringConfig()
	pendingUsers, _ := h.repo.GetUsersWithPendingPicks(selectedWeek.ID)
	userSummaries, _ := h.repo.GetUserWeeklySummaries(selectedWeek.ID)

	var syncStatus espn.SyncStatus
	if h.syncer != nil {
		syncStatus = h.syncer.GetSyncStatus()
	}

	h.renderer.RenderPage(w, "admin.html", map[string]interface{}{
		"ActiveNav":          "admin",
		"User":               user,
		"Weeks":              weeks,
		"SelectedWeek":       selectedWeek,
		"Games":              games,
		"Users":              users,
		"UserSummaries":      userSummaries,
		"ScoringConfig":      scoringCfg,
		"PendingUsers":       pendingUsers,
		"PendingCount":       len(pendingUsers),
		"SyncStatus":         syncStatus,
		"IsWeek1GraceActive": selectedWeek.WeekNumber == 1 && time.Now().Before(db.Week1GraceDeadline),
		"Week1GraceDeadline": db.Week1GraceDeadline,
	})
}

func (h *AdminHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Error en los datos del formulario", "type": "error"}}`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<div class="p-3.5 rounded-xl bg-red-500/10 border border-red-500/20 text-red-300 text-xs flex items-center space-x-2.5"><i class="fa-solid fa-circle-exclamation text-red-400 text-sm"></i><span>Error en los datos del formulario. Verifica los valores ingresados.</span></div>`)
		return
	}

	mode := r.FormValue("scoring_mode")
	if mode != "pure_tiebreaker" {
		mode = "weighted"
	}

	winnerPts, _ := strconv.Atoi(r.FormValue("winner_points"))
	if winnerPts <= 0 {
		if mode == "pure_tiebreaker" {
			winnerPts = 1
		} else {
			winnerPts = 10
		}
	}

	exactBonus, _ := strconv.Atoi(r.FormValue("exact_score_bonus"))
	marginBonus, _ := strconv.Atoi(r.FormValue("margin_bonus"))

	lockMode := r.FormValue("lock_mode")
	if lockMode != "full_week" {
		lockMode = "per_game"
	}

	cfg := &db.ScoringConfig{
		ScoringMode:      mode,
		WinnerPoints:     winnerPts,
		ExactScoreBonus:  exactBonus,
		ExactMarginBonus: marginBonus,
		LockMode:         lockMode,
	}

	if err := h.repo.SetScoringConfig(cfg); err != nil {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Error al guardar la configuración", "type": "error"}}`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `<div class="p-3.5 rounded-xl bg-red-500/10 border border-red-500/20 text-red-300 text-xs flex items-center space-x-2.5"><i class="fa-solid fa-circle-exclamation text-red-400 text-sm"></i><span>Error al guardar las reglas en la base de datos: %s</span></div>`, err.Error())
		return
	}

	// Recalculate scores for all weeks under new scoring mode
	_ = h.calculator.RecalculateAllWeeks(h.seasonYear)

	if h.broker != nil {
		h.broker.BroadcastLeaderboardUpdate()
	}

	now := time.Now()
	if CDMXLocation != nil {
		now = now.In(CDMXLocation)
	}
	timeStr := now.Format("3:04:05 PM")

	w.Header().Set("HX-Trigger", `{"show-toast": {"message": "¡Reglas guardadas y puntuaciones recalculadas exitosamente!", "type": "success"}}`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<div class="p-3.5 rounded-xl bg-emerald-500/10 border border-emerald-500/25 text-emerald-300 text-xs flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 shadow-sm transition-all"><div class="flex items-center space-x-2.5"><i class="fa-solid fa-circle-check text-emerald-400 text-base flex-shrink-0"></i><div><span class="font-bold text-white">¡Reglas guardadas exitosamente!</span> <span class="text-zinc-400">La nueva modalidad de puntuación y política de bloqueo han sido aplicadas y todos los puntajes del torneo fueron recalculados.</span></div></div><div class="flex items-center space-x-1.5 text-[10px] font-mono text-emerald-400/80 bg-emerald-500/15 px-2.5 py-1 rounded-md self-start sm:self-auto"><i class="fa-regular fa-clock"></i><span>Actualizado: %s</span></div></div>`, timeStr)
}

func (h *AdminHandler) SyncESPN(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	weekNum, err := strconv.Atoi(r.FormValue("week"))
	if err != nil || weekNum <= 0 {
		weekNum = 1
	}

	count, err := h.syncer.SyncWeek(weekNum)
	if err != nil {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Error al sincronizar con ESPN", "type": "error"}}`)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<span class="text-red-400 font-bold"><i class="fa-solid fa-triangle-exclamation mr-1"></i> Error: %v</span>`, err)))
		return
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "¡Éxito! %d partidos sincronizados con ESPN", "type": "success"}}`, count))
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`<span class="text-emerald-400 font-bold"><i class="fa-solid fa-circle-check mr-1"></i> ¡Éxito! %d partidos sincronizados con ESPN</span>`, count)))
}

func (h *AdminHandler) RecalculateScores(w http.ResponseWriter, r *http.Request) {
	if err := h.calculator.RecalculateAllWeeks(h.seasonYear); err != nil {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Error al recalcular puntuaciones", "type": "error"}}`)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf(`<span class="text-rose-400 font-bold"><i class="fa-solid fa-triangle-exclamation mr-1"></i> Error: %v</span>`, err)))
		return
	}

	if h.broker != nil {
		h.broker.BroadcastLeaderboardUpdate()
	}

	w.Header().Set("HX-Trigger", `{"show-toast": {"message": "¡Puntuaciones recalculadas exitosamente para todo el torneo!", "type": "success"}}`)
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<span class="text-emerald-400 font-bold"><i class="fa-solid fa-circle-check mr-1"></i> Puntuaciones y tabla general recalculadas exitosamente (` + time.Now().Format("15:04:05") + `)</span>`))
}

func (h *AdminHandler) SaveGameScore(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	gameID, err := strconv.ParseInt(r.FormValue("game_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid game id", http.StatusBadRequest)
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
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

	status := r.FormValue("status")
	if status != "in_progress" && status != "final" {
		status = "scheduled"
	}

	statusDetail := ""
	if status == "final" {
		statusDetail = "Final"
	} else if status == "in_progress" {
		statusDetail = "En Juego"
	}

	if err := h.repo.UpdateGameScoreAndStatus(gameID, homeScore, awayScore, status, statusDetail); err != nil {
		http.Error(w, "Error updating game score", http.StatusInternalServerError)
		return
	}

	// Recalculate leaderboard for this week
	_ = h.calculator.CalculateWeekScores(game.WeekID)

	if h.broker != nil {
		h.broker.Broadcast(fmt.Sprintf("game-%d", gameID), fmt.Sprintf(`{"game_id": %d, "status": "%s"}`, gameID, status))
		h.broker.Broadcast("week-updated", fmt.Sprintf(`{"week_id": %d}`, game.WeekID))
		h.broker.BroadcastLeaderboardUpdate()
	}

	updatedGame, _ := h.repo.GetGameByID(gameID)
	w.Header().Set("HX-Trigger", `{"show-toast": {"message": "¡Marcador y estado actualizados!", "type": "success"}}`)
	h.renderer.RenderPartial(w, h.gameTemplate(r), map[string]interface{}{
		"Game":             updatedGame,
		"FormattedKickoff": h.formatKickoff(updatedGame.KickoffTime),
	})
}

func (h *AdminHandler) gameTemplate(r *http.Request) string {
	if r.FormValue("view") == "card" {
		return "admin_game_card.html"
	}
	return "admin_game_row.html"
}

func (h *AdminHandler) ToggleGameLock(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	gameID, err := strconv.ParseInt(r.FormValue("game_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid game id", http.StatusBadRequest)
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	newLockState := !game.IsLocked
	if err := h.repo.ToggleGameLock(gameID, newLockState); err != nil {
		http.Error(w, "Error toggling lock", http.StatusInternalServerError)
		return
	}

	if h.broker != nil {
		h.broker.Broadcast(fmt.Sprintf("game-%d", gameID), fmt.Sprintf(`{"game_id": %d, "locked": %v}`, gameID, newLockState))
		h.broker.Broadcast("week-updated", fmt.Sprintf(`{"week_id": %d}`, game.WeekID))
	}

	updatedGame, _ := h.repo.GetGameByID(gameID)
	statusMsg := "Partido desbloqueado para pronósticos"
	if newLockState {
		statusMsg = "Partido bloqueado para pronósticos"
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "%s", "type": "info"}}`, statusMsg))
	h.renderer.RenderPartial(w, h.gameTemplate(r), map[string]interface{}{
		"Game":             updatedGame,
		"FormattedKickoff": h.formatKickoff(updatedGame.KickoffTime),
	})
}

func (h *AdminHandler) ToggleTiebreaker(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	gameID, err := strconv.ParseInt(r.FormValue("game_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid game id", http.StatusBadRequest)
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	newTiebreakerState := !game.IsTiebreaker
	if err := h.repo.SetGameTiebreaker(gameID, newTiebreakerState); err != nil {
		http.Error(w, "Error toggling tiebreaker", http.StatusInternalServerError)
		return
	}

	// Recalculate leaderboard
	_ = h.calculator.CalculateWeekScores(game.WeekID)

	if h.broker != nil {
		h.broker.Broadcast(fmt.Sprintf("game-%d", gameID), fmt.Sprintf(`{"game_id": %d, "is_tiebreaker": %v}`, gameID, newTiebreakerState))
		h.broker.BroadcastLeaderboardUpdate()
	}

	updatedGame, _ := h.repo.GetGameByID(gameID)
	tbMsg := "Partido retirado como desempate"
	if newTiebreakerState {
		tbMsg = "Partido configurado como desempate (Tiebreaker)"
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "%s", "type": "info"}}`, tbMsg))
	h.renderer.RenderPartial(w, h.gameTemplate(r), map[string]interface{}{
		"Game":             updatedGame,
		"FormattedKickoff": h.formatKickoff(updatedGame.KickoffTime),
	})
}

func (h *AdminHandler) SendReminders(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	weekID, err := strconv.ParseInt(r.FormValue("week_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid week id", http.StatusBadRequest)
		return
	}

	if h.reminderWorker == nil {
		http.Error(w, "Servicio de notificaciones no disponible", http.StatusInternalServerError)
		return
	}

	reminderType := r.FormValue("reminder_type") // "grace_period" or "kickoff"
	count, err := h.reminderWorker.SendManualReminders(weekID, reminderType)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<div class="p-2 rounded bg-red-500/10 border border-red-500/20 text-red-300 text-xs">Error: %v</div>`, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	reminderLabel := "recordatorios de patada inicial"
	if reminderType == "grace_period" {
		reminderLabel = "avisos urgentes de prórroga"
	}
	fmt.Fprintf(w, `<div class="p-2.5 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-300 text-xs flex items-center space-x-2"><i class="fa-solid fa-circle-check text-emerald-400"></i><span>¡Éxito! Se enviaron <strong>%d</strong> %s a jugadores con pronósticos pendientes.</span></div>`, count, reminderLabel)
}

func (h *AdminHandler) ShowUserPicks(w http.ResponseWriter, r *http.Request) {
	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	targetUser, err := h.repo.GetUserByID(targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	weekIDStr := r.URL.Query().Get("week_id")
	var week *db.Week
	if weekIDStr != "" {
		wID, err := strconv.ParseInt(weekIDStr, 10, 64)
		if err == nil {
			week, _ = h.repo.GetWeekByID(wID)
		}
	}
	if week == nil {
		season, _ := h.repo.GetActiveSeason(h.seasonYear)
		weeks, _ := h.repo.ListWeeks(season.ID)
		if len(weeks) > 0 {
			week = weeks[0]
		}
	}
	if week == nil {
		http.Error(w, "Week not found", http.StatusNotFound)
		return
	}

	games, _ := h.repo.ListGamesByWeek(week.ID)
	userPicks, _ := h.repo.GetUserPicksForWeek(targetUser.ID, week.ID)
	for _, g := range games {
		if userPicks != nil {
			if pick, exists := userPicks[g.ID]; exists {
				pick.InferWinnerFromScores(g)
				g.UserPick = pick
			}
		}
	}

	h.renderer.RenderPartial(w, "admin_user_picks_modal.html", map[string]interface{}{
		"TargetUser": targetUser,
		"Week":       week,
		"Games":      games,
	})
}

func (h *AdminHandler) SaveUserPicks(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	targetUser, err := h.repo.GetUserByID(targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	weekID, err := strconv.ParseInt(r.FormValue("week_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid week ID", http.StatusBadRequest)
		return
	}

	weekGames, err := h.repo.ListGamesByWeek(weekID)
	if err != nil {
		http.Error(w, "Error listing games", http.StatusInternalServerError)
		return
	}

	savedCount := 0
	for _, game := range weekGames {
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

		// Infer winner from scores if not explicitly provided
		if pickedTeamID == nil && homeScore != nil && awayScore != nil {
			if *homeScore > *awayScore {
				pickedTeamID = &game.HomeTeamID
			} else if *awayScore > *homeScore {
				pickedTeamID = &game.AwayTeamID
			}
		}

		if pickedTeamID != nil || homeScore != nil || awayScore != nil {
			_, err := h.repo.SavePick(targetUser.ID, game.ID, pickedTeamID, homeScore, awayScore)
			if err == nil {
				savedCount++
			}
		}
	}

	// Recalculate leaderboard
	_ = h.calculator.CalculateWeekScores(weekID)
	if h.broker != nil {
		h.broker.BroadcastLeaderboardUpdate()
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "¡Se guardaron %d pronósticos para %s exitosamente!", "type": "success"}, "close-admin-modal": true}`, savedCount, targetUser.Username))
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandler) VerifyUserEmail(w http.ResponseWriter, r *http.Request) {
	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	if err := h.repo.SetUserEmailVerified(targetUserID, true); err != nil {
		http.Error(w, "Error verifying user email", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Correo del usuario verificado manualmente", "type": "success"}}`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<span class="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-mono uppercase font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30"><i class="fa-solid fa-circle-check mr-1 text-[9px]"></i> Verificado</span>`)
}

func (h *AdminHandler) ToggleUserRole(w http.ResponseWriter, r *http.Request) {
	currentUser := auth.GetUserFromContext(r.Context())
	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	if currentUser != nil && currentUser.ID == targetUserID {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "No puedes cambiar tu propio rol", "type": "error"}}`)
		http.Error(w, "No puedes revocar tu propio rol de administrador", http.StatusBadRequest)
		return
	}

	targetUser, err := h.repo.GetUserByID(targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	newRole := "admin"
	if targetUser.Role == "admin" {
		newRole = "player"
	}

	if err := h.repo.SetUserRole(targetUserID, newRole); err != nil {
		http.Error(w, "Error toggling user role", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Rol de %s actualizado a %s", "type": "info"}}`, targetUser.Username, newRole))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if newRole == "admin" {
		fmt.Fprint(w, `<span class="px-2 py-0.5 rounded-full text-[10px] font-mono uppercase font-semibold bg-amber-500/10 text-amber-300 border border-amber-500/30">admin</span>`)
	} else {
		fmt.Fprint(w, `<span class="px-2 py-0.5 rounded-full text-[10px] font-mono uppercase font-semibold bg-zinc-900 text-zinc-400 border border-zinc-800">player</span>`)
	}
}

func (h *AdminHandler) ExportWeekPicksCSV(w http.ResponseWriter, r *http.Request) {
	season, err := h.repo.GetActiveSeason(h.seasonYear)
	if err != nil {
		http.Error(w, "Season not found", http.StatusInternalServerError)
		return
	}

	weekNum := 1
	if wStr := r.URL.Query().Get("week"); wStr != "" {
		if wn, err := strconv.Atoi(wStr); err == nil && wn > 0 {
			weekNum = wn
		}
	}

	week, err := h.repo.GetWeekByNumber(season.ID, weekNum)
	if err != nil || week == nil {
		http.Error(w, "Week not found", http.StatusNotFound)
		return
	}

	rows, err := h.repo.GetPicksExportDataForWeek(week.ID)
	if err != nil {
		http.Error(w, "Error fetching export data", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("quiniela_semana_%d_%s.csv", week.WeekNumber, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// UTF-8 BOM for Excel compatibility
	_, _ = w.Write([]byte("\xEF\xBB\xBF"))

	// CSV Header
	_ = writer.Write([]string{
		"Usuario",
		"Correo",
		"Semana",
		"Visita",
		"Local",
		"Seleccion",
		"Marcador Visita Pronosticado",
		"Marcador Local Pronosticado",
		"Marcador Visita Real",
		"Marcador Local Real",
		"Puntos Ganados",
		"Bono",
		"Estado Partido",
		"Fecha Actualizacion",
	})

	for _, r := range rows {
		predAway := "--"
		if r.PredictedAwayScore != nil {
			predAway = strconv.Itoa(*r.PredictedAwayScore)
		}
		predHome := "--"
		if r.PredictedHomeScore != nil {
			predHome = strconv.Itoa(*r.PredictedHomeScore)
		}
		actAway := "--"
		if r.ActualAwayScore != nil {
			actAway = strconv.Itoa(*r.ActualAwayScore)
		}
		actHome := "--"
		if r.ActualHomeScore != nil {
			actHome = strconv.Itoa(*r.ActualHomeScore)
		}

		_ = writer.Write([]string{
			r.Username,
			r.Email,
			strconv.Itoa(r.WeekNumber),
			r.AwayTeamCode,
			r.HomeTeamCode,
			r.PickedTeamCode,
			predAway,
			predHome,
			actAway,
			actHome,
			strconv.Itoa(r.PointsEarned),
			strconv.Itoa(r.BonusPoints),
			r.GameStatus,
			r.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
}

func (h *AdminHandler) formatKickoff(t time.Time) string {
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
