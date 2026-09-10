package espn

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/events"
)

type ScoreCalculator interface {
	CalculateWeekScores(weekID int64) error
}

type Syncer struct {
	client     *Client
	repo       *db.Repository
	calculator ScoreCalculator
	broker     *events.Broker
	seasonYear int
}

func NewSyncer(client *Client, repo *db.Repository, calculator ScoreCalculator, broker *events.Broker, seasonYear int) *Syncer {
	return &Syncer{
		client:     client,
		repo:       repo,
		calculator: calculator,
		broker:     broker,
		seasonYear: seasonYear,
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
		syncedGames = append(syncedGames, game)
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
