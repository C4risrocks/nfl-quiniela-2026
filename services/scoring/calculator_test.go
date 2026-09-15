package scoring

import (
	"os"
	"testing"
	"time"

	"nfl-quiniela-2026/db"
)

func TestScoringCalculatorModes(t *testing.T) {
	testDB := "test_scoring.db"
	defer os.Remove(testDB)

	database, err := db.InitDB("sqlite", testDB)
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	_ = db.SeedDatabase(repo, "admin", "admin@test.com", "pass", 2026)

	calc := NewCalculator(repo)

	// Mode 1: Weighted
	weightedCfg := &db.ScoringConfig{
		ScoringMode:      "weighted",
		WinnerPoints:     10,
		ExactScoreBonus:  5,
		ExactMarginBonus: 2,
	}

	homeScore := 28
	awayScore := 24
	homeID := int64(1) // KC
	awayID := int64(2) // BAL

	finalGame := &db.Game{
		ID:           1,
		HomeTeamID:   homeID,
		AwayTeamID:   awayID,
		HomeScore:    &homeScore,
		AwayScore:    &awayScore,
		Status:       "final",
		IsTiebreaker: true,
	}

	// Case 1: Exact Score Pick
	predH1 := 28
	predA1 := 24
	pick1 := &db.Pick{
		PickedTeamID:       &homeID,
		PredictedHomeScore: &predH1,
		PredictedAwayScore: &predA1,
	}

	pts, bonus, isCorrect := calc.CalculatePickScores(pick1, finalGame, weightedCfg)
	if !*isCorrect || pts != 10 || bonus != 7 { // 10 winner + 5 exact + 2 margin = 17 total
		t.Errorf("Pick1: expected correct=true, pts=10, bonus=7; got correct=%v, pts=%d, bonus=%d", *isCorrect, pts, bonus)
	}

	// Case 2: Correct Winner + Exact Margin (e.g. 21-17, diff = 4), but not exact score
	predH2 := 21
	predA2 := 17
	pick2 := &db.Pick{
		PickedTeamID:       &homeID,
		PredictedHomeScore: &predH2,
		PredictedAwayScore: &predA2,
	}

	pts, bonus, isCorrect = calc.CalculatePickScores(pick2, finalGame, weightedCfg)
	if !*isCorrect || pts != 10 || bonus != 2 { // 10 winner + 2 margin
		t.Errorf("Pick2: expected correct=true, pts=10, bonus=2; got correct=%v, pts=%d, bonus=%d", *isCorrect, pts, bonus)
	}

	// Case 3: Pure Tiebreaker Mode
	pureCfg := &db.ScoringConfig{
		ScoringMode:      "pure_tiebreaker",
		WinnerPoints:     1,
		ExactScoreBonus:  0,
		ExactMarginBonus: 0,
	}

	pts, bonus, isCorrect = calc.CalculatePickScores(pick1, finalGame, pureCfg)
	if !*isCorrect || pts != 1 || bonus != 0 {
		t.Errorf("Pure mode: expected correct=true, pts=1, bonus=0; got correct=%v, pts=%d, bonus=%d", *isCorrect, pts, bonus)
	}
}

func TestLeaderboardTiebreakerResolution(t *testing.T) {
	testDB := "test_lb.db"
	defer os.Remove(testDB)

	database, err := db.InitDB("sqlite", testDB)
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	_ = db.SeedDatabase(repo, "admin", "admin@test.com", "pass", 2026)
	calc := NewCalculator(repo)

	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 2)

	kc, _ := repo.GetTeamByCode("KC")
	bal, _ := repo.GetTeamByCode("BAL")

	hScore := 24
	aScore := 20
	// Game 1: MNF Tiebreaker (Total points = 44)
	game, _ := repo.CreateManualGame(&db.Game{
		WeekID:       week.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(-1 * time.Hour),
		HomeScore:    &hScore,
		AwayScore:    &aScore,
		Status:       "final",
		IsTiebreaker: true,
	})

	// User A: Picked KC, predicted 24-20 (Total 44 -> diff 0)
	userA, _ := repo.CreateUser("Alice", "alice@test.com", "hash", "player")
	pH_A := 24
	pA_A := 20
	_, _ = repo.SavePick(userA.ID, game.ID, &kc.ID, &pH_A, &pA_A)

	// User B: Picked KC, predicted 21-17 (Total 38 -> diff 6)
	userB, _ := repo.CreateUser("Bob", "bob@test.com", "hash", "player")
	pH_B := 21
	pA_B := 17
	_, _ = repo.SavePick(userB.ID, game.ID, &kc.ID, &pH_B, &pA_B)

	// Calculate in pure mode so both get 1 winner point
	_ = repo.SetScoringConfig(&db.ScoringConfig{
		ScoringMode:  "pure_tiebreaker",
		WinnerPoints: 1,
	})

	err = calc.CalculateWeekScores(week.ID)
	if err != nil {
		t.Fatalf("CalculateWeekScores failed: %v", err)
	}

	lb, err := repo.GetWeeklyLeaderboard(week.ID)
	if err != nil {
		t.Fatalf("GetWeeklyLeaderboard failed: %v", err)
	}

	if len(lb) < 2 {
		t.Fatalf("Expected at least 2 users in leaderboard, got %d", len(lb))
	}

	// Alice must be Rank 1 due to 0 tiebreaker error vs Bob's 6
	if lb[0].Username != "Alice" || lb[0].Rank != 1 {
		t.Errorf("Expected Alice at Rank 1, got %+v", lb[0])
	}
	if lb[1].Username != "Bob" || lb[1].Rank != 2 {
		t.Errorf("Expected Bob at Rank 2, got %+v", lb[1])
	}
}

