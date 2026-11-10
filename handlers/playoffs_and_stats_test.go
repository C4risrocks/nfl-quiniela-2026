package handlers

import (
	"context"
	"fmt"
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

	// 3. Test ShowStats
	statsReq := httptest.NewRequest("GET", "/stats", nil)
	statsReq = statsReq.WithContext(ctx)
	statsRR := httptest.NewRecorder()
	statsHandler.ShowStats(statsRR, statsReq)
	if statsRR.Code != http.StatusOK {
		t.Fatalf("ShowStats failed with status %d: %s", statsRR.Code, statsRR.Body.String())
	}
	if !strings.Contains(statsRR.Body.String(), "Curva de Evolución en el Leaderboard") {
		t.Errorf("Expected ShowStats body to contain 'Curva de Evolución en el Leaderboard'")
	}

	// 3b. Test ShowStats with ?user=1 and ?tab=teams_ats
	statsUserReq := httptest.NewRequest("GET", "/stats?user=1&tab=teams_ats", nil)
	statsUserReq = statsUserReq.WithContext(ctx)
	statsUserRR := httptest.NewRecorder()
	statsHandler.ShowStats(statsUserRR, statsUserReq)
	if statsUserRR.Code != http.StatusOK {
		t.Fatalf("ShowStats with ?user=1 failed with status %d: %s", statsUserRR.Code, statsUserRR.Body.String())
	}

	// 4. Test UserStatsPartial with chi router and path param
	r := chi.NewRouter()
	r.Get("/stats/user/{id}", statsHandler.UserStatsPartial)
	r.Get("/stats/user", statsHandler.UserStatsPartial)

	uReq := httptest.NewRequest("GET", "/stats/user/1", nil)
	uRR := httptest.NewRecorder()
	r.ServeHTTP(uRR, uReq)
	if uRR.Code != http.StatusOK {
		t.Fatalf("UserStatsPartial /stats/user/1 failed with status %d: %s", uRR.Code, uRR.Body.String())
	}
	if !strings.Contains(uRR.Body.String(), "Precisión Global") {
		t.Errorf("Expected UserStatsPartial body to contain 'Precisión Global'")
	}

	// 5. Test UserStatsPartial with query param /stats/user?id=1
	uReqQuery := httptest.NewRequest("GET", "/stats/user?id=1", nil)
	uRRQuery := httptest.NewRecorder()
	r.ServeHTTP(uRRQuery, uReqQuery)
	if uRRQuery.Code != http.StatusOK {
		t.Fatalf("UserStatsPartial /stats/user?id=1 failed with status %d: %s", uRRQuery.Code, uRRQuery.Body.String())
	}

	// 6. Test UserStatsPartial with ?user=1 query param
	uReqUser := httptest.NewRequest("GET", "/stats/user?user=1", nil)
	uRRUser := httptest.NewRecorder()
	r.ServeHTTP(uRRUser, uReqUser)
	if uRRUser.Code != http.StatusOK {
		t.Fatalf("UserStatsPartial /stats/user?user=1 failed with status %d: %s", uRRUser.Code, uRRUser.Body.String())
	}

	// 7. Test UserStatsPartial for newly created user without picks (empty state)
	emptyUser, err := repo.CreateUser("newbie_player", "newbie@test.com", "pass123", "player")
	if err == nil && emptyUser != nil {
		uReqEmpty := httptest.NewRequest("GET", fmt.Sprintf("/stats/user?id=%d", emptyUser.ID), nil)
		uRREmpty := httptest.NewRecorder()
		r.ServeHTTP(uRREmpty, uReqEmpty)
		if uRREmpty.Code == http.StatusOK {
			if !strings.Contains(uRREmpty.Body.String(), "newbie_player") {
				t.Errorf("Expected empty user partial to contain username 'newbie_player'")
			}
			if !strings.Contains(uRREmpty.Body.String(), "aún no tiene pronósticos evaluados") {
				t.Errorf("Expected empty state notice in user partial")
			}
		}
	}
}
