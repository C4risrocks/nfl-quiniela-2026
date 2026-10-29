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
	hasNewlyFinishedGames := false
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
				hasNewlyFinishedGames = true
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
			// Sort by kickoff time descending (and ID descending for dual MNF games) to find the last game of the week
			sort.Slice(games, func(i, j int) bool {
				if games[i].KickoffTime.Equal(games[j].KickoffTime) {
					return games[i].ID > games[j].ID
				}
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
			// Check if this week is the current active round determined by schedule / rollover
			season, err := s.repo.GetActiveSeason(s.seasonYear)
			if err == nil && season != nil {
				if activeWk, err := s.repo.GetActiveWeek(season.ID); err == nil && activeWk != nil && activeWk.ID == week.ID {
					newStatus = "active"
				}
			}
			// Also maintain time window around kickoff (-48h to +12h)
			if newStatus != "active" {
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

	if hasNewlyFinishedGames {
		go func() {
			_, _ = s.SyncTeamsStandings(s.seasonYear)
			_ = s.SyncPowerRankingsAndFPI(s.seasonYear, weekNum)
		}()
	}

	log.Printf("[Syncer] Successfully synced %d games for Week %d (status: %s).", len(syncedGames), weekNum, week.Status)
	return len(syncedGames), nil
}

// FetchCurrentWeekNumber retrieves the current active week number, cross-validating ESPN's scoreboard
// against local database state to prevent erratic leaps (such as jumping to Week 18).
func (s *Syncer) FetchCurrentWeekNumber() int {
	season, _ := s.repo.GetActiveSeason(s.seasonYear)
	var dbActiveWeekNum int
	if season != nil {
		if activeW, err := s.repo.GetActiveWeek(season.ID); err == nil && activeW != nil {
			dbActiveWeekNum = activeW.WeekNumber
		}
	}

	sb, err := s.client.FetchCurrentScoreboard()
	if err == nil && sb != nil && sb.Week.Number > 0 {
		espnWk := sb.Week.Number
		// If DB already has active/completed weeks, ensure ESPN doesn't wildly skip forward
		if dbActiveWeekNum > 0 {
			if espnWk >= dbActiveWeekNum && espnWk <= dbActiveWeekNum+1 {
				return espnWk
			}
			// When ESPN reports out of plausible sequence (e.g. 18 during week 8 transition), trust DB
			return dbActiveWeekNum
		}
		if espnWk >= 1 && espnWk <= 22 {
			return espnWk
		}
	}

	if dbActiveWeekNum > 0 {
		return dbActiveWeekNum
	}
	return 1
}

// SyncFullSeasonSchedule syncs all 18 regular season weeks from ESPN so that all games,
// kickoffs, and broadcast channels are pre-seeded in the database, preventing empty gaps.
func (s *Syncer) SyncFullSeasonSchedule(year int) (totalGames int, err error) {
	log.Printf("[Syncer] Ingesting full season schedule for %d (Weeks 1 to 18)...", year)
	for w := 1; w <= 18; w++ {
		count, syncErr := s.SyncWeek(w)
		if syncErr != nil {
			log.Printf("[Syncer] Schedule sync notice for Week %d: %v", w, syncErr)
		} else {
			totalGames += count
		}
	}
	log.Printf("[Syncer] Full season schedule ingestion complete: %d total games ready across 18 weeks.", totalGames)
	return totalGames, nil
}

// StartBackgroundSync periodically syncs the current active week with adaptive frequency (60s during live games)
// and autonomously updates Standings, Injuries, and Power Rankings.
func (s *Syncer) StartBackgroundSync(ctx context.Context, defaultInterval time.Duration) {
	currentInterval := defaultInterval
	ticker := time.NewTicker(currentInterval)
	var lastInjuriesSync time.Time
	var lastStandingsSync time.Time
	var lastRankingsSync time.Time

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

				// 3. Periodic refresh of NFL Injuries (every 4 hours)
				if lastInjuriesSync.IsZero() || time.Since(lastInjuriesSync) >= 4*time.Hour {
					go func() {
						_, _ = s.SyncAllInjuries()
					}()
					lastInjuriesSync = time.Now()
				}

				// 4. Periodic refresh of NFL Standings (every 6 hours)
				if lastStandingsSync.IsZero() || time.Since(lastStandingsSync) >= 6*time.Hour {
					go func() {
						_, _ = s.SyncTeamsStandings(s.seasonYear)
					}()
					lastStandingsSync = time.Now()
				}

				// 5. Periodic refresh of Power Rankings & FPI (every 12 hours)
				if lastRankingsSync.IsZero() || time.Since(lastRankingsSync) >= 12*time.Hour {
					go func(wk int) {
						_, _ = s.SyncAllPowerRankings(s.seasonYear, wk)
						_ = s.SyncPowerRankingsAndFPI(s.seasonYear, wk)
					}(currentWeekNum)
					lastRankingsSync = time.Now()
				}

				// 6. Check if any games are currently live or imminent to adjust polling interval
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

// SyncTeamsStandings syncs NFL standings for the given season year from ESPN (or calculates locally if offline)
func (s *Syncer) SyncTeamsStandings(seasonYear int) (*db.SeasonStandings, error) {
	teams, err := s.repo.ListTeams()
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}
	teamMap := make(map[string]*db.Team, len(teams))
	for _, t := range teams {
		teamMap[t.Code] = t
	}

	var standings *db.SeasonStandings
	var fetchErr error

	if s.client != nil {
		standings, fetchErr = s.client.FetchNFLStandings(seasonYear, s.seasonYear, teamMap)
	}

	if (standings == nil || fetchErr != nil || len(standings.League) == 0) && seasonYear == s.seasonYear {
		log.Printf("[Syncer] ESPN standings fetch failed or unavailable (%v), calculating local standings...", fetchErr)
		standings, err = s.repo.CalculateLocalStandings(s.seasonYear)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate local standings: %w", err)
		}
	}

	if standings != nil && len(standings.League) > 0 {
		if err := s.repo.SaveSeasonStandings(standings); err != nil {
			return nil, fmt.Errorf("failed to save season standings: %w", err)
		}
		if s.broker != nil {
			s.broker.Broadcast("standings-updated", fmt.Sprintf(`{"season": %d}`, seasonYear))
		}
		log.Printf("[Syncer] Successfully synced standings for season %d (%d teams).", seasonYear, len(standings.League))
		return standings, nil
	}

	return nil, fmt.Errorf("no standings data available for season %d", seasonYear)
}

// SyncAllInjuries fetches all NFL injuries from ESPN and updates the database, clearing healed players
func (s *Syncer) SyncAllInjuries() (int, error) {
	if s.client == nil {
		return 0, fmt.Errorf("espn client not initialized")
	}

	injMap, err := s.client.FetchAllNFLInjuries()
	if err != nil {
		return 0, fmt.Errorf("failed to fetch injuries from ESPN: %w", err)
	}

	totalSaved := 0
	for teamCode, injuries := range injMap {
		if err := s.repo.SaveTeamInjuries(teamCode, injuries); err != nil {
			log.Printf("[Syncer] Failed to save injuries for %s: %v", teamCode, err)
			continue
		}
		totalSaved += len(injuries)
	}

	if s.broker != nil {
		s.broker.Broadcast("injuries-updated", fmt.Sprintf(`{"count": %d}`, totalSaved))
	}
	log.Printf("[Syncer] Successfully synced NFL injuries across %d teams (%d total active records).", len(injMap), totalSaved)
	return totalSaved, nil
}

// isAlgorithmicRankings returns true if rankings appear to be generated by the mathematical model fallback.
func isAlgorithmicRankings(rankings []*db.TeamPowerRanking) bool {
	for _, pr := range rankings {
		if strings.Contains(pr.Author, "Modelo Analítico") {
			return true
		}
	}
	return false
}

// hasEditorialRankings checks whether the database already holds official editorial rankings for a week.
func (s *Syncer) hasEditorialRankings(seasonYear, weekNum int) bool {
	if s.repo == nil {
		return false
	}
	existing, err := s.repo.GetPowerRankings(seasonYear, weekNum)
	if err != nil || len(existing) == 0 {
		return false
	}
	for _, pr := range existing {
		if strings.Contains(pr.Author, "Modelo Analítico") {
			return false
		}
	}
	return true
}

// SyncAllPowerRankings iterates from Week 1 up to upToWeek, downloading and storing official ESPN Power Rankings
// for each week to ensure all past and present weeks are fully populated in the database.
func (s *Syncer) SyncAllPowerRankings(seasonYear, upToWeek int) (int, error) {
	if s.client == nil {
		return 0, fmt.Errorf("espn client not initialized")
	}
	if upToWeek < 1 {
		upToWeek = 1
	}

	teams, err := s.repo.ListTeams()
	if err != nil {
		return 0, fmt.Errorf("failed to list teams: %w", err)
	}
	teamMap := make(map[string]*db.Team, len(teams))
	for _, t := range teams {
		teamMap[t.Code] = t
	}

	totalSynced := 0
	for wk := 1; wk <= upToWeek; wk++ {
		rankings, err := s.client.FetchPowerRankings(seasonYear, wk, teamMap)
		if err != nil {
			log.Printf("[Syncer] Warning: unable to fetch power rankings for week %d: %v", wk, err)
			continue
		}
		if len(rankings) > 0 {
			if isAlgorithmicRankings(rankings) && s.hasEditorialRankings(seasonYear, wk) {
				log.Printf("[Syncer] Preserving existing editorial power rankings for week %d; ignoring algorithmic fallback.", wk)
				continue
			}
			if err := s.repo.SavePowerRankings(rankings); err != nil {
				log.Printf("[Syncer] Error saving power rankings for week %d: %v", wk, err)
			} else {
				log.Printf("[Syncer] Successfully saved %d power rankings for week %d.", len(rankings), wk)
				totalSynced++
			}
		}
	}

	if s.broker != nil && totalSynced > 0 {
		s.broker.Broadcast("rankings-updated", fmt.Sprintf(`{"season": %d, "synced_weeks": %d}`, seasonYear, totalSynced))
	}

	return totalSynced, nil
}

// SyncPowerRankingsAndFPI refreshes ESPN Power Rankings and FPI Playoff Probabilities for the given week
func (s *Syncer) SyncPowerRankingsAndFPI(seasonYear, weekNum int) error {
	if s.client == nil {
		return fmt.Errorf("espn client not initialized")
	}

	teams, err := s.repo.ListTeams()
	if err != nil {
		return fmt.Errorf("failed to list teams: %w", err)
	}
	teamMap := make(map[string]*db.Team, len(teams))
	for _, t := range teams {
		teamMap[t.Code] = t
	}

	// 1. Power Rankings
	rankings, err := s.client.FetchPowerRankings(seasonYear, weekNum, teamMap)
	if err == nil && len(rankings) > 0 {
		if isAlgorithmicRankings(rankings) && s.hasEditorialRankings(seasonYear, weekNum) {
			log.Printf("[Syncer] Preserving existing editorial power rankings for week %d; ignoring algorithmic fallback.", weekNum)
		} else {
			if err := s.repo.SavePowerRankings(rankings); err != nil {
				log.Printf("[Syncer] Error saving power rankings for week %d: %v", weekNum, err)
			} else {
				log.Printf("[Syncer] Saved %d power rankings for week %d.", len(rankings), weekNum)
			}
		}
	}

	// 2. Playoff Probabilities
	probs, _, err := s.client.FetchFPIPlayoffProbabilities(seasonYear, weekNum, teamMap)
	if err == nil && len(probs) > 0 {
		if err := s.repo.SavePlayoffProbabilities(probs); err != nil {
			log.Printf("[Syncer] Error saving playoff probabilities for week %d: %v", weekNum, err)
		} else {
			log.Printf("[Syncer] Saved %d playoff probabilities for week %d.", len(probs), weekNum)
		}
	}

	if s.broker != nil {
		s.broker.Broadcast("rankings-updated", fmt.Sprintf(`{"week": %d}`, weekNum))
	}
	return nil
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

