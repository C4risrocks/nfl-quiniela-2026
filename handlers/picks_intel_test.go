package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
