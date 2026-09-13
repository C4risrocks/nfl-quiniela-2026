package scoring

import (
	"nfl-quiniela-2026/db"
)

// evaluateAchievements evaluates unlock criteria for all participants in a week
func (c *Calculator) evaluateAchievements(week *db.Week, games []*db.Game, leaderboardEntries []*db.LeaderboardEntry, userPicks map[int64][]*db.Pick) {
	if week == nil || len(games) == 0 {
		return
	}

	weekNum := week.WeekNumber

	// 1. Precalculate community pick distribution for each game (for Underdog King)
	gamePicksCount := make(map[int64]map[int64]int)
	gameTotalPicks := make(map[int64]int)
	for _, pList := range userPicks {
		for _, p := range pList {
			if p.PickedTeamID != nil {
				if gamePicksCount[p.GameID] == nil {
					gamePicksCount[p.GameID] = make(map[int64]int)
				}
				gamePicksCount[p.GameID][*p.PickedTeamID]++
				gameTotalPicks[p.GameID]++
			}
		}
	}

	// Check if all games in the week are final
	allGamesFinal := true
	for _, g := range games {
		if g.Status != "final" {
			allGamesFinal = false
			break
		}
	}

	for _, entry := range leaderboardEntries {
		picks := userPicks[entry.UserID]
		if len(picks) == 0 {
			continue
		}

		// 1. Pleno Perfecto (perfect_week): 100% correct in week with >= 10 games
		if allGamesFinal && len(games) >= 10 && entry.CorrectPicks == len(games) {
			_, _ = c.repo.AwardAchievement(
				entry.UserID,
				"perfect_week",
				"Pleno Perfecto",
				"Acertaste el 100% de los partidos de una jornada completa",
				"🎯",
				&weekNum,
			)
		}

		// 2. Francotirador MNF (sniper_mnf): exact point sum on finished tiebreaker game
		for _, g := range games {
			if g.IsTiebreaker && g.Status == "final" && g.HomeScore != nil && g.AwayScore != nil {
				for _, p := range picks {
					if p.GameID == g.ID && p.PredictedHomeScore != nil && p.PredictedAwayScore != nil {
						actualTotal := *g.HomeScore + *g.AwayScore
						predTotal := *p.PredictedHomeScore + *p.PredictedAwayScore
						if actualTotal == predTotal {
							_, _ = c.repo.AwardAchievement(
								entry.UserID,
								"sniper_mnf",
								"Francotirador MNF",
								"Acertaste la suma exacta de puntos en el partido de desempate",
								"🎯",
								&weekNum,
							)
						}
					}
				}
			}
		}

		// 3. Racha de Fuego (fire_streak): 5+ consecutive correct picks in the week
		streak := 0
		maxStreak := 0
		for _, g := range games {
			for _, p := range picks {
				if p.GameID == g.ID {
					if p.IsCorrect != nil && *p.IsCorrect {
						streak++
						if streak > maxStreak {
							maxStreak = streak
						}
					} else if p.IsCorrect != nil && !*p.IsCorrect {
						streak = 0
					}
				}
			}
		}
		if maxStreak >= 5 {
			_, _ = c.repo.AwardAchievement(
				entry.UserID,
				"fire_streak",
				"Racha de Fuego",
				"Lograste 5 o más aciertos consecutivos en una misma semana",
				"🔥",
				&weekNum,
			)
		}

		// 4. Rey Underdog (underdog_king): 2+ upset picks with < 35% community share
		underdogWins := 0
		for _, g := range games {
			if g.Status == "final" {
				for _, p := range picks {
					if p.GameID == g.ID && p.IsCorrect != nil && *p.IsCorrect && p.PickedTeamID != nil {
						tot := gameTotalPicks[g.ID]
						if tot >= 3 {
							cnt := gamePicksCount[g.ID][*p.PickedTeamID]
							pct := float64(cnt) / float64(tot) * 100.0
							if pct < 35.0 {
								underdogWins++
							}
						}
					}
				}
			}
		}
		if underdogWins >= 2 {
			_, _ = c.repo.AwardAchievement(
				entry.UserID,
				"underdog_king",
				"Rey Underdog",
				"Acertaste 2 o más sorpresas con menos del 35% de selecciones de la comunidad",
				"🐺",
				&weekNum,
			)
		}

		// 5. Campeón de Jornada (week_champion): Rank 1 in a finalized week
		if allGamesFinal && entry.Rank == 1 {
			_, _ = c.repo.AwardAchievement(
				entry.UserID,
				"week_champion",
				"Campeón de Jornada",
				"Terminaste en el 1er lugar de la tabla semanal",
				"👑",
				&weekNum,
			)
		}

		// 6. Efectividad Élite (elite_accuracy): >= 75% accuracy in a week with >= 12 games
		if allGamesFinal && len(games) >= 12 {
			acc := (float64(entry.CorrectPicks) / float64(len(games))) * 100.0
			if acc >= 75.0 {
				_, _ = c.repo.AwardAchievement(
					entry.UserID,
					"elite_accuracy",
					"Efectividad Élite",
					"Superaste el 75% de efectividad en una jornada",
					"⚡",
					&weekNum,
				)
			}
		}

		// 7. Ojo de Halcón (hawk_eye): >= 10 correct picks in a single week
		if entry.CorrectPicks >= 10 {
			_, _ = c.repo.AwardAchievement(
				entry.UserID,
				"hawk_eye",
				"Ojo de Halcón",
				"Acertaste 10 o más ganadores en una sola jornada",
				"🦅",
				&weekNum,
			)
		}

		// 8. Maestro del Kickoff (tnf_master): hit the earliest/TNF kickoff game
		var earliestGame *db.Game
		for _, g := range games {
			if earliestGame == nil || g.KickoffTime.Before(earliestGame.KickoffTime) {
				earliestGame = g
			}
		}
		if earliestGame != nil && earliestGame.Status == "final" {
			for _, p := range picks {
				if p.GameID == earliestGame.ID && p.IsCorrect != nil && *p.IsCorrect {
					_, _ = c.repo.AwardAchievement(
						entry.UserID,
						"tnf_master",
						"Maestro del Kickoff",
						"Acertaste el partido inaugural de Thursday Night Football de la jornada",
						"⚡",
						&weekNum,
					)
				}
			}
		}

		// 9. Remontada Legendaria (comeback_kid): climbed 4+ positions in weekly rank
		if allGamesFinal && weekNum >= 2 {
			history, _ := c.repo.GetUserWeeklyBreakdown(entry.UserID, week.SeasonID)
			if len(history) >= 2 {
				// Find previous week's rank
				for i := len(history) - 1; i >= 0; i-- {
					if history[i].WeekNumber < weekNum && history[i].Rank > 0 {
						if history[i].Rank-entry.Rank >= 4 {
							_, _ = c.repo.AwardAchievement(
								entry.UserID,
								"comeback_kid",
								"Remontada Legendaria",
								"Escalaste 4 o más posiciones en la tabla dentro de una sola jornada",
								"🚀",
								&weekNum,
							)
						}
						break
					}
				}
			}
		}

		// 10. Verdugo de Rivales (rival_slayer): won 3+ weeks or top finishes in the season
		if allGamesFinal {
			history, _ := c.repo.GetUserWeeklyBreakdown(entry.UserID, week.SeasonID)
			topFinishes := 0
			for _, h := range history {
				if h.Rank == 1 || h.Rank == 2 {
					topFinishes++
				}
			}
			if topFinishes >= 3 {
				_, _ = c.repo.AwardAchievement(
					entry.UserID,
					"rival_slayer",
					"Verdugo de Rivales",
					"Conquistaste 3 o más duelos y victorias directas en la temporada",
					"🥊",
					nil,
				)
			}
		}

		// 11. Veterano de Acero (iron_streak): 4+ consecutive participating weeks
		if weekNum >= 4 {
			history, _ := c.repo.GetUserWeeklyBreakdown(entry.UserID, week.SeasonID)
			consecutiveParticipations := 0
			for _, h := range history {
				if h.TotalGames > 0 {
					consecutiveParticipations++
				} else {
					consecutiveParticipations = 0
				}
				if consecutiveParticipations >= 4 {
					_, _ = c.repo.AwardAchievement(
						entry.UserID,
						"iron_streak",
						"Veterano de Acero",
						"Completaste tus pronósticos durante 4 jornadas consecutivas",
						"🛡️",
						nil,
					)
					break
				}
			}
		}
	}

	// 12. Club de los 100 (century_club): 100+ total points in season
	seasonLB, err := c.repo.GetSeasonLeaderboard(week.SeasonID)
	if err == nil {
		for _, sEntry := range seasonLB {
			if sEntry.TotalPoints >= 100 {
				_, _ = c.repo.AwardAchievement(
					sEntry.UserID,
					"century_club",
					"Club de los 100",
					"Acumulaste 100 o más puntos totales a lo largo de la temporada",
					"💯",
					nil,
				)
			}
		}
	}
}

