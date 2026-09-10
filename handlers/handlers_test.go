package handlers

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
	"nfl-quiniela-2026/services/espn"
	"nfl-quiniela-2026/services/events"
	"nfl-quiniela-2026/services/notifications"
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

	authHandler := NewAuthHandler(authService, repo, nil, renderer)

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
	week, _ := repo.GetWeekByNumber(season.ID, 2)
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

func TestPicksSaveAll(t *testing.T) {
	repo, authService, _, _, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)
	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	user, _ := authService.Register("picker_batch", "batch@test.com", "pass123")
	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 2)
	kc, _ := repo.GetTeamByCode("KC")
	bal, _ := repo.GetTeamByCode("BAL")

	g1, _ := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(24 * time.Hour),
		Status:       "scheduled",
		StatusDetail: "Sun 1:00 PM",
	})
	g2, _ := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   bal.ID,
		AwayTeamID:   kc.ID,
		KickoffTime:  time.Now().Add(30 * time.Hour),
		Status:       "scheduled",
		StatusDetail: "Sun 4:25 PM",
	})

	form := url.Values{}
	form.Set("week_id", strconvFormat(week.ID))
	form.Set(fmt.Sprintf("picked_team_%d", g1.ID), strconvFormat(kc.ID))
	form.Set(fmt.Sprintf("away_score_%d", g1.ID), "21")
	form.Set(fmt.Sprintf("home_score_%d", g1.ID), "28")

	form.Set(fmt.Sprintf("picked_team_%d", g2.ID), strconvFormat(bal.ID))
	form.Set(fmt.Sprintf("away_score_%d", g2.ID), "17")
	form.Set(fmt.Sprintf("home_score_%d", g2.ID), "24")

	req := httptest.NewRequest(http.MethodPost, "/picks/save-all", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(injectUser(req.Context(), user))
	rr := httptest.NewRecorder()

	picksHandler.SaveAll(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("Expected redirect status 303 SeeOther, got %d", rr.Code)
	}

	// Verify both picks and predicted scores in database
	p1, err := repo.GetUserPickForGame(user.ID, g1.ID)
	if err != nil || p1 == nil {
		t.Fatalf("Pick 1 not saved: %v", err)
	}
	if *p1.PickedTeamID != kc.ID {
		t.Errorf("Expected pick 1 to be KC, got %d", *p1.PickedTeamID)
	}
	if p1.PredictedAwayScore == nil || *p1.PredictedAwayScore != 21 {
		t.Errorf("Expected away score 21, got %v", p1.PredictedAwayScore)
	}
	if p1.PredictedHomeScore == nil || *p1.PredictedHomeScore != 28 {
		t.Errorf("Expected home score 28, got %v", p1.PredictedHomeScore)
	}

	p2, err := repo.GetUserPickForGame(user.ID, g2.ID)
	if err != nil || p2 == nil {
		t.Fatalf("Pick 2 not saved: %v", err)
	}
	if *p2.PickedTeamID != bal.ID {
		t.Errorf("Expected pick 2 to be BAL, got %d", *p2.PickedTeamID)
	}
	if p2.PredictedAwayScore == nil || *p2.PredictedAwayScore != 17 {
		t.Errorf("Expected away score 17, got %v", p2.PredictedAwayScore)
	}
	if p2.PredictedHomeScore == nil || *p2.PredictedHomeScore != 24 {
		t.Errorf("Expected home score 24, got %v", p2.PredictedHomeScore)
	}

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "saved=1") || !strings.Contains(loc, "missing=") {
		t.Errorf("Expected redirect URL to contain saved=1 and missing param, got %s", loc)
	}

	// Test partial submission: create g3 and submit only g3 without picked team
	g3, _ := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(36 * time.Hour),
		Status:       "scheduled",
		StatusDetail: "Mon 8:15 PM",
	})
	_ = g3

	formPartial := url.Values{}
	formPartial.Set("week_id", strconvFormat(week.ID))
	formPartial.Set(fmt.Sprintf("picked_team_%d", g1.ID), strconvFormat(kc.ID))
	// g2 and g3 omitted from picked teams

	reqPartial := httptest.NewRequest(http.MethodPost, "/picks/save-all", strings.NewReader(formPartial.Encode()))
	reqPartial.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPartial = reqPartial.WithContext(injectUser(reqPartial.Context(), user))
	rrPartial := httptest.NewRecorder()

	picksHandler.SaveAll(rrPartial, reqPartial)
	locPartial := rrPartial.Header().Get("Location")
	if !strings.Contains(locPartial, "missing=2") {
		t.Errorf("Expected missing=2 in redirect URL, got %s", locPartial)
	}
}

