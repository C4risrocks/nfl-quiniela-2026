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

// StartBackgroundSync periodically syncs the current active week
func (s *Syncer) StartBackgroundSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		log.Printf("[Syncer] Background sync started with interval %v.", interval)
		for {
			select {
			case <-ctx.Done():
				log.Println("[Syncer] Background sync stopped.")
				return
			case <-ticker.C:
				// Determine current week or sync active weeks
				season, err := s.repo.GetActiveSeason(s.seasonYear)
				if err != nil {
					continue
				}
				weeks, err := s.repo.ListWeeks(season.ID)
				if err != nil {
					continue
				}
				for _, w := range weeks {
					if w.Status == "active" || w.Status == "scheduled" {
						_, _ = s.SyncWeek(w.WeekNumber)
						break
					}
				}
			}
		}
	}()
}
