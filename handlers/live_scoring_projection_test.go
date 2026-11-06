package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLiveHandler_ScoringProjectionAccuracy(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Create player
	user, err := repo.CreateUser("projection_tester", "tester@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	season, _ := repo.GetActiveSeason(2026)
	week1, err := repo.GetWeekByNumber(season.ID, 1)
	if err != nil {
		t.Fatalf("failed to get week 1: %v", err)
	}

	games, err := repo.ListGamesByWeek(week1.ID)
	if err != nil || len(games) < 3 {
		t.Fatalf("expected at least 3 games in week 1, got %d", len(games))
	}

	gFinal := games[0]
	gLive1 := games[1]
	gLive2 := games[2]

	// Game 1: FINAL (Home team wins 24 - 17)
	finalHScore := 24
	finalAScore := 17
	isCorrect := true
	ptsEarned := 10
	_, err = repo.GetDB().Exec(
		"UPDATE games SET status = 'final', status_detail = 'Final', home_score = ?, away_score = ? WHERE id = ?",
		finalHScore, finalAScore, gFinal.ID,
	)
	if err != nil {
		t.Fatalf("failed to update gFinal: %v", err)
	}
	pickFinal, err := repo.SavePick(user.ID, gFinal.ID, &gFinal.HomeTeamID, &finalAScore, &finalHScore)
	if err != nil {
		t.Fatalf("failed to save pickFinal: %v", err)
	}
	_ = repo.UpdatePickPoints(pickFinal.ID, ptsEarned, 0, &isCorrect)

	// Game 2: IN_PROGRESS (Home team leading 21 - 10)
	live1HScore := 21
	live1AScore := 10
	_, err = repo.GetDB().Exec(
		"UPDATE games SET status = 'in_progress', status_detail = 'Q3 08:45', home_score = ?, away_score = ? WHERE id = ?",
		live1HScore, live1AScore, gLive1.ID,
	)
	if err != nil {
		t.Fatalf("failed to update gLive1: %v", err)
	}
	_, err = repo.SavePick(user.ID, gLive1.ID, &gLive1.HomeTeamID, &live1AScore, &live1HScore)
	if err != nil {
		t.Fatalf("failed to save pickLive1: %v", err)
	}

	// Game 3: IN_PROGRESS (Away team leading 14 - 7)
	live2HScore := 7
	live2AScore := 14
	_, err = repo.GetDB().Exec(
		"UPDATE games SET status = 'in_progress', status_detail = 'Q2 02:00', home_score = ?, away_score = ? WHERE id = ?",
		live2HScore, live2AScore, gLive2.ID,
	)
	if err != nil {
		t.Fatalf("failed to update gLive2: %v", err)
	}
	_, err = repo.SavePick(user.ID, gLive2.ID, &gLive2.AwayTeamID, &live2AScore, &live2HScore)
	if err != nil {
		t.Fatalf("failed to save pickLive2: %v", err)
	}

	// 2. Request /live for week 1 as authenticated user
	liveHandler := NewLiveHandler(repo, renderer, nil, 2026)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/live?week=%d", week1.WeekNumber), nil)
	req = req.WithContext(injectUser(req.Context(), user))
	rr := httptest.NewRecorder()

	liveHandler.ShowLive(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()

	// 3. Assert Puntos Confirmados = 10 pts
	if !strings.Contains(body, "Puntos Confirmados") {
		t.Errorf("expected body to contain 'Puntos Confirmados'")
	}

	// 4. Assert Liderando en Vivo has +20 pts provisionales (not +2)
	if !strings.Contains(body, "+20") {
		t.Errorf("expected body to contain '+20' provisional points, got body: %s", body)
	}
	if !strings.Contains(body, "pts provisionales") {
		t.Errorf("expected body to contain 'pts provisionales'")
	}
	if !strings.Contains(body, "2 de 2 en juego") {
		t.Errorf("expected body to contain '2 de 2 en juego'")
	}

	// 5. Assert Total Proyectado = 30 pts (not 12 pts!)
	if !strings.Contains(body, "30") {
		t.Errorf("expected body to contain '30' projected points")
	}

	// Ensure bug (10 + 2 = 12) is NOT present
	if strings.Contains(body, "12 <span class=\"text-xs text-zinc-400 font-normal\">pts</span>") {
		t.Errorf("found regression: body contains 12 pts total projected instead of 30 pts")
	}
}
