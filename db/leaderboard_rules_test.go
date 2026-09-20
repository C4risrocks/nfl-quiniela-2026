package db

import (
	"math"
	"os"
	"testing"
)

func TestSeasonLeaderboardRulesCompliance(t *testing.T) {
	testDB := "test_season_rules.db"
	defer os.Remove(testDB)

	database, err := InitDB("sqlite", testDB)
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)
	_ = SeedDatabase(repo, "admin_user", "admin@test.com", "pass", 2026)

	season, err := repo.GetActiveSeason(2026)
	if err != nil || season == nil {
		t.Fatalf("Active season not found: %v", err)
	}

	// Create test users (players and admin)
	userPointsKing, _ := repo.CreateUser("Player_PointsKing", "king@test.com", "hash", "player")
	userAciertosWin, _ := repo.CreateUser("Player_AciertosWin", "aciertos@test.com", "hash", "player")
	userEffWin, _ := repo.CreateUser("Player_EffWin", "eff@test.com", "hash", "player")
	userEffLower, _ := repo.CreateUser("Player_EffLower", "efflow@test.com", "hash", "player")
	userTwin1, _ := repo.CreateUser("Player_Twin1", "twin1@test.com", "hash", "player")
	userTwin2, _ := repo.CreateUser("Player_Twin2", "twin2@test.com", "hash", "player")
	_, _ = repo.CreateUser("Player_ZeroPicks", "zero@test.com", "hash", "player")

	// Ensure admin user exists with 'admin' role
	adminUser, _ := repo.GetUserByUsername("admin_user")
	if adminUser == nil {
		t.Fatalf("Admin user not found")
	}

	// We create artificial weekly_leaderboard entries across weeks to test exact multi-week aggregation:
	// Week 1 (ID=1) & Week 2 (ID=2)
	//
	// SCENARIOS:
	// 1. Player_PointsKing:
	//    Week 1: 100 pts, 10 correct, 16 picks
	//    Week 2: 100 pts, 10 correct, 16 picks
	//    Total: 200 pts, 20 correct, 32 picks (62.5%) -> Should be Rank 1!
	//
	// 2. Player_AciertosWin vs Player_EffWin vs Player_EffLower (All tie at 150 points!):
	//    - Player_AciertosWin:
	//      Week 1: 75 pts, 15 correct, 16 picks
	//      Week 2: 75 pts, 15 correct, 16 picks
	//      Total: 150 pts, 30 correct, 32 picks (93.75%) -> Wins tiebreaker 1 (30 aciertos vs 25)!
	//    - Player_EffWin:
	//      Week 1: 75 pts, 12 correct, 14 picks
	//      Week 2: 75 pts, 13 correct, 14 picks
	//      Total: 150 pts, 25 correct, 28 picks (89.28%) -> Ties aciertos with EffLower, but wins tiebreaker 2 (89.3% vs 78.1%)!
	//    - Player_EffLower:
	//      Week 1: 75 pts, 12 correct, 16 picks
	//      Week 2: 75 pts, 13 correct, 16 picks
	//      Total: 150 pts, 25 correct, 32 picks (78.12%) -> Has lower win% than EffWin.
	//
	// 3. Player_Twin1 & Player_Twin2 (Both tie at 100 points, 10 correct, 10 picks = 100%):
	//    - Twin 1 has MNF tiebreaker error = 0, winner = true
	//    - Twin 2 has MNF tiebreaker error = 50, winner = false
	//    - Rule: MNF tiebreaker does NOT apply to season standings! They MUST share the exact same rank!
	//
	// 4. Admin User:
	//    Week 1: 500 pts -> Must be completely excluded.
	//
	// 5. Player_ZeroPicks:
	//    0 picks, 0 points -> Must appear gracefully at the bottom.

	week1, _ := repo.GetWeekByNumber(season.ID, 1)
	week2, _ := repo.GetWeekByNumber(season.ID, 2)

	// Populate Week 1
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userPointsKing.ID, TotalPoints: 100, CorrectPicks: 10, TotalPicks: 16, Rank: 1}, week1.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userAciertosWin.ID, TotalPoints: 75, CorrectPicks: 15, TotalPicks: 16, Rank: 2}, week1.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userEffWin.ID, TotalPoints: 75, CorrectPicks: 12, TotalPicks: 14, Rank: 3}, week1.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userEffLower.ID, TotalPoints: 75, CorrectPicks: 12, TotalPicks: 16, Rank: 4}, week1.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userTwin1.ID, TotalPoints: 50, CorrectPicks: 5, TotalPicks: 5, TiebreakerError: 0, TiebreakerWinnerCorrect: true, Rank: 5}, week1.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userTwin2.ID, TotalPoints: 50, CorrectPicks: 5, TotalPicks: 5, TiebreakerError: 50, TiebreakerWinnerCorrect: false, Rank: 6}, week1.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: adminUser.ID, TotalPoints: 500, CorrectPicks: 50, TotalPicks: 50, Rank: 1}, week1.ID)

	// Populate Week 2
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userPointsKing.ID, TotalPoints: 100, CorrectPicks: 10, TotalPicks: 16, Rank: 1}, week2.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userAciertosWin.ID, TotalPoints: 75, CorrectPicks: 15, TotalPicks: 16, Rank: 2}, week2.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userEffWin.ID, TotalPoints: 75, CorrectPicks: 13, TotalPicks: 14, Rank: 3}, week2.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userEffLower.ID, TotalPoints: 75, CorrectPicks: 13, TotalPicks: 16, Rank: 4}, week2.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userTwin1.ID, TotalPoints: 50, CorrectPicks: 5, TotalPicks: 5, TiebreakerError: 0, TiebreakerWinnerCorrect: true, Rank: 5}, week2.ID)
	_ = repo.UpsertWeeklyLeaderboard(&LeaderboardEntry{UserID: userTwin2.ID, TotalPoints: 50, CorrectPicks: 5, TotalPicks: 5, TiebreakerError: 50, TiebreakerWinnerCorrect: false, Rank: 6}, week2.ID)

	// Execute GetSeasonLeaderboard
	entries, err := repo.GetSeasonLeaderboard(season.ID)
	if err != nil {
		t.Fatalf("GetSeasonLeaderboard returned error: %v", err)
	}

	// Verify Admin is excluded
	for _, e := range entries {
		if e.UserID == adminUser.ID || e.Username == adminUser.Username {
			t.Fatalf("Admin user %s was not excluded from season leaderboard", e.Username)
		}
	}

	entryMap := make(map[string]*LeaderboardEntry)
	for _, e := range entries {
		entryMap[e.Username] = e
	}

	// Rule 1 Verification: Points Supremacy (Player_PointsKing must be Rank 1)
	king := entryMap["Player_PointsKing"]
	if king == nil || king.Rank != 1 || king.TotalPoints != 200 {
		t.Errorf("Expected Player_PointsKing to be Rank 1 with 200 pts, got: %+v", king)
	}

	// Rule 2 Verification: Desempate 1 (Mayor cantidad de aciertos directos)
	// Player_AciertosWin (30 aciertos) beats Player_EffWin (25 aciertos)
	aciertosWin := entryMap["Player_AciertosWin"]
	effWin := entryMap["Player_EffWin"]
	effLower := entryMap["Player_EffLower"]

	if aciertosWin == nil || effWin == nil || effLower == nil {
		t.Fatalf("Missing test players in season leaderboard")
	}

	if aciertosWin.Rank != 2 {
		t.Errorf("Expected Player_AciertosWin to be Rank 2 due to highest direct correct picks (30), got Rank %d", aciertosWin.Rank)
	}

	// Rule 3 Verification: Desempate 2 (Mayor porcentaje de efectividad)
	// Player_EffWin (25/28 = 89.3%) beats Player_EffLower (25/32 = 78.1%)
	if effWin.Rank != 3 {
		t.Errorf("Expected Player_EffWin to be Rank 3 due to higher win pct (89.3%%), got Rank %d", effWin.Rank)
	}
	if effLower.Rank != 4 {
		t.Errorf("Expected Player_EffLower to be Rank 4 due to lower win pct (78.1%%), got Rank %d", effLower.Rank)
	}

	// Rule 4 & 5 Verification: Shared Rank (Podio Compartido) and Decoupling from MNF tiebreaker
	// Player_Twin1 and Player_Twin2 have identical points (100), identical aciertos (10), identical picks (10).
	// Twin 1 had MNF error 0 and winner correct; Twin 2 had MNF error 50 and winner false.
	// Under season rules, MNF is IGNORED, so BOTH must share Rank 5!
	twin1 := entryMap["Player_Twin1"]
	twin2 := entryMap["Player_Twin2"]
	if twin1 == nil || twin2 == nil {
		t.Fatalf("Twin players missing from leaderboard")
	}

	if twin1.Rank != 5 || twin2.Rank != 5 {
		t.Errorf("Expected Player_Twin1 and Player_Twin2 to both share Rank 5, got twin1=%d, twin2=%d", twin1.Rank, twin2.Rank)
	}
	if twin1.HasTiebreaker || twin2.HasTiebreaker {
		t.Errorf("Expected HasTiebreaker to be false in season leaderboard for twins")
	}
	if twin1.TiebreakerError != 0 || twin2.TiebreakerError != 0 {
		t.Errorf("Expected TiebreakerError to be 0 in season leaderboard")
	}

	// Next player after twins (ZeroPicks) should have Rank 7 (standard competition 1224 ranking: 1, 2, 3, 4, 5, 5, 7)
	zero := entryMap["Player_ZeroPicks"]
	if zero == nil {
		t.Fatalf("Player_ZeroPicks missing from leaderboard")
	}
	if zero.Rank != 7 {
		t.Errorf("Expected Player_ZeroPicks to be Rank 7 following shared rank 5, got Rank %d", zero.Rank)
	}
	if zero.TotalPoints != 0 || zero.CorrectPicks != 0 || zero.TotalPicks != 0 {
		t.Errorf("Expected zero values for Player_ZeroPicks, got %+v", zero)
	}
	if math.Abs(zero.WinPercentage-0.0) > 0.001 {
		t.Errorf("Expected 0.0 WinPercentage for zero picks player, got %f", zero.WinPercentage)
	}
}
