package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"nfl-quiniela-2026/db"
)

// AllowInsecureEndpointsForTesting enables loopback endpoints only during automated tests
var AllowInsecureEndpointsForTesting = false

// ValidatePushEndpoint ensures the target push URL uses HTTPS and does not point to internal/private networks (SSRF prevention)
func ValidatePushEndpoint(endpointStr string) error {
	u, err := url.Parse(endpointStr)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if u.Scheme != "https" {
		if u.Scheme == "http" && AllowInsecureEndpointsForTesting {
			return nil
		}
		return fmt.Errorf("push endpoint must use HTTPS scheme")
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("push endpoint host is missing")
	}

	if AllowInsecureEndpointsForTesting && (host == "127.0.0.1" || host == "localhost") {
		return nil
	}

	// Reject localhost / loopback hostnames
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return fmt.Errorf("push endpoint cannot target local domains: %s", host)
	}

	ip := net.ParseIP(host)
	if ip != nil {
		if IsPrivateOrLoopbackIP(ip) {
			return fmt.Errorf("push endpoint targets restricted IP: %s", host)
		}
	} else {
		// Resolve DNS
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("failed to resolve push endpoint host: %w", err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("no IP address found for host: %s", host)
		}
		for _, resolved := range ips {
			if IsPrivateOrLoopbackIP(resolved) {
				return fmt.Errorf("push endpoint resolves to restricted IP: %s", resolved.String())
			}
		}
	}

	return nil
}

// IsPrivateOrLoopbackIP checks if an IP belongs to loopback, private RFC1918, or link-local ranges
func IsPrivateOrLoopbackIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		// 169.254.0.0/16 (Link Local / Cloud Metadata)
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// 127.0.0.0/8 (Loopback)
		if ip4[0] == 127 {
			return true
		}
		// 0.0.0.0/8
		if ip4[0] == 0 {
			return true
		}
		// 100.64.0.0/10 (Carrier-grade NAT)
		if ip4[0] == 100 && (ip4[1]&192 == 64) {
			return true
		}
	}
	return false
}

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

// NewPushService initializes a new Web Push service instance with anti-SSRF protections
func NewPushService(repo *db.Repository, publicKey, privateKey, subject string) *PushService {
	safeTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !AllowInsecureEndpointsForTesting && IsPrivateOrLoopbackIP(ip) {
					return nil, fmt.Errorf("SSRF protection: push endpoint resolves to private/loopback IP: %s", ip.String())
				}
			}
			dialer := &net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			if len(ips) > 0 {
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
			}
			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &PushService{
		repo:       repo,
		publicKey:  publicKey,
		privateKey: privateKey,
		subject:    subject,
		httpClient: &http.Client{
			Transport: safeTransport,
			Timeout:   10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// WebPush (RFC 8030) gateways do not require redirects.
				// Prohibit redirects to completely block redirect-based SSRF attempts.
				return http.ErrUseLastResponse
			},
		},
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

	if err := ValidatePushEndpoint(sub.Endpoint); err != nil {
		return fmt.Errorf("invalid push subscription endpoint: %w", err)
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
