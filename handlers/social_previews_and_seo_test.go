package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestOpenGraph_DefaultAndDynamicMetaTags(t *testing.T) {
	handler, cleanup := setupTestTeamStats(t)
	defer cleanup()

	r := chi.NewRouter()
	r.Get("/teams", handler.ShowTeams)
	r.Get("/teams/{code}", handler.ShowTeamDetail)

	// 1. Test Default Open Graph Meta Tags on /teams
	reqTeams := httptest.NewRequest(http.MethodGet, "/teams?season=2026", nil)
	rrTeams := httptest.NewRecorder()
	r.ServeHTTP(rrTeams, reqTeams)

	if rrTeams.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /teams, got %d", rrTeams.Code)
	}

	bodyTeams := rrTeams.Body.String()

	expectedDefaultOG := []string{
		`<meta property="og:type" content="website">`,
		`<meta property="og:site_name" content="NFL Quiniela 2026">`,
		`<meta property="og:locale" content="es_MX">`,
		`og:title`,
		`og:description`,
		`https://quiniela.cmoreno.org/static/images/og-preview.jpg`,
		`https://quiniela.cmoreno.org/static/images/og-preview-square.jpg`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<link rel="icon" type="image/x-icon" href="/favicon.ico">`,
		`<link rel="apple-touch-icon" sizes="180x180" href="/static/icons/apple-touch-icon.png">`,
		`<link rel="manifest" href="/static/manifest.json">`,
	}

	for _, exp := range expectedDefaultOG {
		if !strings.Contains(bodyTeams, exp) {
			t.Errorf("expected /teams response to contain %q", exp)
		}
	}

	// 2. Test Dynamic Open Graph Meta Tags on Team Detail (/teams/KC)
	reqKC := httptest.NewRequest(http.MethodGet, "/teams/KC?season=2026", nil)
	rrKC := httptest.NewRecorder()
	r.ServeHTTP(rrKC, reqKC)

	if rrKC.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /teams/KC, got %d", rrKC.Code)
	}

	bodyKC := rrKC.Body.String()

	expectedKCTags := []string{
		"Chiefs",
		"Kansas City Chiefs",
		`@type": "SportsTeam"`,
		`canonical`,
		`https://quiniela.cmoreno.org/teams/KC`,
	}

	for _, exp := range expectedKCTags {
		if !strings.Contains(bodyKC, exp) {
			t.Errorf("expected /teams/KC response to contain %q", exp)
		}
	}
}

func TestOpenGraph_StaticAssetsAndWhatsAppLimits(t *testing.T) {
	// Root project directory relative to handlers
	rootDir := ".."

	assets := []struct {
		relPath string
		maxSize int64 // WhatsApp strict limit is 300KB (307200 bytes)
	}{
		{relPath: "static/images/og-preview.jpg", maxSize: 300 * 1024},
		{relPath: "static/images/og-preview.png", maxSize: 300 * 1024},
		{relPath: "static/images/og-preview-square.jpg", maxSize: 200 * 1024},
		{relPath: "static/images/og-preview-square.png", maxSize: 250 * 1024},
		{relPath: "static/favicon.ico", maxSize: 100 * 1024},
		{relPath: "static/icons/apple-touch-icon.png", maxSize: 100 * 1024},
		{relPath: "static/icons/icon-192.png", maxSize: 150 * 1024},
		{relPath: "static/icons/icon-512.png", maxSize: 300 * 1024},
		{relPath: "static/robots.txt", maxSize: 10 * 1024},
		{relPath: "static/manifest.json", maxSize: 10 * 1024},
	}

	for _, a := range assets {
		fullPath := filepath.Join(rootDir, a.relPath)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("asset %s does not exist: %v", a.relPath, err)
		}

		if info.Size() > a.maxSize {
			t.Errorf("asset %s size %d bytes exceeds max threshold %d bytes", a.relPath, info.Size(), a.maxSize)
		}

		if info.Size() == 0 {
			t.Errorf("asset %s is empty", a.relPath)
		}
	}

	// Verify robots.txt contents
	robotsBytes, err := os.ReadFile(filepath.Join(rootDir, "static/robots.txt"))
	if err != nil {
		t.Fatalf("failed to read robots.txt: %v", err)
	}
	robotsStr := string(robotsBytes)
	if !strings.Contains(robotsStr, "Allow: /") || !strings.Contains(robotsStr, "Disallow: /admin") {
		t.Errorf("robots.txt missing expected directives: %s", robotsStr)
	}

	// Verify manifest.json contents
	manifestBytes, err := os.ReadFile(filepath.Join(rootDir, "static/manifest.json"))
	if err != nil {
		t.Fatalf("failed to read manifest.json: %v", err)
	}
	manifestStr := string(manifestBytes)
	if !strings.Contains(manifestStr, "apple-touch-icon.png") || !strings.Contains(manifestStr, "icon-192.png") {
		t.Errorf("manifest.json missing icon definitions: %s", manifestStr)
	}
}
