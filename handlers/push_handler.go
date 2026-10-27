package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
	"nfl-quiniela-2026/services/notifications"
)

type PushHandler struct {
	repo        *db.Repository
	pushService *notifications.PushService
}

func NewPushHandler(repo *db.Repository, pushService *notifications.PushService) *PushHandler {
	return &PushHandler{
		repo:        repo,
		pushService: pushService,
	}
}

// GetVAPIDPublicKey returns the server's public key for client PushManager subscription
func (h *PushHandler) GetVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.pushService == nil || !h.pushService.IsConfigured() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Notificaciones push no configuradas en el servidor",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{
		"publicKey": h.pushService.PublicKey(),
	})
}

type subscribePayload struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// Subscribe stores or updates a device subscription for the current user
func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"No autenticado"}`, http.StatusUnauthorized)
		return
	}

	var req subscribePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Payload JSON inválido"})
		return
	}

	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Endpoint y llaves criptográficas requeridas"})
		return
	}

	sub := &db.PushSubscription{
		UserID:    user.ID,
		Endpoint:  req.Endpoint,
		P256dh:    req.Keys.P256dh,
		Auth:      req.Keys.Auth,
		UserAgent: r.UserAgent(),
	}

	if err := h.repo.SavePushSubscription(sub); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Error guardando suscripción: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Dispositivo registrado para notificaciones push exitosamente",
	})
}

type unsubscribePayload struct {
	Endpoint string `json:"endpoint"`
}

// Unsubscribe deletes the subscription for the given endpoint
func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"No autenticado"}`, http.StatusUnauthorized)
		return
	}

	var req unsubscribePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Payload JSON inválido"})
		return
	}

	if req.Endpoint == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Endpoint requerido"})
		return
	}

	_ = h.repo.DeleteUserPushSubscription(user.ID, req.Endpoint)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Dispositivo desvinculado de notificaciones push",
	})
}

// SendTest sends an immediate test push notification to the logged-in user
func (h *PushHandler) SendTest(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"No autenticado"}`, http.StatusUnauthorized)
		return
	}

	if h.pushService == nil || !h.pushService.IsConfigured() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Servicio de notificaciones push no configurado"})
		return
	}

	subs, err := h.repo.GetUserPushSubscriptions(user.ID)
	if err != nil || len(subs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "No tienes dispositivos registrados en tu cuenta para recibir notificaciones push. Actívalas primero en este dispositivo.",
		})
		return
	}

	count, sendErr := h.pushService.SendToUser(user.ID, notifications.PushPayload{
		Title: "🏈 Notificación de Prueba",
		Body:  fmt.Sprintf("¡Hola %s! Las notificaciones Push de la Quiniela NFL están activas en tu dispositivo.", user.Username),
		URL:   "/profile",
		Tag:   "test-notification",
	})

	w.Header().Set("Content-Type", "application/json")
	if count == 0 && sendErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("Error al despachar la notificación: %v", sendErr),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"delivered": count,
		"message":   fmt.Sprintf("Notificación de prueba enviada con éxito a %d dispositivo(s)", count),
	})
}

// GetStatus returns the current push status and count of subscriptions for this user
func (h *PushHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"No autenticado"}`, http.StatusUnauthorized)
		return
	}

	subs, _ := h.repo.GetUserPushSubscriptions(user.ID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"configured":    h.pushService != nil && h.pushService.IsConfigured(),
		"publicKey":     h.pushService.PublicKey(),
		"deviceCount":   len(subs),
		"notifyPush":    user.NotifyPush,
		"notifyKickoff": user.NotifyKickoff,
		"notifyRecap":   user.NotifyRecap,
	})
}
