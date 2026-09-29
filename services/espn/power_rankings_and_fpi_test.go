package espn

import (
	"testing"

	"nfl-quiniela-2026/db"
)

func TestFetchPowerRankings_OfflineFallback(t *testing.T) {
	client := NewClient()

	mockTeams := map[string]*db.Team{
		"KC": {ID: 1, Code: "KC", Name: "Chiefs", City: "Kansas City"},
		"SF": {ID: 2, Code: "SF", Name: "49ers", City: "San Francisco"},
	}

	rankings, err := client.FetchPowerRankings(2026, 3, mockTeams)
	if err != nil {
		t.Fatalf("FetchPowerRankings returned error: %v", err)
	}

	if len(rankings) != 32 {
		t.Fatalf("expected 32 power rankings, got %d", len(rankings))
	}

	// Rank 1 must be KC
	if rankings[0].TeamCode != "KC" || rankings[0].Rank != 1 {
		t.Errorf("expected #1 to be KC, got %+v", rankings[0])
	}
	if rankings[0].TeamName != "Chiefs" {
		t.Errorf("expected TeamName Chiefs, got %s", rankings[0].TeamName)
	}
	if rankings[0].Author != "Eric Gómez" {
		t.Errorf("expected author Eric Gómez, got %s", rankings[0].Author)
	}

	// Rank 2 must be SF
	if rankings[1].TeamCode != "SF" || rankings[1].Rank != 2 {
		t.Errorf("expected #2 to be SF, got %+v", rankings[1])
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
