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

func TestWeek1GracePeriodLockLogic(t *testing.T) {
	// Wednesday Sept 9, 2026 20:30 local (02:30 UTC Sept 10)
	nowDuringGrace := time.Date(2026, 9, 10, 2, 30, 0, 0, time.UTC)
	// Friday Sept 11, 2026 01:00 UTC (after Week1GraceDeadline 2026-09-11 00:35:00 UTC)
	nowAfterGrace := time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC)

	// Game 1: Today's game (NE vs SEA, kickoff Sept 10 00:20 UTC)
	todaysGame := &Game{
		ID:          1,
		WeekNumber:  1,
		KickoffTime: time.Date(2026, 9, 10, 0, 20, 0, 0, time.UTC),
		Status:      "in_progress",
	}

	firstKickoff := todaysGame.KickoffTime

	// 1. In progress today's game during grace period MUST NOT be locked
	if todaysGame.IsGameOrWeekLocked(nowDuringGrace, "per_game", &firstKickoff) {
		t.Errorf("Expected today's game to NOT be locked in per_game mode during grace period")
	}
	if todaysGame.IsGameOrWeekLocked(nowDuringGrace, "full_week", &firstKickoff) {
		t.Errorf("Expected today's game to NOT be locked in full_week mode during grace period")
	}
	if todaysGame.IsEffectivelyLocked(nowDuringGrace) {
		t.Errorf("Expected today's game to NOT be effectively locked during grace period")
	}
	if !todaysGame.IsGracePeriodActive(nowDuringGrace) {
		t.Errorf("Expected IsGracePeriodActive to be true for today's game during grace period")
	}

	// 2. Final today's game during grace period MUST NOT be locked
	todaysGame.Status = "final"
	if todaysGame.IsGameOrWeekLocked(nowDuringGrace, "per_game", &firstKickoff) {
		t.Errorf("Expected today's final game to NOT be locked in per_game mode during grace period")
	}
	if todaysGame.IsGameOrWeekLocked(nowDuringGrace, "full_week", &firstKickoff) {
		t.Errorf("Expected today's final game to NOT be locked in full_week mode during grace period")
	}

	// 3. Tomorrow's game (SF vs LAR, kickoff Sept 11 00:35 UTC)
	tomorrowsGame := &Game{
		ID:          2,
		WeekNumber:  1,
		KickoffTime: time.Date(2026, 9, 11, 0, 35, 0, 0, time.UTC),
		Status:      "scheduled",
	}
	if tomorrowsGame.IsGameOrWeekLocked(nowDuringGrace, "per_game", &firstKickoff) {
		t.Errorf("Expected tomorrow's game to NOT be locked during grace period")
	}
	if tomorrowsGame.IsGracePeriodActive(nowDuringGrace) {
		t.Errorf("Expected IsGracePeriodActive to be false for tomorrow's game before its kickoff")
	}

	// 4. After grace period has passed (Friday Sept 11)
	if !todaysGame.IsGameOrWeekLocked(nowAfterGrace, "per_game", &firstKickoff) {
		t.Errorf("Expected today's game to BE locked after grace period has expired")
	}
	if !todaysGame.IsGameOrWeekLocked(nowAfterGrace, "full_week", &firstKickoff) {
		t.Errorf("Expected today's game to BE locked after grace period has expired in full_week mode")
	}
	if todaysGame.IsGracePeriodActive(nowAfterGrace) {
		t.Errorf("Expected IsGracePeriodActive to be false after grace period has expired")
	}
	if !tomorrowsGame.IsGameOrWeekLocked(nowAfterGrace, "per_game", &firstKickoff) {
		t.Errorf("Expected tomorrow's game to BE locked after grace period has expired")
	}

	// 5. Week 2 game (grace exception should NOT apply)
	week2Game := &Game{
		ID:          50,
		WeekNumber:  2,
		KickoffTime: time.Date(2026, 9, 17, 0, 15, 0, 0, time.UTC),
		Status:      "in_progress",
	}
	w2Kickoff := week2Game.KickoffTime
	nowW2 := time.Date(2026, 9, 17, 1, 0, 0, 0, time.UTC)
	if !week2Game.IsGameOrWeekLocked(nowW2, "per_game", &w2Kickoff) {
		t.Errorf("Expected Week 2 in-progress game to be locked")
	}
}
