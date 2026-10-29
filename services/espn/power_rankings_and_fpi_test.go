package espn

import (
	"os"
	"testing"

	"nfl-quiniela-2026/db"
)

func TestFetchPowerRankings_OfflineFallback(t *testing.T) {
	client := NewClient()

	mockTeams := map[string]*db.Team{
		"BUF": {ID: 1, Code: "BUF", Name: "Bills", City: "Buffalo"},
		"SEA": {ID: 2, Code: "SEA", Name: "Seahawks", City: "Seattle"},
	}

	rankings, err := client.FetchPowerRankings(2026, 3, mockTeams)
	if err != nil {
		t.Fatalf("FetchPowerRankings returned error: %v", err)
	}

	if len(rankings) != 32 {
		t.Fatalf("expected 32 power rankings, got %d", len(rankings))
	}

	// Rank 1 must be BUF (Bills) authored by Nick Suss
	if rankings[0].TeamCode != "BUF" || rankings[0].Rank != 1 {
		t.Errorf("expected #1 to be BUF, got %+v", rankings[0])
	}
	if rankings[0].TeamName != "Bills" {
		t.Errorf("expected TeamName Bills, got %s", rankings[0].TeamName)
	}
	if rankings[0].Author != "Nick Suss" {
		t.Errorf("expected author Nick Suss, got %s", rankings[0].Author)
	}

	// Rank 2 must be SEA (Seahawks) authored by Brady Henderson
	if rankings[1].TeamCode != "SEA" || rankings[1].Rank != 2 {
		t.Errorf("expected #2 to be SEA, got %+v", rankings[1])
	}
	if rankings[1].Author != "Brady Henderson" {
		t.Errorf("expected author Brady Henderson, got %s", rankings[1].Author)
	}
}

func TestFetchPowerRankings_Week6Article(t *testing.T) {
	client := NewClient()

	mockTeams := map[string]*db.Team{
		"SF":  {ID: 1, Code: "SF", Name: "49ers", City: "San Francisco", Conference: "NFC", Division: "West"},
		"KC":  {ID: 2, Code: "KC", Name: "Chiefs", City: "Kansas City", Conference: "AFC", Division: "West"},
		"BUF": {ID: 3, Code: "BUF", Name: "Bills", City: "Buffalo", Conference: "AFC", Division: "East"},
		"SEA": {ID: 4, Code: "SEA", Name: "Seahawks", City: "Seattle", Conference: "NFC", Division: "West"},
		"MIA": {ID: 5, Code: "MIA", Name: "Dolphins", City: "Miami", Conference: "AFC", Division: "East"},
	}

	articleRankings, errArticle := client.FetchOfficialArticlePowerRankings(2026, 6, mockTeams)
	if errArticle != nil {
		t.Fatalf("FetchOfficialArticlePowerRankings returned error: %v", errArticle)
	}
	_ = articleRankings
	rankings, err := client.FetchPowerRankings(2026, 6, mockTeams)
	if err != nil {
		t.Fatalf("FetchPowerRankings for Week 6 returned error: %v", err)
	}

	if len(rankings) != 32 {
		t.Fatalf("expected 32 teams in Week 6 Power Rankings, got %d", len(rankings))
	}

	// Verify #1 is SF (5-0)
	if rankings[0].TeamCode != "SF" || rankings[0].Rank != 1 {
		t.Errorf("expected #1 to be SF, got %+v", rankings[0])
	}
	if rankings[0].Record != "5-0" {
		t.Errorf("expected SF record to be 5-0, got %s", rankings[0].Record)
	}
	if rankings[0].Author != "Nick Wagoner" {
		t.Errorf("expected author Nick Wagoner, got %s", rankings[0].Author)
	}

	// Verify #2 is KC (4-0)
	if rankings[1].TeamCode != "KC" || rankings[1].Rank != 2 {
		t.Errorf("expected #2 to be KC, got %+v", rankings[1])
	}

	// Verify #3 is BUF (4-1)
	if rankings[2].TeamCode != "BUF" || rankings[2].Rank != 3 {
		t.Errorf("expected #3 to be BUF, got %+v", rankings[2])
	}

	// Verify #32 is MIA (0-5)
	last := rankings[31]
	if last.TeamCode != "MIA" || last.Rank != 32 {
		t.Errorf("expected #32 to be MIA, got %+v", last)
	}
}

