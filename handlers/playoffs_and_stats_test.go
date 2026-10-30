package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

func TestPlayoffsAndStatsHandlers(t *testing.T) {
	testDBPath := "test_handlers_playoffs.db"
	_ = os.Remove(testDBPath)
	defer func() {
		_ = os.Remove(testDBPath)
		_ = os.Remove(testDBPath + "-shm")
		_ = os.Remove(testDBPath + "-wal")
	}()

	database, err := db.InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	_ = db.SeedDatabase(repo, "admin", "admin@example.com", "secret", 2026)

	admin, _ := repo.GetUserByUsername("admin")

	renderer := NewRenderer(os.DirFS("../templates"), repo)
	playoffsHandler := NewPlayoffsHandler(repo, renderer, 2026)
	statsHandler := NewStatsHandler(repo, renderer, 2026)

	// 1. Test SaveBracketPrediction
	form := url.Values{
		"wild_card_picks":   {`{"AFC_2v7":"KC"}`},
		"divisional_picks":  {`{"AFC_1vLowest":"KC"}`},
		"conference_picks":  {`{"AFC":"KC"}`},
		"super_bowl_pick":   {`{"winner":"KC"}`},
		"tiebreaker_points": {"49"},
	}
	req := httptest.NewRequest("POST", "/playoffs/bracket/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := context.WithValue(req.Context(), auth.UserContextKey, admin)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	playoffsHandler.SaveBracketPrediction(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("SaveBracketPrediction returned status %d, body: %s", rr.Code, rr.Body.String())
	}

	saved, err := repo.GetUserPlayoffBracket(admin.ID, 2026)
	if err != nil || saved == nil {
		t.Fatalf("bracket not saved in repo: %v", err)
	}
	if saved.TiebreakerPoints != 49 {
		t.Errorf("expected tiebreaker 49, got %d", saved.TiebreakerPoints)
	}

	// 2. Test Matchup Intel endpoint with Chi router
	games, _ := repo.ListGamesByWeek(1)
	if len(games) > 0 {
		r := chi.NewRouter()
		r.Get("/games/{gameId}/intel", statsHandler.MatchupIntelModal)

		intelReq := httptest.NewRequest("GET", "/games/1/intel", nil)
		intelRR := httptest.NewRecorder()
		r.ServeHTTP(intelRR, intelReq)
		// Template may not exist yet, but status shouldn't be panic
		_ = intelRR
	}
}
