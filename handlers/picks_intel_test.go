package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

func TestPicksDecisionIntel_BatchEnrichmentAndRender(t *testing.T) {
	testDBPath := "test_picks_intel.db"
	_ = os.Remove(testDBPath)
	defer func() {
		_ = os.Remove(testDBPath)
		_ = os.Remove(testDBPath + "-shm")
		_ = os.Remove(testDBPath + "-wal")
	}()

	database, err := db.InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	if err := db.SeedDatabase(repo, "admin", "admin@example.com", "secret", 2026); err != nil {
		t.Fatalf("SeedDatabase failed: %v", err)
	}

	season, err := repo.GetActiveSeason(2026)
	if err != nil || season == nil {
		t.Fatalf("Failed to get active season: %v", err)
	}

	week1, err := repo.GetWeekByNumber(season.ID, 1)
	if err != nil || week1 == nil {
		t.Fatalf("Failed to get week 1: %v", err)
	}

	// 1. Verify GetWeekMatchupIntel loads for week 1
	intelMap, err := repo.GetWeekMatchupIntel(week1.ID, 2026)
	if err != nil {
		t.Fatalf("GetWeekMatchupIntel failed: %v", err)
	}
	if intelMap == nil {
		t.Fatalf("Expected non-nil intel map")
	}

	games, err := repo.ListGamesByWeek(week1.ID)
	if err != nil || len(games) == 0 {
		t.Fatalf("Failed to list games: %v", err)
	}

	firstGame := games[0]
	intel, exists := intelMap[firstGame.ID]
	if !exists || intel == nil {
		t.Fatalf("Expected intel for game %d", firstGame.ID)
	}

	if intel.HomeTeam == nil || intel.AwayTeam == nil {
		t.Errorf("Expected HomeTeam and AwayTeam on intel, got home=%v away=%v", intel.HomeTeam, intel.AwayTeam)
	}

	if intel.HomeWinProbPct <= 0 || intel.AwayWinProbPct <= 0 {
		t.Errorf("Expected positive win probability percentages, got home=%d away=%d", intel.HomeWinProbPct, intel.AwayWinProbPct)
	}

	// 2. Test PicksHandler.ShowPicks with renderer
	renderer := NewRenderer(os.DirFS("../templates"), repo)
	handler := NewPicksHandler(repo, renderer, nil, 2026)

	user, err := repo.GetUserByUsername("admin")
	if err != nil || user == nil {
		t.Fatalf("Failed to get admin user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/picks?week=1", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, user)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ShowPicks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 from ShowPicks, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	// Verify that the new decision elements are present in the response
	if !strings.Contains(body, "Probabilidad") {
		t.Errorf("Expected 'Probabilidad' strip in rendered picks page")
	}
	if !strings.Contains(body, "Datos para tu Pick") {
		t.Errorf("Expected 'Datos para tu Pick' toggle in rendered picks page")
	}
	if !strings.Contains(body, "showIntel") {
		t.Errorf("Expected 'showIntel' Alpine state in rendered picks page")
	}

	// 3. Test Single Game Intel Endpoint
	statsHandler := NewStatsHandler(repo, renderer, 2026)
	reqModal := httptest.NewRequest(http.MethodGet, "/games/1/intel", nil)
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("gameId", "1")
	reqModal = reqModal.WithContext(context.WithValue(reqModal.Context(), chi.RouteCtxKey, chiCtx))

	recModal := httptest.NewRecorder()
	statsHandler.MatchupIntelModal(recModal, reqModal)

	if recModal.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 from MatchupIntelModal, got %d: %s", recModal.Code, recModal.Body.String())
	}

	modalBody := recModal.Body.String()
	if !strings.Contains(modalBody, "Matchup Intel") {
		t.Errorf("Expected 'Matchup Intel' in modal body")
	}
	if !strings.Contains(modalBody, "activeTab") {
		t.Errorf("Expected activeTab state in modal body")
	}
}

