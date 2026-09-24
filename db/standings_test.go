package db

import (
	"os"
	"testing"
	"time"
)

func TestCalculateLocalStandingsAndTeamStats(t *testing.T) {
	testDBPath := "test_standings.db"
	defer os.Remove(testDBPath)

	database, err := InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)
	if err := SeedDatabase(repo, "admin", "admin@test.com", "pass123", 2026); err != nil {
		t.Fatalf("Failed to seed database: %v", err)
	}

	season, err := repo.GetActiveSeason(2026)
	if err != nil {
		t.Fatalf("Failed to get season: %v", err)
	}

	weeks, err := repo.ListWeeks(season.ID)
	if err != nil || len(weeks) == 0 {
		t.Fatalf("Failed to list weeks: %v", err)
	}
	w1 := weeks[0]

	kc, err := repo.GetTeamByCode("KC")
	if err != nil || kc == nil {
		t.Fatalf("KC team not found: %v", err)
	}
	bal, err := repo.GetTeamByCode("BAL")
	if err != nil || bal == nil {
		t.Fatalf("BAL team not found: %v", err)
	}

	// Clear any seeded games to ensure isolated deterministic test data
	_, _ = database.Exec("DELETE FROM games")

	// Create completed game: KC (home) 27 - BAL (away) 20
	hScore := 27
	aScore := 20
	game1 := &Game{
		WeekID:       w1.ID,
		ESPNGameID:   "test_game_1",
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(-2 * time.Hour),
		HomeScore:    &hScore,
		AwayScore:    &aScore,
		Status:       "final",
		StatusDetail: "Final",
	}
	if err := repo.UpsertGameByESPNID(game1); err != nil {
		t.Fatalf("Failed to upsert game: %v", err)
	}

	// Test CalculateLocalStandings
	standings, err := repo.CalculateLocalStandings(2026)
	if err != nil {
		t.Fatalf("CalculateLocalStandings error: %v", err)
	}
	if standings == nil {
		t.Fatal("expected non-nil standings")
	}

	// Verify KC has 1 Win, BAL has 1 Loss
	var kcStanding, balStanding *TeamStanding
	for _, ts := range standings.League {
		if ts.TeamCode == "KC" {
			kcStanding = ts
		} else if ts.TeamCode == "BAL" {
			balStanding = ts
		}
	}

	if kcStanding == nil {
		t.Fatal("KC not found in local standings")
	}
	if kcStanding.Wins != 1 || kcStanding.Losses != 0 {
		t.Errorf("expected KC 1-0, got %d-%d", kcStanding.Wins, kcStanding.Losses)
	}
	if kcStanding.PointsFor != 27 || kcStanding.PointsAgainst != 20 || kcStanding.PointDiff != 7 {
		t.Errorf("expected KC PF: 27, PA: 20, DIFF: 7, got PF: %d, PA: %d, DIFF: %d",
			kcStanding.PointsFor, kcStanding.PointsAgainst, kcStanding.PointDiff)
	}
	if kcStanding.Streak != "W1" {
		t.Errorf("expected KC streak W1, got %s", kcStanding.Streak)
	}
	if kcStanding.HomeRecord != "1-0" {
		t.Errorf("expected KC HomeRecord 1-0, got %s", kcStanding.HomeRecord)
	}

	if balStanding == nil {
		t.Fatal("BAL not found in local standings")
	}
	if balStanding.Wins != 0 || balStanding.Losses != 1 {
		t.Errorf("expected BAL 0-1, got %d-%d", balStanding.Wins, balStanding.Losses)
	}
	if balStanding.Streak != "L1" {
		t.Errorf("expected BAL streak L1, got %s", balStanding.Streak)
	}
	if balStanding.AwayRecord != "0-1" {
		t.Errorf("expected BAL AwayRecord 0-1, got %s", balStanding.AwayRecord)
	}

	// Verify Summary
	if standings.Summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if standings.Summary.TopRecordTeam == nil || standings.Summary.TopRecordTeam.TeamCode != "KC" {
		t.Errorf("expected TopRecordTeam KC")
	}

	// Test ListTeamGamesBySeason
	kcGames, err := repo.ListTeamGamesBySeason("KC", 2026)
	if err != nil {
		t.Fatalf("ListTeamGamesBySeason error: %v", err)
	}
	if len(kcGames) != 1 {
		t.Fatalf("expected 1 game for KC, got %d", len(kcGames))
	}
	if kcGames[0].OpponentCode != "BAL" || kcGames[0].Result != "W" {
		t.Errorf("expected KC game vs BAL Result W, got %s vs %s", kcGames[0].Result, kcGames[0].OpponentCode)
	}

	// Test GetTeamCommunityStats
	user, err := repo.GetUserByUsername("admin")
	if err != nil || user == nil {
		t.Fatalf("admin user not found: %v", err)
	}
	// Set KC as favorite team
	if err := repo.UpdateUserPreferences(user.ID, "", &kc.ID, true, "", ""); err != nil {
		t.Fatalf("UpdateUserPreferences error: %v", err)
	}

	// Create pick for KC on game1
	gamesW1, err := repo.ListGamesByWeek(w1.ID)
	if err != nil || len(gamesW1) == 0 {
		t.Fatalf("ListGamesByWeek error: %v", err)
	}
	savedGame := gamesW1[0]
	if _, err := repo.SavePick(user.ID, savedGame.ID, &kc.ID, nil, nil); err != nil {
		t.Fatalf("SavePick error: %v", err)
	}
	_, _ = database.Exec("UPDATE picks SET is_correct = 1 WHERE user_id = ? AND game_id = ?", user.ID, savedGame.ID)

	commStats, err := repo.GetTeamCommunityStats(kc.ID, 2026)
	if err != nil {
		t.Fatalf("GetTeamCommunityStats error: %v", err)
	}
	if commStats == nil {
		t.Fatal("expected non-nil community stats")
	}
	if len(commStats.FavoriteUsers) != 1 || commStats.FavoriteUsers[0].Username != "admin" {
		t.Errorf("expected 1 favorite user (admin), got %d", len(commStats.FavoriteUsers))
	}
	if commStats.TotalPicksMade != 1 || commStats.WinningPicks != 1 || commStats.PickWinRate != 100 {
		t.Errorf("expected 1 pick, 1 win (100%%), got %d picks, %d wins, %d%%",
			commStats.TotalPicksMade, commStats.WinningPicks, commStats.PickWinRate)
	}
}
