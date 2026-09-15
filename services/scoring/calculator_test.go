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

func TestTiebreakerWinnerPriorityAndPointsSupremacy(t *testing.T) {
	testDB := "test_tb_priority.db"
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
	week3, _ := repo.GetWeekByNumber(season.ID, 3)

	kc, _ := repo.GetTeamByCode("KC")
	bal, _ := repo.GetTeamByCode("BAL")
	sf, _ := repo.GetTeamByCode("SF")
	lar, _ := repo.GetTeamByCode("LAR")

	// Game 1: Normal Game (KC 20, BAL 10 -> Winner KC)
	hScore1, aScore1 := 20, 10
	game1, err := repo.CreateManualGame(&db.Game{
		WeekID:       week3.ID,
		HomeTeamID:   kc.ID,
		AwayTeamID:   bal.ID,
		KickoffTime:  time.Now().Add(-2 * time.Hour),
		HomeScore:    &hScore1,
		AwayScore:    &aScore1,
		Status:       "final",
		IsTiebreaker: false,
	})
	if err != nil {
		t.Fatalf("CreateManualGame game1 failed: %v", err)
	}

	// Game 2: MNF Tiebreaker (SF 24, LAR 20 -> Winner SF, Total 44)
	hScore2, aScore2 := 24, 20
	tbGame, err := repo.CreateManualGame(&db.Game{
		WeekID:       week3.ID,
		HomeTeamID:   sf.ID,
		AwayTeamID:   lar.ID,
		KickoffTime:  time.Now().Add(-1 * time.Hour),
		HomeScore:    &hScore2,
		AwayScore:    &aScore2,
		Status:       "final",
		IsTiebreaker: true,
	})
	if err != nil {
		t.Fatalf("CreateManualGame tbGame failed: %v", err)
	}

	// User Victor: Picks KC (correct) & SF (correct) -> 2 points
	userVictor, _ := repo.CreateUser("Victor", "victor@test.com", "hash", "player")
	_, _ = repo.SavePick(userVictor.ID, game1.ID, &kc.ID, nil, nil)
	pVH, pVA := 10, 10 // Total 20 -> error 24
	_, _ = repo.SavePick(userVictor.ID, tbGame.ID, &sf.ID, &pVH, &pVA)

	// User Mariana: Picks BAL (incorrect) & SF (correct) -> 1 point, tb error 9
	userMariana, _ := repo.CreateUser("Mariana", "mariana@test.com", "hash", "player")
	_, _ = repo.SavePick(userMariana.ID, game1.ID, &bal.ID, nil, nil)
	pMH, pMA := 20, 15 // Total 35 -> error 9
	_, _ = repo.SavePick(userMariana.ID, tbGame.ID, &sf.ID, &pMH, &pMA)

	// User Oscar: Picks KC (correct) & LAR (INCORRECT winner) -> 1 point, tb error 0
	userOscar, _ := repo.CreateUser("Oscar", "oscar@test.com", "hash", "player")
	_, _ = repo.SavePick(userOscar.ID, game1.ID, &kc.ID, nil, nil)
	pOH, pOA := 24, 20 // Total 44 -> error 0
	_, _ = repo.SavePick(userOscar.ID, tbGame.ID, &lar.ID, &pOH, &pOA)

	_ = repo.SetScoringConfig(&db.ScoringConfig{
		ScoringMode:  "pure_tiebreaker",
		WinnerPoints: 1,
	})

	if err := calc.CalculateWeekScores(week3.ID); err != nil {
		t.Fatalf("CalculateWeekScores failed: %v", err)
	}

	lb, err := repo.GetWeeklyLeaderboard(week3.ID)
	if err != nil {
		t.Fatalf("GetWeeklyLeaderboard failed: %v", err)
	}
	if len(lb) != 3 {
		t.Fatalf("Expected 3 users in leaderboard, got %d", len(lb))
	}

	// 1. Victor MUST be Rank 1 because he has 2 points (most points always wins the week)
	if lb[0].Username != "Victor" || lb[0].Rank != 1 || lb[0].TotalPoints != 2 {
		t.Errorf("Expected Victor at Rank 1 with 2 points, got %+v", lb[0])
	}

	// 2. Mariana and Oscar tie in points (1 pt each).
	// Mariana picked the WINNER of MNF (SF), while Oscar picked LAR (loser).
	// Therefore, Mariana MUST be Rank 2, and Oscar MUST be Rank 3 (despite Oscar's 0 pt error).
	if lb[1].Username != "Mariana" || lb[1].Rank != 2 || !lb[1].TiebreakerWinnerCorrect {
		t.Errorf("Expected Mariana at Rank 2 with TiebreakerWinnerCorrect=true, got %+v", lb[1])
	}
	if lb[2].Username != "Oscar" || lb[2].Rank != 3 || lb[2].TiebreakerWinnerCorrect {
		t.Errorf("Expected Oscar at Rank 3 with TiebreakerWinnerCorrect=false, got %+v", lb[2])
	}

	// 3. Season Leaderboard check:
	// Victor: 2 pts -> Rank 1
	// Mariana & Oscar: 1 pt each -> share Rank 2 (1, 2, 2)
	seasonLB, err := repo.GetSeasonLeaderboard(season.ID)
	if err != nil {
		t.Fatalf("GetSeasonLeaderboard failed: %v", err)
	}
	if len(seasonLB) < 3 {
		t.Fatalf("Expected at least 3 users in season leaderboard")
	}
	var sVictor, sMariana, sOscar *db.LeaderboardEntry
	for _, e := range seasonLB {
		switch e.Username {
		case "Victor":
			sVictor = e
		case "Mariana":
			sMariana = e
		case "Oscar":
			sOscar = e
		}
	}
	if sVictor == nil || sVictor.Rank != 1 {
		t.Errorf("Expected Victor at Rank 1 in season leaderboard, got %+v", sVictor)
	}
	if sMariana == nil || sMariana.Rank != 2 {
		t.Errorf("Expected Mariana at Rank 2 in season leaderboard, got %+v", sMariana)
	}
	if sOscar == nil || sOscar.Rank != 2 {
		t.Errorf("Expected Oscar tied at Rank 2 in season leaderboard, got %+v", sOscar)
	}
}


