package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"nfl-quiniela-2026/db"
)

func TestSessionRevocationOnPasswordChange(t *testing.T) {
	testDBPath := filepath.Join(t.TempDir(), "test_auth_revocation.db")
	database, err := db.InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	if err := db.SeedDatabase(repo, "admin", "admin@example.com", "adminpass", 2026); err != nil {
		t.Fatalf("SeedDatabase failed: %v", err)
	}

	authService := NewAuthService(repo, "test-super-secret-key-1234")

	user, err := authService.Register("testuser", "test@example.com", "originalpassword")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 1. Generate session token for original password (token version = 1)
	originalToken := authService.GenerateSessionToken(user.ID, 24*time.Hour)

	// Validate original token with user
	req := httptest.NewRequest(http.MethodGet, "/picks", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: originalToken,
	})

	extractedUser := authService.ExtractUserFromRequest(req)
	if extractedUser == nil || extractedUser.ID != user.ID {
		t.Fatalf("Expected valid user from initial session token, got %v", extractedUser)
	}

	// 2. Change password (which must increment token_version in DB)
	newHash, err := authService.HashPassword("brandnewpassword")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if err := repo.UpdateUserPassword(user.ID, newHash); err != nil {
		t.Fatalf("UpdateUserPassword failed: %v", err)
	}

	// Verify in DB that token_version was incremented
	freshUser, err := repo.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if freshUser.TokenVersion != 2 {
		t.Fatalf("Expected TokenVersion = 2 after password update, got %d", freshUser.TokenVersion)
	}

	// 3. Old session token MUST now be invalidated (A05)
	invalidatedUser := authService.ExtractUserFromRequest(req)
	if invalidatedUser != nil {
		t.Fatalf("Expected old session token to be INVALIDATED after password change, but user was authenticated!")
	}

	// 4. Authenticate with new password and obtain new token (token version = 2)
	authUser, err := authService.Authenticate("testuser", "brandnewpassword")
	if err != nil {
		t.Fatalf("Authenticate with new password failed: %v", err)
	}

	newToken := authService.GenerateSessionToken(authUser.ID, 24*time.Hour)
	req2 := httptest.NewRequest(http.MethodGet, "/picks", nil)
	req2.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: newToken,
	})

	validUser := authService.ExtractUserFromRequest(req2)
	if validUser == nil || validUser.ID != user.ID {
		t.Fatalf("Expected new session token to authenticate successfully, got %v", validUser)
	}
}

func TestCookieSecureFlagOnHTTPS(t *testing.T) {
	testDBPath := filepath.Join(t.TempDir(), "test_auth_secure.db")
	database, err := db.InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	authService := NewAuthService(repo, "test-secret-secure-check")

	// 1. Plain HTTP request: Secure should be false
	recPlain := httptest.NewRecorder()
	reqPlain := httptest.NewRequest(http.MethodGet, "http://example.com/login", nil)
	authService.SetSessionCookie(recPlain, 100, reqPlain)

	cookiePlain := recPlain.Result().Cookies()[0]
	if cookiePlain.Secure {
		t.Errorf("Expected Secure=false for plain HTTP, got %v", cookiePlain.Secure)
	}

	// 2. Proxied HTTPS request (X-Forwarded-Proto: https): Secure MUST be true
	recProxy := httptest.NewRecorder()
	reqProxy := httptest.NewRequest(http.MethodGet, "http://example.com/login", nil)
	reqProxy.Header.Set("X-Forwarded-Proto", "https")
	authService.SetSessionCookie(recProxy, 100, reqProxy)

	cookieProxy := recProxy.Result().Cookies()[0]
	if !cookieProxy.Secure {
		t.Errorf("Expected Secure=true when X-Forwarded-Proto=https, got %v", cookieProxy.Secure)
	}

	// 3. Direct TLS connection: Secure MUST be true
	recTLS := httptest.NewRecorder()
	reqTLS := httptest.NewRequest(http.MethodGet, "https://example.com/login", nil)
	reqTLS.TLS = &tls.ConnectionState{}
	authService.SetSessionCookie(recTLS, 100, reqTLS)

	cookieTLS := recTLS.Result().Cookies()[0]
	if !cookieTLS.Secure {
		t.Errorf("Expected Secure=true for direct TLS connection, got %v", cookieTLS.Secure)
	}
}
