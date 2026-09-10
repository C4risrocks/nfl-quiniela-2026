package scoring

import (
	"fmt"
	"log"
	"math"
	"sort"

	"nfl-quiniela-2026/db"
)

type Calculator struct {
	repo *db.Repository
}

func NewCalculator(repo *db.Repository) *Calculator {
	return &Calculator{repo: repo}
}

// CalculatePickScores calculates points for a single pick on a finished game
func (c *Calculator) CalculatePickScores(pick *db.Pick, game *db.Game, cfg *db.ScoringConfig) (pointsEarned int, bonusPoints int, isCorrect *bool) {
	if game.Status != "final" || game.HomeScore == nil || game.AwayScore == nil {
		return 0, 0, nil
	}

	actualHome := *game.HomeScore
	actualAway := *game.AwayScore
	winningTeamID := game.WinningTeamID()

	// Check winner
	var winnerCorrect bool
	if winningTeamID != nil && pick.PickedTeamID != nil && *pick.PickedTeamID == *winningTeamID {
		winnerCorrect = true
		pointsEarned = cfg.WinnerPoints
	} else {
		winnerCorrect = false
		pointsEarned = 0
	}
	isCorrect = &winnerCorrect

	// Calculate bonus points if in weighted mode
	bonusPoints = 0
	if cfg.ScoringMode == "weighted" && pick.PredictedHomeScore != nil && pick.PredictedAwayScore != nil {
		predHome := *pick.PredictedHomeScore
		predAway := *pick.PredictedAwayScore

		// Exact score bonus
		if predHome == actualHome && predAway == actualAway {
			bonusPoints += cfg.ExactScoreBonus
		}

		// Exact margin of victory bonus
		actualDiff := actualHome - actualAway
		predDiff := predHome - predAway
		if actualDiff == predDiff {
			bonusPoints += cfg.ExactMarginBonus
		}
	}

	return pointsEarned, bonusPoints, isCorrect
}

