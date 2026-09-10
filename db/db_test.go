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

	// Test Leaderboard with unevaluated vs evaluated tiebreaker
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{
		UserID:          user.ID,
		TotalPoints:     10,
		CorrectPicks:    1,
		TotalPicks:      1,
		TiebreakerError: 999, // unevaluated week
		Rank:            1,
	}, weeks[0].ID)

	seasonLB, err := repo.GetSeasonLeaderboard(season.ID)
	if err != nil {
		t.Fatalf("Failed to get season leaderboard: %v", err)
	}
	if len(seasonLB) == 0 {
		t.Fatalf("Expected entries in season leaderboard")
	}
	// Should not have evaluated tiebreaker (HasTiebreaker=false, error=0 not 999 or 21978)
	if seasonLB[0].HasTiebreaker {
		t.Errorf("Expected HasTiebreaker to be false for unevaluated week, got true")
	}
	if seasonLB[0].TiebreakerError != 0 {
		t.Errorf("Expected TiebreakerError to be 0 for unevaluated week, got %d", seasonLB[0].TiebreakerError)
	}

	// Now simulate a week with evaluated tiebreaker error = 3
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{
		UserID:          user.ID,
		TotalPoints:     20,
		CorrectPicks:    2,
		TotalPicks:      2,
		TiebreakerError: 3, // evaluated!
		Rank:            1,
	}, weeks[1].ID)

	seasonLBAfter, err := repo.GetSeasonLeaderboard(season.ID)
	if err != nil {
		t.Fatalf("Failed to get season leaderboard: %v", err)
	}
	if !seasonLBAfter[0].HasTiebreaker {
		t.Errorf("Expected HasTiebreaker to be true after evaluated week")
	}
	if seasonLBAfter[0].TiebreakerError != 3 {
		t.Errorf("Expected TiebreakerError to be 3, got %d", seasonLBAfter[0].TiebreakerError)
	}
}

func TestStoragePersistenceAndCheckpoint(t *testing.T) {
	testDB := "test_persist.db"
	defer os.Remove(testDB)
	defer os.Remove(testDB + "-wal")
	defer os.Remove(testDB + "-shm")

	database, err := InitDB("sqlite", testDB)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	if database.DBPath == "" {
		t.Errorf("Expected DBPath to be populated")
	}

	if !database.IsStorageWritable() {
		t.Errorf("Expected storage to be writable")
	}

	if err := database.CheckpointWAL(); err != nil {
		t.Errorf("CheckpointWAL failed: %v", err)
	}
}

func TestScoringSettingsPersistAcrossRestarts(t *testing.T) {
	testDB := "test_settings_persist.db"
	defer os.Remove(testDB)
	defer os.Remove(testDB + "-wal")
	defer os.Remove(testDB + "-shm")

	database, err := InitDB("sqlite", testDB)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)

	// First startup: Seed database
	if err := SeedDatabase(repo, "admin", "admin@test.com", "pass123", 2026); err != nil {
		t.Fatalf("First seed failed: %v", err)
	}

	// Admin customizes settings to full_week and simple
	customCfg := &ScoringConfig{
		ScoringMode:      "simple",
		WinnerPoints:     5,
		ExactScoreBonus:  0,
		ExactMarginBonus: 0,
		LockMode:         "full_week",
	}
	if err := repo.SetScoringConfig(customCfg); err != nil {
		t.Fatalf("SetScoringConfig failed: %v", err)
	}

	// Verify customized settings are saved
	cfg1, _ := repo.GetScoringConfig()
	if cfg1.LockMode != "full_week" || cfg1.ScoringMode != "simple" {
		t.Fatalf("Expected full_week/simple, got %s/%s", cfg1.LockMode, cfg1.ScoringMode)
	}

	// Second startup / redeployment: SeedDatabase runs again!
	if err := SeedDatabase(repo, "admin", "admin@test.com", "pass123", 2026); err != nil {
		t.Fatalf("Second seed failed: %v", err)
	}

	// Verify custom settings were NOT overwritten by defaults!
	cfg2, _ := repo.GetScoringConfig()
	if cfg2.LockMode != "full_week" {
		t.Errorf("Expected LockMode to remain full_week after restart, got %s", cfg2.LockMode)
	}
	if cfg2.ScoringMode != "simple" {
		t.Errorf("Expected ScoringMode to remain simple after restart, got %s", cfg2.ScoringMode)
	}
	if cfg2.WinnerPoints != 5 {
		t.Errorf("Expected WinnerPoints to remain 5 after restart, got %d", cfg2.WinnerPoints)
	}
}
