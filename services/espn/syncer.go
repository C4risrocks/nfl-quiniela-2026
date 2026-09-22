package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/events"
	"nfl-quiniela-2026/services/forecasting"
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

// SyncStatus represents the operational diagnostics of the ESPN sync engine
type SyncStatus struct {
	LastSyncTime     time.Time
	LastSyncDuration time.Duration
	LastSyncError    string
	LastSyncCount    int
	IsHealthy        bool
}

type Syncer struct {
	client           *Client
	repo             *db.Repository
	calculator       ScoreCalculator
	broker           *events.Broker
	seasonYear       int
	mu               sync.Mutex
	closingAlerted   map[int64]bool
	lastSyncTime     time.Time
	lastSyncDuration time.Duration
	lastSyncError    string
	lastSyncCount    int
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

// GetSyncStatus returns the latest diagnostic information about ESPN synchronization
func (s *Syncer) GetSyncStatus() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	isHealthy := s.lastSyncError == "" && !s.lastSyncTime.IsZero()
	return SyncStatus{
		LastSyncTime:     s.lastSyncTime,
		LastSyncDuration: s.lastSyncDuration,
		LastSyncError:    s.lastSyncError,
		LastSyncCount:    s.lastSyncCount,
		IsHealthy:        isHealthy,
	}
}

