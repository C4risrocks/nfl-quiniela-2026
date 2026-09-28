package db

import (
	"testing"
)

func TestGameCommunityPickHelpers(t *testing.T) {
	game := &Game{
		HomeTeamID:    10,
		AwayTeamID:    20,
		TotalPicks:    0,
		HomePickCount: 0,
		AwayPickCount: 0,
	}

	// Case 1: Zero total picks
	if pct := game.HomePickPct(); pct != 50 {
		t.Errorf("Expected 50%% home pick pct when TotalPicks=0, got %d", pct)
	}
	if pct := game.AwayPickPct(); pct != 50 {
		t.Errorf("Expected 50%% away pick pct when TotalPicks=0, got %d", pct)
	}
	if fav := game.CommunityFavoriteTeamID(); fav != 0 {
		t.Errorf("Expected 0 favorite when TotalPicks=0, got %d", fav)
	}

	// Case 2: Home is favored (15 vs 5 out of 20)
	game.TotalPicks = 20
	game.HomePickCount = 15
	game.AwayPickCount = 5

	if pct := game.HomePickPct(); pct != 75 {
		t.Errorf("Expected 75%% home pick pct, got %d", pct)
	}
	if pct := game.AwayPickPct(); pct != 25 {
		t.Errorf("Expected 25%% away pick pct, got %d", pct)
	}
	if fav := game.CommunityFavoriteTeamID(); fav != 10 {
		t.Errorf("Expected Home (10) to be favorite, got %d", fav)
	}

	// Case 3: Away is favored (2 vs 8 out of 10)
	game.TotalPicks = 10
	game.HomePickCount = 2
	game.AwayPickCount = 8

	if pct := game.HomePickPct(); pct != 20 {
		t.Errorf("Expected 20%% home pick pct, got %d", pct)
	}
	if pct := game.AwayPickPct(); pct != 80 {
		t.Errorf("Expected 80%% away pick pct, got %d", pct)
	}
	if fav := game.CommunityFavoriteTeamID(); fav != 20 {
		t.Errorf("Expected Away (20) to be favorite, got %d", fav)
	}

	// Case 4: Tie (5 vs 5 out of 10)
	game.TotalPicks = 10
	game.HomePickCount = 5
	game.AwayPickCount = 5

	if pct := game.HomePickPct(); pct != 50 {
		t.Errorf("Expected 50%% home pick pct, got %d", pct)
	}
	if pct := game.AwayPickPct(); pct != 50 {
		t.Errorf("Expected 50%% away pick pct, got %d", pct)
	}
	if fav := game.CommunityFavoriteTeamID(); fav != 0 {
		t.Errorf("Expected 0 favorite on tie, got %d", fav)
	}
}

func TestWinProbabilityHelpers(t *testing.T) {
	summary := &GameDetailedSummary{
		HasWinProb:        true,
		CurrentHomeWinPct: 72,
		CurrentAwayWinPct: 28,
		WinProbability: []WinProbabilityPoint{
			{PlayID: "p1", Quarter: 1, Clock: "12:00", HomeWinPercentage: 0.50, SwingDelta: 0.05, Text: "Play 1"},
			{PlayID: "p2", Quarter: 2, Clock: "08:00", HomeWinPercentage: 0.65, SwingDelta: 0.15, Text: "Play 2 (Key interception)"},
			{PlayID: "p3", Quarter: 3, Clock: "04:00", HomeWinPercentage: 0.40, SwingDelta: 0.25, Text: "Play 3 (Pick six)"},
			{PlayID: "p4", Quarter: 4, Clock: "01:00", HomeWinPercentage: 0.85, SwingDelta: 0.45, Text: "Play 4 (Game winning TD)"},
		},
	}

	if hPct := summary.WinProbHome(); hPct != 72 {
		t.Errorf("Expected WinProbHome 72%%, got %d%%", hPct)
	}
	if aPct := summary.WinProbAway(); aPct != 28 {
		t.Errorf("Expected WinProbAway 28%%, got %d%%", aPct)
	}

	pivotal := summary.PivotalPlays(2)
	if len(pivotal) != 2 {
		t.Fatalf("Expected 2 pivotal plays, got %d", len(pivotal))
	}
	if pivotal[0].PlayID != "p4" {
		t.Errorf("Expected highest swing play to be p4 (0.45), got %s", pivotal[0].PlayID)
	}
	if pivotal[0].SwingDeltaPct() != 45 {
		t.Errorf("Expected SwingDeltaPct to be 45, got %d", pivotal[0].SwingDeltaPct())
	}
	if pivotal[1].PlayID != "p3" {
		t.Errorf("Expected second swing play to be p3 (0.25), got %s", pivotal[1].PlayID)
	}

	// Chart generation
	chart := summary.WinProbChart(640, 220)
	if chart == nil {
		t.Fatal("Expected chart not to be nil")
	}
	if chart.Width != 640 || chart.Height != 220 {
		t.Errorf("Expected chart dimensions 640x220, got %.0fx%.0f", chart.Width, chart.Height)
	}
	if chart.LinePath == "" {
		t.Error("Expected non-empty LinePath for SVG line")
	}
	if chart.HomeAreaPath == "" {
		t.Error("Expected non-empty HomeAreaPath for SVG area fill")
	}
	if chart.PointsJSON == "" || chart.PointsJSON == "[]" {
		t.Error("Expected populated PointsJSON for Alpine reactivity")
	}
}

