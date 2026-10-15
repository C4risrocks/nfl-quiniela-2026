package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

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

	featureFlags, _ := h.repo.ListFeatureFlags()

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
		"SyncStatus":          syncStatus,
		"FeatureFlags":        featureFlags,
		"RegistrationEnabled": h.repo.IsRegistrationEnabled(),
		"IsWeek1GraceActive":  selectedWeek.WeekNumber == 1 && time.Now().Before(db.Week1GraceDeadline),
		"Week1GraceDeadline":  db.Week1GraceDeadline,
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

// SyncTeams syncs standings, power rankings, and playoff probabilities for the current season
func (h *AdminHandler) SyncTeams(w http.ResponseWriter, r *http.Request) {
	if h.syncer == nil {
		http.Error(w, "Syncer unavailable", http.StatusServiceUnavailable)
		return
	}

	standings, err := h.syncer.SyncTeamsStandings(h.seasonYear)
	if err != nil {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Error al sincronizar standings de equipos", "type": "error"}}`)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<span class="text-rose-400 font-bold"><i class="fa-solid fa-triangle-exclamation mr-1"></i> Error: %v</span>`, err)))
		return
	}

	currentWk := h.syncer.FetchCurrentWeekNumber()
	_ = h.syncer.SyncPowerRankingsAndFPI(h.seasonYear, currentWk)

	count := 0
	if standings != nil {
		count = len(standings.League)
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "¡Éxito! Standings de %d equipos y Power Rankings sincronizados", "type": "success"}}`, count))
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`<span class="text-emerald-400 font-bold"><i class="fa-solid fa-circle-check mr-1"></i> ¡Éxito! %d equipos sincronizados (`+time.Now().Format("15:04:05")+`)</span>`, count)))
}

// SyncInjuries syncs NFL injury reports from ESPN and clears healed players
func (h *AdminHandler) SyncInjuries(w http.ResponseWriter, r *http.Request) {
	if h.syncer == nil {
		http.Error(w, "Syncer unavailable", http.StatusServiceUnavailable)
		return
	}

	count, err := h.syncer.SyncAllInjuries()
	if err != nil {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Error al sincronizar lesiones", "type": "error"}}`)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<span class="text-rose-400 font-bold"><i class="fa-solid fa-triangle-exclamation mr-1"></i> Error: %v</span>`, err)))
		return
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "¡Éxito! %d reportes de lesiones sincronizados", "type": "success"}}`, count))
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`<span class="text-emerald-400 font-bold"><i class="fa-solid fa-circle-check mr-1"></i> ¡Éxito! %d lesiones actualizadas (`+time.Now().Format("15:04:05")+`)</span>`, count)))
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

	// If game is marked final, ensure it has detailed stats generated/persisted
	if status == "final" {
		if updatedG, _ := h.repo.GetGameByID(gameID); updatedG != nil {
			curSummary := updatedG.DetailedSummary()
			if curSummary == nil || !curSummary.HasStats {
				genSummary := espn.GenerateRealisticSummary(updatedG)
				if genSummary != nil {
					if b, err := json.Marshal(genSummary); err == nil {
						_ = h.repo.UpdateGameStatsJSON(gameID, string(b))
					}
				}
			}
		}
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	reminderLabel := "recordatorios de patada inicial"
	if reminderType == "grace_period" {
		reminderLabel = "avisos urgentes de prórroga"
	}

	if err != nil {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Error al enviar recordatorios: %s", "type": "error"}}`, strings.ReplaceAll(err.Error(), `"`, `'`)))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<div class="p-3.5 rounded-xl bg-red-500/10 border border-red-500/25 text-red-300 text-xs flex items-center space-x-2.5"><i class="fa-solid fa-circle-exclamation text-red-400 text-base"></i><span>Error al procesar recordatorios: %v</span></div>`, err)
		return
	}

	if count == 0 {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "¡Al día! No hay recordatorios pendientes por enviar para esta jornada.", "type": "info"}}`)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<div class="p-3.5 rounded-xl bg-blue-500/10 border border-blue-500/25 text-blue-300 text-xs flex items-center space-x-2.5 shadow-sm"><i class="fa-solid fa-circle-check text-blue-400 text-base"></i><span>¡Éxito! Todos los jugadores registrados tienen sus pronósticos al día o ya cuentan con aviso previo de %s.</span></div>`, reminderLabel)
		return
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "¡Éxito! Se enviaron %d %s", "type": "success"}}`, count, reminderLabel))
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<div class="p-3.5 rounded-xl bg-emerald-500/10 border border-emerald-500/25 text-emerald-300 text-xs flex items-center space-x-2.5 shadow-sm"><i class="fa-solid fa-circle-check text-emerald-400 text-base"></i><span>¡Éxito! Se enviaron <strong>%d</strong> %s a jugadores con pronósticos pendientes.</span></div>`, count, reminderLabel)
}