// CalculateWeekScores computes pick points and generates updated leaderboard standings for a week
func (c *Calculator) CalculateWeekScores(weekID int64) error {
	cfg, err := c.repo.GetScoringConfig()
	if err != nil {
		return fmt.Errorf("loading scoring config: %w", err)
	}

	games, err := c.repo.ListGamesByWeek(weekID)
	if err != nil {
		return fmt.Errorf("listing games for week: %w", err)
	}

	gameMap := make(map[int64]*db.Game)
	var tiebreakerGame *db.Game
	for _, g := range games {
		gameMap[g.ID] = g
		if g.IsTiebreaker {
			tiebreakerGame = g
		}
	}

	picks, err := c.repo.ListAllPicksForWeek(weekID)
	if err != nil {
		return fmt.Errorf("listing picks for week: %w", err)
	}

	// Group picks by user
	userPicks := make(map[int64][]*db.Pick)
	for _, p := range picks {
		game, exists := gameMap[p.GameID]
		if !exists {
			continue
		}

		pts, bonus, isCorrect := c.CalculatePickScores(p, game, cfg)
		if err := c.repo.UpdatePickPoints(p.ID, pts, bonus, isCorrect); err != nil {
			log.Printf("[Scoring] Error updating pick #%d points: %v", p.ID, err)
		}
		p.PointsEarned = pts
		p.BonusPoints = bonus
		p.IsCorrect = isCorrect

		userPicks[p.UserID] = append(userPicks[p.UserID], p)
	}

	// Calculate user weekly totals
	users, err := c.repo.ListUsers()
	if err != nil {
		return fmt.Errorf("listing users: %w", err)
	}

	hasLiveGames := false
	for _, g := range games {
		if g.Status == "in_progress" {
			hasLiveGames = true
			break
		}
	}

	var leaderboardEntries []*db.LeaderboardEntry

	for _, u := range users {
		picksList := userPicks[u.ID]
		totalPts := 0
		correctCount := 0
		totalCount := len(picksList)
		tbError := 999 // default high error
		liveProjectedPts := 0

		for _, p := range picksList {
			g, exists := gameMap[p.GameID]
			if !exists {
				continue
			}

			// Final game points
			totalPts += p.PointsEarned + p.BonusPoints
			if p.IsCorrect != nil && *p.IsCorrect {
				correctCount++
			}

			// Provisional live points for in_progress games
			if g.Status == "in_progress" {
				provWinner := p.ProvisionalWinnerCorrect(g)
				if provWinner != nil && *provWinner {
					liveProjectedPts += cfg.WinnerPoints
					if cfg.ScoringMode == "weighted" && p.PredictedHomeScore != nil && p.PredictedAwayScore != nil && g.HomeScore != nil && g.AwayScore != nil {
						if *p.PredictedHomeScore == *g.HomeScore && *p.PredictedAwayScore == *g.AwayScore {
							liveProjectedPts += cfg.ExactScoreBonus
						}
						if (*p.PredictedHomeScore - *p.PredictedAwayScore) == (*g.HomeScore - *g.AwayScore) {
							liveProjectedPts += cfg.ExactMarginBonus
						}
					}
				}
			}

			// If this is the tiebreaker game, compute score distance
			if tiebreakerGame != nil && p.GameID == tiebreakerGame.ID &&
				tiebreakerGame.HomeScore != nil && tiebreakerGame.AwayScore != nil &&
				p.PredictedHomeScore != nil && p.PredictedAwayScore != nil {
				actualTotal := *tiebreakerGame.HomeScore + *tiebreakerGame.AwayScore
				predTotal := *p.PredictedHomeScore + *p.PredictedAwayScore
				tbError = int(math.Abs(float64(actualTotal - predTotal)))
			}
		}

		entry := &db.LeaderboardEntry{
			UserID:              u.ID,
			Username:            u.Username,
			AvatarURL:           u.AvatarURL,
			TotalPoints:         totalPts,
			LiveProjectedPoints: totalPts + liveProjectedPts,
			CorrectPicks:        correctCount,
			TotalPicks:          totalCount,
			TiebreakerError:     tbError,
			HasLiveGames:        hasLiveGames,
		}
		leaderboardEntries = append(leaderboardEntries, entry)
	}

	// Sort leaderboard entries
	sort.Slice(leaderboardEntries, func(i, j int) bool {
		a := leaderboardEntries[i]
		b := leaderboardEntries[j]

		// 1. Total Points Descending
		if a.TotalPoints != b.TotalPoints {
			return a.TotalPoints > b.TotalPoints
		}
		// 2. Correct Picks Descending
		if a.CorrectPicks != b.CorrectPicks {
			return a.CorrectPicks > b.CorrectPicks
		}
		// 3. Tiebreaker error Ascending (lower is closer)
		if a.TiebreakerError != b.TiebreakerError {
			return a.TiebreakerError < b.TiebreakerError
		}
		// 4. Username Ascending
		return a.Username < b.Username
	})

	// Assign rank and upsert
	for idx, entry := range leaderboardEntries {
		entry.Rank = idx + 1
		if err := c.repo.UpsertWeeklyLeaderboard(entry, weekID); err != nil {
			log.Printf("[Scoring] Error saving weekly leaderboard for user %d: %v", entry.UserID, err)
		}
	}

	log.Printf("[Scoring] Calculated leaderboard for Week ID #%d (%d players ranked).", weekID, len(leaderboardEntries))
	return nil
}

// RecalculateAllWeeks recomputes scores for all weeks in the active season
func (c *Calculator) RecalculateAllWeeks(seasonYear int) error {
	season, err := c.repo.GetActiveSeason(seasonYear)
	if err != nil {
		return err
	}
	weeks, err := c.repo.ListWeeks(season.ID)
	if err != nil {
		return err
	}
	for _, w := range weeks {
		if err := c.CalculateWeekScores(w.ID); err != nil {
			log.Printf("[Scoring] Error recalculating week %d: %v", w.WeekNumber, err)
		}
	}
	return nil
}
