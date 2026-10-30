package security

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// CSRFProtectionMiddleware protects state-changing requests against Cross-Site Request Forgery (CSRF).
// It verifies that incoming POST/PUT/DELETE/PATCH requests originate from the same site.
func CSRFProtectionMiddleware(appBaseURL string) func(http.Handler) http.Handler {
	allowedOrigin := normalizedOrigin(appBaseURL)

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

			if originHeader != "" {
				origin := normalizedOrigin(originHeader)
				if u, err := url.Parse(originHeader); err != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
					origin = ""
				}
				if origin == "" || origin != allowedOrigin {
					log.Printf("[CSRF] Blocked invalid Origin '%s' (target: '%s') from IP %s", originHeader, allowedOrigin, GetClientIP(r))
					rejectCSRF(w, r)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Fallback to Referer if Origin is missing
			if refererHeader != "" {
				origin := normalizedOrigin(refererHeader)
				if origin == "" || origin != allowedOrigin {
					log.Printf("[CSRF] Blocked invalid Referer '%s' (target: '%s') from IP %s", refererHeader, allowedOrigin, GetClientIP(r))
					rejectCSRF(w, r)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if r.Header.Get("Sec-Fetch-Site") != "same-origin" || allowedOrigin == "" {
				rejectCSRF(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func normalizedOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Hostname() == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

func rejectCSRF(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Trigger", `{"show-toast": {"message": "Solicitud bloqueada: origen no autorizado (CSRF).", "type": "error"}}`)
	}
	http.Error(w, "403 Forbidden: Cross-Site Request Blocked", http.StatusForbidden)
}