func (h *AdminHandler) SendWeeklyRecap(w http.ResponseWriter, r *http.Request) {
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

	count, err := h.reminderWorker.SendManualWeeklyRecap(weekID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err != nil {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Error al despachar resumen semanal: %s", "type": "error"}}`, strings.ReplaceAll(err.Error(), `"`, `'`)))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<div class="p-3.5 rounded-xl bg-red-500/10 border border-red-500/25 text-red-300 text-xs flex items-center space-x-2.5"><i class="fa-solid fa-circle-exclamation text-red-400 text-base"></i><span>Error al despachar resumen: %v</span></div>`, err)
		return
	}

	if count == 0 {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "No hubo resúmenes semanales nuevos pendientes por despachar.", "type": "info"}}`)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<div class="p-3.5 rounded-xl bg-blue-500/10 border border-blue-500/25 text-blue-300 text-xs flex items-center space-x-2.5 shadow-sm"><i class="fa-solid fa-circle-info text-blue-400 text-base"></i><span>¡Éxito! Todos los jugadores participantes ya han recibido sus resúmenes semanales o no hay destinatarios pendientes.</span></div>`)
		return
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "¡Éxito! Se despacharon %d resúmenes semanales", "type": "success"}}`, count))
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<div class="p-3.5 rounded-xl bg-yellow-500/10 border border-yellow-500/25 text-yellow-300 text-xs flex items-center space-x-2.5 shadow-sm"><i class="fa-solid fa-trophy text-yellow-400 text-base"></i><span>¡Éxito! Se despacharon <strong>%d</strong> resúmenes semanales (notificación interna y correo).</span></div>`, count)
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