func TestAdminSettingsAndRecalculate(t *testing.T) {
	repo, _, _, calculator, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)
	espnClient := espn.NewClient()
	broker := events.NewBroker()
	sender := notifications.NewEmailSender("", 587, "", "", "test@test.com", "http://localhost:8080")
	reminderWorker := notifications.NewReminderWorker(repo, sender, 2026)
	syncer := espn.NewSyncer(espnClient, repo, calculator, broker, 2026)

	adminHandler := NewAdminHandler(repo, renderer, syncer, calculator, broker, reminderWorker, 2026)

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

	hxTrigger := rr.Header().Get("HX-Trigger")
	if !strings.Contains(hxTrigger, "show-toast") || !strings.Contains(hxTrigger, "Reglas guardadas") {
		t.Errorf("Expected HX-Trigger with show-toast and success message, got %s", hxTrigger)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "¡Reglas guardadas exitosamente!") {
		t.Errorf("Expected feedback banner in response body, got %s", body)
	}

	cfg, _ := repo.GetScoringConfig()
	if cfg.ScoringMode != "pure_tiebreaker" || cfg.WinnerPoints != 1 || cfg.LockMode != "full_week" {
		t.Errorf("Expected pure_tiebreaker with 1 winner point and full_week lock, got %+v", cfg)
	}
}

func TestSendRemindersEndpoint(t *testing.T) {
	repo, _, renderer, calculator, cleanup := setupTestApp(t)
	defer cleanup()

	broker := events.NewBroker()
	sender := notifications.NewEmailSender("", 587, "", "", "test@test.com", "http://localhost:8080")
	reminderWorker := notifications.NewReminderWorker(repo, sender, 2026)
	espnClient := espn.NewClient()
	syncer := espn.NewSyncer(espnClient, repo, calculator, broker, 2026)
	adminHandler := NewAdminHandler(repo, renderer, syncer, calculator, broker, reminderWorker, 2026)

	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 1)

	form := url.Values{}
	form.Set("week_id", strconvFormat(week.ID))

	req := httptest.NewRequest(http.MethodPost, "/admin/reminders/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	adminHandler.SendReminders(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Éxito") {
		t.Errorf("Expected success HTML response, got: %s", rr.Body.String())
	}
}

