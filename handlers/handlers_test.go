package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime/multipart"
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

func TestScoreWinnerInferenceAndCompletion(t *testing.T) {
	repo, authService, _, _, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)
	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	user, _ := authService.Register("score_picker", "score_picker@test.com", "pass123")
	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 3)
	kc, _ := repo.GetTeamByCode("KC")
	bal, _ := repo.GetTeamByCode("BAL")

	// Create a regular game and a tiebreaker game
	gRegular, _ := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(24 * time.Hour),
		Status:       "scheduled",
		StatusDetail: "Sun 1:00 PM",
		IsTiebreaker: false,
	})
	gTiebreaker, _ := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(48 * time.Hour),
		Status:       "scheduled",
		StatusDetail: "Mon 8:15 PM",
		IsTiebreaker: true,
	})

	// Case 1: Submit scores only for gRegular (Away=31, Home=20 -> Away BAL should be inferred winner)
	// For gTiebreaker, submit only picked team (KC) without scores (incomplete tiebreaker)
	form := url.Values{}
	form.Set("week_id", strconvFormat(week.ID))
	// gRegular: NO picked_team parameter sent! Only scores!
	form.Set(fmt.Sprintf("away_score_%d", gRegular.ID), "31")
	form.Set(fmt.Sprintf("home_score_%d", gRegular.ID), "20")
	// gTiebreaker: picked team sent, but scores missing
	form.Set(fmt.Sprintf("picked_team_%d", gTiebreaker.ID), strconvFormat(kc.ID))

	req := httptest.NewRequest(http.MethodPost, "/picks/save-all", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(injectUser(req.Context(), user))
	rr := httptest.NewRecorder()

	picksHandler.SaveAll(rr, req)

	// In gRegular, BAL won (31 > 20). In DB, picked_team_id should be BAL.ID!
	pReg, err := repo.GetUserPickForGame(user.ID, gRegular.ID)
	if err != nil || pReg == nil {
		t.Fatalf("Regular game pick not saved: %v", err)
	}
	if pReg.PickedTeamID == nil || *pReg.PickedTeamID != bal.ID {
		t.Fatalf("Expected inferred winner to be BAL (%d), got %v", bal.ID, pReg.PickedTeamID)
	}
	if !pReg.IsCompleteForGame(gRegular) {
		t.Errorf("Expected regular game pick to be complete")
	}

	// Tiebreaker game is missing scores, so it should NOT be complete
	pTb, _ := repo.GetUserPickForGame(user.ID, gTiebreaker.ID)
	if pTb == nil {
		t.Fatalf("Tiebreaker pick not saved")
	}
	if pTb.IsCompleteForGame(gTiebreaker) {
		t.Errorf("Expected tiebreaker game pick to be incomplete because scores are missing")
	}
	if !pTb.HasMissingTiebreakerScores(gTiebreaker) {
		t.Errorf("Expected HasMissingTiebreakerScores to be true")
	}

	// Case 2: Now provide scores for tiebreaker game via SaveScore
	formScore := url.Values{}
	formScore.Set("game_id", strconvFormat(gTiebreaker.ID))
	formScore.Set("away_score", "17")
	formScore.Set("home_score", "24")

	reqScore := httptest.NewRequest(http.MethodPost, "/picks/save-score", strings.NewReader(formScore.Encode()))
	reqScore.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqScore = reqScore.WithContext(injectUser(reqScore.Context(), user))
	rrScore := httptest.NewRecorder()

	picksHandler.SaveScore(rrScore, reqScore)
	if rrScore.Code != http.StatusOK {
		t.Errorf("Expected SaveScore 200 OK, got %d", rrScore.Code)
	}

	// Now tiebreaker has both winner and scores, so it should be complete
	pTbUpdated, _ := repo.GetUserPickForGame(user.ID, gTiebreaker.ID)
	if !pTbUpdated.IsCompleteForGame(gTiebreaker) {
		t.Errorf("Expected tiebreaker pick to be complete after adding scores")
	}

	// Case 3: ShowPicks should render both as complete and PicksCount should be 2
	reqShow := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks?week=%d", week.WeekNumber), nil)
	reqShow = reqShow.WithContext(injectUser(reqShow.Context(), user))
	rrShow := httptest.NewRecorder()

	picksHandler.ShowPicks(rrShow, reqShow)
	if rrShow.Code != http.StatusOK {
		t.Errorf("Expected ShowPicks 200 OK, got %d", rrShow.Code)
	}
	body := rrShow.Body.String()
	// Should show 2 / 2 Pronosticados
	if !strings.Contains(body, "2 <span class=\"text-xs text-zinc-500 font-normal\">/ 2</span>") {
		t.Errorf("Expected ShowPicks to show 2 / 2 Pronosticados in body")
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

	testUploads := t.TempDir()
	profileHandler := NewProfileHandler(repo, authService, renderer, testUploads)
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

	// 2. Update preferences including customizations
	form := url.Values{}
	form.Set("favorite_team_id", "1")
	form.Set("notify_email", "1")
	form.Set("avatar_url", "https://a.espncdn.com/i/teamlogos/nfl/500/dal.png")
	form.Set("bio", "¡Esta temporada es de los Cowboys! Vamos con todo por el campeonato.")
	form.Set("featured_badge_code", "season_champion") // Not yet earned! Should be cleared

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
	if freshUser.AvatarURL != "https://a.espncdn.com/i/teamlogos/nfl/500/dal.png" {
		t.Errorf("Expected AvatarURL updated, got %s", freshUser.AvatarURL)
	}
	if len([]rune(freshUser.Bio)) > 60 {
		t.Errorf("Expected bio length <= 60, got %d", len([]rune(freshUser.Bio)))
	}
	if freshUser.FeaturedBadgeCode != "" {
		t.Errorf("Expected unearned featured badge to be rejected, got %s", freshUser.FeaturedBadgeCode)
	}

	// Now unlock an achievement and equip it
	_, _ = repo.AwardAchievement(u.ID, "first_blood", "Primer Acierto", "Tu primer pick correcto", "🎯", nil)
	form.Set("featured_badge_code", "first_blood")
	req = httptest.NewRequest(http.MethodPost, "/profile/preferences", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(injectUser(req.Context(), u))
	rr = httptest.NewRecorder()
	profileHandler.HandleUpdatePreferences(rr, req)

	freshUser, _ = repo.GetUserByID(u.ID)
	if freshUser.FeaturedBadgeCode != "first_blood" {
		t.Errorf("Expected FeaturedBadgeCode = first_blood, got %s", freshUser.FeaturedBadgeCode)
	}

	// 3. Show profile again with full customizations (favorite team, bio, badge) to ensure template renders cleanly
	req = httptest.NewRequest(http.MethodGet, "/profile", nil)
	req = req.WithContext(injectUser(req.Context(), freshUser))
	rr = httptest.NewRecorder()
	profileHandler.ShowProfile(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK rendering customized profile, got %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "error calling eq") {
		t.Errorf("Template rendering error detected in profile: %s", body)
	}
}

func TestHandleUploadAvatar(t *testing.T) {
	repo, authService, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	testUploads := t.TempDir()
	profileHandler := NewProfileHandler(repo, authService, renderer, testUploads)
	u, err := repo.CreateUser("uploader", "uploader@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// 1. Unauthorized request
	req := httptest.NewRequest(http.MethodPost, "/profile/avatar/upload", nil)
	rr := httptest.NewRecorder()
	profileHandler.HandleUploadAvatar(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got %d", rr.Code)
	}

	// 2. Upload invalid text file
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("avatar", "test.txt")
	_, _ = part.Write([]byte("this is plain text not an image"))
	_ = mw.Close()

	req = httptest.NewRequest(http.MethodPost, "/profile/avatar/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(injectUser(req.Context(), u))
	rr = httptest.NewRecorder()
	profileHandler.HandleUploadAvatar(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for non-image, got %d", rr.Code)
	}

	// 3. Upload valid PNG image
	// Valid minimal 1x1 transparent PNG
	pngData := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}

	buf.Reset()
	mw = multipart.NewWriter(&buf)
	part, _ = mw.CreateFormFile("avatar", "avatar.png")
	_, _ = part.Write(pngData)
	_ = mw.Close()

	req = httptest.NewRequest(http.MethodPost, "/profile/avatar/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(injectUser(req.Context(), u))
	rr = httptest.NewRecorder()
	profileHandler.HandleUploadAvatar(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for valid PNG upload, got %d: %s", rr.Code, rr.Body.String())
	}

	var res map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to parse upload JSON response: %v", err)
	}
	avatarURL, ok := res["avatar_url"].(string)
	if !ok || !strings.HasPrefix(avatarURL, "/uploads/avatars/avatar_") {
		t.Errorf("Unexpected avatar_url in response: %v", res["avatar_url"])
	}

	// 4. Test HandleUpdatePreferences with direct multipart avatar_file upload
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	_ = mw.WriteField("favorite_team_id", "1")
	_ = mw.WriteField("bio", "Upload test bio")
	part, _ = mw.CreateFormFile("avatar_file", "direct.png")
	_, _ = part.Write(pngData)
	_ = mw.Close()

	req = httptest.NewRequest(http.MethodPost, "/profile/preferences", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(injectUser(req.Context(), u))
	rr = httptest.NewRecorder()
	profileHandler.HandleUpdatePreferences(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("Expected 303 SeeOther, got %d", rr.Code)
	}

	updatedUser, _ := repo.GetUserByID(u.ID)
	if !strings.HasPrefix(updatedUser.AvatarURL, "/uploads/avatars/avatar_") {
		t.Errorf("Expected user AvatarURL updated via multipart form, got %s", updatedUser.AvatarURL)
	}
	if updatedUser.Bio != "Upload test bio" {
		t.Errorf("Expected Bio updated, got %s", updatedUser.Bio)
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
	origDeadline := db.Week1GraceDeadline
	db.Week1GraceDeadline = time.Now().Add(24 * time.Hour)
	defer func() { db.Week1GraceDeadline = origDeadline }()

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
	if !strings.Contains(rrShow.Body.String(), "shareable-week-summary-card") {
		t.Errorf("Expected ShowPicks Week 1 to include shareable-week-summary-card")
	}
	if !strings.Contains(rrShow.Body.String(), "Compacta") {
		t.Errorf("Expected ShowPicks Week 1 to include Compacta view switcher")
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

	// Toggle back to player to ensure dual-way toggle works and playerjoe remains a competitor
	rrRole2 := httptest.NewRecorder()
	adminHandler.ToggleUserRole(rrRole2, reqRole)
	if rrRole2.Code != http.StatusOK {
		t.Errorf("Expected 200 OK toggling role back to player, got %d", rrRole2.Code)
	}
	uRoleCheck2, _ := repo.GetUserByID(playerUser.ID)
	if uRoleCheck2.Role != "player" {
		t.Errorf("Expected playerjoe to become player again, got %s", uRoleCheck2.Role)
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

func TestCommunityPicksAndHeadToHeadHandler(t *testing.T) {
	repo, authService, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	user1, _ := authService.Register("player_one", "p1@test.com", "pass123")
	user2, _ := authService.Register("player_two", "p2@test.com", "pass123")

	season, _ := repo.GetActiveSeason(2026)
	weeks, _ := repo.ListWeeks(season.ID)
	week1 := weeks[0]
	games, _ := repo.ListGamesByWeek(week1.ID)
	g1 := games[0]

	// Save picks for user1 and user2
	_, _ = repo.SavePick(user1.ID, g1.ID, &g1.HomeTeamID, nil, nil)
	_, _ = repo.SavePick(user2.ID, g1.ID, &g1.AwayTeamID, nil, nil)

	// 1. Test ComparePicks endpoint
	reqCompare := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks/compare?rival_id=%d&week_id=%d", user2.ID, week1.ID), nil)
	reqCompare = reqCompare.WithContext(injectUser(reqCompare.Context(), user1))
	rrCompare := httptest.NewRecorder()

	picksHandler.ComparePicks(rrCompare, reqCompare)
	if rrCompare.Code != http.StatusOK {
		t.Errorf("Expected 200 OK from ComparePicks, got %d: %s", rrCompare.Code, rrCompare.Body.String())
	}
	bodyCompare := rrCompare.Body.String()
	if !strings.Contains(bodyCompare, "Duelo Cara a Cara") {
		t.Errorf("Expected ComparePicks to contain 'Duelo Cara a Cara'")
	}
	if !strings.Contains(bodyCompare, "player_two") {
		t.Errorf("Expected ComparePicks to contain rival username 'player_two'")
	}
	if !strings.Contains(bodyCompare, "Duelo Directo") {
		t.Errorf("Expected divergent game to be marked with 'Duelo Directo'")
	}

	// 2. Test CommunityPicks for a game locked or finalized
	_ = repo.ToggleGameLock(g1.ID, true) // force lock
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("gameId", strconvFormat(g1.ID))

	reqComm := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks/community/%d", g1.ID), nil)
	reqComm = reqComm.WithContext(context.WithValue(reqComm.Context(), chi.RouteCtxKey, rctx))
	reqComm = reqComm.WithContext(injectUser(reqComm.Context(), user1))
	rrComm := httptest.NewRecorder()

	picksHandler.CommunityPicks(rrComm, reqComm)
	if rrComm.Code != http.StatusOK {
		t.Errorf("Expected 200 OK from CommunityPicks, got %d: %s", rrComm.Code, rrComm.Body.String())
	}
	bodyComm := rrComm.Body.String()
	if !strings.Contains(bodyComm, "Distribución de") {
		t.Errorf("Expected CommunityPicks to contain 'Distribución de'")
	}
	if !strings.Contains(bodyComm, "player_two") {
		t.Errorf("Expected CommunityPicks to list 'player_two'")
	}
}

func TestLiveHandlerRendering(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	liveHandler := NewLiveHandler(repo, renderer, nil, 2026)

	// 1. Test ShowLive unauthenticated
	req1 := httptest.NewRequest(http.MethodGet, "/live", nil)
	rr1 := httptest.NewRecorder()
	liveHandler.ShowLive(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ShowLive unauthenticated, got %d: %s", rr1.Code, rr1.Body.String())
	}
	if !strings.Contains(rr1.Body.String(), "Game Center en Vivo") {
		t.Errorf("Expected ShowLive to contain 'Game Center en Vivo'")
	}
	if !strings.Contains(rr1.Body.String(), "Simulador What-If") {
		t.Errorf("Expected ShowLive to contain 'Simulador What-If'")
	}
	if !strings.Contains(rr1.Body.String(), "Mejor para:") {
		t.Errorf("Expected ShowLive to contain 'Mejor para:'")
	}
	if !strings.Contains(rr1.Body.String(), "Peor para:") {
		t.Errorf("Expected ShowLive to contain 'Peor para:'")
	}

	// 2. Test LiveContent partial unauthenticated
	req2 := httptest.NewRequest(http.MethodGet, "/live/content", nil)
	rr2 := httptest.NewRecorder()
	liveHandler.LiveContent(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from LiveContent partial unauthenticated, got %d: %s", rr2.Code, rr2.Body.String())
	}
	if !strings.Contains(rr2.Body.String(), "live-content-container") {
		t.Errorf("Expected LiveContent to contain 'live-content-container'")
	}
	if !strings.Contains(rr2.Body.String(), "whatif-payload-data") {
		t.Errorf("Expected LiveContent to contain 'whatif-payload-data'")
	}
	if !strings.Contains(rr2.Body.String(), "Compacta") {
		t.Errorf("Expected LiveContent to contain 'Compacta'")
	}

	// 3. Test with authenticated user and picks
	user, _ := repo.GetUserByUsername("admin")
	season, _ := repo.GetActiveSeason(2026)
	week1, _ := repo.GetWeekByNumber(season.ID, 1)
	games, _ := repo.ListGamesByWeek(week1.ID)
	if len(games) > 0 {
		_, _ = repo.SavePick(user.ID, games[0].ID, &games[0].HomeTeamID, nil, nil)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/live/content?week=1", nil)
	req3 = req3.WithContext(injectUser(req3.Context(), user))
	rr3 := httptest.NewRecorder()
	liveHandler.LiveContent(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from LiveContent authenticated, got %d: %s", rr3.Code, rr3.Body.String())
	}
	if !strings.Contains(rr3.Body.String(), "Puntos Confirmados") {
		t.Errorf("Expected LiveContent authenticated to contain 'Puntos Confirmados'")
	}
}

func TestLeaderboardAndRulesRendering(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	leaderboardHandler := NewLeaderboardHandler(repo, renderer, 2026)
	rulesHandler := NewRulesHandler(repo, renderer)

	// 1. Test ShowLeaderboard (defaults to weekly mode and active week)
	req1 := httptest.NewRequest(http.MethodGet, "/leaderboard", nil)
	rr1 := httptest.NewRecorder()
	leaderboardHandler.ShowLeaderboard(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ShowLeaderboard, got %d: %s", rr1.Code, rr1.Body.String())
	}
	body1 := rr1.Body.String()
	if !strings.Contains(body1, "Tabla de Posiciones") {
		t.Errorf("Expected ShowLeaderboard to contain 'Tabla de Posiciones'")
	}
	if !strings.Contains(body1, "Compartir") {
		t.Errorf("Expected ShowLeaderboard to contain 'Compartir'")
	}
	if !strings.Contains(body1, "shareable-leaderboard-card") {
		t.Errorf("Expected ShowLeaderboard to contain 'shareable-leaderboard-card'")
	}
	// Default must be weekly of active week (Semana 1)
	if !strings.Contains(body1, "Semana 1") {
		t.Errorf("Expected default ShowLeaderboard to show active week (Semana 1)")
	}
	if !strings.Contains(body1, "Desempate MNF") {
		t.Errorf("Expected default ShowLeaderboard to include weekly tiebreaker 'Desempate MNF'")
	}

	// 1b. Test ShowLeaderboard with explicit mode=season
	req1Season := httptest.NewRequest(http.MethodGet, "/leaderboard?mode=season", nil)
	rr1Season := httptest.NewRecorder()
	leaderboardHandler.ShowLeaderboard(rr1Season, req1Season)
	if rr1Season.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ShowLeaderboard mode=season, got %d: %s", rr1Season.Code, rr1Season.Body.String())
	}
	body1Season := rr1Season.Body.String()
	if !strings.Contains(body1Season, "Temporada Regular 2026") {
		t.Errorf("Expected season mode to display 'Temporada Regular 2026'")
	}

	// 2. Test LeaderboardTable partial (defaults to weekly)
	req2Default := httptest.NewRequest(http.MethodGet, "/leaderboard/table", nil)
	rr2Default := httptest.NewRecorder()
	leaderboardHandler.LeaderboardTable(rr2Default, req2Default)
	if rr2Default.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from LeaderboardTable default, got %d: %s", rr2Default.Code, rr2Default.Body.String())
	}
	body2Default := rr2Default.Body.String()
	if !strings.Contains(body2Default, "Desempate MNF") {
		t.Errorf("Expected default LeaderboardTable partial to include 'Desempate MNF'")
	}

	// 2b. Test LeaderboardTable partial with mode=season
	req2 := httptest.NewRequest(http.MethodGet, "/leaderboard/table?mode=season", nil)
	rr2 := httptest.NewRecorder()
	leaderboardHandler.LeaderboardTable(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from LeaderboardTable, got %d: %s", rr2.Code, rr2.Body.String())
	}
	body2 := rr2.Body.String()
	if !strings.Contains(body2, "md:hidden") {
		t.Errorf("Expected LeaderboardTable to contain mobile view 'md:hidden'")
	}
	if !strings.Contains(body2, "hidden md:block") {
		t.Errorf("Expected LeaderboardTable to contain desktop view 'hidden md:block'")
	}
	if strings.Contains(body2, "Desempate MNF") {
		t.Errorf("Expected season mode LeaderboardTable partial NOT to contain 'Desempate MNF'")
	}

	// 3. Test ShowRules
	req3 := httptest.NewRequest(http.MethodGet, "/rules", nil)
	rr3 := httptest.NewRecorder()
	rulesHandler.ShowRules(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ShowRules, got %d: %s", rr3.Code, rr3.Body.String())
	}
	body3 := rr3.Body.String()
	if !strings.Contains(body3, "Reglamento") {
		t.Errorf("Expected ShowRules to contain 'Reglamento'")
	}
	if !strings.Contains(body3, "Prórroga Oficial de Registro en Semana 1") {
		t.Errorf("Expected ShowRules to contain 'Prórroga Oficial de Registro en Semana 1'")
	}
}

func TestAdminRecalculateScores(t *testing.T) {
	repo, _, renderer, calculator, cleanup := setupTestApp(t)
	defer cleanup()

	broker := events.NewBroker()
	adminHandler := NewAdminHandler(repo, renderer, nil, calculator, broker, nil, 2026)

	req := httptest.NewRequest(http.MethodPost, "/admin/recalculate", nil)
	rr := httptest.NewRecorder()

	adminHandler.RecalculateScores(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from RecalculateScores, got %d: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "recalculadas exitosamente") {
		t.Errorf("Expected body to contain 'recalculadas exitosamente', got: %s", body)
	}

	hxTrigger := rr.Header().Get("HX-Trigger")
	if !strings.Contains(hxTrigger, "show-toast") {
		t.Errorf("Expected HX-Trigger header with toast notification, got: %s", hxTrigger)
	}
}

func TestPicksMatrixHandler(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	user, err := repo.CreateUser("matrix_tester", "matrix@test.com", "secret123", "player")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	_ = repo.SetUserBetaTester(user.ID, true)
	user, _ = repo.GetUserByID(user.ID)

	// 1. GET /picks/matrix as full page
	req := httptest.NewRequest(http.MethodGet, "/picks/matrix?week=1", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	picksHandler.ShowPicksMatrix(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ShowPicksMatrix, got %d: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Matriz de Pronósticos") {
		t.Errorf("Expected page to contain 'Matriz de Pronósticos'")
	}
	if !strings.Contains(body, "matrix-table-container") {
		t.Errorf("Expected page to contain 'matrix-table-container'")
	}
	if !strings.Contains(body, "matrixFilter") {
		t.Errorf("Expected page to contain matrixFilter")
	}

	// 2. GET /picks/matrix as HTMX partial
	reqHTMX := httptest.NewRequest(http.MethodGet, "/picks/matrix?week=1", nil)
	reqHTMX.Header.Set("HX-Request", "true")
	reqHTMX = reqHTMX.WithContext(ctx)

	rrHTMX := httptest.NewRecorder()
	picksHandler.ShowPicksMatrix(rrHTMX, reqHTMX)

	if rrHTMX.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from HTMX ShowPicksMatrix, got %d", rrHTMX.Code)
	}
	bodyHTMX := rrHTMX.Body.String()
	if !strings.Contains(bodyHTMX, "sticky left-0") {
		t.Errorf("Expected partial to contain sticky mobile column 'sticky left-0'")
	}
	if !strings.Contains(bodyHTMX, "Consenso") {
		t.Errorf("Expected partial to contain 'Consenso'")
	}
	if !strings.Contains(bodyHTMX, "matchesRow") {
		t.Errorf("Expected partial to contain 'matchesRow'")
	}
}

func TestComparePicksHandler(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	userA, _ := repo.CreateUser("user_a", "a@test.com", "pass", "player")
	userB, _ := repo.CreateUser("user_b", "b@test.com", "pass", "player")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks/compare?rival_id=%d", userB.ID), nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, userA)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	picksHandler.ComparePicks(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ComparePicks, got %d: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Duelo Cara a Cara") {
		t.Errorf("Expected modal to contain 'Duelo Cara a Cara'")
	}
	if !strings.Contains(body, userB.Username) {
		t.Errorf("Expected modal to contain rival username '%s'", userB.Username)
	}
}

func TestGameCardCommunityButtonHasButtonType(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)
	user, _ := repo.CreateUser("test_card_user", "card@test.com", "pass", "player")

	req := httptest.NewRequest(http.MethodGet, "/picks?week=1", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	picksHandler.ShowPicks(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ShowPicks, got %d", rr.Code)
	}

	body := rr.Body.String()
	// Verify that any button with hx-get="/picks/community/" has type="button" to prevent accidental form submission
	if strings.Contains(body, "hx-get=\"/picks/community/") {
		if !strings.Contains(body, "<button type=\"button\"") {
			t.Errorf("Expected community picks button to have explicit type=\"button\"")
		}
		if !strings.Contains(body, "@click.prevent") {
			t.Errorf("Expected community picks button to prevent click propagation")
		}
	}
}

func TestGameStatsModalLivePolling(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	liveHandler := NewLiveHandler(repo, renderer, nil, 2026)
	user, _ := repo.CreateUser("stats_user", "stats@test.com", "pass", "player")

	games, err := repo.ListGamesByWeek(1)
	if err != nil || len(games) == 0 {
		t.Fatalf("Failed to list games: %v", err)
	}
	g := games[0]

	homeScore := 10
	awayScore := 7
	newG, err := repo.CreateManualGame(&db.Game{
		WeekID:       1,
		ESPNGameID:   "",
		HomeTeamID:   g.HomeTeamID,
		AwayTeamID:   g.AwayTeamID,
		KickoffTime:  time.Now(),
		HomeScore:    &homeScore,
		AwayScore:    &awayScore,
		Status:       "in_progress",
		StatusDetail: "9:45 - 2nd",
	})
	if err != nil || newG == nil {
		t.Fatalf("CreateManualGame failed: %v", err)
	}

	statsJSON := `{"has_stats":true,"has_player_stats":true,"away_stats":{"team_code":"BAL","total_yards":"210"},"home_stats":{"team_code":"KC","total_yards":"195"},"away_player_stats":{"team_code":"BAL","categories":[{"name":"passing","title":"Pase","labels":["C/ATT","YDS","TD","INT","QBR"],"players":[{"name":"L. Jackson","jersey":"8","position":"QB","headshot_url":"","stats":["14/19","180","1","0","88.5"]}]}]}}`
	_ = repo.UpdateGameLiveStats(newG.ID, statsJSON, &homeScore, &awayScore, "9:45 - 2nd", `{"away":["7","0"],"home":["3","7"]}`)

	// Request modal with ?tab=players&team=away&cat=passing
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("gameId", strconvFormat(newG.ID))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/games/%d/stats?tab=players&team=away&cat=passing", newG.ID), nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(injectUser(req.Context(), user))
	rr := httptest.NewRecorder()

	liveHandler.GameStatsModal(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from GameStatsModal, got %d: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()

	// Verify live polling trigger and attributes
	if !strings.Contains(body, "hx-trigger=\"every 15s\"") {
		t.Errorf("Expected modal to contain 'hx-trigger=\"every 15s\"'")
	}
	if !strings.Contains(body, "id=\"game-stats-modal-wrapper\"") {
		t.Errorf("Expected modal to contain wrapper id")
	}
	if !strings.Contains(body, "data-tab=\"players\"") {
		t.Errorf("Expected modal data-tab to be 'players'")
	}
	if !strings.Contains(body, "En vivo cada 15s") {
		t.Errorf("Expected modal to contain 'En vivo cada 15s' indicator")
	}
	if !strings.Contains(body, "Actualizar estadísticas en vivo ahora") {
		t.Errorf("Expected modal to contain manual refresh button")
	}
	if !strings.Contains(body, "L. Jackson") {
		t.Errorf("Expected modal to contain player name 'L. Jackson'")
	}
}

func TestWeek2LiveHandling(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	liveHandler := NewLiveHandler(repo, renderer, nil, 2026)

	// Mark week 1 as completed
	_ = repo.UpdateWeekStatus(1, "completed")
	_ = repo.UpdateWeekStatus(2, "active")

	activeW, err := repo.GetActiveWeek(1)
	if err != nil || activeW == nil {
		t.Fatalf("Failed to get active week: %v", err)
	}
	if activeW.WeekNumber != 2 {
		t.Fatalf("Expected active week to be 2, got %d", activeW.WeekNumber)
	}

	// Create a live game in week 2
	homeScore := 27
	awayScore := 7
	game := &db.Game{
		WeekID:       2,
		ESPNGameID:   "401872932",
		HomeTeamID:   6,
		AwayTeamID:   5,
		KickoffTime:  time.Now(),
		HomeScore:    &homeScore,
		AwayScore:    &awayScore,
		Status:       "in_progress",
		StatusDetail: "1:27 - 2nd Quarter",
		StatsJSON:    `{"has_stats":true,"away_stats":{"team_code":"DET","first_downs":"10","total_yards":"150"},"home_stats":{"team_code":"BUF","first_downs":"15","total_yards":"240"}}`,
	}
	_ = repo.UpsertGameByESPNID(game)

	// Test ShowLive defaults to Week 2
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	rr := httptest.NewRecorder()
	liveHandler.ShowLive(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ShowLive for Week 2, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Semana 2") {
		t.Errorf("Expected ShowLive body to contain 'Semana 2'")
	}
	if !strings.Contains(body, "EN JUEGO") {
		t.Errorf("Expected ShowLive body to indicate live game")
	}

	// Test GameStatsModal for Week 2 live game
	reqModal := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/games/%d/stats", game.ID), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("gameId", fmt.Sprintf("%d", game.ID))
	reqModal = reqModal.WithContext(context.WithValue(reqModal.Context(), chi.RouteCtxKey, rctx))
	rrModal := httptest.NewRecorder()
	liveHandler.GameStatsModal(rrModal, reqModal)

	if rrModal.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from GameStatsModal for Week 2 game, got %d", rrModal.Code)
	}
	modalBody := rrModal.Body.String()
	if !strings.Contains(modalBody, "EN VIVO") {
		t.Errorf("Expected modal body to contain 'EN VIVO'")
	}
}

func TestNotificationHandlerEndpoints(t *testing.T) {
	repo, authService, _, _, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)
	notifHandler := NewNotificationHandler(repo, renderer)

	user, err := authService.Register("notif_user", "notif@test.com", "pass123")
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	// 1. Unauthenticated badge check: returns 200
	reqUnauth := httptest.NewRequest(http.MethodGet, "/notifications/badge", nil)
	rrUnauth := httptest.NewRecorder()
	notifHandler.GetUnreadBadge(rrUnauth, reqUnauth)
	if rrUnauth.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for unauthenticated badge check, got %d", rrUnauth.Code)
	}

	// 2. Unread badge with 0 notifications
	reqBadge0 := httptest.NewRequest(http.MethodGet, "/notifications/badge", nil)
	reqBadge0 = reqBadge0.WithContext(injectUser(reqBadge0.Context(), user))
	rrBadge0 := httptest.NewRecorder()
	notifHandler.GetUnreadBadge(rrBadge0, reqBadge0)
	if rrBadge0.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", rrBadge0.Code)
	}
	if !strings.Contains(rrBadge0.Body.String(), "hidden") {
		t.Errorf("Expected badge to be hidden when 0 unread, got %s", rrBadge0.Body.String())
	}

	// 3. Create 2 notifications
	n1, err := repo.CreateInAppNotification(user.ID, "kickoff_24h", "Recordatorio de Kickoff", "Faltan menos de 24 horas para el partido", "/picks")
	if err != nil {
		t.Fatalf("Failed to create notification 1: %v", err)
	}
	_, err = repo.CreateInAppNotification(user.ID, "weekly_recap", "Resumen Semanal", "Revisa el podio de la jornada", "/leaderboard")
	if err != nil {
		t.Fatalf("Failed to create notification 2: %v", err)
	}

	// 4. Check badge now reflects 2
	reqBadge2 := httptest.NewRequest(http.MethodGet, "/notifications/badge", nil)
	reqBadge2 = reqBadge2.WithContext(injectUser(reqBadge2.Context(), user))
	rrBadge2 := httptest.NewRecorder()
	notifHandler.GetUnreadBadge(rrBadge2, reqBadge2)
	if !strings.Contains(rrBadge2.Body.String(), ">2<") {
		t.Errorf("Expected badge to contain '>2<', got %s", rrBadge2.Body.String())
	}

	// 5. Get Notifications dropdown list
	reqDropdown := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	reqDropdown = reqDropdown.WithContext(injectUser(reqDropdown.Context(), user))
	rrDropdown := httptest.NewRecorder()
	notifHandler.GetNotifications(rrDropdown, reqDropdown)
	if rrDropdown.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for notifications dropdown, got %d", rrDropdown.Code)
	}
	dropdownHTML := rrDropdown.Body.String()
	if !strings.Contains(dropdownHTML, "Recordatorio de Kickoff") {
		t.Errorf("Expected dropdown to contain 'Recordatorio de Kickoff'")
	}
	if !strings.Contains(dropdownHTML, "Resumen Semanal") {
		t.Errorf("Expected dropdown to contain 'Resumen Semanal'")
	}

	// 6. Mark single notification as read
	reqMark1 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/notifications/%d/read", n1.ID), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", n1.ID))
	reqMark1 = reqMark1.WithContext(context.WithValue(injectUser(reqMark1.Context(), user), chi.RouteCtxKey, rctx))
	rrMark1 := httptest.NewRecorder()
	notifHandler.MarkAsRead(rrMark1, reqMark1)
	if rrMark1.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for MarkAsRead, got %d", rrMark1.Code)
	}

	unreadCount, _ := repo.GetUnreadNotificationsCount(user.ID)
	if unreadCount != 1 {
		t.Errorf("Expected 1 unread notification after marking one read, got %d", unreadCount)
	}

	// 7. Mark all as read
	reqMarkAll := httptest.NewRequest(http.MethodPost, "/notifications/read-all", nil)
	reqMarkAll = reqMarkAll.WithContext(injectUser(reqMarkAll.Context(), user))
	rrMarkAll := httptest.NewRecorder()
	notifHandler.MarkAllAsRead(rrMarkAll, reqMarkAll)
	if rrMarkAll.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for MarkAllAsRead, got %d", rrMarkAll.Code)
	}

	unreadCount, _ = repo.GetUnreadNotificationsCount(user.ID)
	if unreadCount != 0 {
		t.Errorf("Expected 0 unread notifications after MarkAllAsRead, got %d", unreadCount)
	}
}

