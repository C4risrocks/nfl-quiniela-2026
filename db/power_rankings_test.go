package db

import (
	"os"
	"testing"
)

func TestPowerRankingsPersistence(t *testing.T) {
	testDBPath := "test_power_rankings.db"
	defer func() {
		_ = os.Remove(testDBPath)
		_ = os.Remove("db/" + testDBPath)
		_ = os.Remove("db/" + testDBPath + "-shm")
		_ = os.Remove("db/" + testDBPath + "-wal")
	}()

	database, err := InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)

	// Verify initially empty
	latestWeek, rankings, err := repo.GetLatestPowerRankings(2026)
	if err != nil {
		t.Fatalf("GetLatestPowerRankings error: %v", err)
	}
	if latestWeek != 0 || len(rankings) != 0 {
		t.Errorf("expected 0 power rankings initially, got week %d, len %d", latestWeek, len(rankings))
	}

	// Insert power rankings for Week 3
	testRankings := []*TeamPowerRanking{
		{
			SeasonYear:   2026,
			WeekNumber:   3,
			TeamCode:     "KC",
			Rank:         1,
			PreviousRank: 4,
			RankChange:   3,
			Record:       "3-0",
			Analysis:     "¿Volvieron los Chiefs? Volvieron los Chiefs. Un 3-0 bien ganado.",
			Author:       "Eric Gómez",
		},
		{
			SeasonYear:   2026,
			WeekNumber:   3,
			TeamCode:     "SF",
			Rank:         2,
			PreviousRank: 3,
			RankChange:   1,
			Record:       "3-0",
			Analysis:     "Tantas lesiones y se las arreglaron para seguir invictos en la campaña.",
			Author:       "Fernando Villa",
		},
	}

	if err := repo.SavePowerRankings(testRankings); err != nil {
		t.Fatalf("SavePowerRankings error: %v", err)
	}

	// Retrieve
	latestWeek, loaded, err := repo.GetLatestPowerRankings(2026)
	if err != nil {
		t.Fatalf("GetLatestPowerRankings after save error: %v", err)
	}
	if latestWeek != 3 {
		t.Errorf("expected latestWeek 3, got %d", latestWeek)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 rankings, got %d", len(loaded))
	}

	if loaded[0].TeamCode != "KC" || loaded[0].Rank != 1 || loaded[0].RankChange != 3 {
		t.Errorf("unexpected top ranking: %+v", loaded[0])
	}
	if loaded[0].Author != "Eric Gómez" {
		t.Errorf("expected author Eric Gómez, got %s", loaded[0].Author)
	}
	if loaded[1].TeamCode != "SF" || loaded[1].Rank != 2 || loaded[1].RankChange != 1 {
		t.Errorf("unexpected #2 ranking: %+v", loaded[1])
	}
}

func TestPlayoffProbabilitiesPersistence(t *testing.T) {
	testDBPath := "test_playoff_probs.db"
	defer func() {
		_ = os.Remove(testDBPath)
		_ = os.Remove("db/" + testDBPath)
		_ = os.Remove("db/" + testDBPath + "-shm")
		_ = os.Remove("db/" + testDBPath + "-wal")
	}()

	database, err := InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)

	// Verify initially empty
	latestWeek, probs, err := repo.GetLatestPlayoffProbabilities(2026)
	if err != nil {
		t.Fatalf("GetLatestPlayoffProbabilities error: %v", err)
	}
	if latestWeek != 0 || len(probs) != 0 {
		t.Errorf("expected 0 probs initially, got week %d, len %d", latestWeek, len(probs))
	}

	// Insert probabilities for Week 4
	testProbs := []*TeamPlayoffProbability{
		{
			SeasonYear:         2026,
			WeekNumber:         4,
			TeamCode:           "BUF",
			Conference:         "AFC",
			MakePlayoffsPct:    0.967,
			ClinchDivisionPct:  0.898,
			ClinchFirstSeedPct: 0.331,
			WildCardPct:        0.069,
			NextOpponentCode:   "BAL",
			WinProjPct:         0.52,
			PlayoffPctWithWin:  0.98,
			PlayoffPctWithLoss: 0.94,
			IsFavorite:         true,
		},
		{
			SeasonYear:         2026,
			WeekNumber:         4,
			TeamCode:           "PIT",
			Conference:         "AFC",
			MakePlayoffsPct:    0.34,
			ClinchDivisionPct:  0.15,
			ClinchFirstSeedPct: 0.02,
			WildCardPct:        0.19,
			NextOpponentCode:   "CLE",
			WinProjPct:         0.55,
			PlayoffPctWithWin:  0.43,
			PlayoffPctWithLoss: 0.22,
			IsFavorite:         true,
		},
	}

	if err := repo.SavePlayoffProbabilities(testProbs); err != nil {
		t.Fatalf("SavePlayoffProbabilities error: %v", err)
	}

	// Retrieve
	latestWeek, loaded, err := repo.GetLatestPlayoffProbabilities(2026)
	if err != nil {
		t.Fatalf("GetLatestPlayoffProbabilities after save error: %v", err)
	}
	if latestWeek != 4 {
		t.Errorf("expected latestWeek 4, got %d", latestWeek)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded probs, got %d", len(loaded))
	}

	// Top prob should be BUF (0.967)
	if loaded[0].TeamCode != "BUF" || loaded[0].MakePlayoffsPct != 0.967 {
		t.Errorf("expected top prob to be BUF, got %+v", loaded[0])
	}

	// Check PIT leverage: 0.43 - 0.22 = 0.21
	pit := loaded[1]
	if pit.TeamCode != "PIT" {
		t.Fatalf("expected second team to be PIT, got %s", pit.TeamCode)
	}
	if pit.PlayoffLeverage != 0.21 {
		t.Errorf("expected PIT PlayoffLeverage 0.21, got %f", pit.PlayoffLeverage)
	}
}
