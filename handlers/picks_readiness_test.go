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

func TestPicksReadinessModal_Success(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Create 3 regular players (not admins, not beta testers)
	p1, err := repo.CreateUser("alice_ready", "alice@test.com", "hash1", "player")
	if err != nil {
		t.Fatalf("failed to create p1: %v", err)
	}
	p2, err := repo.CreateUser("bob_progress", "bob@test.com", "hash2", "player")
	if err != nil {
		t.Fatalf("failed to create p2: %v", err)
	}
	if _, err := repo.CreateUser("charlie_pending", "charlie@test.com", "hash3", "player"); err != nil {
		t.Fatalf("failed to create p3: %v", err)
	}

	season, _ := repo.GetActiveSeason(2026)
	week3, err := repo.GetWeekByNumber(season.ID, 3)
	if err != nil {
		t.Fatalf("failed to get week 3: %v", err)
	}

	kcTeam, _ := repo.GetTeamByCode("KC")
	balTeam, _ := repo.GetTeamByCode("BAL")
	sfTeam, _ := repo.GetTeamByCode("SF")
	larTeam, _ := repo.GetTeamByCode("LAR")

	// Create 2 games in Week 3
	g1, err := repo.CreateManualGame(&db.Game{
		WeekID:       week3.ID,
		HomeTeamID:   kcTeam.ID,
		AwayTeamID:   balTeam.ID,
		KickoffTime:  time.Now().Add(48 * time.Hour),
		Status:       "scheduled",
		IsTiebreaker: true,
	})
	if err != nil {
		t.Fatalf("failed to create g1: %v", err)
	}

	g2, err := repo.CreateManualGame(&db.Game{
		WeekID:       week3.ID,
		HomeTeamID:   sfTeam.ID,
		AwayTeamID:   larTeam.ID,
		KickoffTime:  time.Now().Add(52 * time.Hour),
		Status:       "scheduled",
		IsTiebreaker: false,
	})
	if err != nil {
		t.Fatalf("failed to create g2: %v", err)
	}

	homeScore := 28
	awayScore := 24

	// Alice (p1): completes all 2 games + tiebreaker
	_, _ = repo.SavePick(p1.ID, g1.ID, &kcTeam.ID, &awayScore, &homeScore)
	_, _ = repo.SavePick(p1.ID, g2.ID, &sfTeam.ID, nil, nil)

	// Bob (p2): completes 1 of 2 games
	_, _ = repo.SavePick(p2.ID, g1.ID, &balTeam.ID, nil, nil)

	// Charlie (p3): 0 picks

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	// 2. Request /picks/readiness?week_id=... as regular player Alice
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks/readiness?week_id=%d", week3.ID), nil)
	req = req.WithContext(injectUser(req.Context(), p1))
	rr := httptest.NewRecorder()

	picksHandler.PicksReadinessModal(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()

	// Verify Header and Modal Title
	if !strings.Contains(body, "Estatus de Participación") {
		t.Errorf("expected body to contain modal title 'Estatus de Participación'")
	}
	if !strings.Contains(body, "WhatsApp") {
		t.Errorf("expected body to contain WhatsApp reminder button")
	}
	if !strings.Contains(body, "Fair Play") {
		t.Errorf("expected body to contain Fair Play notice")
	}

	// Verify player statuses
	if !strings.Contains(body, "alice_ready") || !strings.Contains(body, "bob_progress") || !strings.Contains(body, "charlie_pending") {
		t.Errorf("expected all players to appear in the readiness modal")
	}

	// Verify Alice has "Listo" and "TÚ" badge
	if !strings.Contains(body, "Listo") {
		t.Errorf("expected Alice to be marked as Listo")
	}
	if !strings.Contains(body, "TÚ") {
		t.Errorf("expected Alice to see 'TÚ' badge for her own account")
	}

	// Verify Charlie is marked as "Sin enviar"
	if !strings.Contains(body, "Sin enviar") {
		t.Errorf("expected Charlie to be marked as Sin enviar")
	}

	// 3. FAIR PLAY CHECK: Verify that specific picked teams or scores are NOT leaked in the readiness view!
	// Alice picked KC, Bob picked BAL. The modal should show counts (2/2, 1/2) but NOT reveal who picked BAL or KC
	// before the games start!
	if strings.Contains(body, "data-picked-team") || strings.Contains(body, "Kansas City") {
		t.Errorf("FAIR PLAY VIOLATION: specific team selections leaked in readiness modal")
	}
}

func TestPicksReadinessModal_Unauthorized(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	// No user in context
	req := httptest.NewRequest(http.MethodGet, "/picks/readiness?week_id=1", nil)
	rr := httptest.NewRecorder()

	picksHandler.PicksReadinessModal(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated request, got %d", rr.Code)
	}
}

func TestShowPicks_IncludesReadinessButton(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	p1, _ := repo.CreateUser("ready_tester", "tester@test.com", "hash", "player")
	season, _ := repo.GetActiveSeason(2026)
	week1, _ := repo.GetWeekByNumber(season.ID, 1)

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/picks?week=%d", week1.WeekNumber), nil)
	req = req.WithContext(injectUser(req.Context(), p1))
	rr := httptest.NewRecorder()

	picksHandler.ShowPicks(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()
	// Header must contain the readiness button linking to /picks/readiness
	if !strings.Contains(body, "/picks/readiness?week_id=") {
		t.Errorf("expected ShowPicks page to contain /picks/readiness button")
	}
	if !strings.Contains(body, "listos") {
		t.Errorf("expected ShowPicks page to display 'listos' pill counter")
	}
}
