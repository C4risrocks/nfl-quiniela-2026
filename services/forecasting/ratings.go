package forecasting

// TeamRatings maps NFL team codes to their FiveThirtyEight Elo ratings for the 2026 season.
// Baseline values calibrated from 2025 finish + 2026 preseason roster adjustments.
var TeamRatings = map[string]float64{
	// AFC East
	"BUF": 1615.0,
	"MIA": 1540.0,
	"NYJ": 1510.0,
	"NE":  1410.0,

	// AFC North
	"BAL": 1640.0,
	"CIN": 1550.0,
	"CLE": 1490.0,
	"PIT": 1530.0,

	// AFC South
	"HOU": 1565.0,
	"IND": 1495.0,
	"JAX": 1460.0,
	"TEN": 1420.0,

	// AFC West
	"KC":  1690.0,
	"LAC": 1520.0,
	"DEN": 1440.0,
	"LV":  1435.0,

	// NFC East
	"PHI": 1625.0,
	"DAL": 1570.0,
	"WAS": 1490.0,
	"NYG": 1415.0,

	// NFC North
	"DET": 1650.0,
	"GB":  1580.0,
	"CHI": 1485.0,
	"MIN": 1515.0,

	// NFC South
	"TB":  1510.0,
	"ATL": 1500.0,
	"NO":  1470.0,
	"CAR": 1380.0,

	// NFC West
	"SF":  1655.0,
	"LAR": 1545.0,
	"ARI": 1480.0,
	"SEA": 1490.0,
}

// HomeFieldAdvantage is the standard FiveThirtyEight Elo bonus for the home team (+48.0 points).
const HomeFieldAdvantage = 48.0

// GetTeamRating retrieves the Elo rating for a team code, defaulting to league average (1500.0).
func GetTeamRating(teamCode string) float64 {
	if r, ok := TeamRatings[teamCode]; ok {
		return r
	}
	return 1500.0
}
