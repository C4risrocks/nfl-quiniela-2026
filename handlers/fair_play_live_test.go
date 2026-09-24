package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nfl-quiniela-2026/db"
)

func TestLiveHandler_FairPlayHidesRivalsBeforeLock(t *testing.T) {
	repo, _, renderer, calculator, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Create two players: Player1 and RivalPlayer
	p1, err := repo.CreateUser("player_one", "p1@test.com", "hash1", "player")
	if err != nil {
		t.Fatalf("failed to create p1: %v", err)
	}
	rival, err := repo.CreateUser("rival_player", "rival@test.com", "hash2", "player")
	if err != nil {
		t.Fatalf("failed to create rival: %v", err)
	}

	season, _ := repo.GetActiveSeason(2026)
	week3, err := repo.GetWeekByNumber(season.ID, 3)
	if err != nil {
		t.Fatalf("failed to get week 3: %v", err)
	}

	kcTeam, _ := repo.GetTeamByCode("KC")
	balTeam, _ := repo.GetTeamByCode("BAL")
	homeScore := 27
	awayScore := 20

	futureTime := time.Now().Add(48 * time.Hour)
	g1, err := repo.CreateManualGame(&db.Game{
		WeekID:       week3.ID,
		HomeTeamID:   kcTeam.ID,
		AwayTeamID:   balTeam.ID,
		KickoffTime:  futureTime,
		Status:       "scheduled",
		IsTiebreaker: true,
	})
	if err != nil {
		t.Fatalf("failed to create game: %v", err)
	}

	// Rival picks BAL with 20-27
	_, err = repo.SavePick(rival.ID, g1.ID, &balTeam.ID, &awayScore, &homeScore)
	if err != nil {
		t.Fatalf("failed to save rival pick: %v", err)
	}
	// Player1 picks KC with 27-20
	_, err = repo.SavePick(p1.ID, g1.ID, &kcTeam.ID, &awayScore, &homeScore)
	if err != nil {
		t.Fatalf("failed to save p1 pick: %v", err)
	}

	liveHandler := NewLiveHandler(repo, renderer, nil, 2026)

	// 2. Request /live as Player1 while game is UNLOCKED (pre-kickoff)
	req := httptest.NewRequest(http.MethodGet, "/live?week=3", nil)
	req = req.WithContext(injectUser(req.Context(), p1))
	rr := httptest.NewRecorder()

	liveHandler.ShowLive(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()

	// Must contain Fair Play alert banner
	if !strings.Contains(body, "Fair Play activo") {
		t.Errorf("expected body to contain Fair Play banner before kickoff")
	}

	// Must contain locked indicators for rival
	if !strings.Contains(body, "🔒 Oculto") && !strings.Contains(body, "Protegido hasta el kickoff") {
		t.Errorf("expected rival pick to be protected/hidden before kickoff")
	}

	// Player1's own pick should be visible to them
	if !strings.Contains(body, "TÚ") {
		t.Errorf("expected player's own pick badge 'TÚ' to be visible")
	}

	// 3. Now simulate game is LOCKED (in the past or in_progress)
	pastTime := time.Now().Add(-2 * time.Hour)
	_, _ = repo.GetDB().Exec("UPDATE games SET kickoff_time = ?, status = 'in_progress' WHERE id = ?", pastTime, g1.ID)

	rrLocked := httptest.NewRecorder()
	liveHandler.ShowLive(rrLocked, req)
	if rrLocked.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rrLocked.Code)
	}

	bodyLocked := rrLocked.Body.String()
	// Should NOT say "Protegido hasta el kickoff" for the featured game once locked
	if strings.Contains(bodyLocked, "Protegido hasta el kickoff") {
		t.Errorf("expected picks to be revealed once game is in_progress/locked")
	}
	// Rival's team code should now be revealed
	if !strings.Contains(bodyLocked, "BAL") {
		t.Errorf("expected rival's team code BAL to be revealed once game is in_progress")
	}

	_ = calculator
}