func (h *AdminHandler) ToggleUserVerification(w http.ResponseWriter, r *http.Request) {
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

	newVerified := !targetUser.EmailVerified
	if err := h.repo.SetUserEmailVerified(targetUserID, newVerified); err != nil {
		http.Error(w, "Error updating user verification", http.StatusInternalServerError)
		return
	}

	// Recalculate leaderboard so user inclusion/exclusion updates immediately
	activeSeason, _ := h.repo.GetActiveSeason(h.seasonYear)
	if activeSeason != nil {
		activeWeek, _ := h.repo.GetActiveWeek(activeSeason.ID)
		if activeWeek != nil {
			_ = h.calculator.CalculateWeekScores(activeWeek.ID)
			if h.broker != nil {
				h.broker.BroadcastLeaderboardUpdate()
			}
		}
	}

	statusMsg := "Verificación revocada"
	if newVerified {
		statusMsg = "Usuario verificado exitosamente"
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "%s para %s", "type": "info"}}`, statusMsg, targetUser.Username))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if newVerified {
		fmt.Fprintf(w, `<button hx-post="/admin/users/%d/toggle-verify" hx-swap="outerHTML" class="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-mono uppercase font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 hover:bg-rose-500/15 hover:text-rose-400 hover:border-rose-500/30 transition-colors cursor-pointer" title="Clic para revocar verificación"><i class="fa-solid fa-circle-check mr-1 text-[9px]"></i> Verificado</button>`, targetUserID)
	} else {
		fmt.Fprintf(w, `<button hx-post="/admin/users/%d/toggle-verify" hx-swap="outerHTML" class="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-mono uppercase font-semibold bg-amber-500/15 text-amber-300 border border-amber-500/30 hover:bg-emerald-500/15 hover:text-emerald-400 hover:border-emerald-500/30 transition-colors cursor-pointer" title="Clic para validar manualmente"><i class="fa-solid fa-clock mr-1 text-[9px]"></i> Sin Verificar</button>`, targetUserID)
	}
}

func (h *AdminHandler) VerifyUserEmail(w http.ResponseWriter, r *http.Request) {
	h.ToggleUserVerification(w, r)
}

func (h *AdminHandler) AdminResendVerificationEmail(w http.ResponseWriter, r *http.Request) {
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

	token, err := auth.GenerateRandomToken(32)
	if err != nil {
		http.Error(w, "Error generating token", http.StatusInternalServerError)
		return
	}

	if err := h.repo.SetVerificationToken(targetUserID, token); err != nil {
		http.Error(w, "Error saving token", http.StatusInternalServerError)
		return
	}

	if h.reminderWorker != nil && h.reminderWorker.Sender() != nil {
		baseURL := getBaseURLFromRequest(r, h.reminderWorker.Sender().BaseURL())
		go func(u *db.User, tok, bURL string) {
			_ = h.reminderWorker.Sender().SendVerificationEmail(u, tok, bURL)
		}(targetUser, token, baseURL)
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Correo de activación reenviado a %s", "type": "success"}}`, targetUser.Email))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<span class="text-[10px] text-emerald-400 font-mono"><i class="fa-solid fa-paper-plane mr-1"></i>Enviado</span>`)
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

func (h *AdminHandler) ToggleUserBeta(w http.ResponseWriter, r *http.Request) {
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

	newBetaState := !targetUser.IsBetaTester
	if err := h.repo.SetUserBetaTester(targetUserID, newBetaState); err != nil {
		http.Error(w, "Error toggling beta status", http.StatusInternalServerError)
		return
	}

	statusText := "removido de"
	if newBetaState {
		statusText = "agregado a"
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Usuario %s %s Beta Testers", "type": "info"}}`, targetUser.Username, statusText))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if newBetaState {
		fmt.Fprintf(w, `<button hx-post="/admin/users/%d/toggle-beta" hx-swap="outerHTML" class="px-2 py-0.5 rounded-full text-[10px] font-mono uppercase font-semibold cursor-pointer hover:opacity-80 transition-opacity bg-purple-500/20 text-purple-300 border border-purple-500/40 shadow-xs"><i class="fa-solid fa-flask mr-1 text-[8px] text-purple-400"></i>Beta Tester</button>`, targetUserID)
	} else {
		fmt.Fprintf(w, `<button hx-post="/admin/users/%d/toggle-beta" hx-swap="outerHTML" class="px-2 py-0.5 rounded-full text-[10px] font-mono uppercase font-semibold cursor-pointer hover:opacity-80 transition-opacity bg-zinc-900 text-zinc-500 border border-zinc-800">Estándar</button>`, targetUserID)
	}
}