func TestPicksTemplate_BufferAndScoresComparison(t *testing.T) {
	testDBPath := filepath.Join(t.TempDir(), "test_render_buffer.db")
	database, err := db.InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	if err := db.SeedDatabase(repo, "admin", "admin@example.com", "secret", 2026); err != nil {
		t.Fatalf("SeedDatabase failed: %v", err)
	}

	season, err := repo.GetActiveSeason(2026)
	if err != nil || season == nil {
		t.Fatalf("Failed to get active season: %v", err)
	}

	week1, err := repo.GetWeekByNumber(season.ID, 1)
	if err != nil || week1 == nil {
		t.Fatalf("Failed to get week 1: %v", err)
	}

	// Set scores on games in week 1 to ensure score comparisons execute in templates
	games, err := repo.ListGamesByWeek(week1.ID)
	if err != nil || len(games) < 2 {
		t.Fatalf("Expected at least 2 games in week 1")
	}

	hScore1, aScore1 := 28, 24 // Home won
	_ = repo.UpdateGameScoreAndStatus(games[0].ID, &hScore1, &aScore1, "final", "Final")

	hScore2, aScore2 := 17, 31 // Away won
	_ = repo.UpdateGameScoreAndStatus(games[1].ID, &hScore2, &aScore2, "final", "Final")

	renderer := NewRenderer(os.DirFS("../templates"), repo)
	handler := NewPicksHandler(repo, renderer, nil, 2026)

	user, err := repo.GetUserByUsername("admin")
	if err != nil || user == nil {
		t.Fatalf("Failed to get admin user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/picks?week=1", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, user)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ShowPicks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	// A01: Ensure template rendered completely down to the final closing tags
	if !strings.Contains(body, "</html>") {
		t.Fatalf("A01 regression: picks page was truncated! </html> closing tag missing.")
	}
	if !strings.Contains(body, "</body>") {
		t.Fatalf("A01 regression: picks page was truncated! </body> closing tag missing.")
	}
}

func TestPickValidation_RejectsInvalidTeam(t *testing.T) {
	testDBPath := filepath.Join(t.TempDir(), "test_pick_validation.db")
	database, err := db.InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	if err := db.SeedDatabase(repo, "auditadmin", "a@example.invalid", "fictional-password", 2026); err != nil {
		t.Fatalf("SeedDatabase failed: %v", err)
	}

	u, err := repo.CreateUser("picktester", "tester@example.invalid", "hash", "player")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	_ = repo.SetUserEmailVerified(u.ID, true)
	u, _ = repo.GetUserByID(u.ID)

	season, err := repo.GetActiveSeason(2026)
	if err != nil {
		t.Fatalf("GetActiveSeason failed: %v", err)
	}
	week, err := repo.GetWeekByNumber(season.ID, 2)
	if err != nil {
		t.Fatalf("GetWeekByNumber failed: %v", err)
	}

	kc, err := repo.GetTeamByCode("KC")
	if err != nil {
		t.Fatalf("GetTeamByCode KC failed: %v", err)
	}
	bal, err := repo.GetTeamByCode("BAL")
	if err != nil {
		t.Fatalf("GetTeamByCode BAL failed: %v", err)
	}
	third, err := repo.GetTeamByCode("BUF")
	if err != nil {
		t.Fatalf("GetTeamByCode BUF failed: %v", err)
	}

	// Matchup: KC vs BAL (BUF does NOT participate)
	game, err := repo.CreateManualGame(&db.Game{
		WeekID:      week.ID,
		HomeTeamID:  kc.ID,
		AwayTeamID:  bal.ID,
		KickoffTime: time.Now().Add(24 * time.Hour), // Future game (unlocked)
		Status:      "scheduled",
	})
	if err != nil {
		t.Fatalf("CreateManualGame failed: %v", err)
	}

	// 1. Test repository level rejection
	hs, as := 24, 17
	_, repoErr := repo.SavePick(u.ID, game.ID, &third.ID, &hs, &as)
	if repoErr == nil {
		t.Fatalf("Expected repo.SavePick to REJECT third team (BUF), but succeeded!")
	}

	// 2. Test HTTP handler level rejection
	renderer := NewRenderer(os.DirFS("../templates"), repo)
	handler := NewPicksHandler(repo, renderer, nil, 2026)

	form := strings.NewReader(fmt.Sprintf("game_id=%d&picked_team_id=%d", game.ID, third.ID))
	req := httptest.NewRequest(http.MethodPost, "/picks/save", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	ctx := context.WithValue(req.Context(), auth.UserContextKey, u)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.SavePick(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected HTTP 400 Bad Request for third team pick, got HTTP %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Ensure nothing was persisted in database
	persistedPick, err := repo.GetUserPickForGame(u.ID, game.ID)
	if err != nil {
		t.Fatalf("GetUserPickForGame failed: %v", err)
	}
	if persistedPick != nil && persistedPick.PickedTeamID != nil {
		t.Fatalf("Expected no pick saved for game %d, but found picked_team_id=%d", game.ID, *persistedPick.PickedTeamID)
	}
}

func TestPickClearingAndDeletion_DATA02(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_picks_clearing.db")
	database, err := db.InitDB("sqlite", dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	if err := db.SeedDatabase(repo, "auditadmin", "a@example.invalid", "fictional-password", 2026); err != nil {
		t.Fatalf("SeedDatabase failed: %v", err)
	}

	season, err := repo.GetActiveSeason(2026)
	if err != nil {
		t.Fatalf("GetActiveSeason failed: %v", err)
	}

	week, err := repo.GetWeekByNumber(season.ID, 1)
	if err != nil {
		t.Fatalf("GetWeekByNumber failed: %v", err)
	}

	kc, _ := repo.GetTeamByCode("KC")
	bal, _ := repo.GetTeamByCode("BAL")
	u, err := repo.CreateUser("cleartest", "cleartest@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	_ = repo.SetUserEmailVerified(u.ID, true)

	game, err := repo.CreateManualGame(&db.Game{
		WeekID:      week.ID,
		HomeTeamID:  kc.ID,
		AwayTeamID:  bal.ID,
		KickoffTime: time.Now().Add(24 * time.Hour),
		Status:      "scheduled",
	})
	if err != nil {
		t.Fatalf("CreateManualGame failed: %v", err)
	}

	// 1. Initial save with scores
	hs, as := 28, 20
	pick1, err := repo.SavePick(u.ID, game.ID, &kc.ID, &hs, &as)
	if err != nil {
		t.Fatalf("Initial SavePick failed: %v", err)
	}
	if pick1.PredictedHomeScore == nil || *pick1.PredictedHomeScore != 28 {
		t.Fatalf("Expected PredictedHomeScore=28, got %v", pick1.PredictedHomeScore)
	}

	// 2. Clear scores explicitly using SavePickFull without COALESCE reviving them
	pick2, err := repo.SavePickFull(u.ID, game.ID, &kc.ID, nil, nil)
	if err != nil {
		t.Fatalf("SavePickFull failed: %v", err)
	}
	if pick2.PredictedHomeScore != nil {
		t.Fatalf("Expected PredictedHomeScore to be cleared (nil), but got: %d", *pick2.PredictedHomeScore)
	}
	if pick2.PredictedAwayScore != nil {
		t.Fatalf("Expected PredictedAwayScore to be cleared (nil), but got: %d", *pick2.PredictedAwayScore)
	}
	if pick2.PickedTeamID == nil || *pick2.PickedTeamID != kc.ID {
		t.Fatalf("Expected PickedTeamID to remain KC, got: %v", pick2.PickedTeamID)
	}

	// 3. Clear pick completely
	if err := repo.DeletePick(u.ID, game.ID); err != nil {
		t.Fatalf("DeletePick failed: %v", err)
	}
	clearedPick, err := repo.GetUserPickForGame(u.ID, game.ID)
	if err != nil {
		t.Fatalf("GetUserPickForGame failed: %v", err)
	}
	if clearedPick != nil {
		t.Fatalf("Expected pick to be deleted from database, but found pick ID=%d", clearedPick.ID)
	}
}



