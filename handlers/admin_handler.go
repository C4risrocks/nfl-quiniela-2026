package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

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

	h.renderer.RenderPage(w, "admin.html", map[string]interface{}{
		"ActiveNav":     "admin",
		"User":          user,
		"Weeks":         weeks,
		"SelectedWeek":  selectedWeek,
		"Games":         games,
		"Users":         users,
		"ScoringConfig": scoringCfg,
		"PendingUsers":  pendingUsers,
		"PendingCount":  len(pendingUsers),
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
	h.renderer.RenderPartial(w, "admin_game_row.html", map[string]interface{}{
		"Game":             updatedGame,
		"FormattedKickoff": h.formatKickoff(updatedGame.KickoffTime),
	})
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
	h.renderer.RenderPartial(w, "admin_game_row.html", map[string]interface{}{
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
	h.renderer.RenderPartial(w, "admin_game_row.html", map[string]interface{}{
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

	count, err := h.reminderWorker.SendManualReminders(weekID)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<div class="p-2 rounded bg-red-500/10 border border-red-500/20 text-red-300 text-xs">Error: %v</div>`, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<div class="p-2 rounded bg-emerald-500/10 border border-emerald-500/20 text-emerald-300 text-xs flex items-center space-x-1.5"><i class="fa-solid fa-circle-check"></i><span>¡Éxito! Se enviaron <strong>%d</strong> recordatorios a jugadores con picks pendientes.</span></div>`, count)
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