func (h *AdminHandler) SaveFeatureFlags(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Error en los datos del formulario", "type": "error"}}`)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	flags, err := h.repo.ListFeatureFlags()
	if err != nil {
		http.Error(w, "Error reading feature flags", http.StatusInternalServerError)
		return
	}

	updatedCount := 0
	for _, flag := range flags {
		accessLevel := strings.TrimSpace(r.FormValue("access_level_" + flag.Key))
		if accessLevel == "" {
			accessLevel = flag.AccessLevel
		}
		if accessLevel != "all" && accessLevel != "beta" && accessLevel != "admin" && accessLevel != "disabled" {
			accessLevel = "all"
		}

		isBeta := r.FormValue("is_beta_"+flag.Key) == "1" || r.FormValue("is_beta_"+flag.Key) == "on"

		if err := h.repo.UpdateFeatureFlag(flag.Key, accessLevel, isBeta); err == nil {
			updatedCount++
		}
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "¡Configuración de %d características guardada con éxito!", "type": "success"}}`, updatedCount))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<div class="p-3 rounded-xl bg-purple-500/10 border border-purple-500/30 text-purple-300 text-xs flex items-center space-x-2"><i class="fa-solid fa-flask text-purple-400"></i><span>¡Control de características actualizado! Se aplicaron los niveles de acceso y etiquetas Beta en tiempo real.</span></div>`)
}

func (h *AdminHandler) ShowEditUserModal(w http.ResponseWriter, r *http.Request) {
	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		http.Error(w, "ID de usuario inválido", http.StatusBadRequest)
		return
	}

	targetUser, err := h.repo.GetUserByID(targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, "Usuario no encontrado", http.StatusNotFound)
		return
	}

	teams, _ := h.repo.ListTeams()
	currentUser := auth.GetUserFromContext(r.Context())
	isSelf := currentUser != nil && currentUser.ID == targetUserID

	var selectedTeamID int64
	if targetUser.FavoriteTeamID != nil {
		selectedTeamID = *targetUser.FavoriteTeamID
	}

	h.renderer.RenderPartial(w, "admin_user_edit_modal.html", map[string]interface{}{
		"TargetUser":     targetUser,
		"Teams":          teams,
		"IsSelf":         isSelf,
		"SelectedTeamID": selectedTeamID,
	})
}

