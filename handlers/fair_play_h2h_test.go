package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nfl-quiniela-2026/db"
)

func TestHeadToHeadFairPlayMasking(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	season, err := repo.GetActiveSeason(2026)
	if err != nil {
		t.Fatalf("Failed to get active season: %v", err)
	}
	week, err := repo.GetWeekByNumber(season.ID, 3) // Week 3 (future scheduled games)
	if err != nil {
		t.Fatalf("Failed to get week 3: %v", err)
	}

	kc, err := repo.GetTeamByCode("KC")
	if err != nil {
		t.Fatalf("Failed to get team KC: %v", err)
	}
	bal, err := repo.GetTeamByCode("BAL")
	if err != nil {
		t.Fatalf("Failed to get team BAL: %v", err)
	}
	sf, err := repo.GetTeamByCode("SF")
	if err != nil {
		t.Fatalf("Failed to get team SF: %v", err)
	}
	lar, err := repo.GetTeamByCode("LAR")
	if err != nil {
		t.Fatalf("Failed to get team LAR: %v", err)
	}

	// Game 1: Unlocked future game (kickoff in 48 hours)
	futureKickoff := time.Now().Add(48 * time.Hour)
	gameFuture, err := repo.CreateManualGame(&db.Game{
		WeekID:      week.ID,
		HomeTeamID:  kc.ID,
		AwayTeamID:  bal.ID,
		KickoffTime: futureKickoff,
		Status:      "scheduled",
	})
	if err != nil {
		t.Fatalf("Failed to create future game: %v", err)
	}

	// Game 2: Locked finished game (kickoff in the past, final)
	pastKickoff := time.Now().Add(-2 * time.Hour)
	hScore := 28
	aScore := 24
	gameFinal, err := repo.CreateManualGame(&db.Game{
		WeekID:      week.ID,
		HomeTeamID:  sf.ID,
		AwayTeamID:  lar.ID,
		KickoffTime: pastKickoff,
		HomeScore:   &hScore,
		AwayScore:   &aScore,
		Status:      "final",
	})
	if err != nil {
		t.Fatalf("Failed to create final game: %v", err)
	}

	// User A (Player viewing the comparison)
	userA, err := repo.CreateUser("Alice_Viewer", "alice_view@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("Failed to create user A: %v", err)
	}
	// User B (Rival)
	userB, err := repo.CreateUser("Bob_Rival", "bob_rival@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("Failed to create user B: %v", err)
	}

	// User A picks KC on future game, SF on final game
	pA1 := 24
	pA2 := 20
	if _, err := repo.SavePick(userA.ID, gameFuture.ID, &kc.ID, &pA1, &pA2); err != nil {
		t.Fatalf("Failed to save pick A1: %v", err)
	}
	if _, err := repo.SavePick(userA.ID, gameFinal.ID, &sf.ID, &pA1, &pA2); err != nil {
		t.Fatalf("Failed to save pick A2: %v", err)
	}

	// User B picks BAL (divergent!) on future game, LAR on final game
	pB1 := 27
	pB2 := 23
	if _, err := repo.SavePick(userB.ID, gameFuture.ID, &bal.ID, &pB1, &pB2); err != nil {
		t.Fatalf("Failed to save pick B1: %v", err)
	}
	if _, err := repo.SavePick(userB.ID, gameFinal.ID, &lar.ID, &pB1, &pB2); err != nil {
		t.Fatalf("Failed to save pick B2: %v", err)
	}

	// Test 1: Query GetHeadToHeadComparison directly from repository
	comp, err := repo.GetHeadToHeadComparison(week.ID, userA.ID, userB.ID)
	if err != nil {
		t.Fatalf("GetHeadToHeadComparison failed: %v", err)
	}

	var futureMatchup, finalMatchup *db.HeadToHeadMatchup
	for _, m := range comp.Matchups {
		if m.Game.ID == gameFuture.ID {
			futureMatchup = m
		}
		if m.Game.ID == gameFinal.ID {
			finalMatchup = m
		}
	}

	if futureMatchup == nil || finalMatchup == nil {
		t.Fatalf("Expected both matchups to be present in comparison")
	}

	// FAIR PLAY VERIFICATION ON FUTURE GAME:
	// User A's pick must be intact
	if futureMatchup.UserAPick == nil || futureMatchup.UserAPickedTeam == nil || futureMatchup.UserAPickedTeam.Code != "KC" {
		t.Errorf("Expected User A pick to be visible and KC, got %+v", futureMatchup.UserAPickedTeam)
	}
	// Rival User B's pick MUST BE MASKED:
	if futureMatchup.UserBPick != nil || futureMatchup.UserBPickedTeam != nil {
		t.Errorf("FAIR PLAY BREACH: Expected rival pick to be masked for future game, but got team %+v", futureMatchup.UserBPickedTeam)
	}
	if !futureMatchup.IsMaskedForFairPlay {
		t.Errorf("Expected IsMaskedForFairPlay=true for future game")
	}
	if futureMatchup.IsDivergent {
		t.Errorf("Expected IsDivergent=false for future game so divergence is not leaked")
	}

	// VERIFICATION ON FINAL GAME:
	// Both picks must be revealed
	if finalMatchup.IsMaskedForFairPlay {
		t.Errorf("Expected IsMaskedForFairPlay=false for final game")
	}
	if finalMatchup.UserBPickedTeam == nil || finalMatchup.UserBPickedTeam.Code != "LAR" {
		t.Errorf("Expected rival pick to be revealed for final game (LAR), got %+v", finalMatchup.UserBPickedTeam)
	}
	if !finalMatchup.IsDivergent {
		t.Errorf("Expected IsDivergent=true for final game (SF vs LAR)")
	}

	// Test 2: HTTP Handler integration test (/picks/compare)
	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks/compare?rival_id=%d&week_id=%d", userB.ID, week.ID), nil)
	req = req.WithContext(injectUser(req.Context(), userA))

	rr := httptest.NewRecorder()
	picksHandler.ComparePicks(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from ComparePicks, got %d: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	// Must contain the fair play badge for the unstarted game
	if !strings.Contains(body, "Oculto hasta kickoff") {
		t.Errorf("Expected HTML response to contain 'Oculto hasta kickoff' for unlocked game")
	}
	// Must contain the revealed team code for the final game
	if !strings.Contains(body, "LAR") {
		t.Errorf("Expected HTML response to contain revealed rival team 'LAR' for final game")
	}
	// Must contain user A's own pick
	if !strings.Contains(body, "KC") {
		t.Errorf("Expected HTML response to contain user's own team 'KC'")
	}
}
