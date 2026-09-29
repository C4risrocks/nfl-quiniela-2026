package forecasting

import (
	"fmt"
	"math"
	"nfl-quiniela-2026/db"
	"time"
)

// AdvisorEngine calculates risk matrix quadrants, expected value (+EV),
// and builds 3 distinct tactical strategies (Conservative, Balanced, Aggressive).
type AdvisorEngine struct {
	predictor *Predictor
}

func NewAdvisorEngine() *AdvisorEngine {
	return &AdvisorEngine{
		predictor: NewPredictor(),
	}
}

// BuildWeeklyAdvisor generates recommendations for all 3 presets for a given week.
func (e *AdvisorEngine) BuildWeeklyAdvisor(
	week *db.Week,
	allWeeks []*db.Week,
	games []*db.Game,
	forecasts map[int64]*db.GameForecast,
	commStats map[int64]*db.GameCommunityStats,
	userPicks map[int64]*db.Pick,
	scoringCfg *db.ScoringConfig,
	activePresetID string,
	firstKickoff *time.Time,
	now time.Time,
) *db.AdvisorWeeklyOverview {
	if activePresetID == "" {
		activePresetID = "balanced"
	}

	winnerPts := 10
	if scoringCfg != nil && scoringCfg.WinnerPoints > 0 {
		winnerPts = scoringCfg.WinnerPoints
	}
	lockMode := "per_game"
	if scoringCfg != nil && scoringCfg.LockMode != "" {
		lockMode = scoringCfg.LockMode
	}

	totalGames := len(games)
	openGames := 0
	lockedGames := 0
	for _, g := range games {
		if g.IsGameOrWeekLocked(now, lockMode, firstKickoff) {
			lockedGames++
		} else {
			openGames++
		}
	}

	// 1. Precompute matchup base evaluations (probabilities, community, quadrants)
	type matchupBase struct {
		game               *db.Game
		forecast           *db.GameForecast
		comm               *db.GameCommunityStats
		homeProb           int
		awayProb           int
		homePickPct        int
		awayPickPct        int
		favTeam            *db.Team
		dogTeam            *db.Team
		favProb            int
		dogProb            int
		favPickPct         int
		dogPickPct         int
		isHomeFav          bool
		evFav              float64
		evDog              float64
		quadrant           db.TacticalQuadrant
		quadrantLabel      string
		quadrantBadgeClass string
		userPickID         *int64
		isLocked           bool
	}

	bases := make([]*matchupBase, 0, len(games))
	anchorsCount := 0
	gemsCount := 0
	upsetsCount := 0
	coinTossesCount := 0

	for _, g := range games {
		fc := forecasts[g.ID]
		if fc == nil && g.HomeTeam != nil && g.AwayTeam != nil {
			fc = e.predictor.PredictGame(g, g.HomeTeam, g.AwayTeam, nil)
		}

		homeProb := 50
		awayProb := 50
		spread := 0.0
		if fc != nil {
			homeProb = int(math.Round(fc.EloHomeProb * 100))
			awayProb = int(math.Round(fc.EloAwayProb * 100))
			spread = fc.EloSpread
		}

		cStat := commStats[g.ID]
		homePickPct := 50
		awayPickPct := 50
		if cStat != nil && cStat.TotalPicks > 0 {
			homePickPct = cStat.HomePct
			awayPickPct = cStat.AwayPct
		}

		isHomeFav := homeProb >= awayProb
		var favTeam, dogTeam *db.Team
		var favProb, dogProb, favPickPct, dogPickPct int
		if isHomeFav {
			favTeam = g.HomeTeam
			dogTeam = g.AwayTeam
			favProb = homeProb
			dogProb = awayProb
			favPickPct = homePickPct
			dogPickPct = awayPickPct
		} else {
			favTeam = g.AwayTeam
			dogTeam = g.HomeTeam
			favProb = awayProb
			dogProb = homeProb
			favPickPct = awayPickPct
			dogPickPct = homePickPct
		}

		evFav := float64(favProb) - float64(favPickPct)
		evDog := float64(dogProb) - float64(dogPickPct)

		var quad db.TacticalQuadrant
		var quadLabel, quadBadge string

		// Quadrant Decision Rules
		if favProb >= 67 && favPickPct >= 65 {
			quad = db.QuadrantAnchor
			quadLabel = "Ancla Segura"
			quadBadge = "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30"
			anchorsCount++
		} else if (evFav >= 12 && favProb >= 54) || (evDog >= 10 && dogProb >= 40) {
			quad = db.QuadrantValueGem
			quadLabel = "Gema de Valor (+EV)"
			quadBadge = "bg-cyan-500/20 text-cyan-400 border border-cyan-500/30"
			gemsCount++
		} else if dogProb >= 35 && favPickPct >= 72 && math.Abs(spread) <= 4.5 {
			quad = db.QuadrantUpset
			quadLabel = "Alerta Sorpresa"
			quadBadge = "bg-amber-500/20 text-amber-300 border border-amber-500/40"
			upsetsCount++
		} else {
			quad = db.QuadrantCoinToss
			quadLabel = "Moneda al Aire"
			quadBadge = "bg-purple-500/20 text-purple-300 border border-purple-500/30"
			coinTossesCount++
		}

		var userPickID *int64
		if userPicks != nil {
			if up, ok := userPicks[g.ID]; ok && up != nil && up.PickedTeamID != nil {
				userPickID = up.PickedTeamID
			}
		}

		isLocked := g.IsGameOrWeekLocked(now, lockMode, firstKickoff)

		bases = append(bases, &matchupBase{
			game:               g,
			forecast:           fc,
			comm:               cStat,
			homeProb:           homeProb,
			awayProb:           awayProb,
			homePickPct:        homePickPct,
			awayPickPct:        awayPickPct,
			favTeam:            favTeam,
			dogTeam:            dogTeam,
			favProb:            favProb,
			dogProb:            dogProb,
			favPickPct:         favPickPct,
			dogPickPct:         dogPickPct,
			isHomeFav:          isHomeFav,
			evFav:              evFav,
			evDog:              evDog,
			quadrant:           quad,
			quadrantLabel:      quadLabel,
			quadrantBadgeClass: quadBadge,
			userPickID:         userPickID,
			isLocked:           isLocked,
		})
	}

	// 2. Build the 3 Presets
	buildPreset := func(presetID, name, icon, badgeColor, desc, target, risk, riskBadge string) *db.AdvisorStrategyPreset {
		recs := make([]*db.AdvisorMatchupRecommendation, 0, len(bases))
		projPtsTotal := 0.0
		divergences := 0
		sumProb := 0

		for _, b := range bases {
			var pickedWinner *db.Team
			var pickedWinProb int
			var oppWinProb int
			var pickedPickPct int
			var oppPickPct int
			var evScore float64
			var headline, reasoning string

			// Selection Strategy by Preset:
			switch presetID {
			case "conservative":
				// Strictly choose favorite with highest probability
				pickedWinner = b.favTeam
				pickedWinProb = b.favProb
				oppWinProb = b.dogProb
				pickedPickPct = b.favPickPct
				oppPickPct = b.dogPickPct
				evScore = b.evFav
				headline = fmt.Sprintf("Apuesta a Favorito: %s (%d%% de probabilidad)", b.favTeam.Name, b.favProb)
				reasoning = fmt.Sprintf("Estrategia conservadora: se respalda la mayor probabilidad estadística (%d%% vs %d%%) para asegurar el punto base.", b.favProb, b.dogProb)

			case "aggressive":
				// Hunt underdogs in Upsets, Gems and close Coin Tosses if community is heavily biased
				if (b.quadrant == db.QuadrantUpset || b.quadrant == db.QuadrantValueGem || b.quadrant == db.QuadrantCoinToss) && b.dogProb >= 36 && b.favPickPct >= 65 {
					pickedWinner = b.dogTeam
					pickedWinProb = b.dogProb
					oppWinProb = b.favProb
					pickedPickPct = b.dogPickPct
					oppPickPct = b.favPickPct
					evScore = b.evDog
					headline = fmt.Sprintf("Golpe Táctico: %s (Apalancamiento vs %d%% de la masa)", b.dogTeam.Name, b.favPickPct)
					reasoning = fmt.Sprintf("Oportunidad de remontada: el %d%% de la quiniela eligió a %s. Una victoria de %s (%d%% probabilidad) otorga ventaja masiva sobre los rivales.",
						b.favPickPct, b.favTeam.Name, b.dogTeam.Name, b.dogProb)
				} else {
					pickedWinner = b.favTeam
					pickedWinProb = b.favProb
					oppWinProb = b.dogProb
					pickedPickPct = b.favPickPct
					oppPickPct = b.dogPickPct
					evScore = b.evFav
					headline = fmt.Sprintf("Ancla Protectora: %s (%d%%)", b.favTeam.Name, b.favProb)
					reasoning = fmt.Sprintf("Favorito confiable. Arriesgar aquí no ofrece suficiente valor matemático frente al riesgo de ceder puntos.", )
				}

			default: // "balanced"
				// Optimal blend: Pick favorite by default, but pivot on Value Gems where underdog EV is noticeably superior
				if b.quadrant == db.QuadrantValueGem && b.evDog >= 14 && b.dogProb >= 42 && b.favPickPct >= 70 {
					pickedWinner = b.dogTeam
					pickedWinProb = b.dogProb
					oppWinProb = b.favProb
					pickedPickPct = b.dogPickPct
					oppPickPct = b.favPickPct
					evScore = b.evDog
					headline = fmt.Sprintf("Gema de Alto Valor: %s (+EV)", b.dogTeam.Name)
					reasoning = fmt.Sprintf("Excelente relación valor-riesgo: %s tiene un %d%% real de victoria, pero solo el %d%% de la comunidad lo eligió. Pick óptimo de diferenciación controlada.",
						b.dogTeam.Name, b.dogProb, b.dogPickPct)
				} else {
					pickedWinner = b.favTeam
					pickedWinProb = b.favProb
					oppWinProb = b.dogProb
					pickedPickPct = b.favPickPct
					oppPickPct = b.dogPickPct
					evScore = b.evFav
					headline = fmt.Sprintf("Elección Óptima: %s (%d%% probabilidad)", b.favTeam.Name, b.favProb)
					reasoning = fmt.Sprintf("Consenso sólido y probabilidad matemática a favor de %s (%d%%) con spread proyectado favorable.", b.favTeam.Name, b.favProb)
				}
			}

			// Scores from forecast
			homeScore := 24
			awayScore := 20
			if b.forecast != nil {
				homeScore = b.forecast.ProjHomeScore
				awayScore = b.forecast.ProjAwayScore
			}

			// Adjust scores so recommended winner always matches projected score winner
			if pickedWinner != nil && b.game.HomeTeam != nil && b.game.AwayTeam != nil {
				isHomeWinner := (pickedWinner.ID == b.game.HomeTeam.ID)
				if isHomeWinner && homeScore <= awayScore {
					homeScore = awayScore + 3
				} else if !isHomeWinner && awayScore <= homeScore {
					awayScore = homeScore + 3
				}
			}

			// Confidence score (1-100)
			confScore := int(math.Round(float64(pickedWinProb)*0.7 + math.Min(math.Max(evScore+20, 0), 40)*0.75))
			if confScore > 98 {
				confScore = 98
			}
			if confScore < 30 {
				confScore = 30
			}

			// Check user pick alignment
			matchesUserPick := false
			if b.userPickID != nil && pickedWinner != nil && *b.userPickID == pickedWinner.ID {
				matchesUserPick = true
			}

			// Divergence vs community majority pick
			commMajorityIsFav := b.favPickPct >= 50
			pickedIsFav := (pickedWinner != nil && pickedWinner.ID == b.favTeam.ID)
			if commMajorityIsFav != pickedIsFav {
				divergences++
			}

			// Projected points contribution = (Prob / 100) * winnerPts
			projPtsTotal += (float64(pickedWinProb) / 100.0) * float64(winnerPts)
			sumProb += pickedWinProb

			rec := &db.AdvisorMatchupRecommendation{
				Game:                 b.game,
				Forecast:             b.forecast,
				CommunityStats:       b.comm,
				RecommendedWinner:    pickedWinner,
				RecommendedHomeScore: homeScore,
				RecommendedAwayScore: awayScore,
				WinProbability:       pickedWinProb,
				OpponentWinProb:      oppWinProb,
				CommunityPickPct:     pickedPickPct,
				OpponentPickPct:      oppPickPct,
				ExpectedValueScore:   math.Round(evScore*10) / 10,
				ConfidenceScore:      confScore,
				Quadrant:             b.quadrant,
				QuadrantLabel:        b.quadrantLabel,
				QuadrantBadgeClass:   b.quadrantBadgeClass,
				TacticalHeadline:     headline,
				TacticalReasoning:    reasoning,
				UserCurrentPickID:    b.userPickID,
				MatchesUserPick:      matchesUserPick,
				IsLocked:             b.isLocked,
			}
			recs = append(recs, rec)
		}

		avgProb := 50
		if len(recs) > 0 {
			avgProb = int(math.Round(float64(sumProb) / float64(len(recs))))
		}

		return &db.AdvisorStrategyPreset{
			ID:              presetID,
			Name:            name,
			Icon:            icon,
			BadgeColor:      badgeColor,
			Description:     desc,
			TargetAudience:  target,
			ProjectedPoints: math.Round(projPtsTotal*10) / 10,
			DivergenceCount: divergences,
			AverageWinProb:  avgProb,
			RiskLevel:       risk,
			RiskBadgeClass:  riskBadge,
			Recommendations: recs,
		}
	}

	conservativePreset := buildPreset(
		"conservative",
		"Conservador",
		"fa-solid fa-shield-halved",
		"text-emerald-400 bg-emerald-500/10 border-emerald-500/30",
		"Maximiza probabilidad pura de aciertos respaldando a todos los favoritos estadísticos.",
		"Ideal para líderes que buscan asegurar y defender su posición en la cima del podio.",
		"Bajo",
		"bg-emerald-500/20 text-emerald-400 border border-emerald-500/30",
	)

	balancedPreset := buildPreset(
		"balanced",
		"Equilibrado (+EV)",
		"fa-solid fa-scale-balanced",
		"text-cyan-400 bg-cyan-500/10 border-cyan-500/30",
		"Base segura de favoritos combinada con 1-2 gemas de valor (+EV) donde la comunidad está desalineada.",
		"Estrategia estándar óptima para ascender posiciones sin asumir riesgos desmedidos.",
		"Moderado",
		"bg-cyan-500/20 text-cyan-400 border border-cyan-500/30",
	)

	aggressivePreset := buildPreset(
		"aggressive",
		"Agresivo (Caza-Líderes)",
		"fa-solid fa-bolt",
		"text-rose-400 bg-rose-500/10 border-rose-500/30",
		"Busca activamente las mayores divergencias comunitarias en partidos cerrados para recortar puntos rápidamente.",
		"Ideal si vas rezagado en la tabla y necesitas dar un salto radical en la clasificación.",
		"Alto (Remontada)",
		"bg-rose-500/20 text-rose-400 border border-rose-500/30",
	)

	allPresets := []*db.AdvisorStrategyPreset{
		conservativePreset,
		balancedPreset,
		aggressivePreset,
	}

	var activePreset *db.AdvisorStrategyPreset
	for _, p := range allPresets {
		if p.ID == activePresetID {
			activePreset = p
			break
		}
	}
	if activePreset == nil {
		activePreset = balancedPreset
		activePresetID = "balanced"
	}

	return &db.AdvisorWeeklyOverview{
		Week:             week,
		Weeks:            allWeeks,
		ActivePresetID:   activePresetID,
		ActivePreset:     activePreset,
		AllPresets:       allPresets,
		TotalGames:       totalGames,
		OpenGamesCount:   openGames,
		LockedGamesCount: lockedGames,
		AnchorsCount:     anchorsCount,
		GemsCount:        gemsCount,
		UpsetsCount:      upsetsCount,
		CoinTossesCount:  coinTossesCount,
	}
}
