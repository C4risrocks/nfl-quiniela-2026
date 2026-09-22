package forecasting

import (
	"log"

	"nfl-quiniela-2026/db"
)

// BotWorker manages the automated participation of the official AI bot (@ia_quiniela).
type BotWorker struct {
	repo      *db.Repository
	predictor *Predictor
}

func NewBotWorker(repo *db.Repository) *BotWorker {
	return &BotWorker{
		repo:      repo,
		predictor: NewPredictor(),
	}
}

// EnsureBotPicksForWeek generates and saves forecasts and picks for the bot user (@ia_quiniela).
func (b *BotWorker) EnsureBotPicksForWeek(weekID int64) error {
	botUser, err := b.repo.GetUserByUsername("ia_quiniela")
	if err != nil || botUser == nil {
		log.Printf("[BotWorker] Bot user 'ia_quiniela' not found, skipping picks generation")
		return err
	}

	games, err := b.repo.ListGamesByWeek(weekID)
	if err != nil || len(games) == 0 {
		return err
	}

	forecasts, _ := b.repo.GetWeekForecasts(weekID)

	for _, g := range games {
		forecast, exists := forecasts[g.ID]
		if !exists || forecast == nil {
			// Compute on the fly if not already cached
			homeTeam, errH := b.repo.GetTeamByID(g.HomeTeamID)
			awayTeam, errA := b.repo.GetTeamByID(g.AwayTeamID)
			if errH != nil || errA != nil || homeTeam == nil || awayTeam == nil {
				continue
			}
			forecast = b.predictor.PredictGame(g, homeTeam, awayTeam, nil)
			_ = b.repo.SaveGameForecast(forecast)
		}

		// Save bot pick
		winnerID := forecast.PredictedWinnerID
		homeScore := forecast.ProjHomeScore
		awayScore := forecast.ProjAwayScore

		var pickHomeScore, pickAwayScore *int
		if g.IsTiebreaker {
			pickHomeScore = &homeScore
			pickAwayScore = &awayScore
		}

		if _, err := b.repo.SavePick(botUser.ID, g.ID, &winnerID, pickHomeScore, pickAwayScore); err != nil {
			log.Printf("[BotWorker] Error saving bot pick for game %d: %v", g.ID, err)
		}
	}

	return nil
}

// SeedHistoricalWeeks ensures @ia_quiniela has picks and rankings for already played weeks.
func (b *BotWorker) SeedHistoricalWeeks(seasonID int64) {
	weeks, err := b.repo.ListWeeks(seasonID)
	if err != nil {
		return
	}

	for _, w := range weeks {
		if w.WeekNumber <= 2 {
			_ = b.EnsureBotPicksForWeek(w.ID)
		}
	}
}
