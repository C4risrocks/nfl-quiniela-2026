package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/events"
)

type ScoreCalculator interface {
	CalculateWeekScores(weekID int64) error
}

// LiveAlert represents a real-time game event notification
type LiveAlert struct {
	Type      string `json:"type"` // "game_start", "lead_change", "pick_closing", "game_final"
	Title     string `json:"title"`
	Message   string `json:"message"`
	GameID    int64  `json:"game_id"`
	AwayCode  string `json:"away_code"`
	HomeCode  string `json:"home_code"`
	AwayScore int    `json:"away_score,omitempty"`
	HomeScore int    `json:"home_score,omitempty"`
	Timestamp string `json:"timestamp"`
}

type Syncer struct {
	client         *Client
	repo           *db.Repository
	calculator     ScoreCalculator
	broker         *events.Broker
	seasonYear     int
	mu             sync.Mutex
	closingAlerted map[int64]bool
}

func NewSyncer(client *Client, repo *db.Repository, calculator ScoreCalculator, broker *events.Broker, seasonYear int) *Syncer {
	return &Syncer{
		client:         client,
		repo:           repo,
		calculator:     calculator,
		broker:         broker,
		seasonYear:     seasonYear,
		closingAlerted: make(map[int64]bool),
	}
}

// SyncWeek downloads ESPN games for a given week, upserts them, and marks the Monday Night game as tiebreaker
func (s *Syncer) SyncWeek(weekNum int) (int, error) {
	season, err := s.repo.GetActiveSeason(s.seasonYear)
	if err != nil {
		return 0, fmt.Errorf("getting active season: %w", err)
	}

	week, err := s.repo.GetWeekByNumber(season.ID, weekNum)
	if err != nil {
		return 0, fmt.Errorf("getting week %d: %w", weekNum, err)
	}

	seasonType := 2 // Regular season
	if weekNum > 18 {
		seasonType = 3 // Postseason
	}

	sb, err := s.client.FetchWeekScoreboard(s.seasonYear, weekNum, seasonType)
	if err != nil {
		return 0, fmt.Errorf("fetching espn week %d: %w", weekNum, err)
	}

	teams, err := s.repo.ListTeams()
	if err != nil {
		return 0, fmt.Errorf("listing teams: %w", err)
	}

	teamMap := make(map[string]*db.Team)
	for _, t := range teams {
		teamMap[t.Code] = t
	}

	existingGames, _ := s.repo.ListGamesByWeek(week.ID)
	existingMap := make(map[string]*db.Game)
	for _, g := range existingGames {
		existingMap[g.ESPNGameID] = g
	}

	var syncedGames []*db.Game
	for _, ev := range sb.Events {
		event := ev
		game, err := MapESPNEventToGame(&event, week.ID, teamMap)
		if err != nil {
			log.Printf("[Syncer] Warning mapping event %s (%s): %v", ev.ID, ev.ShortName, err)
			continue
		}
		if err := s.repo.UpsertGameByESPNID(game); err != nil {
			log.Printf("[Syncer] Error upserting game %s: %v", ev.ID, err)
			continue
		}

		// Detect state changes for real-time live alerts
		if oldG, exists := existingMap[game.ESPNGameID]; exists && oldG != nil && game.ID > 0 {
			awayCode, homeCode := "", ""
			if game.AwayTeam != nil {
				awayCode = game.AwayTeam.Code
			} else if oldG.AwayTeam != nil {
				awayCode = oldG.AwayTeam.Code
			}
			if game.HomeTeam != nil {
				homeCode = game.HomeTeam.Code
			} else if oldG.HomeTeam != nil {
				homeCode = oldG.HomeTeam.Code
			}

			// 1. Kickoff / Game Start
			if oldG.Status == "scheduled" && game.Status == "in_progress" {
				s.broadcastAlert(LiveAlert{
					Type:      "game_start",
					Title:     "🏈 ¡Comenzó el partido!",
					Message:   fmt.Sprintf("%s @ %s acaba de dar inicio.", awayCode, homeCode),
					GameID:    game.ID,
					AwayCode:  awayCode,
					HomeCode:  homeCode,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
			}

			// 2. Game Final
			if oldG.Status != "final" && game.Status == "final" {
				awayS, homeS := 0, 0
				if game.AwayScore != nil {
					awayS = *game.AwayScore
				}
				if game.HomeScore != nil {
					homeS = *game.HomeScore
				}
				s.broadcastAlert(LiveAlert{
					Type:      "game_final",
					Title:     "🏁 Partido Finalizado",
					Message:   fmt.Sprintf("Marcador Final: %s %d - %d %s", awayCode, awayS, homeS, homeCode),
					GameID:    game.ID,
					AwayCode:  awayCode,
					HomeCode:  homeCode,
					AwayScore: awayS,
					HomeScore: homeS,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
			}

			// 3. Lead Change
			if oldG.Status == "in_progress" && game.Status == "in_progress" &&
				oldG.HomeScore != nil && oldG.AwayScore != nil &&
				game.HomeScore != nil && game.AwayScore != nil {
				oldDiff := *oldG.HomeScore - *oldG.AwayScore
				newDiff := *game.HomeScore - *game.AwayScore
				if (oldDiff <= 0 && newDiff > 0) || (oldDiff >= 0 && newDiff < 0) {
					leader := homeCode
					trailed := awayCode
					if newDiff < 0 {
						leader = awayCode
						trailed = homeCode
					}
					s.broadcastAlert(LiveAlert{
						Type:      "lead_change",
						Title:     "🔥 ¡Cambio de Líder!",
						Message:   fmt.Sprintf("%s toma la delantera (%d - %d) frente a %s", leader, *game.HomeScore, *game.AwayScore, trailed),
						GameID:    game.ID,
						AwayCode:  awayCode,
						HomeCode:  homeCode,
						AwayScore: *game.AwayScore,
						HomeScore: *game.HomeScore,
						Timestamp: time.Now().UTC().Format(time.RFC3339),
					})
				}
			}
		}

		syncedGames = append(syncedGames, game)
	}

	// 4. Pick Closing Alerts (Approaching Kickoff within 15 mins)
	now := time.Now()
	for _, g := range syncedGames {
		if g.Status == "scheduled" && !g.IsLocked && g.ID > 0 {
			timeUntilKickoff := g.KickoffTime.Sub(now)
			if timeUntilKickoff > 0 && timeUntilKickoff <= 15*time.Minute {
				s.mu.Lock()
				alreadyAlerted := s.closingAlerted[g.ID]
				if !alreadyAlerted {
					s.closingAlerted[g.ID] = true
					s.mu.Unlock()

					awayCode, homeCode := "", ""
					if g.AwayTeam != nil {
						awayCode = g.AwayTeam.Code
					}
					if g.HomeTeam != nil {
						homeCode = g.HomeTeam.Code
					}

					s.broadcastAlert(LiveAlert{
						Type:      "pick_closing",
						Title:     "⏰ ¡Cierre de Picks Próximo!",
						Message:   fmt.Sprintf("Faltan menos de 15 min para el inicio de %s @ %s. ¡Asegura tus pronósticos!", awayCode, homeCode),
						GameID:    g.ID,
						AwayCode:  awayCode,
						HomeCode:  homeCode,
						Timestamp: time.Now().UTC().Format(time.RFC3339),
					})
				} else {
					s.mu.Unlock()
				}
			}
		}
	}

	// If we successfully synced games from ESPN, purge any leftover dummy seed games
	if len(syncedGames) > 0 {
		_ = s.repo.DeletePlaceholderSeedGames(week.ID)
	}

	// Auto-designate the latest game (usually Monday Night Football) as Tiebreaker if not already set
	if len(syncedGames) > 0 {
		games, _ := s.repo.ListGamesByWeek(week.ID)
		hasTiebreaker := false
		for _, g := range games {
			if g.IsTiebreaker {
				hasTiebreaker = true
				break
			}
		}

		if !hasTiebreaker && len(games) > 0 {
			// Sort by kickoff time descending to find the last game of the week
			sort.Slice(games, func(i, j int) bool {
				return games[i].KickoffTime.After(games[j].KickoffTime)
			})
			lastGame := games[0]
			_ = s.repo.SetGameTiebreaker(lastGame.ID, true)
			log.Printf("[Syncer] Marked game #%d (%s vs %s) as Week %d tiebreaker.", lastGame.ID, lastGame.AwayTeam.Code, lastGame.HomeTeam.Code, weekNum)
		}
	}

	// Trigger score recalculation
	if s.calculator != nil {
		if err := s.calculator.CalculateWeekScores(week.ID); err != nil {
			log.Printf("[Syncer] Error calculating week %d scores: %v", weekNum, err)
		}
	}

	// Broadcast real-time SSE updates
	if s.broker != nil && len(syncedGames) > 0 {
		for _, g := range syncedGames {
			s.broker.Broadcast(fmt.Sprintf("game-%d", g.ID), fmt.Sprintf(`{"game_id": %d, "status": "%s"}`, g.ID, g.Status))
		}
		s.broker.Broadcast("week-updated", fmt.Sprintf(`{"week_num": %d}`, weekNum))
		s.broker.BroadcastLeaderboardUpdate()
	}

	log.Printf("[Syncer] Successfully synced %d games for Week %d.", len(syncedGames), weekNum)
	return len(syncedGames), nil
}

// StartBackgroundSync periodically syncs the current active week with adaptive frequency (60s during live games)
func (s *Syncer) StartBackgroundSync(ctx context.Context, defaultInterval time.Duration) {
	currentInterval := defaultInterval
	ticker := time.NewTicker(currentInterval)
	go func() {
		log.Printf("[Syncer] Background sync started with default interval %v (adaptive to 60s during live games).", defaultInterval)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				log.Println("[Syncer] Background sync stopped.")
				return
			case <-ticker.C:
				season, err := s.repo.GetActiveSeason(s.seasonYear)
				if err != nil {
					continue
				}
				weeks, err := s.repo.ListWeeks(season.ID)
				if err != nil {
					continue
				}

				hasActiveGames := false
				for _, w := range weeks {
					if w.Status == "active" || w.Status == "scheduled" {
						games, err := s.repo.ListGamesByWeek(w.ID)
						if err == nil {
							now := time.Now()
							for _, g := range games {
								if g.Status == "in_progress" {
									hasActiveGames = true
									break
								}
								// Also treat games kicking off within 15 mins or past kickoff within 4 hours as potentially live
								if now.After(g.KickoffTime.Add(-15*time.Minute)) && now.Before(g.KickoffTime.Add(4*time.Hour)) && g.Status != "final" {
									hasActiveGames = true
									break
								}
							}
						}

						_, _ = s.SyncWeek(w.WeekNumber)
						break
					}
				}

				// Adjust ticker speed based on active games
				targetInterval := defaultInterval
				if hasActiveGames {
					targetInterval = 60 * time.Second
				}
				if targetInterval != currentInterval {
					currentInterval = targetInterval
					ticker.Reset(currentInterval)
					log.Printf("[Syncer] Switched sync interval to %v (live games active: %v).", currentInterval, hasActiveGames)
				}
			}
		}
	}()
}

func (s *Syncer) broadcastAlert(alert LiveAlert) {
	if s.broker == nil {
		return
	}
	payload, err := json.Marshal(alert)
	if err != nil {
		return
	}
	s.broker.Broadcast("live-alert", string(payload))
	log.Printf("[Syncer] Dispatched live-alert: [%s] %s", alert.Type, alert.Title)
}