func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		http.Error(w, "ID de usuario inválido", http.StatusBadRequest)
		return
	}

	currentUser := auth.GetUserFromContext(r.Context())
	isSelf := currentUser != nil && currentUser.ID == targetUserID

	targetUser, err := h.repo.GetUserByID(targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, "Usuario no encontrado", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Datos de formulario inválidos", "type": "error"}}`)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	role := strings.TrimSpace(r.FormValue("role"))
	if role != "admin" && role != "player" {
		role = targetUser.Role
	}
	// Prevent self-demotion from admin to player
	if isSelf && role != "admin" {
		role = "admin"
	}

	emailVerified := r.FormValue("email_verified") == "1" || r.FormValue("email_verified") == "on" || r.FormValue("email_verified") == "true"
	isBeta := r.FormValue("is_beta_tester") == "1" || r.FormValue("is_beta_tester") == "on" || r.FormValue("is_beta_tester") == "true"
	bio := strings.TrimSpace(r.FormValue("bio"))

	var favTeamID *int64
	if ftStr := strings.TrimSpace(r.FormValue("favorite_team_id")); ftStr != "" && ftStr != "0" {
		if id, err := strconv.ParseInt(ftStr, 10, 64); err == nil && id > 0 {
			favTeamID = &id
		}
	}

	if username == "" || email == "" {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "El nombre de usuario y correo son obligatorios", "type": "error"}}`)
		http.Error(w, "Username and email required", http.StatusBadRequest)
		return
	}

	newPassword := strings.TrimSpace(r.FormValue("new_password"))
	var newPasswordHash string
	if newPassword != "" {
		if len(newPassword) < 6 {
			w.Header().Set("HX-Trigger", `{"show-toast": {"message": "La nueva contraseña debe tener al menos 6 caracteres", "type": "error"}}`)
			http.Error(w, "Password too short", http.StatusBadRequest)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Error al procesar la contraseña", "type": "error"}}`)
			http.Error(w, "Error hashing password", http.StatusInternalServerError)
			return
		}
		newPasswordHash = string(hash)
	}

	if err := h.repo.AdminUpdateUser(targetUserID, username, email, role, emailVerified, isBeta, favTeamID, bio, newPasswordHash); err != nil {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Error al actualizar: %s", "type": "error"}}`, err.Error()))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If verification status changed, recalculate leaderboards
	if emailVerified != targetUser.EmailVerified {
		_ = h.calculator.RecalculateAllWeeks(h.seasonYear)
		if h.broker != nil {
			h.broker.BroadcastLeaderboardUpdate()
		}
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Cuenta de %s actualizada exitosamente", "type": "success"}, "close-admin-modal": true}`, username))
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		http.Error(w, "ID de usuario inválido", http.StatusBadRequest)
		return
	}

	currentUser := auth.GetUserFromContext(r.Context())
	if currentUser != nil && currentUser.ID == targetUserID {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "No puedes eliminar tu propia cuenta de administrador", "type": "error"}}`)
		http.Error(w, "No puedes eliminar tu propia cuenta", http.StatusBadRequest)
		return
	}

	targetUser, err := h.repo.GetUserByID(targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, "Usuario no encontrado", http.StatusNotFound)
		return
	}

	if targetUser.Username == "ia_quiniela" {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "No se puede eliminar la cuenta del bot del sistema", "type": "error"}}`)
		http.Error(w, "No se puede eliminar la cuenta de bot", http.StatusBadRequest)
		return
	}

	if err := h.repo.AdminDeleteUser(targetUserID); err != nil {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Error al eliminar usuario: %s", "type": "error"}}`, err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Recalculate leaderboards across all weeks
	_ = h.calculator.RecalculateAllWeeks(h.seasonYear)
	if h.broker != nil {
		h.broker.BroadcastLeaderboardUpdate()
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Cuenta de %s eliminada permanentemente", "type": "success"}}`, targetUser.Username))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "")
}

func (h *AdminHandler) ToggleRegistration(w http.ResponseWriter, r *http.Request) {
	currentState := h.repo.IsRegistrationEnabled()
	newState := !currentState

	if err := h.repo.SetRegistrationEnabled(newState); err != nil {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Error al actualizar estado de registros", "type": "error"}}`)
		http.Error(w, "Error saving registration state", http.StatusInternalServerError)
		return
	}

	statusMsg := "Registros de nuevos usuarios bloqueados"
	toastType := "warning"
	if newState {
		statusMsg = "Registros de nuevos usuarios permitidos"
		toastType = "success"
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "%s", "type": "%s"}}`, statusMsg, toastType))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if newState {
		fmt.Fprint(w, `<div id="registration-toggle-control" class="inline-flex items-center space-x-2 bg-zinc-900 border border-zinc-800 rounded-xl px-3 py-1.5 shadow-sm">
    <span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
    <span class="text-xs font-semibold text-emerald-400">Registros Abiertos</span>
    <button hx-post="/admin/settings/toggle-registration"
            hx-target="#registration-toggle-control"
            hx-swap="outerHTML"
            class="ml-1 px-2.5 py-1 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-300 border border-rose-500/30 text-[11px] font-semibold transition-all cursor-pointer">
        <i class="fa-solid fa-lock mr-1 text-[10px]"></i> Bloquear
    </button>
</div>`)
	} else {
		fmt.Fprint(w, `<div id="registration-toggle-control" class="inline-flex items-center space-x-2 bg-zinc-900 border border-rose-900/50 rounded-xl px-3 py-1.5 shadow-sm">
    <span class="w-2 h-2 rounded-full bg-rose-500"></span>
    <span class="text-xs font-semibold text-rose-400">Registros Bloqueados</span>
    <button hx-post="/admin/settings/toggle-registration"
            hx-target="#registration-toggle-control"
            hx-swap="outerHTML"
            class="ml-1 px-2.5 py-1 rounded-lg bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-300 border border-emerald-500/30 text-[11px] font-semibold transition-all cursor-pointer">
        <i class="fa-solid fa-lock-open mr-1 text-[10px]"></i> Permitir
    </button>
</div>`)
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