func TestLiveEventsSSE(t *testing.T) {
	broker := events.NewBroker()
	eventsHandler := NewEventsHandler(broker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/events/live", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	go func() {
		time.Sleep(50 * time.Millisecond)
		broker.Broadcast("game-updated", `{"test": true}`)
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	eventsHandler.StreamLiveEvents(rr, req)

	output := rr.Body.String()
	if !strings.Contains(output, "event: connected") {
		t.Errorf("Expected output to contain 'event: connected', got: %s", output)
	}
	if !strings.Contains(output, "event: game-updated") {
		t.Errorf("Expected output to contain 'event: game-updated', got: %s", output)
	}
}

func TestEmailVerificationAndResetFlow(t *testing.T) {
	repo, authService, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	authHandler := NewAuthHandler(authService, repo, nil, renderer)

	// 1. Register User
	form := url.Values{}
	form.Set("username", "verifyuser")
	form.Set("email", "verify@test.com")
	form.Set("password", "secret123")

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	authHandler.HandleRegister(rr, req)

	// Retrieve user and token
	u, err := repo.GetUserByEmail("verify@test.com")
	if err != nil || u == nil {
		t.Fatalf("Failed to fetch user: %v", err)
	}
	if u.EmailVerified {
		t.Errorf("Expected EmailVerified to be false initially")
	}
	if u.VerificationToken == nil || *u.VerificationToken == "" {
		t.Fatalf("Expected non-empty VerificationToken")
	}

	token := *u.VerificationToken

	// 2. Verify with valid token
	req = httptest.NewRequest(http.MethodGet, "/verify-email?token="+token, nil)
	rr = httptest.NewRecorder()
	authHandler.HandleVerifyEmail(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK for verification, got %d", rr.Code)
	}

	// Confirm user is now verified
	u, _ = repo.GetUserByID(u.ID)
	if !u.EmailVerified {
		t.Errorf("Expected EmailVerified to be true after verification")
	}

	// 3. Request Password Reset
	form = url.Values{}
	form.Set("email", "verify@test.com")
	req = httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	authHandler.HandleForgotPassword(rr, req)

	u, _ = repo.GetUserByID(u.ID)
	if u.ResetToken == nil || *u.ResetToken == "" {
		t.Fatalf("Expected non-empty ResetToken")
	}
	resetTok := *u.ResetToken

	// 4. Reset Password
	form = url.Values{}
	form.Set("token", resetTok)
	form.Set("password", "newpassword123")
	form.Set("confirm_password", "newpassword123")
	req = httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	authHandler.HandleResetPassword(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("Expected redirect 303, got %d", rr.Code)
	}

	// 5. Authenticate with new password
	authUser, err := authService.Authenticate("verifyuser", "newpassword123")
	if err != nil || authUser == nil {
		t.Errorf("Failed to authenticate with new password: %v", err)
	}
}

func TestProfileHandler(t *testing.T) {
	repo, authService, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	profileHandler := NewProfileHandler(repo, authService, renderer)
	u, err := repo.CreateUser("profiletest", "profile@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// 1. Show profile
	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req = req.WithContext(injectUser(req.Context(), u))
	rr := httptest.NewRecorder()
	profileHandler.ShowProfile(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", rr.Code)
	}

	// 2. Update preferences
	form := url.Values{}
	form.Set("favorite_team_id", "1")
	form.Set("notify_email", "1")
	req = httptest.NewRequest(http.MethodPost, "/profile/preferences", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(injectUser(req.Context(), u))
	rr = httptest.NewRecorder()
	profileHandler.HandleUpdatePreferences(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("Expected 303 SeeOther, got %d", rr.Code)
	}

	freshUser, _ := repo.GetUserByID(u.ID)
	if freshUser.FavoriteTeamID == nil || *freshUser.FavoriteTeamID != 1 {
		t.Errorf("Expected FavoriteTeamID = 1, got %v", freshUser.FavoriteTeamID)
	}
	if !freshUser.NotifyEmail {
		t.Errorf("Expected NotifyEmail = true")
	}
}

// Helpers
func strconvFormat(n int64) string {
	return strconv.FormatInt(n, 10)
}

func injectUser(ctx context.Context, u *db.User) context.Context {
	return context.WithValue(ctx, auth.UserContextKey, u)
}

func TestWeek1GracePeriodException(t *testing.T) {
	repo, authService, _, _, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)
	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	user, _ := authService.Register("grace_tester", "grace@test.com", "pass123")
	season, _ := repo.GetActiveSeason(2026)
	week1, _ := repo.GetWeekByNumber(season.ID, 1)

	// Fetch week 1 games
	week1Games, err := repo.ListGamesByWeek(week1.ID)
	if err != nil || len(week1Games) == 0 {
		t.Fatalf("Expected seeded Week 1 games: %v", err)
	}

	// First game (NE vs SEA) had kickoff earlier tonight
	g1 := week1Games[0]
	if !time.Now().After(g1.KickoffTime) {
		// If test runs in time before kickoff, this is still valid
		t.Logf("Game 1 kickoff: %v, now: %v", g1.KickoffTime, time.Now())
	}

	// Even under full_week lock mode, Week 1 games MUST be pickable due to the grace period!
	_ = repo.SetSetting("lock_mode", "full_week")

	form := url.Values{}
	form.Set("game_id", strconvFormat(g1.ID))
	form.Set("picked_team_id", strconvFormat(g1.HomeTeamID))

	req := httptest.NewRequest(http.MethodPost, "/picks/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(injectUser(req.Context(), user))
	rr := httptest.NewRecorder()

	picksHandler.SavePick(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for Week 1 game under grace period, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify pick was saved
	pick, err := repo.GetUserPickForGame(user.ID, g1.ID)
	if err != nil || pick == nil {
		t.Fatalf("Pick was not saved under Week 1 grace period: %v", err)
	}
	if *pick.PickedTeamID != g1.HomeTeamID {
		t.Errorf("Expected picked team %d, got %d", g1.HomeTeamID, *pick.PickedTeamID)
	}

	// Verify ShowPicks renders Week 1 templates with no template execution errors
	reqShow := httptest.NewRequest(http.MethodGet, "/picks?week=1", nil)
	reqShow = reqShow.WithContext(injectUser(reqShow.Context(), user))
	rrShow := httptest.NewRecorder()
	picksHandler.ShowPicks(rrShow, reqShow)

	if rrShow.Code != http.StatusOK {
		t.Errorf("Expected 200 OK rendering ShowPicks Week 1, got %d: %s", rrShow.Code, rrShow.Body.String())
	}
	if !strings.Contains(rrShow.Body.String(), "Prórroga") {
		t.Errorf("Expected ShowPicks Week 1 to include Prórroga banner or badge")
	}
}

func TestAdminUserManagementAndOverride(t *testing.T) {
	repo, authService, _, _, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)

	emailSender := notifications.NewEmailSender("", 587, "", "", "noreply@test.com", "http://localhost:8080")
	reminderWorker := notifications.NewReminderWorker(repo, emailSender, 2026)
	scoringCalc := scoring.NewCalculator(repo)
	adminHandler := NewAdminHandler(repo, renderer, nil, scoringCalc, nil, reminderWorker, 2026)

	adminUser, _ := authService.Register("superadmin", "superadmin@test.com", "pass123")
	_ = repo.SetUserRole(adminUser.ID, "admin")
	adminUser, _ = repo.GetUserByID(adminUser.ID)

	playerUser, _ := authService.Register("playerjoe", "joe@test.com", "pass123")

	season, _ := repo.GetActiveSeason(2026)
	weeks, _ := repo.ListWeeks(season.ID)
	week1 := weeks[0]
	games, _ := repo.ListGamesByWeek(week1.ID)

	// 1. Test VerifyUserEmail
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userId", strconvFormat(playerUser.ID))
	reqVerify := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/users/%d/verify-email", playerUser.ID), nil)
	reqVerify = reqVerify.WithContext(context.WithValue(reqVerify.Context(), chi.RouteCtxKey, rctx))
	reqVerify = reqVerify.WithContext(injectUser(reqVerify.Context(), adminUser))
	rrVerify := httptest.NewRecorder()

	adminHandler.VerifyUserEmail(rrVerify, reqVerify)
	if rrVerify.Code != http.StatusOK {
		t.Errorf("Expected 200 OK verifying email, got %d", rrVerify.Code)
	}
	uCheck, _ := repo.GetUserByID(playerUser.ID)
	if !uCheck.EmailVerified {
		t.Errorf("Expected user email to be verified in DB")
	}

	// 2. Test ToggleUserRole
	reqRole := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/users/%d/toggle-role", playerUser.ID), nil)
	reqRole = reqRole.WithContext(context.WithValue(reqRole.Context(), chi.RouteCtxKey, rctx))
	reqRole = reqRole.WithContext(injectUser(reqRole.Context(), adminUser))
	rrRole := httptest.NewRecorder()

	adminHandler.ToggleUserRole(rrRole, reqRole)
	if rrRole.Code != http.StatusOK {
		t.Errorf("Expected 200 OK toggling role, got %d", rrRole.Code)
	}
	uRoleCheck, _ := repo.GetUserByID(playerUser.ID)
	if uRoleCheck.Role != "admin" {
		t.Errorf("Expected playerjoe to become admin, got %s", uRoleCheck.Role)
	}

	// 3. Test ShowUserPicks Modal
	reqPicks := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/admin/users/%d/picks?week_id=%d", playerUser.ID, week1.ID), nil)
	reqPicks = reqPicks.WithContext(context.WithValue(reqPicks.Context(), chi.RouteCtxKey, rctx))
	reqPicks = reqPicks.WithContext(injectUser(reqPicks.Context(), adminUser))
	rrPicks := httptest.NewRecorder()

	adminHandler.ShowUserPicks(rrPicks, reqPicks)
	if rrPicks.Code != http.StatusOK {
		t.Errorf("Expected 200 OK showing user picks modal, got %d: %s", rrPicks.Code, rrPicks.Body.String())
	}
	if !strings.Contains(rrPicks.Body.String(), "playerjoe") {
		t.Errorf("Expected modal body to contain playerjoe")
	}

	// 4. Test SaveUserPicks (Admin Override)
	formSave := url.Values{}
	formSave.Set("week_id", strconvFormat(week1.ID))
	if len(games) > 0 {
		g := games[0]
		formSave.Set(fmt.Sprintf("picked_team_%d", g.ID), strconvFormat(g.HomeTeamID))
		formSave.Set(fmt.Sprintf("home_score_%d", g.ID), "24")
		formSave.Set(fmt.Sprintf("away_score_%d", g.ID), "17")
	}

	reqSave := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/users/%d/picks/save", playerUser.ID), strings.NewReader(formSave.Encode()))
	reqSave.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSave = reqSave.WithContext(context.WithValue(reqSave.Context(), chi.RouteCtxKey, rctx))
	reqSave = reqSave.WithContext(injectUser(reqSave.Context(), adminUser))
	rrSave := httptest.NewRecorder()

	adminHandler.SaveUserPicks(rrSave, reqSave)
	if rrSave.Code != http.StatusOK {
		t.Errorf("Expected 200 OK saving user picks, got %d", rrSave.Code)
	}

	// Verify pick in DB
	if len(games) > 0 {
		p, err := repo.GetUserPickForGame(playerUser.ID, games[0].ID)
		if err != nil || p == nil {
			t.Fatalf("Pick was not saved in DB for user: %v", err)
		}
		if *p.PickedTeamID != games[0].HomeTeamID {
			t.Errorf("Expected picked team %d, got %d", games[0].HomeTeamID, *p.PickedTeamID)
		}
		if p.PredictedHomeScore == nil || *p.PredictedHomeScore != 24 {
			t.Errorf("Expected home score 24, got %v", p.PredictedHomeScore)
		}
	}

	// 5. Test ExportWeekPicksCSV
	reqExport := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/admin/picks/export?week=%d", week1.WeekNumber), nil)
	reqExport = reqExport.WithContext(injectUser(reqExport.Context(), adminUser))
	rrExport := httptest.NewRecorder()

	adminHandler.ExportWeekPicksCSV(rrExport, reqExport)
	if rrExport.Code != http.StatusOK {
		t.Errorf("Expected 200 OK exporting CSV, got %d", rrExport.Code)
	}
	if !strings.Contains(rrExport.Header().Get("Content-Type"), "text/csv") {
		t.Errorf("Expected text/csv content type, got %s", rrExport.Header().Get("Content-Type"))
	}
	if !strings.Contains(rrExport.Body.String(), "playerjoe") {
		t.Errorf("Expected CSV output to contain playerjoe")
	}

	// 6. Test SendReminders (Grace Period)
	formReminder := url.Values{}
	formReminder.Set("week_id", strconvFormat(week1.ID))
	formReminder.Set("reminder_type", "grace_period")
	reqRemind := httptest.NewRequest(http.MethodPost, "/admin/reminders/send", strings.NewReader(formReminder.Encode()))
	reqRemind.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqRemind = reqRemind.WithContext(injectUser(reqRemind.Context(), adminUser))
	rrRemind := httptest.NewRecorder()

	adminHandler.SendReminders(rrRemind, reqRemind)
	if rrRemind.Code != http.StatusOK {
		t.Errorf("Expected 200 OK sending reminders, got %d", rrRemind.Code)
	}
	if !strings.Contains(rrRemind.Body.String(), "prórroga") {
		t.Errorf("Expected response to mention prórroga")
	}
}
