package db

import (
	"os"
	"testing"
	"time"
)

func TestDatabaseAndRepository(t *testing.T) {
	testDBPath := "test_quiniela.db"
	defer os.Remove(testDBPath)

	database, err := InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)

	// Test Seeds
	err = SeedDatabase(repo, "testadmin", "testadmin@test.com", "pass123", 2026)
	if err != nil {
		t.Fatalf("Failed to seed database: %v", err)
	}

	// Test Teams
	teams, err := repo.ListTeams()
	if err != nil || len(teams) != 32 {
		t.Fatalf("Expected 32 teams, got %d, err: %v", len(teams), err)
	}

	// Test Season & Weeks
	season, err := repo.GetActiveSeason(2026)
	if err != nil || season == nil {
		t.Fatalf("Expected active season, err: %v", err)
	}

	weeks, err := repo.ListWeeks(season.ID)
	if err != nil || len(weeks) != 22 {
		t.Fatalf("Expected 22 weeks, got %d, err: %v", len(weeks), err)
	}

	// Test Game creation
	kc, _ := repo.GetTeamByCode("KC")
	bal, _ := repo.GetTeamByCode("BAL")
	if kc == nil || bal == nil {
		t.Fatalf("Teams KC or BAL not found")
	}

	game, err := repo.CreateManualGame(&Game{
		WeekID:       weeks[0].ID,
		ESPNGameID:   "test-espn-1",
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(24 * time.Hour),
		Status:       "scheduled",
		StatusDetail: "Sun, 1:00 PM",
		IsTiebreaker: true,
	})
	if err != nil || game == nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Test Pick saving
	user, err := repo.CreateUser("player1", "player1@test.com", "hash", "player")
	if err != nil || user == nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	predHome := 27
	predAway := 20
	pick, err := repo.SavePick(user.ID, game.ID, &kc.ID, &predHome, &predAway)
	if err != nil || pick == nil {
		t.Fatalf("Failed to save pick: %v", err)
	}

	if *pick.PickedTeamID != kc.ID {
		t.Fatalf("Expected picked team to be KC, got %v", pick.PickedTeamID)
	}

	// Test scoring config
	cfg, err := repo.GetScoringConfig()
	if err != nil {
		t.Fatalf("Failed to get scoring config: %v", err)
	}
	if cfg.ScoringMode != "weighted" {
		t.Fatalf("Expected default scoring mode 'weighted', got '%s'", cfg.ScoringMode)
	}

	cfg.ScoringMode = "pure_tiebreaker"
	cfg.WinnerPoints = 1
	err = repo.SetScoringConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to set scoring config: %v", err)
	}

	cfgUpdated, _ := repo.GetScoringConfig()
	if cfgUpdated.ScoringMode != "pure_tiebreaker" || cfgUpdated.WinnerPoints != 1 {
		t.Fatalf("Expected updated scoring config, got %+v", cfgUpdated)
	}
}
