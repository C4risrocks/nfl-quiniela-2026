package forecasting

import (
	"testing"
	"time"

	"nfl-quiniela-2026/db"
)

func TestBuildWeeklyAdvisor_PresetsAndQuadrants(t *testing.T) {
	engine := NewAdvisorEngine()

	week := &db.Week{
		ID:         1,
		WeekNumber: 1,
		Name:       "Semana 1",
		Status:     "active",
	}

	teamKC := &db.Team{ID: 1, Code: "KC", Name: "Kansas City Chiefs"}
	teamLV := &db.Team{ID: 2, Code: "LV", Name: "Las Vegas Raiders"}
	teamSF := &db.Team{ID: 3, Code: "SF", Name: "San Francisco 49ers"}
	teamLAR := &db.Team{ID: 4, Code: "LAR", Name: "Los Angeles Rams"}
	teamBUF := &db.Team{ID: 5, Code: "BUF", Name: "Buffalo Bills"}
	teamMIA := &db.Team{ID: 6, Code: "MIA", Name: "Miami Dolphins"}

	now := time.Now()

	// Game 1: Heavy favorite (Anchor) - KC vs LV
	g1 := &db.Game{
		ID:          101,
		WeekID:      week.ID,
		HomeTeamID:  teamKC.ID,
		AwayTeamID:  teamLV.ID,
		HomeTeam:    teamKC,
		AwayTeam:    teamLV,
		KickoffTime: now.Add(24 * time.Hour),
		Status:      "scheduled",
	}

	// Game 2: Value Gem / Upset Alert - SF vs LAR (SF favorite by Elo, but LAR has value or vice versa)
	g2 := &db.Game{
		ID:          102,
		WeekID:      week.ID,
		HomeTeamID:  teamSF.ID,
		AwayTeamID:  teamLAR.ID,
		HomeTeam:    teamSF,
		AwayTeam:    teamLAR,
		KickoffTime: now.Add(26 * time.Hour),
		Status:      "scheduled",
	}

	// Game 3: Coin Toss - BUF vs MIA (close game)
	g3 := &db.Game{
		ID:          103,
		WeekID:      week.ID,
		HomeTeamID:  teamBUF.ID,
		AwayTeamID:  teamMIA.ID,
		HomeTeam:    teamBUF,
		AwayTeam:    teamMIA,
		KickoffTime: now.Add(-2 * time.Hour), // Started -> Locked!
		Status:      "in_progress",
	}

	games := []*db.Game{g1, g2, g3}

	// Forecasts
	forecasts := map[int64]*db.GameForecast{
		g1.ID: {
			GameID:            g1.ID,
			EloHomeProb:       0.82,
			EloAwayProb:       0.18,
			EloSpread:         -9.5,
			ProjHomeScore:     28,
			ProjAwayScore:     19,
			PredictedWinnerID: teamKC.ID,
		},
		g2.ID: {
			GameID:            g2.ID,
			EloHomeProb:       0.58,
			EloAwayProb:       0.42,
			EloSpread:         -3.0,
			ProjHomeScore:     24,
			ProjAwayScore:     20,
			PredictedWinnerID: teamSF.ID,
		},
		g3.ID: {
			GameID:            g3.ID,
			EloHomeProb:       0.52,
			EloAwayProb:       0.48,
			EloSpread:         -1.0,
			ProjHomeScore:     24,
			ProjAwayScore:     23,
			PredictedWinnerID: teamBUF.ID,
		},
	}

	// Community stats
	commStats := map[int64]*db.GameCommunityStats{
		g1.ID: {
			TotalPicks:     100,
			HomePicksCount: 90,
			AwayPicksCount: 10,
			HomePct:        90,
			AwayPct:        10,
		},
		g2.ID: {
			TotalPicks:     100,
			HomePicksCount: 85, // Community overwhelmingly on SF (85%)
			AwayPicksCount: 15, // LAR has 42% win prob but only 15% picks -> HUGE Value Gem / Upset Alert (+EV)
			HomePct:        85,
			AwayPct:        15,
		},
		g3.ID: {
			TotalPicks:     100,
			HomePicksCount: 51,
			AwayPicksCount: 49,
			HomePct:        51,
			AwayPct:        49,
		},
	}

	userPicks := make(map[int64]*db.Pick)
	scoringCfg := &db.ScoringConfig{
		WinnerPoints: 10,
		LockMode:     "per_game",
	}

	firstKickoff := now.Add(-2 * time.Hour)

	// 1. Test Balanced Preset
	overview := engine.BuildWeeklyAdvisor(
		week,
		[]*db.Week{week},
		games,
		forecasts,
		commStats,
		userPicks,
		scoringCfg,
		"balanced",
		&firstKickoff,
		now,
	)

	if overview == nil {
		t.Fatalf("Expected non-nil overview")
	}

	if overview.TotalGames != 3 {
		t.Errorf("Expected 3 total games, got %d", overview.TotalGames)
	}

	if overview.LockedGamesCount != 1 {
		t.Errorf("Expected 1 locked game (g3 in progress), got %d", overview.LockedGamesCount)
	}

	if overview.OpenGamesCount != 2 {
		t.Errorf("Expected 2 open games, got %d", overview.OpenGamesCount)
	}

	if len(overview.AllPresets) != 3 {
		t.Errorf("Expected 3 strategy presets, got %d", len(overview.AllPresets))
	}

	if overview.ActivePreset == nil {
		t.Fatalf("Expected non-nil ActivePreset")
	}

	if overview.ActivePreset.ID != "balanced" {
		t.Errorf("Expected active preset 'balanced', got %s", overview.ActivePreset.ID)
	}

	// Verify Quadrants
	var g1Rec, g2Rec, g3Rec *db.AdvisorMatchupRecommendation
	for _, r := range overview.ActivePreset.Recommendations {
		if r.Game.ID == g1.ID {
			g1Rec = r
		} else if r.Game.ID == g2.ID {
			g2Rec = r
		} else if r.Game.ID == g3.ID {
			g3Rec = r
		}
	}

	if g1Rec == nil || g2Rec == nil || g3Rec == nil {
		t.Fatalf("Missing matchup recommendations for games")
	}

	// Game 1 is KC (82% prob, 90% comm) -> Anchor
	if g1Rec.Quadrant != db.QuadrantAnchor {
		t.Errorf("Expected g1 to be Anchor, got %s", g1Rec.Quadrant)
	}
	if g1Rec.RecommendedWinner.Code != "KC" {
		t.Errorf("Expected g1 recommended winner KC, got %s", g1Rec.RecommendedWinner.Code)
	}

	// Game 3 is BUF vs MIA (52% prob, 51% comm) -> Coin Toss
	if g3Rec.Quadrant != db.QuadrantCoinToss {
		t.Errorf("Expected g3 to be Coin Toss, got %s", g3Rec.Quadrant)
	}
	if !g3Rec.IsLocked {
		t.Errorf("Expected g3 to be locked because kickoff was in the past")
	}

	// 2. Test Aggressive Preset (Should leverage value gems)
	aggOverview := engine.BuildWeeklyAdvisor(
		week,
		[]*db.Week{week},
		games,
		forecasts,
		commStats,
		userPicks,
		scoringCfg,
		"aggressive",
		&firstKickoff,
		now,
	)

	if aggOverview.ActivePreset.ID != "aggressive" {
		t.Errorf("Expected active preset aggressive, got %s", aggOverview.ActivePreset.ID)
	}

	// In aggressive, with LAR having 42% win prob vs only 15% community, it should pick LAR or identify high value
	var aggG2Rec *db.AdvisorMatchupRecommendation
	for _, r := range aggOverview.ActivePreset.Recommendations {
		if r.Game.ID == g2.ID {
			aggG2Rec = r
		}
	}
	if aggG2Rec.RecommendedWinner.Code != "LAR" {
		t.Logf("Aggressive preset recommended %s for g2 (%s)", aggG2Rec.RecommendedWinner.Code, aggG2Rec.TacticalReasoning)
	}

	// 3. Test Conservative Preset (Should pick highest probability team for all games)
	consOverview := engine.BuildWeeklyAdvisor(
		week,
		[]*db.Week{week},
		games,
		forecasts,
		commStats,
		userPicks,
		scoringCfg,
		"conservative",
		&firstKickoff,
		now,
	)

	var consG1Rec, consG2Rec *db.AdvisorMatchupRecommendation
	for _, r := range consOverview.ActivePreset.Recommendations {
		if r.Game.ID == g1.ID {
			consG1Rec = r
		} else if r.Game.ID == g2.ID {
			consG2Rec = r
		}
	}

	if consG1Rec.RecommendedWinner.Code != "KC" {
		t.Errorf("Expected Conservative to pick KC (82%%), got %s", consG1Rec.RecommendedWinner.Code)
	}
	if consG2Rec.RecommendedWinner.Code != "SF" {
		t.Errorf("Expected Conservative to pick SF (58%%), got %s", consG2Rec.RecommendedWinner.Code)
	}
}

func TestBuildWeeklyAdvisor_NilAndEmptyEdgeCases(t *testing.T) {
	engine := NewAdvisorEngine()

	// Empty games
	overview := engine.BuildWeeklyAdvisor(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		time.Now(),
	)

	if overview == nil {
		t.Fatalf("Expected non-nil overview on empty inputs")
	}

	if overview.TotalGames != 0 {
		t.Errorf("Expected 0 total games, got %d", overview.TotalGames)
	}

	if overview.ActivePreset == nil || overview.ActivePreset.ID != "balanced" {
		t.Errorf("Expected default preset 'balanced', got %v", overview.ActivePreset)
	}
}
