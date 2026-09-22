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