func TestPicksKickoffCountdownDeadlineBanner(t *testing.T) {
	repo, authService, _, _, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)
	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	user, _ := authService.Register("count_user", "count@test.com", "pass123")
	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 4)
	kc, _ := repo.GetTeamByCode("KC")
	bal, _ := repo.GetTeamByCode("BAL")

	// Case A: Game in 12 hours (< 24h deadline)
	now := time.Now().UTC()
	_, err := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  now.Add(12 * time.Hour),
		Status:       "scheduled",
		StatusDetail: "Sun, 1:00 PM",
	})
	if err != nil {
		t.Fatalf("Failed to create manual game: %v", err)
	}

	reqPicks := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks?week=%d", week.WeekNumber), nil)
	reqPicks = reqPicks.WithContext(injectUser(reqPicks.Context(), user))
	rrPicks := httptest.NewRecorder()
	picksHandler.ShowPicks(rrPicks, reqPicks)

	if rrPicks.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ShowPicks, got %d", rrPicks.Code)
	}
	body := rrPicks.Body.String()
	if !strings.Contains(body, "Cierre en Menos de 24 Horas") {
		t.Errorf("Expected countdown banner 'Cierre en Menos de 24 Horas' in HTML when kickoff is in 12h")
	}

	// Case B: In Week 5 where kickoff is in 48 hours (> 24h deadline)
	week5, _ := repo.GetWeekByNumber(season.ID, 5)
	_, _ = repo.CreateManualGame(&db.Game{
		WeekID:       week5.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  now.Add(48 * time.Hour),
		Status:       "scheduled",
		StatusDetail: "Sun, 1:00 PM",
	})

	reqPicks5 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks?week=%d", week5.WeekNumber), nil)
	reqPicks5 = reqPicks5.WithContext(injectUser(reqPicks5.Context(), user))
	rrPicks5 := httptest.NewRecorder()
	picksHandler.ShowPicks(rrPicks5, reqPicks5)

	if rrPicks5.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ShowPicks week 5, got %d", rrPicks5.Code)
	}
	body5 := rrPicks5.Body.String()
	if strings.Contains(body5, "Cierre en Menos de 24 Horas") {
		t.Errorf("Expected countdown banner NOT to show when kickoff is in 48h")
	}
}

