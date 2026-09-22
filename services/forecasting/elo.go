package forecasting

import (
	"math"
)

// DefaultNFLTotalPoints is the baseline expected total score across modern NFL games.
const DefaultNFLTotalPoints = 44.0

// EloWinProbability calculates the home and away win probabilities using the FiveThirtyEight Elo logistic formula.
// R_diff = homeRating - awayRating + HomeFieldAdvantage
// P_home = 1 / (10^(-R_diff / 400) + 1)
func EloWinProbability(homeRating, awayRating float64) (homeProb, awayProb float64) {
	diff := (homeRating - awayRating) + HomeFieldAdvantage
	homeProb = 1.0 / (math.Pow(10.0, -diff/400.0) + 1.0)
	awayProb = 1.0 - homeProb
	return homeProb, awayProb
}

// EloSpread converts an Elo differential to an American point spread.
// In FiveThirtyEight NFL models, 25 Elo points roughly equal 1 point of spread.
// A negative spread indicates the home team is favored (e.g. -6.5 means home is favored by 6.5 pts).
func EloSpread(homeRating, awayRating float64) float64 {
	diff := (homeRating - awayRating) + HomeFieldAdvantage
	return -(diff / 25.0)
}

// ProjectedScores derives realistic projected final scores from spread and over/under.
// If totalOverUnder is 0 or negative, DefaultNFLTotalPoints (44.0) is used.
func ProjectedScores(spread, totalOverUnder float64) (homeScore, awayScore int) {
	total := totalOverUnder
	if total <= 0 {
		total = DefaultNFLTotalPoints
	}

	// Home - Away = -spread
	// Home + Away = total
	homeProj := (total - spread) / 2.0
	awayProj := (total + spread) / 2.0

	homeScore = int(math.Round(homeProj))
	awayScore = int(math.Round(awayProj))

	// Ensure NFL scores stay within realistic bounds
	if homeScore < 6 {
		homeScore = 6
	}
	if awayScore < 6 {
		awayScore = 6
	}

	// Break projection ties (games cannot finish tied in regular season projections)
	if homeScore == awayScore {
		if spread < 0 {
			homeScore++
		} else {
			awayScore++
		}
	}

	return homeScore, awayScore
}
