package handlers

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
	"nfl-quiniela-2026/services/espn"
	"nfl-quiniela-2026/services/scoring"
)

func setupTestApp(t *testing.T) (*db.Repository, *auth.AuthService, *Renderer, *scoring.Calculator, func()) {
	testDB := "test_handlers.db"
	database, err := db.InitDB("sqlite", testDB)
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}

	repo := db.NewRepository(database)
	_ = db.SeedDatabase(repo, "admin", "admin@test.com", "admin123", 2026)

	authService := auth.NewAuthService(repo, "test-secret-key-123")
	calculator := scoring.NewCalculator(repo)

	// In tests, read templates from local directory
	templatesFS := os.DirFS("../templates")
	renderer := NewRenderer(templatesFS)

	cleanup := func() {
		database.Close()
		os.Remove(testDB)
	}

	return repo, authService, renderer, calculator, cleanup
}

func TestAuthFlow(t *testing.T) {
	repo, authService, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	authHandler := NewAuthHandler(authService, renderer)

	// 1. Register User
	form := url.Values{}
	form.Set("username", "testplayer")
	form.Set("email", "player@test.com")
	form.Set("password", "secret123")

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	authHandler.HandleRegister(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("Expected status 303 SeeOther, got %d", rr.Code)
	}

	cookie := rr.Result().Header.Get("Set-Cookie")
	if !strings.Contains(cookie, auth.SessionCookieName) {
		t.Errorf("Expected session cookie in response, got %s", cookie)
	}

	// 2. Authenticate
	user, err := authService.Authenticate("testplayer", "secret123")
	if err != nil || user == nil {
		t.Fatalf("Failed to authenticate registered user: %v", err)
	}
	if user.Username != "testplayer" {
		t.Errorf("Expected username testplayer, got %s", user.Username)
	}

	// Verify User in DB
	dbUser, err := repo.GetUserByUsername("testplayer")
	if err != nil || dbUser == nil {
		t.Fatalf("User not in database: %v", err)
	}
}

func TestPicksSaveAndLockEnforcement(t *testing.T) {
	repo, authService, _, _, cleanup := setupTestApp(t)
	defer cleanup()

	// In test, use os.DirFS
	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	// Create user
	user, _ := authService.Register("picker1", "picker1@test.com", "pass123")

	// Get Season, Week, and Teams
	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 1)
	kc, _ := repo.GetTeamByCode("KC")
	bal, _ := repo.GetTeamByCode("BAL")

	// 1. Open Game (Tomorrow)
	openGame, _ := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(24 * time.Hour),
		Status:       "scheduled",
		StatusDetail: "Sun, 1:00 PM",
	})

	// Submit pick for open game
	form := url.Values{}
	form.Set("game_id", strings.TrimSpace(strconvFormat(openGame.ID)))
	form.Set("picked_team_id", strings.TrimSpace(strconvFormat(kc.ID)))

	req := httptest.NewRequest(http.MethodPost, "/picks/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(injectUser(req.Context(), user))
	rr := httptest.NewRecorder()

	picksHandler.SavePick(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d, body: %s", rr.Code, rr.Body.String())
	}

	// Verify pick in database
	savedPick, err := repo.GetUserPickForGame(user.ID, openGame.ID)
	if err != nil || savedPick == nil {
		t.Fatalf("Pick was not saved in DB: %v", err)
	}
	if *savedPick.PickedTeamID != kc.ID {
		t.Errorf("Expected picked team KC (%d), got %d", kc.ID, *savedPick.PickedTeamID)
	}

	// 2. Locked Game (Kickoff 2 hours ago)
	lockedGame, _ := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(-2 * time.Hour),
		Status:       "in_progress",
		StatusDetail: "Q2 08:30",
	})

	formLocked := url.Values{}
	formLocked.Set("game_id", strconvFormat(lockedGame.ID))
	formLocked.Set("picked_team_id", strconvFormat(bal.ID))

	reqLocked := httptest.NewRequest(http.MethodPost, "/picks/save", strings.NewReader(formLocked.Encode()))
	reqLocked.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLocked = reqLocked.WithContext(injectUser(reqLocked.Context(), user))
	rrLocked := httptest.NewRecorder()

	picksHandler.SavePick(rrLocked, reqLocked)

	// Must be rejected with 403 Forbidden!
	if rrLocked.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden for locked game, got %d", rrLocked.Code)
	}

	// 3. Test Full-Week Lock Mode
	// Enable full_week lock mode
	_ = repo.SetSetting("lock_mode", "full_week")

	// Create future Sunday game in same week where lockedGame (past kickoff) exists
	sundayGame, _ := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(48 * time.Hour), // Future game!
		Status:       "scheduled",
		StatusDetail: "Sun, 4:25 PM",
	})

	formSunday := url.Values{}
	formSunday.Set("game_id", strconvFormat(sundayGame.ID))
	formSunday.Set("picked_team_id", strconvFormat(kc.ID))

	reqSunday := httptest.NewRequest(http.MethodPost, "/picks/save", strings.NewReader(formSunday.Encode()))
	reqSunday.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSunday = reqSunday.WithContext(injectUser(reqSunday.Context(), user))
	rrSunday := httptest.NewRecorder()

	picksHandler.SavePick(rrSunday, reqSunday)

	// Under full_week lock, because the week already has a game that started (lockedGame),
	// the future Sunday game MUST BE REJECTED with 403!
	if rrSunday.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden under full_week lock when week already started, got %d", rrSunday.Code)
	}

	// Switch back to per_game lock mode: now Sunday game must succeed!
	_ = repo.SetSetting("lock_mode", "per_game")
	rrSundayPerGame := httptest.NewRecorder()
	reqSundayPerGame := httptest.NewRequest(http.MethodPost, "/picks/save", strings.NewReader(formSunday.Encode()))
	reqSundayPerGame.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSundayPerGame = reqSundayPerGame.WithContext(injectUser(reqSundayPerGame.Context(), user))

	picksHandler.SavePick(rrSundayPerGame, reqSundayPerGame)
	if rrSundayPerGame.Code != http.StatusOK {
		t.Errorf("Expected 200 OK under per_game lock for future Sunday game, got %d", rrSundayPerGame.Code)
	}
}

func TestAdminSettingsAndRecalculate(t *testing.T) {
	repo, _, _, calculator, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)
	espnClient := espn.NewClient()
	syncer := espn.NewSyncer(espnClient, repo, calculator, 2026)

	adminHandler := NewAdminHandler(repo, renderer, syncer, calculator, 2026)

	form := url.Values{}
	form.Set("scoring_mode", "pure_tiebreaker")
	form.Set("winner_points", "1")
	form.Set("exact_score_bonus", "0")
	form.Set("margin_bonus", "0")
	form.Set("lock_mode", "full_week")

	req := httptest.NewRequest(http.MethodPost, "/admin/settings/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	adminHandler.SaveSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", rr.Code)
	}

	cfg, _ := repo.GetScoringConfig()
	if cfg.ScoringMode != "pure_tiebreaker" || cfg.WinnerPoints != 1 || cfg.LockMode != "full_week" {
		t.Errorf("Expected pure_tiebreaker with 1 winner point and full_week lock, got %+v", cfg)
	}
}

// Helpers
func strconvFormat(n int64) string {
	return strconv.FormatInt(n, 10)
}

func injectUser(ctx context.Context, u *db.User) context.Context {
	return context.WithValue(ctx, auth.UserContextKey, u)
}
