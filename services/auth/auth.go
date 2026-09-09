package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nfl-quiniela-2026/db"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const (
	UserContextKey    contextKey = "authenticated_user"
	SessionCookieName            = "quiniela_session"
)

type AuthService struct {
	repo          *db.Repository
	sessionSecret []byte
}

func NewAuthService(repo *db.Repository, sessionSecret string) *AuthService {
	return &AuthService{
		repo:          repo,
		sessionSecret: []byte(sessionSecret),
	}
}

// HashPassword hashes a plain text password with bcrypt
func (s *AuthService) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash compares a hashed password with plain text
func (s *AuthService) CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Authenticate verifies username/email and password, returning the user
func (s *AuthService) Authenticate(usernameOrEmail, password string) (*db.User, error) {
	var user *db.User
	var err error

	if strings.Contains(usernameOrEmail, "@") {
		user, err = s.repo.GetUserByEmail(usernameOrEmail)
	} else {
		user, err = s.repo.GetUserByUsername(usernameOrEmail)
	}

	if err != nil || user == nil {
		return nil, errors.New("credenciales inválidas")
	}

	if !s.CheckPasswordHash(password, user.PasswordHash) {
		return nil, errors.New("credenciales inválidas")
	}

	return user, nil
}

// Register creates a new player account
func (s *AuthService) Register(username, email, password string) (*db.User, error) {
	return s.RegisterWithVerification(username, email, password, "")
}

// RegisterWithVerification creates a player account with an initial email verification token
func (s *AuthService) RegisterWithVerification(username, email, password, token string) (*db.User, error) {
	if len(strings.TrimSpace(username)) < 3 {
		return nil, errors.New("el nombre de usuario debe tener al menos 3 caracteres")
	}
	if len(password) < 6 {
		return nil, errors.New("la contraseña debe tener al menos 6 caracteres")
	}
	if !strings.Contains(email, "@") {
		return nil, errors.New("el correo electrónico no es válido")
	}

	// Check if exists
	if existing, _ := s.repo.GetUserByUsername(username); existing != nil {
		return nil, errors.New("el nombre de usuario ya está en uso")
	}
	if existing, _ := s.repo.GetUserByEmail(email); existing != nil {
		return nil, errors.New("el correo electrónico ya está registrado")
	}

	hash, err := s.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("error al cifrar la contraseña: %w", err)
	}

	return s.repo.CreateUserWithVerification(username, email, hash, "player", token)
}

// GenerateRandomToken generates cryptographically secure hex strings (e.g. for verification or password resets)
func GenerateRandomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateSessionToken creates a signed token format: "userID:expiry:signature"
func (s *AuthService) GenerateSessionToken(userID int64, duration time.Duration) string {
	expiry := time.Now().Add(duration).Unix()
	payload := fmt.Sprintf("%d:%d", userID, expiry)

	mac := hmac.New(sha256.New, s.sessionSecret)
	mac.Write([]byte(payload))
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s:%s", payload, sig)
}

// ValidateSessionToken parses and validates a signed session token
func (s *AuthService) ValidateSessionToken(token string) (int64, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		return 0, errors.New("token format invalid")
	}

	userIDStr := parts[0]
	expiryStr := parts[1]
	providedSig := parts[2]

	payload := fmt.Sprintf("%s:%s", userIDStr, expiryStr)
	mac := hmac.New(sha256.New, s.sessionSecret)
	mac.Write([]byte(payload))
	expectedSig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(providedSig), []byte(expectedSig)) {
		return 0, errors.New("token signature mismatch")
	}

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return 0, errors.New("token expired")
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return 0, errors.New("invalid user id in token")
	}

	return userID, nil
}

// SetSessionCookie sets a secure HTTP-only cookie with the session token
func (s *AuthService) SetSessionCookie(w http.ResponseWriter, userID int64) {
	token := s.GenerateSessionToken(userID, 30*24*time.Hour) // 30 days
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false, // Set to true behind TLS/HTTPS in production
	})
}

// ClearSessionCookie removes the authentication cookie
func (s *AuthService) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// ExtractUserFromRequest extracts and verifies the user from request cookies
func (s *AuthService) ExtractUserFromRequest(r *http.Request) *db.User {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}

	userID, err := s.ValidateSessionToken(cookie.Value)
	if err != nil {
		return nil
	}

	user, err := s.repo.GetUserByID(userID)
	if err != nil {
		return nil
	}

	return user
}

// Middleware: Injects authenticated user into context if present
func (s *AuthService) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := s.ExtractUserFromRequest(r)
		if user != nil {
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Middleware: Requires authenticated user
func (s *AuthService) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(UserContextKey).(*db.User)
		if !ok || user == nil {
			// Check if HTMX request
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Middleware: Requires admin role
func (s *AuthService) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(UserContextKey).(*db.User)
		if !ok || user == nil || !user.IsAdmin() {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/")
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserFromContext retrieves authenticated user from request context
func GetUserFromContext(ctx context.Context) *db.User {
	if u, ok := ctx.Value(UserContextKey).(*db.User); ok {
		return u
	}
	return nil
}
