package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

type NotificationHandler struct {
	repo     *db.Repository
	renderer *Renderer
}

func NewNotificationHandler(repo *db.Repository, renderer *Renderer) *NotificationHandler {
	return &NotificationHandler{
		repo:     repo,
		renderer: renderer,
	}
}

// GetNotifications returns the HTML dropdown list of notifications and unread badge count
func (h *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	notifs, err := h.repo.ListUserNotifications(user.ID, 15)
	if err != nil {
		http.Error(w, "Error loading notifications", http.StatusInternalServerError)
		return
	}

	unreadCount, _ := h.repo.GetUnreadNotificationsCount(user.ID)

	h.renderer.RenderPartial(w, "notifications_dropdown.html", map[string]interface{}{
		"Notifications": notifs,
		"UnreadCount":   unreadCount,
		"CurrentUser":   user,
	})
}

// GetUnreadBadge returns only the numeric unread badge HTML snippet
func (h *NotificationHandler) GetUnreadBadge(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	unreadCount, _ := h.repo.GetUnreadNotificationsCount(user.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if unreadCount > 0 {
		displayCount := strconv.Itoa(unreadCount)
		if unreadCount > 9 {
			displayCount = "9+"
		}
		fmt.Fprintf(w, `<span id="notification-badge" class="absolute -top-1 -right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-red-600 px-1 text-[9px] font-black text-white shadow-sm ring-2 ring-black animate-pulse">%s</span>`, displayCount)
	} else {
		fmt.Fprintf(w, `<span id="notification-badge" class="hidden"></span>`)
	}
}

// MarkAsRead marks a specific notification as read and updates the list
func (h *NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	notifID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}

	_ = h.repo.MarkNotificationAsRead(notifID, user.ID)

	// Return updated dropdown list
	h.GetNotifications(w, r)
}

// MarkAllAsRead marks all user's notifications as read and updates the dropdown
func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_ = h.repo.MarkAllNotificationsAsRead(user.ID)

	// Return updated dropdown list
	h.GetNotifications(w, r)
}
