package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"nfl-quiniela-2026/db"
)

func TestShowPicksAdvisor_BetaGating(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Create a regular player (not a beta tester)
	regularUser, err := repo.CreateUser("regular_joe", "regular@test.com", "hash1", "player")
	if err != nil {
		t.Fatalf("failed to create regular user: %v", err)
	}

	// 2. Create a beta tester
	betaUser, err := repo.CreateUser("beta_tester_bob", "beta@test.com", "hash2", "player")
	if err != nil {
		t.Fatalf("failed to create beta user: %v", err)
	}
	betaUser.IsBetaTester = true
	if err := repo.SetUserBetaTester(betaUser.ID, true); err != nil {
		t.Fatalf("failed to set beta user: %v", err)
	}

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	// Test A: Regular user accesses /picks/advisor -> Should see beta_locked page
	reqA := httptest.NewRequest(http.MethodGet, "/picks/advisor", nil)
	reqA = reqA.WithContext(injectUser(reqA.Context(), regularUser))
	rrA := httptest.NewRecorder()

	picksHandler.ShowPicksAdvisor(rrA, reqA)

	if rrA.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for beta locked page, got %d", rrA.Code)
	}
	bodyA := rrA.Body.String()
	if !strings.Contains(bodyA, "Función Exclusiva para Beta Testers") && !strings.Contains(bodyA, "Beta Testers") {
		t.Errorf("expected beta locked notice in body, got: %s", bodyA)
	}

	// Test B: Beta tester accesses /picks/advisor -> Should see the full advisor page
	reqB := httptest.NewRequest(http.MethodGet, "/picks/advisor", nil)
	reqB = reqB.WithContext(injectUser(reqB.Context(), betaUser))
	rrB := httptest.NewRecorder()

	picksHandler.ShowPicksAdvisor(rrB, reqB)

	if rrB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for beta user, got %d", rrB.Code)
	}
	bodyB := rrB.Body.String()
	if !strings.Contains(bodyB, "Asistente de Picks") && !strings.Contains(bodyB, "Matriz de Riesgo") {
		t.Errorf("expected advisor content in body, got: %s", bodyB)
	}
	if !strings.Contains(bodyB, "Estrategia Activa") {
		t.Errorf("expected strategy section in body, got: %s", bodyB)
	}

	// Test C: HTMX partial swap request
	reqC := httptest.NewRequest(http.MethodGet, "/picks/advisor?preset=aggressive", nil)
	reqC.Header.Set("HX-Request", "true")
	reqC = reqC.WithContext(injectUser(reqC.Context(), betaUser))
	rrC := httptest.NewRecorder()

	picksHandler.ShowPicksAdvisor(rrC, reqC)

	if rrC.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for HTMX partial, got %d", rrC.Code)
	}
	bodyC := rrC.Body.String()
	// Partial should contain preset info and not full HTML base wrapper
	if !strings.Contains(bodyC, "matchesQuadrant") {
		t.Errorf("expected partial content with alpine logic, got: %s", bodyC)
	}
	if strings.Contains(bodyC, "<!DOCTYPE html>") {
		t.Errorf("expected partial, but found full HTML layout doctype")
	}
}

