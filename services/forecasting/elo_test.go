package forecasting

import (
	"strings"
	"testing"

	"nfl-quiniela-2026/db"
)

func TestEloWinProbability(t *testing.T) {
	// Equal teams at neutral site without HFA would be 50-50
	// With +48 HFA, home team should be favored ~56.8%
	homeProb, awayProb := EloWinProbability(1500.0, 1500.0)
	if homeProb < 0.55 || homeProb > 0.58 {
		t.Errorf("Expected homeProb with +48 HFA ~0.568, got %f", homeProb)
	}
	if homeProb+awayProb < 0.999 || homeProb+awayProb > 1.001 {
		t.Errorf("Probabilities must sum to 1.0, got %f", homeProb+awayProb)
	}

	// Dominant home team (Chiefs 1690 vs Panthers 1380)
	kcProb, carProb := EloWinProbability(1690.0, 1380.0)
	if kcProb < 0.85 {
		t.Errorf("Expected KC > 85%% against CAR, got %f", kcProb)
	}
	if carProb > 0.15 {
		t.Errorf("Expected CAR < 15%% at KC, got %f", carProb)
	}
}

func TestEloSpreadAndProjectedScores(t *testing.T) {
	// 50 Elo point difference with +48 HFA -> ~98 Elo diff -> ~-3.92 spread
	spread := EloSpread(1550.0, 1500.0)
	if spread >= 0 {
		t.Errorf("Home team should have negative spread (favored), got %f", spread)
	}

	// Projected scores for spread -7.0, overUnder 45.0
	homeScore, awayScore := ProjectedScores(-7.0, 45.0)
	if homeScore <= awayScore {
		t.Errorf("Home team must have higher projected score, got Home=%d, Away=%d", homeScore, awayScore)
	}
	if homeScore-awayScore < 6 || homeScore-awayScore > 8 {
		t.Errorf("Expected score differential ~7, got %d", homeScore-awayScore)
	}
}

func TestPredictGameFallbackWhenESPNOffline(t *testing.T) {
	predictor := NewPredictor()

	game := &db.Game{ID: 101}
	homeTeam := &db.Team{ID: 1, Code: "KC", Name: "Chiefs"}
	awayTeam := &db.Team{ID: 2, Code: "DEN", Name: "Broncos"}

	// Test with nil odds (simulating ESPN unavailable)
	forecast := predictor.PredictGame(game, homeTeam, awayTeam, nil)

	if forecast.ESPNAvailable {
		t.Errorf("Expected ESPNAvailable=false when odds are nil")
	}
	if forecast.ConsensusLevel != "elo_pure" {
		t.Errorf("Expected ConsensusLevel='elo_pure', got %s", forecast.ConsensusLevel)
	}
	if !strings.Contains(forecast.AuditNotes, "Cuotas de ESPN no disponibles") {
		t.Errorf("Expected audit note explaining ESPN unavailability, got: %s", forecast.AuditNotes)
	}
	if forecast.PredictedWinnerID != homeTeam.ID {
		t.Errorf("Expected KC to be predicted winner against DEN")
	}
}

func TestPredictGameHybridConsensusAndUpsetAlert(t *testing.T) {
	predictor := NewPredictor()

	game := &db.Game{ID: 202}
	homeTeam := &db.Team{ID: 1, Code: "KC", Name: "Chiefs"}
	awayTeam := &db.Team{ID: 2, Code: "BAL", Name: "Ravens"}

	// Case 1: Vegas aligns with model
	oddsHigh := &OddsInput{
		Details:          "KC -4.5",
		OverUnder:        48.5,
		Spread:           -4.5,
		Provider:         "DraftKings",
		FavoriteTeamCode: "KC",
	}
	fHigh := predictor.PredictGame(game, homeTeam, awayTeam, oddsHigh)
	if !fHigh.ESPNAvailable {
		t.Errorf("Expected ESPNAvailable=true")
	}
	if fHigh.ConsensusLevel != "high" {
		t.Errorf("Expected ConsensusLevel='high', got %s", fHigh.ConsensusLevel)
	}
	if !strings.Contains(fHigh.SourcesSummary, "DraftKings") {
		t.Errorf("Expected SourcesSummary to mention DraftKings, got: %s", fHigh.SourcesSummary)
	}

	// Case 2: Upset Alert (Model favors KC, but Vegas favors BAL)
	oddsUpset := &OddsInput{
		Details:          "BAL -2.5",
		OverUnder:        46.0,
		Spread:           2.5,
		Provider:         "DraftKings",
		FavoriteTeamCode: "BAL",
	}
	fUpset := predictor.PredictGame(game, homeTeam, awayTeam, oddsUpset)
	if fUpset.ConsensusLevel != "upset_alert" {
		t.Errorf("Expected ConsensusLevel='upset_alert', got %s", fUpset.ConsensusLevel)
	}
	if !strings.Contains(fUpset.AuditNotes, "Alerta Sorpresa") {
		t.Errorf("Expected audit note to flag Alerta Sorpresa, got: %s", fUpset.AuditNotes)
	}
}
