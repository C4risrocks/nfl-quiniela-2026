package forecasting

import (
	"fmt"
	"math"
	"strings"
	"time"

	"nfl-quiniela-2026/db"
)

// OddsInput represents external betting line data (e.g. from ESPN / DraftKings).
type OddsInput struct {
	Details          string  // e.g. "KC -6.5" or "EVEN"
	OverUnder        float64 // e.g. 44.5
	Spread           float64 // e.g. -6.5
	Provider         string  // e.g. "DraftKings"
	FavoriteTeamCode string  // e.g. "KC"
}

// Predictor orchestrates the hybrid consensus forecasting engine.
type Predictor struct{}

func NewPredictor() *Predictor {
	return &Predictor{}
}

// PredictGame computes the full AI forecast, score projections, and audit trace for a matchup.
func (p *Predictor) PredictGame(game *db.Game, homeTeam, awayTeam *db.Team, odds *OddsInput) *db.GameForecast {
	homeRating := GetTeamRating(homeTeam.Code)
	awayRating := GetTeamRating(awayTeam.Code)

	homeProb, awayProb := EloWinProbability(homeRating, awayRating)
	eloSpread := EloSpread(homeRating, awayRating)

	overUnder := DefaultNFLTotalPoints
	if odds != nil && odds.OverUnder > 0 {
		overUnder = odds.OverUnder
	}

	// Calculate projected scores from spread and over/under
	effectiveSpread := eloSpread
	if odds != nil && odds.Spread != 0 {
		// Hybrid blend: 60% Vegas line + 40% Pure Elo
		effectiveSpread = (odds.Spread * 0.60) + (eloSpread * 0.40)
	}
	projHome, projAway := ProjectedScores(effectiveSpread, overUnder)

	// Determine predicted winner
	var predWinnerID int64
	var predWinnerCode string
	if homeProb >= awayProb {
		predWinnerID = homeTeam.ID
		predWinnerCode = homeTeam.Code
	} else {
		predWinnerID = awayTeam.ID
		predWinnerCode = awayTeam.Code
	}

	forecast := &db.GameForecast{
		GameID:            game.ID,
		EloHomeProb:       math.Round(homeProb*1000) / 1000,
		EloAwayProb:       math.Round(awayProb*1000) / 1000,
		EloSpread:         math.Round(eloSpread*10) / 10,
		ProjHomeScore:     projHome,
		ProjAwayScore:     projAway,
		PredictedWinnerID: predWinnerID,
		PredictedWinner:   homeTeam,
		CalculatedAt:      time.Now(),
	}
	if predWinnerID == awayTeam.ID {
		forecast.PredictedWinner = awayTeam
	}

	// Fallback vs Hybrid Consensus evaluation with Audit Traceability
	if odds == nil || (odds.Details == "" && odds.Spread == 0 && odds.OverUnder == 0) {
		// CASE A: ESPN / Vegas Odds NOT available (Graceful Degradation)
		forecast.ESPNAvailable = false
		forecast.ConsensusLevel = "elo_pure"
		forecast.SourcesSummary = fmt.Sprintf("Modelo Go Elo Puro (FiveThirtyEight: %s %.0f vs %s %.0f)", homeTeam.Code, homeRating, awayTeam.Code, awayRating)
		forecast.AuditNotes = "⚠️ Nota: Cuotas de ESPN no disponibles al momento de esta corrida. Se utilizó Modelo Estadístico Go Elo Puro."
	} else {
		// CASE B: Hybrid Consensus Available
		forecast.ESPNAvailable = true
		provider := odds.Provider
		if provider == "" {
			provider = "DraftKings"
		}

		forecast.VegasSpread = &odds.Spread
		if odds.FavoriteTeamCode != "" {
			if strings.EqualFold(odds.FavoriteTeamCode, homeTeam.Code) {
				forecast.VegasFavoriteID = &homeTeam.ID
			} else if strings.EqualFold(odds.FavoriteTeamCode, awayTeam.Code) {
				forecast.VegasFavoriteID = &awayTeam.ID
			}
		}

		forecast.SourcesSummary = fmt.Sprintf("Modelo Go Elo + %s (Línea: %s, O/U: %.1f)", provider, odds.Details, odds.OverUnder)

		// Check consensus alignment
		vegasFavCode := odds.FavoriteTeamCode
		if vegasFavCode != "" && !strings.EqualFold(vegasFavCode, predWinnerCode) {
			forecast.ConsensusLevel = "upset_alert"
			forecast.AuditNotes = fmt.Sprintf("⚡ Alerta Sorpresa: Modelo matemático proyecta victoria de %s (%d%%), pero %s favorece a %s (%s).",
				predWinnerCode, forecast.WinProbabilityPct(), provider, vegasFavCode, odds.Details)
		} else {
			if math.Abs(effectiveSpread) >= 4.0 {
				forecast.ConsensusLevel = "high"
				forecast.AuditNotes = fmt.Sprintf("Consenso Fuerte: Modelo estadístico y línea de %s coinciden con ventaja clara para %s.", provider, predWinnerCode)
			} else {
				forecast.ConsensusLevel = "moderate"
				forecast.AuditNotes = fmt.Sprintf("Consenso Moderado: Partido cerrado con margen proyectado de %.1f pts.", math.Abs(effectiveSpread))
			}
		}
	}

	return forecast
}
