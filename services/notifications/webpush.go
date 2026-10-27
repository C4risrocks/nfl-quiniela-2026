package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"nfl-quiniela-2026/db"
)

// PushPayload represents the JSON payload dispatched to the client Service Worker
type PushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Icon  string `json:"icon,omitempty"`
	Badge string `json:"badge,omitempty"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// PushService manages Web Push notifications using VAPID (RFC 8292)
type PushService struct {
	repo       *db.Repository
	publicKey  string
	privateKey string
	subject    string
	httpClient *http.Client
}

// NewPushService initializes a new Web Push service instance
func NewPushService(repo *db.Repository, publicKey, privateKey, subject string) *PushService {
	return &PushService{
		repo:       repo,
		publicKey:  publicKey,
		privateKey: privateKey,
		subject:    subject,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// PublicKey returns the VAPID public key needed by browser clients to subscribe
func (s *PushService) PublicKey() string {
	if s == nil {
		return ""
	}
	return s.publicKey
}

// IsConfigured returns true if both public and private VAPID keys are present
func (s *PushService) IsConfigured() bool {
	return s != nil && s.publicKey != "" && s.privateKey != ""
}

// SendNotification sends a push message to a specific device subscription
func (s *PushService) SendNotification(ctx context.Context, sub *db.PushSubscription, payload PushPayload) error {
	if !s.IsConfigured() {
		return fmt.Errorf("webpush service is not configured with VAPID keys")
	}

	if payload.Icon == "" {
		payload.Icon = "/static/icons/icon.svg"
	}
	if payload.Badge == "" {
		payload.Badge = "/static/icons/icon.svg"
	}

	msgBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal push payload: %w", err)
	}

	wpSub := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}

	options := &webpush.Options{
		Subscriber:      s.subject,
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
		TTL:             86400, // 24 hours
		Urgency:         webpush.UrgencyHigh,
		HTTPClient:      s.httpClient,
	}

	resp, err := webpush.SendNotificationWithContext(ctx, msgBytes, wpSub, options)
	if err != nil {
		return fmt.Errorf("webpush dispatch error: %w", err)
	}
	defer resp.Body.Close()

	// Prune expired or unregistered endpoints automatically (RFC 8292)
	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		log.Printf("[WebPush] Subscription expired (%d). Pruning endpoint %s", resp.StatusCode, sub.Endpoint)
		_ = s.repo.DeletePushSubscription(sub.Endpoint)
		return fmt.Errorf("subscription expired (status %d)", resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webpush gateway error (status %d)", resp.StatusCode)
	}

	return nil
}

// SendToUser dispatches a push notification to all devices registered to a specific user
func (s *PushService) SendToUser(userID int64, payload PushPayload) (int, error) {
	if !s.IsConfigured() {
		return 0, nil
	}

	subs, err := s.repo.GetUserPushSubscriptions(userID)
	if err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	successCount := 0
	var lastErr error
	for _, sub := range subs {
		if err := s.SendNotification(ctx, sub, payload); err == nil {
			successCount++
		} else {
			lastErr = err
			log.Printf("[WebPush] Note sending push to subscription %d (user %d): %v", sub.ID, userID, err)
		}
	}

	return successCount, lastErr
}

// SendToAll dispatches a push notification to all registered devices in the quiniela
func (s *PushService) SendToAll(payload PushPayload) (int, error) {
	if !s.IsConfigured() {
		return 0, nil
	}

	subs, err := s.repo.GetAllPushSubscriptions()
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	successCount := 0
	for _, sub := range subs {
		if err := s.SendNotification(ctx, sub, payload); err == nil {
			successCount++
		}
	}

	return successCount, nil
}
