package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

type ProfileHandler struct {
	repo        *db.Repository
	authService *auth.AuthService
	renderer    *Renderer
}

func NewProfileHandler(repo *db.Repository, authService *auth.AuthService, renderer *Renderer) *ProfileHandler {
	return &ProfileHandler{
		repo:        repo,
		authService: authService,
		renderer:    renderer,
	}
}

func (h *ProfileHandler) ShowProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	freshUser, err := h.repo.GetUserByID(user.ID)
	if err != nil {
		http.Error(w, "Error cargando usuario", http.StatusInternalServerError)
		return
	}

	stats, _ := h.repo.GetUserStats(user.ID)
	teams, _ := h.repo.ListTeams()

	season, _ := h.repo.GetActiveSeason(2026)
	var seasonID int64
	if season != nil {
		seasonID = season.ID
	}
	rank, totalPlayers, _ := h.repo.GetUserRank(user.ID, seasonID)
	weeklyHistory, _ := h.repo.GetUserWeeklyBreakdown(user.ID, seasonID)

	var bestWeek *db.UserWeeklyPerformance
	for _, w := range weeklyHistory {
		if w.Points > 0 {
			if bestWeek == nil || w.Points > bestWeek.Points {
				bestWeek = w
			}
		}
	}

	var successMsg, errorMsg string
	if r.URL.Query().Get("updated") == "1" {
		successMsg = "Tus preferencias han sido guardadas exitosamente."
	}
	if r.URL.Query().Get("pwd_updated") == "1" {
		successMsg = "Tu contraseña ha sido actualizada exitosamente."
	}
	if errParam := r.URL.Query().Get("err"); errParam != "" {
		errorMsg = errParam
	}

	h.renderer.RenderPage(w, "profile.html", map[string]interface{}{
		"ActiveNav":     "profile",
		"User":          freshUser,
		"Stats":         stats,
		"Teams":         teams,
		"Rank":          rank,
		"TotalPlayers":  totalPlayers,
		"WeeklyHistory": weeklyHistory,
		"BestWeek":      bestWeek,
		"Success":       successMsg,
		"Error":         errorMsg,
	})
}

func (h *ProfileHandler) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	favTeamStr := r.FormValue("favorite_team_id")
	var favTeamID *int64
	if favTeamStr != "" && favTeamStr != "0" {
		if id, err := strconv.ParseInt(favTeamStr, 10, 64); err == nil && id > 0 {
			favTeamID = &id
		}
	}

	notifyEmail := r.FormValue("notify_email") == "on" || r.FormValue("notify_email") == "true" || r.FormValue("notify_email") == "1"

	if err := h.repo.UpdateUserPreferences(user.ID, favTeamID, notifyEmail); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/profile?err=%s", "Error al guardar preferencias"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/profile?updated=1", http.StatusSeeOther)
}

func (h *ProfileHandler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := strings.TrimSpace(r.FormValue("new_password"))
	confirmPassword := strings.TrimSpace(r.FormValue("confirm_password"))

	freshUser, err := h.repo.GetUserByID(user.ID)
	if err != nil {
		http.Redirect(w, r, "/profile?err=Usuario+no+encontrado", http.StatusSeeOther)
		return
	}

	if !h.authService.CheckPasswordHash(currentPassword, freshUser.PasswordHash) {
		http.Redirect(w, r, "/profile?err=La+contraseña+actual+es+incorrecta", http.StatusSeeOther)
		return
	}

	if len(newPassword) < 6 {
		http.Redirect(w, r, "/profile?err=La+nueva+contraseña+debe+tener+al+menos+6+caracteres", http.StatusSeeOther)
		return
	}

	if newPassword != confirmPassword {
		http.Redirect(w, r, "/profile?err=Las+nuevas+contraseñas+no+coinciden", http.StatusSeeOther)
		return
	}

	newHash, err := h.authService.HashPassword(newPassword)
	if err != nil {
		http.Redirect(w, r, "/profile?err=Error+procesando+contraseña", http.StatusSeeOther)
		return
	}

	if err := h.repo.UpdateUserPassword(user.ID, newHash); err != nil {
		http.Redirect(w, r, "/profile?err=Error+actualizando+contraseña", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/profile?pwd_updated=1", http.StatusSeeOther)
}
