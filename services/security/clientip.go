package security

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type contextKey string

const (
	ClientIPContextKey contextKey = "security_client_ip"
)

// IPNetList holds parsed CIDR blocks for trusted proxies.
type IPNetList []*net.IPNet

func (l IPNetList) Contains(ip net.IP) bool {
	for _, block := range l {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIPResolver securely resolves the client IP without trusting arbitrary spoofed headers.
type ClientIPResolver struct {
	behindProxy    bool
	trustedProxies IPNetList
}

// NewClientIPResolver initializes a resolver with explicit or default trusted proxies.
func NewClientIPResolver(trustedProxies []string, behindProxy bool) *ClientIPResolver {
	resolver := &ClientIPResolver{
		behindProxy: behindProxy,
	}

	// Default trusted loopback networks
	defaultBlocks := []string{
		"127.0.0.0/8",
		"::1/128",
	}

	// If behind a proxy in a containerized environment (Docker/Kubernetes),
	// trust private RFC 1918 subnets for reverse proxy hops by default.
	if behindProxy {
		defaultBlocks = append(defaultBlocks,
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"fc00::/7",
		)
	}

	for _, cidr := range defaultBlocks {
		if _, ipNet, err := net.ParseCIDR(cidr); err == nil {
			resolver.trustedProxies = append(resolver.trustedProxies, ipNet)
		}
	}

	// Add any explicitly user-configured trusted proxy CIDRs/IPs
	for _, proxy := range trustedProxies {
		proxy = strings.TrimSpace(proxy)
		if proxy == "" {
			continue
		}
		if !strings.Contains(proxy, "/") {
			// Bare IP: convert to single-host CIDR
			if parsed := net.ParseIP(proxy); parsed != nil {
				if parsed.To4() != nil {
					proxy += "/32"
				} else {
					proxy += "/128"
				}
			}
		}
		if _, ipNet, err := net.ParseCIDR(proxy); err == nil {
			resolver.trustedProxies = append(resolver.trustedProxies, ipNet)
		}
	}

	return resolver
}

// ResolveClientIP extracts the genuine client IP address.
func (r *ClientIPResolver) ResolveClientIP(req *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		remoteHost = req.RemoteAddr
	}
	remoteIP := net.ParseIP(strings.TrimSpace(remoteHost))
	if remoteIP == nil {
		return remoteHost
	}

	// If connection is not from a trusted proxy, do NOT trust X-Forwarded-For or X-Real-IP
	if !r.trustedProxies.Contains(remoteIP) {
		return remoteIP.String()
	}

	// Process X-Forwarded-For from right to left to avoid spoofing
	xff := req.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidateStr := strings.TrimSpace(parts[i])
			candidateIP := net.ParseIP(candidateStr)
			if candidateIP == nil {
				continue
			}
			// The first IP from the right that is not a trusted proxy is the real client IP
			if !r.trustedProxies.Contains(candidateIP) {
				return candidateIP.String()
			}
		}
		// If all were trusted or only one present
		if len(parts) > 0 {
			if first := net.ParseIP(strings.TrimSpace(parts[0])); first != nil {
				return first.String()
			}
		}
	}

	// Fallback to X-Real-IP if present
	if xri := strings.TrimSpace(req.Header.Get("X-Real-IP")); xri != "" {
		if xriIP := net.ParseIP(xri); xriIP != nil {
			return xriIP.String()
		}
	}

	return remoteIP.String()
}

// Middleware returns an http.Handler middleware that attaches the verified client IP to the request context.
func (r *ClientIPResolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		clientIP := r.ResolveClientIP(req)
		ctx := context.WithValue(req.Context(), ClientIPContextKey, clientIP)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

// GetClientIP retrieves the resolved client IP from request context or resolves it on the fly.
func GetClientIP(req *http.Request) string {
	if val, ok := req.Context().Value(ClientIPContextKey).(string); ok && val != "" {
		return val
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return req.RemoteAddr
}
