package security

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// clientBucket tracks rate limit tokens for a single IP address.
type clientBucket struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

// IPRateLimiter implements an in-memory, thread-safe token bucket rate limiter per IP.
type IPRateLimiter struct {
	mu         sync.Mutex
	clients    map[string]*clientBucket
	rate       float64 // tokens per second
	capacity   float64 // maximum burst capacity
	stopChan   chan struct{}
	cleanupInt time.Duration
}

// NewIPRateLimiter creates a new limiter with requests per minute and burst capacity.
func NewIPRateLimiter(reqPerMinute int, burstCapacity int) *IPRateLimiter {
	if reqPerMinute <= 0 {
		reqPerMinute = 60
	}
	if burstCapacity <= 0 {
		burstCapacity = reqPerMinute / 2
		if burstCapacity < 1 {
			burstCapacity = 1
		}
	}

	rate := float64(reqPerMinute) / 60.0
	limiter := &IPRateLimiter{
		clients:    make(map[string]*clientBucket),
		rate:       rate,
		capacity:   float64(burstCapacity),
		stopChan:   make(chan struct{}),
		cleanupInt: 5 * time.Minute,
	}

	// Launch background cleanup goroutine
	go limiter.cleanupLoop()

	return limiter
}

// Allow checks if the given IP is allowed to proceed. Returns allowed boolean and wait duration if blocked.
func (l *IPRateLimiter) Allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.clients[ip]
	if !exists {
		l.clients[ip] = &clientBucket{
			tokens:     l.capacity - 1.0,
			lastRefill: now,
			lastSeen:   now,
		}
		return true, 0
	}

	b.lastSeen = now

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = math.Min(l.capacity, b.tokens+elapsed*l.rate)
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true, 0
	}

	// Calculate wait time until at least 1 token is available
	missing := 1.0 - b.tokens
	waitSec := missing / l.rate
	if waitSec < 1.0 {
		waitSec = 1.0
	}
	return false, time.Duration(waitSec * float64(time.Second))
}

// Stop terminates the background cleanup goroutine.
func (l *IPRateLimiter) Stop() {
	select {
	case <-l.stopChan:
		return
	default:
		close(l.stopChan)
	}
}

func (l *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(l.cleanupInt)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopChan:
			return
		case now := <-ticker.C:
			l.mu.Lock()
			// Evict clients not seen for 15 minutes
			for ip, b := range l.clients {
				if now.Sub(b.lastSeen) > 15*time.Minute {
					delete(l.clients, ip)
				}
			}
			l.mu.Unlock()
		}
	}
}

// GlobalRateLimitMiddleware applies general rate limiting to all dynamic routes.
func GlobalRateLimitMiddleware(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Bypass static assets, web worker, and real-time SSE stream
			if strings.HasPrefix(path, "/static/") ||
				path == "/favicon.ico" ||
				path == "/robots.txt" ||
				path == "/sw.js" ||
				path == "/events" {
				next.ServeHTTP(w, r)
				return
			}

			ip := GetClientIP(r)
			allowed, waitDuration := limiter.Allow(ip)
			if !allowed {
				retryAfterSec := int(math.Ceil(waitDuration.Seconds()))
				if retryAfterSec < 1 {
					retryAfterSec = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSec))

				if r.Header.Get("HX-Request") == "true" {
					w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Demasiadas solicitudes. Por favor espera %d segundos.", "type": "warning"}}`, retryAfterSec))
				}
				http.Error(w, fmt.Sprintf("429 Too Many Requests: Límite de peticiones excedido. Reintenta en %d segundos.", retryAfterSec), http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AuthRateLimitMiddleware applies stricter rate limiting to sensitive authentication/recovery endpoints.
func AuthRateLimitMiddleware(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only apply to mutating requests (POST) on auth endpoints
			if r.Method == http.MethodPost {
				ip := GetClientIP(r)
				allowed, waitDuration := limiter.Allow(ip)
				if !allowed {
					retryAfterSec := int(math.Ceil(waitDuration.Seconds()))
					if retryAfterSec < 1 {
						retryAfterSec = 1
					}
					w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSec))

					if r.Header.Get("HX-Request") == "true" {
						w.Header().Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"message": "Demasiados intentos de acceso. Espera %d segundos.", "type": "error"}}`, retryAfterSec))
					}
					http.Error(w, fmt.Sprintf("429 Too Many Requests: Demasiados intentos de autenticación. Por favor espera %d segundos antes de reintentar.", retryAfterSec), http.StatusTooManyRequests)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