func TestAdminSendWeeklyRecap(t *testing.T) {
	repo, _, _, calculator, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)

	worker := notifications.NewReminderWorker(repo, nil, 2026)
	adminHandler := NewAdminHandler(repo, renderer, nil, calculator, nil, worker, 2026)

	adminUser, _ := repo.GetUserByUsername("admin")
	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 1)

	form := url.Values{}
	form.Set("week_id", strconvFormat(week.ID))

	req := httptest.NewRequest(http.MethodPost, "/admin/reminders/weekly-recap", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(injectUser(req.Context(), adminUser))
	rr := httptest.NewRecorder()

	adminHandler.SendWeeklyRecap(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from SendWeeklyRecap, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "resúmenes semanales") {
		t.Errorf("Expected success response with 'resúmenes semanales', got: %s", rr.Body.String())
	}
}

func TestProfileHandlerPreferencesWithGranularNotifications(t *testing.T) {
	repo, authService, _, _, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)
	profileHandler := NewProfileHandler(repo, authService, renderer, t.TempDir())

	u, err := authService.Register("prefuser", "pref@test.com", "pass123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	form := url.Values{}
	form.Set("has_notif_prefs", "1")
	form.Set("notify_email", "1")
	form.Set("notify_kickoff", "1")
	form.Set("notify_recap", "0") // disabled recap

	req := httptest.NewRequest(http.MethodPost, "/profile/preferences", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(injectUser(req.Context(), u))
	rr := httptest.NewRecorder()

	profileHandler.HandleUpdatePreferences(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("Expected 303 SeeOther, got %d", rr.Code)
	}

	freshUser, _ := repo.GetUserByID(u.ID)
	if !freshUser.NotifyEmail {
		t.Errorf("Expected NotifyEmail=true")
	}
	if !freshUser.NotifyKickoff {
		t.Errorf("Expected NotifyKickoff=true")
	}
	if freshUser.NotifyRecap {
		t.Errorf("Expected NotifyRecap=false")
	}
}

func TestAdminToggleUserBeta(t *testing.T) {
	repo, authService, renderer, calculator, cleanup := setupTestApp(t)
	defer cleanup()

	adminHandler := NewAdminHandler(repo, renderer, nil, calculator, nil, nil, 2026)
	adminUser, _ := repo.GetUserByUsername("admin")

	playerUser, err := authService.Register("testbetauser", "testbeta@test.com", "pass123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if playerUser.IsBetaTester {
		t.Fatalf("New user should not be a beta tester by default")
	}

	// 1. Toggle to TRUE
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userId", strconvFormat(playerUser.ID))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/users/%d/toggle-beta", playerUser.ID), nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(injectUser(req.Context(), adminUser))
	rr := httptest.NewRecorder()

	adminHandler.ToggleUserBeta(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Beta Tester") {
		t.Errorf("Expected response to contain 'Beta Tester', got: %s", rr.Body.String())
	}

	updatedUser, err := repo.GetUserByID(playerUser.ID)
	if err != nil || !updatedUser.IsBetaTester {
		t.Fatalf("Expected user to be beta tester in DB, got %+v", updatedUser)
	}

	// 2. Toggle to FALSE
	rctx2 := chi.NewRouteContext()
	rctx2.URLParams.Add("userId", strconvFormat(playerUser.ID))
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/users/%d/toggle-beta", playerUser.ID), nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), chi.RouteCtxKey, rctx2))
	req2 = req2.WithContext(injectUser(req2.Context(), adminUser))
	rr2 := httptest.NewRecorder()

	adminHandler.ToggleUserBeta(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "Estándar") {
		t.Errorf("Expected response to contain 'Estándar', got: %s", rr2.Body.String())
	}

	updatedUser2, err := repo.GetUserByID(playerUser.ID)
	if err != nil || updatedUser2.IsBetaTester {
		t.Fatalf("Expected user beta tester to be false in DB, got %+v", updatedUser2)
	}

	// 3. Invalid ID
	rctxInvalid := chi.NewRouteContext()
	rctxInvalid.URLParams.Add("userId", "abc")
	reqInvalid := httptest.NewRequest(http.MethodPost, "/admin/users/abc/toggle-beta", nil)
	reqInvalid = reqInvalid.WithContext(context.WithValue(reqInvalid.Context(), chi.RouteCtxKey, rctxInvalid))
	rrInvalid := httptest.NewRecorder()
	adminHandler.ToggleUserBeta(rrInvalid, reqInvalid)
	if rrInvalid.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for invalid user ID, got %d", rrInvalid.Code)
	}
}