func TestFetchFPIPlayoffProbabilities_OfflineFallback(t *testing.T) {
	// Create client with dummy URLs that force fallback
	client := NewClientWithCustomURL("http://localhost:59999/standings", "http://localhost:59999/schedule")

	mockTeams := map[string]*db.Team{
		"BUF": {ID: 1, Code: "BUF", Name: "Bills", City: "Buffalo", Conference: "AFC", Division: "East"},
		"KC":  {ID: 2, Code: "KC", Name: "Chiefs", City: "Kansas City", Conference: "AFC", Division: "West"},
		"PIT": {ID: 3, Code: "PIT", Name: "Steelers", City: "Pittsburgh", Conference: "AFC", Division: "North"},
		"CLE": {ID: 4, Code: "CLE", Name: "Browns", City: "Cleveland", Conference: "AFC", Division: "North"},
	}

	probs, matchups, err := client.FetchFPIPlayoffProbabilities(2026, 4, mockTeams)
	if err != nil {
		t.Fatalf("FetchFPIPlayoffProbabilities returned error: %v", err)
	}

	if len(probs) != 32 {
		t.Fatalf("expected 32 team probabilities, got %d", len(probs))
	}

	if len(matchups) != 16 {
		t.Fatalf("expected 16 matchups, got %d", len(matchups))
	}

	// Verify top probability team is either BUF or SF (> 95%)
	top := probs[0]
	if top.MakePlayoffsPct < 0.95 {
		t.Errorf("expected top playoff probability >= 0.95, got %f for %s", top.MakePlayoffsPct, top.TeamCode)
	}

	// Verify PIT matchup leverage
	var pitProb *db.TeamPlayoffProbability
	for _, p := range probs {
		if p.TeamCode == "PIT" {
			pitProb = p
			break
		}
	}

	if pitProb == nil {
		t.Fatalf("PIT not found in probabilities")
	}

	if pitProb.NextOpponentCode != "CLE" {
		t.Errorf("expected PIT next opponent to be CLE, got %s", pitProb.NextOpponentCode)
	}
	if pitProb.PlayoffPctWithWin <= pitProb.PlayoffPctWithLoss {
		t.Errorf("expected PlayoffPctWithWin > PlayoffPctWithLoss for PIT, got win %f vs loss %f",
			pitProb.PlayoffPctWithWin, pitProb.PlayoffPctWithLoss)
	}
}

func TestSyncAllPowerRankings(t *testing.T) {
	testDBPath := "test_sync_all_pr.db"
	database, err := db.InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer func() {
		_ = database.Close()
		_ = os.Remove(testDBPath)
		_ = os.Remove("services/espn/" + testDBPath)
		_ = os.Remove("services/espn/" + testDBPath + "-shm")
		_ = os.Remove("services/espn/" + testDBPath + "-wal")
	}()

	repo := db.NewRepository(database)
	client := NewClient()
	syncer := NewSyncer(client, repo, nil, nil, 2026)

	// Test backfilling weeks 1 to 2
	synced, err := syncer.SyncAllPowerRankings(2026, 2)
	if err != nil {
		t.Fatalf("SyncAllPowerRankings returned error: %v", err)
	}
	if synced != 2 {
		t.Errorf("expected 2 weeks synced, got %d", synced)
	}

	for w := 1; w <= 2; w++ {
		rankings, err := repo.GetPowerRankings(2026, w)
		if err != nil {
			t.Fatalf("GetPowerRankings for week %d returned error: %v", w, err)
		}
		if len(rankings) != 32 {
			t.Errorf("expected 32 teams for week %d, got %d", w, len(rankings))
		}
	}
}
