package scoring

import (
	"testing"

	"nfl-quiniela-2026/db"
)

func TestPlayoffScoringMultipliers(t *testing.T) {
	// 1. Check multipliers
	if mult := GetPlayoffMultiplier(1); mult != 1 {
		t.Errorf("expected regular season week 1 multiplier 1, got %d", mult)
	}
	if mult := GetPlayoffMultiplier(18); mult != 1 {
		t.Errorf("expected regular season week 18 multiplier 1, got %d", mult)
	}
	if mult := GetPlayoffMultiplier(19); mult != 1 {
		t.Errorf("expected wild card week 19 multiplier 1, got %d", mult)
	}
	if mult := GetPlayoffMultiplier(20); mult != 2 {
		t.Errorf("expected divisional week 20 multiplier 2, got %d", mult)
	}
	if mult := GetPlayoffMultiplier(21); mult != 3 {
		t.Errorf("expected conference week 21 multiplier 3, got %d", mult)
	}
	if mult := GetPlayoffMultiplier(22); mult != 5 {
		t.Errorf("expected super bowl week 22 multiplier 5, got %d", mult)
	}

	// 2. Test pick scoring with multipliers
	calc := NewCalculator(nil)
	cfg := &db.ScoringConfig{
		ScoringMode:  "pure_tiebreaker",
		WinnerPoints: 1,
	}

	winnerID := int64(14) // Chiefs
	otherID := int64(31)  // 49ers
	homeScore := 24
	awayScore := 20
	game := &db.Game{
		Status:        "final",
		HomeTeamID:    winnerID,
		AwayTeamID:    otherID,
		HomeScore:     &homeScore,
		AwayScore:     &awayScore,
	}

	correctPick := &db.Pick{
		PickedTeamID: &winnerID,
	}

	// Regular season (1x): 1 pt
	pts, _, isCorr := calc.CalculatePickScores(correctPick, game, cfg, 5)
	if !*isCorr || pts != 1 {
		t.Errorf("expected 1 pt for regular season, got %d", pts)
	}

	// Divisional (2x): 2 pts
	pts, _, isCorr = calc.CalculatePickScores(correctPick, game, cfg, 20)
	if !*isCorr || pts != 2 {
		t.Errorf("expected 2 pts for divisional round, got %d", pts)
	}

	// Super Bowl (5x): 5 pts
	pts, _, isCorr = calc.CalculatePickScores(correctPick, game, cfg, 22)
	if !*isCorr || pts != 5 {
		t.Errorf("expected 5 pts for super bowl, got %d", pts)
	}
}