func TestAdminSaveFeatureFlags(t *testing.T) {
	repo, _, renderer, calculator, cleanup := setupTestApp(t)
	defer cleanup()

	adminHandler := NewAdminHandler(repo, renderer, nil, calculator, nil, nil, 2026)
	adminUser, _ := repo.GetUserByUsername("admin")

	form := url.Values{}
	form.Set("access_level_picks_matrix", "beta")
	form.Set("is_beta_picks_matrix", "1")
	form.Set("access_level_picks_compare", "disabled")
	form.Set("is_beta_picks_compare", "0")

	req := httptest.NewRequest(http.MethodPost, "/admin/features/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(injectUser(req.Context(), adminUser))
	rr := httptest.NewRecorder()

	adminHandler.SaveFeatureFlags(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from SaveFeatureFlags, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Control de características actualizado") {
		t.Errorf("Expected response to confirm update, got: %s", rr.Body.String())
	}

	flagMatrix, err := repo.GetFeatureFlag("picks_matrix")
	if err != nil || flagMatrix.AccessLevel != "beta" || !flagMatrix.IsBeta {
		t.Errorf("Expected picks_matrix to be access=beta, is_beta=true, got %+v", flagMatrix)
	}

	flagCompare, err := repo.GetFeatureFlag("picks_compare")
	if err != nil || flagCompare.AccessLevel != "disabled" || flagCompare.IsBeta {
		t.Errorf("Expected picks_compare to be access=disabled, is_beta=false, got %+v", flagCompare)
	}
}

func TestFeatureFlagAccessControlGating(t *testing.T) {
	repo, authService, _, _, cleanup := setupTestApp(t)
	defer cleanup()

	subTemplatesFS, _ := fs.Sub(os.DirFS(".."), "templates")
	renderer := NewRenderer(subTemplatesFS)
	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	normalPlayer, err := authService.Register("normaluser", "norm@test.com", "pass123")
	if err != nil {
		t.Fatalf("Failed to create normal player: %v", err)
	}

	betaPlayer, err := authService.Register("betauser", "beta@test.com", "pass123")
	if err != nil {
		t.Fatalf("Failed to create beta player: %v", err)
	}
	_ = repo.SetUserBetaTester(betaPlayer.ID, true)
	betaPlayer, _ = repo.GetUserByID(betaPlayer.ID)

	adminUser, _ := repo.GetUserByUsername("admin")

	// Restrict picks_matrix to 'beta'
	_ = repo.UpdateFeatureFlag("picks_matrix", "beta", true)

	// 1. Normal player accesses picks_matrix -> Locked
	reqNormal := httptest.NewRequest(http.MethodGet, "/picks/matrix?week=1", nil)
	reqNormal = reqNormal.WithContext(injectUser(reqNormal.Context(), normalPlayer))
	rrNormal := httptest.NewRecorder()
	picksHandler.ShowPicksMatrix(rrNormal, reqNormal)

	if rrNormal.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK with beta locked template, got %d", rrNormal.Code)
	}
	bodyNormal := rrNormal.Body.String()
	if !strings.Contains(bodyNormal, "Acceso Exclusivo para Beta Testers") {
		t.Errorf("Expected locked page with 'Acceso Exclusivo para Beta Testers', got body: %s", bodyNormal)
	}
	if strings.Contains(bodyNormal, "matrix-table-container") {
		t.Errorf("Did not expect matrix table container in locked view")
	}

	// 2. Beta player accesses picks_matrix -> Allowed
	reqBeta := httptest.NewRequest(http.MethodGet, "/picks/matrix?week=1", nil)
	reqBeta = reqBeta.WithContext(injectUser(reqBeta.Context(), betaPlayer))
	rrBeta := httptest.NewRecorder()
	picksHandler.ShowPicksMatrix(rrBeta, reqBeta)

	if rrBeta.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for beta player, got %d", rrBeta.Code)
	}
	bodyBeta := rrBeta.Body.String()
	if strings.Contains(bodyBeta, "Función en Fase Beta Privada") {
		t.Errorf("Beta tester should not be locked out")
	}
	if !strings.Contains(bodyBeta, "Matriz de Pronósticos") {
		t.Errorf("Beta tester should see matrix page title")
	}

	// 3. Admin accesses picks_matrix -> Allowed
	reqAdmin := httptest.NewRequest(http.MethodGet, "/picks/matrix?week=1", nil)
	reqAdmin = reqAdmin.WithContext(injectUser(reqAdmin.Context(), adminUser))
	rrAdmin := httptest.NewRecorder()
	picksHandler.ShowPicksMatrix(rrAdmin, reqAdmin)

	if rrAdmin.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for admin, got %d", rrAdmin.Code)
	}
	bodyAdmin := rrAdmin.Body.String()
	if strings.Contains(bodyAdmin, "Función en Fase Beta Privada") {
		t.Errorf("Admin should not be locked out")
	}

	// 4. Test ComparePicks gating
	_ = repo.UpdateFeatureFlag("picks_compare", "beta", true)

	// Non-beta tries to compare
	reqCompareNormal := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks/compare?rival_id=%d", betaPlayer.ID), nil)
	reqCompareNormal = reqCompareNormal.WithContext(injectUser(reqCompareNormal.Context(), normalPlayer))
	rrCompareNormal := httptest.NewRecorder()
	picksHandler.ComparePicks(rrCompareNormal, reqCompareNormal)

	if !strings.Contains(rrCompareNormal.Body.String(), "Beta Privada") {
		t.Errorf("Expected 'Beta Privada' for normal user, got: %s", rrCompareNormal.Body.String())
	}

	// Beta player tries to compare
	reqCompareBeta := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks/compare?rival_id=%d", normalPlayer.ID), nil)
	reqCompareBeta = reqCompareBeta.WithContext(injectUser(reqCompareBeta.Context(), betaPlayer))
	rrCompareBeta := httptest.NewRecorder()
	picksHandler.ComparePicks(rrCompareBeta, reqCompareBeta)

	if strings.Contains(rrCompareBeta.Body.String(), "Beta Privada") {
		t.Errorf("Beta tester should have access to compare picks")
	}
}




