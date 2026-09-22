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
	currentUser := auth.GetUserFromContext(r.Context())
	if currentUser == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	targetUserID := currentUser.ID
	isOwnProfile := true

	if targetIDStr := r.URL.Query().Get("user_id"); targetIDStr != "" {
		if tid, err := strconv.ParseInt(targetIDStr, 10, 64); err == nil && tid > 0 {
			targetUserID = tid
			isOwnProfile = (currentUser.ID == targetUserID)
		}
	}

	profileUser, err := h.repo.GetUserByID(targetUserID)
	if err != nil || profileUser == nil {
		http.Error(w, "Perfil de usuario no encontrado", http.StatusNotFound)
		return
	}

	advStats, _ := h.repo.GetAdvancedUserStats(profileUser.ID)
	var stats *db.UserStats
	if advStats != nil {
		stats = &advStats.UserStats
	}
	teams, _ := h.repo.ListTeams()

	season, _ := h.repo.GetActiveSeason(2026)
	var seasonID int64
	if season != nil {
		seasonID = season.ID
	}
	rank, totalPlayers, _ := h.repo.GetUserRank(profileUser.ID, seasonID)
	weeklyHistory, _ := h.repo.GetUserWeeklyBreakdown(profileUser.ID, seasonID)

	var bestWeek *db.UserWeeklyPerformance
	for _, w := range weeklyHistory {
		if w.Points > 0 {
			if bestWeek == nil || w.Points > bestWeek.Points {
				bestWeek = w
			}
		}
	}

	userAchievements, _ := h.repo.GetUserAchievements(profileUser.ID)
	achMap := make(map[string]*db.UserAchievement)
	for _, a := range userAchievements {
		achMap[a.BadgeCode] = a
	}

	allBadges := db.GetAllBadgeDefinitions()
	achievementDisplays := make([]db.UserAchievementDisplay, 0, len(allBadges))
	unlockedCount := 0
	for _, b := range allBadges {
		disp := db.UserAchievementDisplay{
			Definition: b,
		}
		if ach, exists := achMap[b.Code]; exists {
			disp.IsUnlocked = true
			disp.UnlockedAt = &ach.UnlockedAt
			disp.WeekNumber = ach.WeekNumber
			unlockedCount++
		}
		achievementDisplays = append(achievementDisplays, disp)
	}

	var favoriteTeam *db.Team
	if profileUser.FavoriteTeamID != nil {
		for _, t := range teams {
			if t.ID == *profileUser.FavoriteTeamID {
				favoriteTeam = t
				break
			}
		}
	}

	var featuredBadge *db.BadgeDefinition
	if profileUser.FeaturedBadgeCode != "" {
		for _, b := range allBadges {
			if b.Code == profileUser.FeaturedBadgeCode {
				badgeCopy := b
				featuredBadge = &badgeCopy
				break
			}
		}
	}

	var unlockedBadges []db.BadgeDefinition
	for _, disp := range achievementDisplays {
		if disp.IsUnlocked {
			unlockedBadges = append(unlockedBadges, disp.Definition)
		}
	}

	var successMsg, errorMsg string
	if isOwnProfile {
		if r.URL.Query().Get("updated") == "1" {
			successMsg = "Tus preferencias y personalización han sido guardadas exitosamente."
		}
		if r.URL.Query().Get("pwd_updated") == "1" {
			successMsg = "Tu contraseña ha sido actualizada exitosamente."
		}
	}
	if errParam := r.URL.Query().Get("err"); errParam != "" {
		errorMsg = errParam
	}

	h.renderer.RenderPage(w, "profile.html", map[string]interface{}{
		"ActiveNav":                 "profile",
		"User":                      currentUser,
		"ProfileUser":               profileUser,
		"IsOwnProfile":              isOwnProfile,
		"Stats":                     stats,
		"AdvancedStats":             advStats,
		"Teams":                     teams,
		"FavoriteTeam":              favoriteTeam,
		"FeaturedBadge":             featuredBadge,
		"UnlockedBadges":            unlockedBadges,
		"Rank":                      rank,
		"TotalPlayers":              totalPlayers,
		"WeeklyHistory":             weeklyHistory,
		"BestWeek":                  bestWeek,
		"AchievementDisplays":       achievementDisplays,
		"UnlockedAchievementsCount": unlockedCount,
		"TotalBadgesCount":          len(allBadges),
		"Success":                   successMsg,
		"Error":                     errorMsg,
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

	avatarURL := strings.TrimSpace(r.FormValue("avatar_url"))
	bio := strings.TrimSpace(r.FormValue("bio"))
	bioRunes := []rune(bio)
	if len(bioRunes) > 60 {
		bio = string(bioRunes[:60])
	}

	favTeamStr := r.FormValue("favorite_team_id")
	var favTeamID *int64
	if favTeamStr != "" && favTeamStr != "0" {
		if id, err := strconv.ParseInt(favTeamStr, 10, 64); err == nil && id > 0 {
			favTeamID = &id
		}
	}

	featuredBadgeCode := strings.TrimSpace(r.FormValue("featured_badge_code"))
	if featuredBadgeCode != "" {
		userAchievements, _ := h.repo.GetUserAchievements(user.ID)
		hasBadge := false
		for _, ach := range userAchievements {
			if ach.BadgeCode == featuredBadgeCode {
				hasBadge = true
				break
			}
		}
		if !hasBadge {
			featuredBadgeCode = "" // Cannot feature unearned badges
		}
	}

	notifyEmail := r.FormValue("notify_email") == "on" || r.FormValue("notify_email") == "true" || r.FormValue("notify_email") == "1"

	if err := h.repo.UpdateUserPreferences(user.ID, avatarURL, favTeamID, notifyEmail, bio, featuredBadgeCode); err != nil {
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