func TestStandardCompetitionRankingAndTiebreakers(t *testing.T) {
	testDB := "test_comp_rank.db"
	defer os.Remove(testDB)

	database, err := db.InitDB("sqlite", testDB)
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)
	_ = db.SeedDatabase(repo, "admin", "admin@test.com", "pass", 2026)
	calc := NewCalculator(repo)

	season, _ := repo.GetActiveSeason(2026)
	week2, _ := repo.GetWeekByNumber(season.ID, 2)

	kc, _ := repo.GetTeamByCode("KC")
	bal, _ := repo.GetTeamByCode("BAL")

	hScore := 24
	aScore := 20
	// Game in Week 2: MNF Tiebreaker (Total points = 44)
	tbGame, err := repo.CreateManualGame(&db.Game{
		WeekID:       week2.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(-1 * time.Hour),
		HomeScore:    &hScore,
		AwayScore:    &aScore,
		Status:       "final",
		IsTiebreaker: true,
	})
	if err != nil {
		t.Fatalf("CreateManualGame failed: %v", err)
	}

	userC, _ := repo.CreateUser("Charlie", "c@test.com", "hash", "player")
	userD, _ := repo.CreateUser("David", "d@test.com", "hash", "player")
	userE, _ := repo.CreateUser("Eve", "e@test.com", "hash", "player")

	// Charlie: picked KC, predicted 24-20 (diff 0)
	pC_H, pC_A := 24, 20
	_, _ = repo.SavePick(userC.ID, tbGame.ID, &kc.ID, &pC_H, &pC_A)

	// David: picked KC, predicted 24-20 (diff 0)
	pD_H, pD_A := 24, 20
	_, _ = repo.SavePick(userD.ID, tbGame.ID, &kc.ID, &pD_H, &pD_A)

	// Eve: picked KC, predicted 20-15 (diff 9)
	pE_H, pE_A := 20, 15
	_, _ = repo.SavePick(userE.ID, tbGame.ID, &kc.ID, &pE_H, &pE_A)

	_ = repo.SetScoringConfig(&db.ScoringConfig{
		ScoringMode:  "pure_tiebreaker",
		WinnerPoints: 1,
	})

	if err := calc.CalculateWeekScores(week2.ID); err != nil {
		t.Fatalf("CalculateWeekScores week 2 failed: %v", err)
	}

	lb, err := repo.GetWeeklyLeaderboard(week2.ID)
	if err != nil {
		t.Fatalf("GetWeeklyLeaderboard failed: %v", err)
	}
	if len(lb) != 3 {
		t.Fatalf("Expected 3 entries in weekly leaderboard, got %d", len(lb))
	}

	// Charlie and David should tie for Rank 1 (both have 1 pt, diff 0)
	if lb[0].Rank != 1 || lb[1].Rank != 1 {
		t.Errorf("Expected both first two players to share Rank 1, got ranks %d and %d", lb[0].Rank, lb[1].Rank)
	}
	// Eve should have Rank 3 (standard competition ranking: 1, 1, 3)
	if lb[2].Username != "Eve" || lb[2].Rank != 3 {
		t.Errorf("Expected Eve at Rank 3, got %+v", lb[2])
	}
	if lb[2].TiebreakerError != 9 || !lb[2].HasTiebreaker {
		t.Errorf("Expected Eve to have TiebreakerError=9 and HasTiebreaker=true, got %+v", lb[2])
	}

	// Verify Season Leaderboard decouples MNF tiebreaker
	seasonLB, err := repo.GetSeasonLeaderboard(season.ID)
	if err != nil {
		t.Fatalf("GetSeasonLeaderboard failed: %v", err)
	}
	for _, entry := range seasonLB {
		if entry.HasTiebreaker {
			t.Errorf("Expected season leaderboard HasTiebreaker=false, got true for %s", entry.Username)
		}
		if entry.TiebreakerError != 0 {
			t.Errorf("Expected season leaderboard TiebreakerError=0, got %d for %s", entry.TiebreakerError, entry.Username)
		}
		// Since all 3 have 1 pt and 1 pick in Season, all 3 should share Rank 1
		if entry.Rank != 1 {
			t.Errorf("Expected %s to be tied at Rank 1 in season leaderboard, got %d", entry.Username, entry.Rank)
		}
	}
}

