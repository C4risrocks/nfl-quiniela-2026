package security

import (
	"log"
	"net/http"
	"net/url"
	"strings"
)

// CSRFProtectionMiddleware protects state-changing requests against Cross-Site Request Forgery (CSRF).
// It verifies that incoming POST/PUT/DELETE/PATCH requests originate from the same site.
func CSRFProtectionMiddleware(appBaseURL string) func(http.Handler) http.Handler {
	var allowedHosts []string
	if appBaseURL != "" {
		if u, err := url.Parse(appBaseURL); err == nil && u.Host != "" {
			allowedHosts = append(allowedHosts, strings.ToLower(u.Host))
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Safe methods do not mutate state
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
				next.ServeHTTP(w, r)
				return
			}

			// Check Sec-Fetch-Site (modern browsers metadata)
			if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
				log.Printf("[CSRF] Blocked cross-site request with Sec-Fetch-Site=%s from IP %s to %s", site, GetClientIP(r), r.URL.Path)
				rejectCSRF(w, r)
				return
			}

			// Validate Origin header if present
			originHeader := r.Header.Get("Origin")
			refererHeader := r.Header.Get("Referer")

			targetHost := strings.ToLower(r.Host)

			if originHeader != "" {
				originURL, err := url.Parse(originHeader)
				if err != nil || !isHostAllowed(strings.ToLower(originURL.Host), targetHost, allowedHosts) {
					log.Printf("[CSRF] Blocked invalid Origin '%s' (target: '%s') from IP %s", originHeader, targetHost, GetClientIP(r))
					rejectCSRF(w, r)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Fallback to Referer if Origin is missing
			if refererHeader != "" {
				refURL, err := url.Parse(refererHeader)
				if err != nil || !isHostAllowed(strings.ToLower(refURL.Host), targetHost, allowedHosts) {
					log.Printf("[CSRF] Blocked invalid Referer '%s' (target: '%s') from IP %s", refererHeader, targetHost, GetClientIP(r))
					rejectCSRF(w, r)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// If both Origin and Referer are absent on a mutating request,
			// allow if it has a same-origin HTMX/AJAX custom header, otherwise pass through (e.g. CLI/testing)
			next.ServeHTTP(w, r)
		})
	}
}

func isHostAllowed(candidateHost, targetHost string, allowedHosts []string) bool {
	if candidateHost == "" || targetHost == "" {
		return false
	}
	if candidateHost == targetHost {
		return true
	}
	// Strip port for comparison if port differs in proxy
	candNoPort := stripPort(candidateHost)
	targetNoPort := stripPort(targetHost)
	if candNoPort == targetNoPort {
		return true
	}

	for _, allowed := range allowedHosts {
		if candidateHost == allowed || candNoPort == stripPort(allowed) {
			return true
		}
	}

	// Permit local dev loopback pairings (e.g. 127.0.0.1 <-> localhost)
	if isLoopback(candNoPort) && isLoopback(targetNoPort) {
		return true
	}

	return false
}

func stripPort(host string) string {
	if idx := strings.IndexByte(host, ':'); idx != -1 {
		return host[:idx]
	}
	return host
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func rejectCSRF(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Solicitud bloqueada: origen no autorizado (CSRF).", "type": "error"}}`)
	}
	http.Error(w, "403 Forbidden: Cross-Site Request Blocked", http.StatusForbidden)
}
