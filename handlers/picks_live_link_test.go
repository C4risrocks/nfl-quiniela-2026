package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPicksHandler_LiveGameInteractiveLinks(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Create a test user
	user, err := repo.CreateUser("live_player", "live@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	season, _ := repo.GetActiveSeason(2026)
	week1, err := repo.GetWeekByNumber(season.ID, 1)
	if err != nil {
		t.Fatalf("failed to get week 1: %v", err)
	}

	games, err := repo.ListGamesByWeek(week1.ID)
	if err != nil || len(games) == 0 {
		t.Fatalf("failed to list games for week 1: %v", err)
	}

	targetGame := games[0]

	// 2. Mark the target game as in_progress with live score
	hScore := 21
	aScore := 14
	statusDetail := "Q3 04:12"
	_, err = repo.GetDB().Exec(
		"UPDATE games SET status = 'in_progress', status_detail = ?, home_score = ?, away_score = ? WHERE id = ?",
		statusDetail, hScore, aScore, targetGame.ID,
	)
	if err != nil {
		t.Fatalf("failed to update game to in_progress: %v", err)
	}

	// 3. User makes a pick on this game
	_, err = repo.SavePick(user.ID, targetGame.ID, &targetGame.HomeTeamID, &aScore, &hScore)
	if err != nil {
		t.Fatalf("failed to save pick: %v", err)
	}

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	// 4. Request /picks?week=1 as the authenticated user
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks?week=%d", week1.WeekNumber), nil)
	req = req.WithContext(injectUser(req.Context(), user))
	rr := httptest.NewRecorder()

	picksHandler.ShowPicks(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()

	// Expected target live link path (may be HTML entity escaped or raw)
	expectedRaw := fmt.Sprintf("/live?week=%d&game_id=%d", week1.WeekNumber, targetGame.ID)
	expectedEscaped := fmt.Sprintf("/live?week=%d&amp;game_id=%d", week1.WeekNumber, targetGame.ID)

	if !strings.Contains(body, expectedRaw) && !strings.Contains(body, expectedEscaped) {
		t.Errorf("expected /picks body to contain live link to %s or %s, but not found", expectedRaw, expectedEscaped)
	}

	// Verify Header Badge is clickable
	if !strings.Contains(body, "Ver partido en directo en el Game Center en Vivo") {
		t.Errorf("expected body to contain header badge title 'Ver partido en directo en el Game Center en Vivo'")
	}

	// Verify in-progress score banner has Game Center link
	if !strings.Contains(body, "Abrir Game Center en Vivo para este partido") {
		t.Errorf("expected body to contain banner title 'Abrir Game Center en Vivo para este partido'")
	}

	// Verify the top toolbar chip shows 1 En Vivo
	expectedToolbarChip := fmt.Sprintf("/live?week=%d", week1.WeekNumber)
	if !strings.Contains(body, expectedToolbarChip) {
		t.Errorf("expected toolbar chip linking to %s", expectedToolbarChip)
	}
	if !strings.Contains(body, "1 En Vivo") {
		t.Errorf("expected toolbar chip text '1 En Vivo'")
	}

	// 5. Verify single game card partial render
	updatedGame, err := repo.GetGameByID(targetGame.ID)
	if err != nil {
		t.Fatalf("failed to fetch game: %v", err)
	}
	partialRecorder := httptest.NewRecorder()
	renderer.RenderPartial(partialRecorder, "game_card.html", map[string]interface{}{
		"Game":             updatedGame,
		"CurrentTime":      time.Now(),
		"FormattedKickoff": "Dom 12:00",
		"LockMode":         "individual",
		"ScoringMode":      "pure_tiebreaker",
		"User":             user,
	})
	if partialRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from partial render, got %d", partialRecorder.Code)
	}
	partialBody := partialRecorder.Body.String()
	if !strings.Contains(partialBody, expectedRaw) && !strings.Contains(partialBody, expectedEscaped) {
		t.Errorf("expected partial to contain live link %s", expectedRaw)
	}
	if !strings.Contains(partialBody, "Ver partido en directo en el Game Center en Vivo") {
		t.Errorf("expected partial to contain header badge link")
	}
}