func TestApplyAdvisorPicks_SuccessAndFairPlay(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Create a beta tester
	betaUser, err := repo.CreateUser("beta_super", "betasuper@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("failed to create beta user: %v", err)
	}
	betaUser.IsBetaTester = true
	_ = repo.SetUserBetaTester(betaUser.ID, true)

	// Create non-beta user
	regularUser, err := repo.CreateUser("regular_pete", "regularpete@test.com", "hash", "player")
	if err != nil {
		t.Fatalf("failed to create regular user: %v", err)
	}

	season, _ := repo.GetActiveSeason(2026)
	week, err := repo.GetWeekByNumber(season.ID, 1)
	if err != nil {
		t.Fatalf("failed to get week 1: %v", err)
	}

	kcTeam, _ := repo.GetTeamByCode("KC")
	balTeam, _ := repo.GetTeamByCode("BAL")
	sfTeam, _ := repo.GetTeamByCode("SF")
	larTeam, _ := repo.GetTeamByCode("LAR")

	// Game 1: Open game in future
	g1, err := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kcTeam.ID,
		AwayTeamID:   balTeam.ID,
		KickoffTime:  time.Now().Add(24 * time.Hour),
		Status:       "scheduled",
		IsTiebreaker: false,
	})
	if err != nil {
		t.Fatalf("failed to create g1: %v", err)
	}

	// Game 2: Locked game in past
	g2, err := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   sfTeam.ID,
		AwayTeamID:   larTeam.ID,
		KickoffTime:  time.Now().Add(-2 * time.Hour),
		Status:       "in_progress",
		IsTiebreaker: false,
	})
	if err != nil {
		t.Fatalf("failed to create g2: %v", err)
	}

	// User already has an existing pick on locked game 2 (picked LAR)
	_, _ = repo.SavePick(betaUser.ID, g2.ID, &larTeam.ID, nil, nil)

	picksHandler := NewPicksHandler(repo, renderer, nil, 2026)

	// Test A: Regular user tries to POST apply -> 403 Forbidden
	form := url.Values{}
	form.Set("week_id", fmt.Sprintf("%d", week.ID))
	form.Set("preset", "conservative")

	reqA := httptest.NewRequest(http.MethodPost, "/picks/advisor/apply", strings.NewReader(form.Encode()))
	reqA.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqA = reqA.WithContext(injectUser(reqA.Context(), regularUser))
	rrA := httptest.NewRecorder()

	picksHandler.ApplyAdvisorPicks(rrA, reqA)

	if rrA.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-beta user, got %d", rrA.Code)
	}

	// Test B: Beta tester applies conservative strategy
	reqB := httptest.NewRequest(http.MethodPost, "/picks/advisor/apply", strings.NewReader(form.Encode()))
	reqB.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqB = reqB.WithContext(injectUser(reqB.Context(), betaUser))
	rrB := httptest.NewRecorder()

	picksHandler.ApplyAdvisorPicks(rrB, reqB)

	if rrB.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 SeeOther redirect, got %d", rrB.Code)
	}
	loc := rrB.Header().Get("Location")
	if !strings.Contains(loc, "applied=1") {
		t.Errorf("expected redirect location to contain applied=1, got: %s", loc)
	}

	// Verify Game 1 now has a pick saved in DB
	userPicks, err := repo.GetUserPicksForWeek(betaUser.ID, week.ID)
	if err != nil {
		t.Fatalf("failed to get user picks: %v", err)
	}

	p1 := userPicks[g1.ID]
	if p1 == nil || p1.PickedTeamID == nil {
		t.Errorf("expected pick for open game g1 to be saved, got nil")
	}

	// Verify locked Game 2 was NOT changed (should still be LAR, not overwritten by advisor)
	p2 := userPicks[g2.ID]
	if p2 == nil || p2.PickedTeamID == nil || *p2.PickedTeamID != larTeam.ID {
		t.Errorf("expected locked game g2 pick to remain unchanged (LAR), got %v", p2)
	}

	// Test C: HTMX request check (returns HX-Redirect header)
	reqC := httptest.NewRequest(http.MethodPost, "/picks/advisor/apply", strings.NewReader(form.Encode()))
	reqC.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqC.Header.Set("HX-Request", "true")
	reqC = reqC.WithContext(injectUser(reqC.Context(), betaUser))
	rrC := httptest.NewRecorder()

	picksHandler.ApplyAdvisorPicks(rrC, reqC)

	if rrC.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with HX-Redirect, got %d", rrC.Code)
	}
	hxRedirect := rrC.Header().Get("HX-Redirect")
	if !strings.Contains(hxRedirect, "applied=1") {
		t.Errorf("expected HX-Redirect header with applied=1, got: %s", hxRedirect)
	}
}
