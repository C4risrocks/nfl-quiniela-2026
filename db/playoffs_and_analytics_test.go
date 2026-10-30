package db

import (
	"os"
	"testing"
)

func TestUserPlayoffBracket_SaveAndGet(t *testing.T) {
	testDBPath := "test_playoffs_db.db"
	_ = os.Remove(testDBPath)
	defer func() {
		_ = os.Remove(testDBPath)
		_ = os.Remove(testDBPath + "-shm")
		_ = os.Remove(testDBPath + "-wal")
	}()

	database, err := InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)

	// Create test user
	user, err := repo.CreateUser("playoff_tester", "tester@example.com", "hash", "player")
	if err != nil {
		t.Fatalf("CreateUser error: %v", err)
	}

	// 1. Initial bracket save
	bracket := &UserPlayoffBracket{
		UserID:           user.ID,
		SeasonYear:       2026,
		WildCardPicks:    `{"AFC_2v7":"KC","AFC_3v6":"BUF","AFC_4v5":"BAL","NFC_2v7":"SF","NFC_3v6":"DET","NFC_4v5":"GB"}`,
		DivisionalPicks:  `{"AFC_1vLowest":"KC","AFC_Other":"BUF","NFC_1vLowest":"SF","NFC_Other":"DET"}`,
		ConferencePicks:  `{"AFC":"KC","NFC":"SF"}`,
		SuperBowlPick:    `{"winner":"KC","opponent":"SF"}`,
		TiebreakerPoints: 51,
	}

	if err := repo.SaveUserPlayoffBracket(bracket); err != nil {
		t.Fatalf("SaveUserPlayoffBracket error: %v", err)
	}

	// 2. Retrieve bracket
	retrieved, err := repo.GetUserPlayoffBracket(user.ID, 2026)
	if err != nil {
		t.Fatalf("GetUserPlayoffBracket error: %v", err)
	}
	if retrieved == nil {
		t.Fatalf("expected non-nil bracket")
	}

	if retrieved.TiebreakerPoints != 51 {
		t.Errorf("expected TiebreakerPoints 51, got %d", retrieved.TiebreakerPoints)
	}
	if retrieved.WildCardPicks != bracket.WildCardPicks {
		t.Errorf("expected wild card picks %s, got %s", bracket.WildCardPicks, retrieved.WildCardPicks)
	}

	// 3. Update bracket (UPSERT test)
	bracket.TiebreakerPoints = 54
	bracket.SuperBowlPick = `{"winner":"SF","opponent":"KC"}`
	if err := repo.SaveUserPlayoffBracket(bracket); err != nil {
		t.Fatalf("SaveUserPlayoffBracket update error: %v", err)
	}

	updated, err := repo.GetUserPlayoffBracket(user.ID, 2026)
	if err != nil {
		t.Fatalf("GetUserPlayoffBracket error: %v", err)
	}
	if updated.TiebreakerPoints != 54 {
		t.Errorf("expected updated TiebreakerPoints 54, got %d", updated.TiebreakerPoints)
	}
}

func TestAnalytics_TeamATSAndUserStats(t *testing.T) {
	testDBPath := "test_analytics_db.db"
	_ = os.Remove(testDBPath)
	defer func() {
		_ = os.Remove(testDBPath)
		_ = os.Remove(testDBPath + "-shm")
		_ = os.Remove(testDBPath + "-wal")
	}()

	database, err := InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)

	// Seed basic data
	if err := SeedDatabase(repo, "admin", "admin@example.com", "secret", 2026); err != nil {
		t.Fatalf("SeedDatabase error: %v", err)
	}

	// Test Team ATS calculation (even with initial games)
	atsTrends, err := repo.GetTeamATSTrends(2026)
	if err != nil {
		t.Fatalf("GetTeamATSTrends error: %v", err)
	}
	if len(atsTrends) != 32 {
		t.Errorf("expected 32 teams in ATS trends, got %d", len(atsTrends))
	}

	// Test user analytics with default admin
	admin, err := repo.GetUserByUsername("admin")
	if err != nil || admin == nil {
		t.Fatalf("GetUserByUsername admin error: %v", err)
	}

	analytics, err := repo.GetUserAnalytics(admin.ID, 2026)
	if err != nil {
		t.Fatalf("GetUserAnalytics error: %v", err)
	}
	if analytics == nil {
		t.Fatalf("expected non-nil UserAnalytics")
	}
	if len(analytics.TimeSlots) != 6 {
		t.Errorf("expected 6 time slot categories, got %d", len(analytics.TimeSlots))
	}

	// Test Matchup Intel on a game
	games, err := repo.ListGamesByWeek(1)
	if err != nil || len(games) == 0 {
		t.Fatalf("listing week 1 games: %v", err)
	}

	intel, err := repo.GetMatchupIntel(games[0].ID)
	if err != nil {
		t.Fatalf("GetMatchupIntel error: %v", err)
	}
	if intel == nil {
		t.Fatalf("expected non-nil MatchupIntel")
	}
	if intel.HomeTeam == nil || intel.AwayTeam == nil {
		t.Errorf("expected HomeTeam and AwayTeam to be populated")
	}
}