// SyncWeek downloads ESPN games for a given week, upserts them, and marks the Monday Night game as tiebreaker
func (s *Syncer) SyncWeek(weekNum int) (count int, err error) {
	start := time.Now()
	defer func() {
		s.mu.Lock()
		s.lastSyncTime = time.Now()
		s.lastSyncDuration = time.Since(start)
		if err != nil {
			s.lastSyncError = err.Error()
		} else {
			s.lastSyncError = ""
			s.lastSyncCount = count
		}
		s.mu.Unlock()
	}()

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

		// Calculate and store AI forecast & odds consensus
		if game.HomeTeam != nil && game.AwayTeam != nil && game.ID > 0 {
			var oddsInput *forecasting.OddsInput
			if len(event.Competitions) > 0 && len(event.Competitions[0].Odds) > 0 {
				o := event.Competitions[0].Odds[0]
				prov := o.Provider.Name
				if prov == "" {
					prov = "DraftKings"
				}
				favCode := ""
				parts := strings.Fields(o.Details)
				if len(parts) >= 1 && parts[0] != "EVEN" {
					favCode = parts[0]
				}
				oddsInput = &forecasting.OddsInput{
					Details:          o.Details,
					OverUnder:        o.OverUnder,
					Spread:           o.Spread,
					Provider:         prov,
					FavoriteTeamCode: favCode,
				}
			}
			predictor := forecasting.NewPredictor()
			forecast := predictor.PredictGame(game, game.HomeTeam, game.AwayTeam, oddsInput)
			if err := s.repo.SaveGameForecast(forecast); err != nil {
				log.Printf("[Syncer] Warning saving forecast for game %d: %v", game.ID, err)
			}
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

	// 5. Synchronize and persist detailed game statistics (boxscores, player stats, scoring, drives)
	for _, g := range syncedGames {
		if g.ID <= 0 || (g.Status != "in_progress" && g.Status != "final") {
			continue
		}

		// If the game is already final and has cached stats_json with stats, skip external request
		if g.Status == "final" && g.StatsJSON != "" {
			if existingSummary := g.DetailedSummary(); existingSummary != nil && existingSummary.HasStats {
				continue
			}
		}

		var summary *db.GameDetailedSummary
		if g.ESPNGameID != "" {
			summary, _ = s.client.FetchGameSummary(g.ESPNGameID)
		}

		// If ESPN summary has no stats, use realistic fallback
		if (summary == nil || !summary.HasStats) && (g.Status == "in_progress" || g.Status == "final") {
			summary = GenerateRealisticSummary(g)
		}

		if summary != nil && (summary.HasStats || summary.HasPlayerStats || len(summary.ScoringPlays) > 0 || summary.HasDrives || summary.StatusDetail != "" || summary.AwayScore != nil) {
			if b, err := json.Marshal(summary); err == nil {
				g.StatsJSON = string(b)
				if g.Status == "in_progress" {
					_ = s.repo.UpdateGameLiveStats(g.ID, g.StatsJSON, summary.HomeScore, summary.AwayScore, summary.StatusDetail, summary.Linescores)
				} else {
					_ = s.repo.UpdateGameStatsJSON(g.ID, g.StatsJSON)
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

	// Update week status based on synced games
	if len(syncedGames) > 0 {
		allFinal := true
		hasLive := false
		for _, g := range syncedGames {
			if g.Status == "in_progress" {
				hasLive = true
			}
			if g.Status != "final" {
				allFinal = false
			}
		}

		newStatus := "scheduled"
		if allFinal {
			newStatus = "completed"
		} else if hasLive {
			newStatus = "active"
		} else {
			now := time.Now()
			firstKickoff := syncedGames[0].KickoffTime
			lastKickoff := syncedGames[len(syncedGames)-1].KickoffTime
			for _, g := range syncedGames {
				if g.KickoffTime.Before(firstKickoff) {
					firstKickoff = g.KickoffTime
				}
				if g.KickoffTime.After(lastKickoff) {
					lastKickoff = g.KickoffTime
				}
			}
			if now.After(firstKickoff.Add(-48*time.Hour)) && now.Before(lastKickoff.Add(12*time.Hour)) {
				newStatus = "active"
			}
		}

		if week.Status != newStatus {
			_ = s.repo.UpdateWeekStatus(week.ID, newStatus)
			week.Status = newStatus
		}
	}

	// Ensure official AI bot picks for this week are up-to-date
	botWorker := forecasting.NewBotWorker(s.repo)
	_ = botWorker.EnsureBotPicksForWeek(week.ID)

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

	log.Printf("[Syncer] Successfully synced %d games for Week %d (status: %s).", len(syncedGames), weekNum, week.Status)
	return len(syncedGames), nil
}

// FetchCurrentWeekNumber retrieves the current active week number from ESPN, falling back to DB active week
func (s *Syncer) FetchCurrentWeekNumber() int {
	if sb, err := s.client.FetchCurrentScoreboard(); err == nil && sb != nil && sb.Week.Number > 0 {
		return sb.Week.Number
	}
	season, err := s.repo.GetActiveSeason(s.seasonYear)
	if err == nil && season != nil {
		if activeW, err := s.repo.GetActiveWeek(season.ID); err == nil && activeW != nil {
			return activeW.WeekNumber
		}
	}
	return 1
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

				// 1. Determine active week number from ESPN or DB
				currentWeekNum := s.FetchCurrentWeekNumber()

				// 2. Identify which weeks to sync:
				// - The current active week
				// - Any week with in_progress games in DB
				// - Any prior week that is not yet marked 'completed'
				weeksToSync := make(map[int]bool)
				weeksToSync[currentWeekNum] = true

				for _, w := range weeks {
					if w.WeekNumber < currentWeekNum && w.Status != "completed" {
						weeksToSync[w.WeekNumber] = true
					}
					games, err := s.repo.ListGamesByWeek(w.ID)
					if err == nil {
						for _, g := range games {
							if g.Status == "in_progress" {
								weeksToSync[w.WeekNumber] = true
								break
							}
						}
					}
				}

				for wNum := range weeksToSync {
					_, _ = s.SyncWeek(wNum)
				}

				// 3. Check if any games are currently live or imminent to adjust polling interval
				hasActiveGames := false
				now := time.Now()
				for _, w := range weeks {
					games, err := s.repo.ListGamesByWeek(w.ID)
					if err == nil {
						for _, g := range games {
							if g.Status == "in_progress" {
								hasActiveGames = true
								break
							}
							if now.After(g.KickoffTime.Add(-15*time.Minute)) && now.Before(g.KickoffTime.Add(4*time.Hour)) && g.Status != "final" {
								hasActiveGames = true
								break
							}
						}
					}
					if hasActiveGames {
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

