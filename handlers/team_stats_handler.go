package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
	"nfl-quiniela-2026/services/espn"
)

var defaultAvailableSeasons = []int{2026, 2025, 2024, 2023, 2022}

type TeamStatsHandler struct {
	repo       *db.Repository
	renderer   *Renderer
	espnClient *espn.Client
	seasonYear int
}

func NewTeamStatsHandler(repo *db.Repository, renderer *Renderer, espnClient *espn.Client, seasonYear int) *TeamStatsHandler {
	return &TeamStatsHandler{
		repo:       repo,
		renderer:   renderer,
		espnClient: espnClient,
		seasonYear: seasonYear,
	}
}

// ShowTeams renders the main team statistics and standings dashboard
func (h *TeamStatsHandler) ShowTeams(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())

	seasonYear := h.parseSeasonParam(r)
	viewMode := h.parseViewParam(r)

	standings := h.getStandingsWithFallback(seasonYear)
	allTeams, _ := h.repo.ListTeams()

	h.renderer.RenderPage(w, "teams.html", map[string]interface{}{
		"ActiveNav":        "teams",
		"User":             user,
		"Standings":        standings,
		"AllTeams":         allTeams,
		"SelectedSeason":   seasonYear,
		"SelectedView":     viewMode,
		"AvailableSeasons": defaultAvailableSeasons,
		"CurrentSeason":    h.seasonYear,
	})
}

// CompareTeams handles head-to-head comparison between two NFL franchises
func (h *TeamStatsHandler) CompareTeams(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	seasonYear := h.parseSeasonParam(r)

	teamCodeA := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("team1")))
	if teamCodeA == "" {
		teamCodeA = strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("t1")))
	}
	teamCodeB := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("team2")))
	if teamCodeB == "" {
		teamCodeB = strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("t2")))
	}

	allTeams, _ := h.repo.ListTeams()
	if teamCodeA == "" && len(allTeams) > 0 {
		teamCodeA = allTeams[0].Code
	}
	if teamCodeB == "" && len(allTeams) > 1 {
		teamCodeB = allTeams[1].Code
	}

	teamCodeA = espn.NormalizeTeamCode(teamCodeA)
	teamCodeB = espn.NormalizeTeamCode(teamCodeB)

	comparison := h.buildTeamH2HComparison(teamCodeA, teamCodeB, seasonYear)

	data := map[string]interface{}{
		"ActiveNav":        "teams",
		"User":             user,
		"Comparison":       comparison,
		"AllTeams":         allTeams,
		"SelectedSeason":   seasonYear,
		"AvailableSeasons": defaultAvailableSeasons,
		"CurrentSeason":    h.seasonYear,
	}

	if r.Header.Get("HX-Request") != "" && r.URL.Query().Get("format") != "page" {
		h.renderer.RenderPartial(w, "team_h2h_modal.html", data)
		return
	}

	h.renderer.RenderPage(w, "team_compare.html", data)
}

// TeamsTablePartial renders just the dynamic standings table (for HTMX swaps when toggling seasons or views)
func (h *TeamStatsHandler) TeamsTablePartial(w http.ResponseWriter, r *http.Request) {
	seasonYear := h.parseSeasonParam(r)
	viewMode := h.parseViewParam(r)

	standings := h.getStandingsWithFallback(seasonYear)

	h.renderer.RenderPartial(w, "teams_standings_table.html", map[string]interface{}{
		"Standings":      standings,
		"SelectedSeason": seasonYear,
		"SelectedView":   viewMode,
		"CurrentSeason":  h.seasonYear,
	})
}

// ShowTeamDetail renders the full dedicated page for a single NFL franchise
func (h *TeamStatsHandler) ShowTeamDetail(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	teamCode := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "code")))
	teamCode = espn.NormalizeTeamCode(teamCode)

	team, err := h.repo.GetTeamByCode(teamCode)
	if err != nil || team == nil {
		http.NotFound(w, r)
		return
	}

	seasonYear := h.parseSeasonParam(r)
	detailData := h.buildTeamDetailData(team, seasonYear)

	h.renderer.RenderPage(w, "team_detail.html", map[string]interface{}{
		"ActiveNav":        "teams",
		"User":             user,
		"Team":             team,
		"Detail":           detailData,
		"SelectedSeason":   seasonYear,
		"AvailableSeasons": defaultAvailableSeasons,
		"CurrentSeason":    h.seasonYear,
	})
}

// TeamDetailModal renders the quick modal drawer for an NFL franchise
func (h *TeamStatsHandler) TeamDetailModal(w http.ResponseWriter, r *http.Request) {
	teamCode := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "code")))
	teamCode = espn.NormalizeTeamCode(teamCode)

	team, err := h.repo.GetTeamByCode(teamCode)
	if err != nil || team == nil {
		http.Error(w, "Equipo no encontrado", http.StatusNotFound)
		return
	}

	seasonYear := h.parseSeasonParam(r)
	detailData := h.buildTeamDetailData(team, seasonYear)

	h.renderer.RenderPartial(w, "team_detail_modal.html", map[string]interface{}{
		"Team":           team,
		"Detail":         detailData,
		"SelectedSeason": seasonYear,
		"CurrentSeason":  h.seasonYear,
	})
}

// Helper methods

func (h *TeamStatsHandler) parseSeasonParam(r *http.Request) int {
	seasonYear := h.seasonYear
	if sStr := r.URL.Query().Get("season"); sStr != "" {
		if s, err := strconv.Atoi(sStr); err == nil && s >= 2000 && s <= 2030 {
			seasonYear = s
		}
	}
	return seasonYear
}

func (h *TeamStatsHandler) parseViewParam(r *http.Request) string {
	view := r.URL.Query().Get("view")
	switch view {
	case "conference", "league":
		return view
	default:
		return "division"
	}
}

func (h *TeamStatsHandler) getTeamCodeMap() map[string]*db.Team {
	teams, err := h.repo.ListTeams()
	if err != nil {
		return make(map[string]*db.Team)
	}
	m := make(map[string]*db.Team, len(teams))
	for _, t := range teams {
		m[t.Code] = t
	}
	return m
}

func (h *TeamStatsHandler) getStandingsWithFallback(seasonYear int) *db.SeasonStandings {
	teamMap := h.getTeamCodeMap()

	var standings *db.SeasonStandings
	var err error

	// 1. For historical seasons (< currentSeason), check Database first (0ms, 100% offline-ready)
	if seasonYear < h.seasonYear && h.repo != nil {
		standings, _ = h.repo.GetSeasonStandings(seasonYear)
		if standings != nil && len(standings.League) > 0 {
			return standings
		}
	}

	// 2. Fetch from ESPN
	if h.espnClient != nil {
		standings, err = h.espnClient.FetchNFLStandings(seasonYear, h.seasonYear, teamMap)
		if standings != nil && len(standings.League) > 0 && h.repo != nil {
			// Persist immediately to Database
			_ = h.repo.SaveSeasonStandings(standings)
		}
	}

	// 3. If ESPN failed, check Database as fallback
	if (standings == nil || err != nil || len(standings.League) == 0) && h.repo != nil {
		dbStandings, _ := h.repo.GetSeasonStandings(seasonYear)
		if dbStandings != nil && len(dbStandings.League) > 0 {
			standings = dbStandings
		}
	}

	// 4. Fallback to local SQLite calculation for active season if ESPN & DB both failed
	if (standings == nil || len(standings.League) == 0) && seasonYear == h.seasonYear {
		standings, _ = h.repo.CalculateLocalStandings(h.seasonYear)
		if standings != nil && len(standings.League) > 0 && h.repo != nil {
			_ = h.repo.SaveSeasonStandings(standings)
		}
	}

	// 5. If still nil, construct an empty fallback model
	if standings == nil {
		standings = &db.SeasonStandings{
			Year:        seasonYear,
			IsCurrent:   (seasonYear == h.seasonYear),
			League:      make([]*db.TeamStanding, 0),
			Conferences: make([]*db.ConferenceStandings, 0),
			Divisions:   make([]*db.DivisionStandings, 0),
		}
	}

	// Enrich current season teams with quiniela community statistics
	if seasonYear == h.seasonYear && len(standings.League) > 0 {
		var mostPopularTeam *db.TeamStanding
		maxFans := -1

		for _, ts := range standings.League {
			if ts.TeamID > 0 {
				comm, _ := h.repo.GetTeamCommunityStats(ts.TeamID, h.seasonYear)
				if comm != nil {
					ts.FavoriteFansCount = len(comm.FavoriteUsers)
					ts.QuinielaPickCount = comm.TotalPicksMade
					ts.QuinielaWinCount = comm.WinningPicks
					ts.QuinielaWinRate = comm.PickWinRate

					if ts.FavoriteFansCount > maxFans {
						maxFans = ts.FavoriteFansCount
						mostPopularTeam = ts
					}
				}
			}
		}

		if standings.Summary != nil && mostPopularTeam != nil && maxFans > 0 {
			standings.Summary.QuinielaMostPopular = mostPopularTeam
		}
	}

	// Enrich standings with Pythagorean wins, L5 form, one-score records and Playoff pictures
	h.enrichStandingsWithAdvancedMetrics(standings, seasonYear)

	return standings
}

type TeamDetailPayload struct {
	Standing       *db.TeamStanding
	Schedule       []*db.TeamScheduleItem
	CommunityStats *db.TeamCommunityStats
	CompletedGames int
	UpcomingGames  int
	HomeWins       int
	HomeLosses     int
	AwayWins       int
	AwayLosses     int
	HasPlayoffs    bool
}

func (h *TeamStatsHandler) buildTeamDetailData(team *db.Team, seasonYear int) *TeamDetailPayload {
	teamMap := h.getTeamCodeMap()
	standings := h.getStandingsWithFallback(seasonYear)

	var teamStanding *db.TeamStanding
	for _, ts := range standings.League {
		if ts.TeamCode == team.Code {
			teamStanding = ts
			break
		}
	}

	if teamStanding == nil {
		teamStanding = &db.TeamStanding{
			TeamID:         team.ID,
			TeamCode:       team.Code,
			TeamName:       team.Name,
			TeamCity:       team.City,
			LogoURL:        team.LogoURL,
			PrimaryColor:   team.PrimaryColor,
			SecondaryColor: team.SecondaryColor,
			Conference:     team.Conference,
			Division:       team.Division,
			Streak:         "-",
			GamesBehind:    "-",
		}
	}

	// Fetch schedule
	var schedule []*db.TeamScheduleItem

	// For current season, local database has all 18 weeks of matchups
	if seasonYear == h.seasonYear {
		schedule, _ = h.repo.ListTeamGamesBySeason(team.Code, seasonYear)
	}

	// For past seasons, check local DB first (0ms, offline-ready)
	if len(schedule) == 0 && h.repo != nil {
		schedule, _ = h.repo.GetTeamSchedule(team.Code, seasonYear)
	}

	// If schedule is still empty, query ESPN and persist to DB
	if len(schedule) == 0 && h.espnClient != nil {
		schedule, _ = h.espnClient.FetchTeamSchedule(team.Code, seasonYear, teamMap)
		if len(schedule) > 0 && h.repo != nil {
			_ = h.repo.SaveTeamSchedule(team.Code, seasonYear, schedule)
		}
	}

	completedCount := 0
	upcomingCount := 0
	for _, itm := range schedule {
		if itm.Result == "W" || itm.Result == "L" || itm.Result == "T" {
			completedCount++
		} else {
			upcomingCount++
		}
	}

	var commStats *db.TeamCommunityStats
	if seasonYear == h.seasonYear {
		commStats, _ = h.repo.GetTeamCommunityStats(team.ID, h.seasonYear)
	}

	return &TeamDetailPayload{
		Standing:       teamStanding,
		Schedule:       schedule,
		CommunityStats: commStats,
		CompletedGames: completedCount,
		UpcomingGames:  upcomingCount,
		HasPlayoffs:    teamStanding.ConferenceSeed >= 1 && teamStanding.ConferenceSeed <= 7,
	}
}

func (h *TeamStatsHandler) enrichStandingsWithAdvancedMetrics(standings *db.SeasonStandings, seasonYear int) {
	if standings == nil || len(standings.League) == 0 {
		return
	}

	// 1. Compute Pythagorean wins, diff, status, and L5/one-score record for each team
	for _, ts := range standings.League {
		// Pythagorean wins calculation (Bill James formula adapted for NFL with exponent 2.37)
		if ts.GamesPlayed > 0 && (ts.PointsFor > 0 || ts.PointsAgainst > 0) {
			pfExp := math.Pow(float64(ts.PointsFor), 2.37)
			paExp := math.Pow(float64(ts.PointsAgainst), 2.37)
			if pfExp+paExp > 0 {
				expectedWinPct := pfExp / (pfExp + paExp)
				ts.PythagoreanWins = math.Round(expectedWinPct*float64(ts.GamesPlayed)*10) / 10
				diff := float64(ts.Wins) - ts.PythagoreanWins
				ts.PythagoreanDiff = math.Round(diff*10) / 10

				if diff >= 1.5 {
					ts.PythagoreanStatus = "overperforming" // Trampa / Sobre-rindiendo
				} else if diff <= -1.5 {
					ts.PythagoreanStatus = "underperforming" // Sleeper / Sub-rindiendo
				} else {
					ts.PythagoreanStatus = "balanced"
				}
			}
		} else {
			ts.PythagoreanStatus = "balanced"
		}

		// Schedule items for L5 and One-Score games
		var sched []*db.TeamScheduleItem
		if seasonYear == h.seasonYear && h.repo != nil {
			sched, _ = h.repo.ListTeamGamesBySeason(ts.TeamCode, seasonYear)
		}
		if len(sched) == 0 && h.repo != nil {
			sched, _ = h.repo.GetTeamSchedule(ts.TeamCode, seasonYear)
		}

		completedGames := make([]*db.TeamScheduleItem, 0)
		oneScoreWins := 0
		oneScoreLosses := 0

		for _, item := range sched {
			if item.Result == "W" || item.Result == "L" || item.Result == "T" {
				completedGames = append(completedGames, item)
				if item.TeamScore != nil && item.OpponentScore != nil {
					diff := int(math.Abs(float64(*item.TeamScore - *item.OpponentScore)))
					if diff <= 8 {
						if item.Result == "W" {
							oneScoreWins++
						} else if item.Result == "L" {
							oneScoreLosses++
						}
					}
				}
			}
		}

		if len(completedGames) > 0 {
			ts.OneScoreRecord = fmt.Sprintf("%d-%d", oneScoreWins, oneScoreLosses)

			// Take last 5 completed games
			startIdx := len(completedGames) - 5
			if startIdx < 0 {
				startIdx = 0
			}
			l5Slice := completedGames[startIdx:]
			ts.LastFiveResults = make([]db.TeamFormItem, 0, len(l5Slice))
			for _, g := range l5Slice {
				scoreStr := ""
				if g.TeamScore != nil && g.OpponentScore != nil {
					scoreStr = fmt.Sprintf("%d-%d", *g.TeamScore, *g.OpponentScore)
				}
				ts.LastFiveResults = append(ts.LastFiveResults, db.TeamFormItem{
					Result:       g.Result,
					Score:        scoreStr,
					OpponentCode: g.OpponentCode,
					IsHome:       g.IsHome,
					WeekNumber:   g.WeekNumber,
				})
			}
		} else {
			ts.OneScoreRecord = "0-0"
			ts.LastFiveResults = make([]db.TeamFormItem, 0)
		}
	}

	// 2. Playoff Pictures for Conferences
	standings.PlayoffPictures = make([]*db.ConferencePlayoffPicture, 0)

	for _, conf := range standings.Conferences {
		if len(conf.Teams) == 0 {
			continue
		}

		picture := &db.ConferencePlayoffPicture{
			Conference: conf.Conference,
			Name:       conf.Name,
			Matchups:   make([]db.PlayoffMatchupMockup, 0),
			InTheHunt:  make([]*db.TeamStanding, 0),
			Eliminated: make([]*db.TeamStanding, 0),
		}

		// Seed 1: Bye
		picture.ByeTeam = conf.Teams[0]
		picture.ByeTeam.PlayoffStatus = "clinched_bye"

		// Seeds 2-4: Division champions
		for i := 1; i <= 3 && i < len(conf.Teams); i++ {
			conf.Teams[i].PlayoffStatus = "clinched_division"
		}

		// Seeds 5-7: Wild Cards
		for i := 4; i <= 6 && i < len(conf.Teams); i++ {
			conf.Teams[i].PlayoffStatus = "clinched_playoff"
		}

		// Matchups: 2 vs 7, 3 vs 6, 4 vs 5
		if len(conf.Teams) >= 7 {
			picture.Matchups = append(picture.Matchups, db.PlayoffMatchupMockup{
				HighSeed: conf.Teams[1], // Seed 2
				LowSeed:  conf.Teams[6], // Seed 7
				Label:    "Wild Card: #2 vs #7",
			})
			picture.Matchups = append(picture.Matchups, db.PlayoffMatchupMockup{
				HighSeed: conf.Teams[2], // Seed 3
				LowSeed:  conf.Teams[5], // Seed 6
				Label:    "Wild Card: #3 vs #6",
			})
			picture.Matchups = append(picture.Matchups, db.PlayoffMatchupMockup{
				HighSeed: conf.Teams[3], // Seed 4
				LowSeed:  conf.Teams[4], // Seed 5
				Label:    "Wild Card: #4 vs #5",
			})
		}

		// 7th seed for Games Behind calculation
		var seed7Wins, seed7Losses int
		if len(conf.Teams) >= 7 {
			seed7Wins = conf.Teams[6].Wins
			seed7Losses = conf.Teams[6].Losses
		}

		// Seeds 8-11: In The Hunt
		for i := 7; i < len(conf.Teams); i++ {
			t := conf.Teams[i]
			gb := float64((seed7Wins-t.Wins)+(t.Losses-seed7Losses)) / 2.0
			if gb <= 0 {
				t.PlayoffGB = "-"
			} else {
				if gb == math.Floor(gb) {
					t.PlayoffGB = fmt.Sprintf("%.0f", gb)
				} else {
					t.PlayoffGB = fmt.Sprintf("%.1f", gb)
				}
			}

			if i < 11 {
				t.PlayoffStatus = "in_hunt"
				picture.InTheHunt = append(picture.InTheHunt, t)
			} else {
				t.PlayoffStatus = "eliminated"
				picture.Eliminated = append(picture.Eliminated, t)
			}
		}

		standings.PlayoffPictures = append(standings.PlayoffPictures, picture)
	}
}

func (h *TeamStatsHandler) buildTeamH2HComparison(teamCodeA, teamCodeB string, seasonYear int) *db.TeamH2HComparison {
	standings := h.getStandingsWithFallback(seasonYear)

	var teamAStanding, teamBStanding *db.TeamStanding
	for _, ts := range standings.League {
		if ts.TeamCode == teamCodeA {
			teamAStanding = ts
		}
		if ts.TeamCode == teamCodeB {
			teamBStanding = ts
		}
	}

	if teamAStanding == nil {
		if t, _ := h.repo.GetTeamByCode(teamCodeA); t != nil {
			teamAStanding = &db.TeamStanding{
				TeamID: t.ID, TeamCode: t.Code, TeamName: t.Name, TeamCity: t.City,
				LogoURL: t.LogoURL, PrimaryColor: t.PrimaryColor, SecondaryColor: t.SecondaryColor,
				Conference: t.Conference, Division: t.Division, Streak: "-", GamesBehind: "-",
			}
		}
	}

	if teamBStanding == nil {
		if t, _ := h.repo.GetTeamByCode(teamCodeB); t != nil {
			teamBStanding = &db.TeamStanding{
				TeamID: t.ID, TeamCode: t.Code, TeamName: t.Name, TeamCity: t.City,
				LogoURL: t.LogoURL, PrimaryColor: t.PrimaryColor, SecondaryColor: t.SecondaryColor,
				Conference: t.Conference, Division: t.Division, Streak: "-", GamesBehind: "-",
			}
		}
	}

	matchups, winsA, winsB, ties, _ := h.repo.GetTeamH2HComparison(teamCodeA, teamCodeB)

	// Community Advantage description
	commAdv := "Sin picks suficientes registrados en la quiniela."
	if teamAStanding != nil && teamBStanding != nil && (teamAStanding.QuinielaPickCount > 0 || teamBStanding.QuinielaPickCount > 0) {
		if teamAStanding.QuinielaWinRate > teamBStanding.QuinielaWinRate {
			commAdv = fmt.Sprintf("Los quinielistas confían más en %s con %d%% de efectividad vs %d%% de %s.",
				teamAStanding.TeamName, teamAStanding.QuinielaWinRate, teamBStanding.QuinielaWinRate, teamBStanding.TeamName)
		} else if teamBStanding.QuinielaWinRate > teamAStanding.QuinielaWinRate {
			commAdv = fmt.Sprintf("Los quinielistas confían más en %s con %d%% de efectividad vs %d%% de %s.",
				teamBStanding.TeamName, teamBStanding.QuinielaWinRate, teamAStanding.QuinielaWinRate, teamAStanding.TeamName)
		} else {
			commAdv = fmt.Sprintf("Ambos equipos comparten un %d%% de efectividad en las elecciones de la comunidad.", teamAStanding.QuinielaWinRate)
		}
	}

	// Analytical Verdict
	scoreA := 0.0
	scoreB := 0.0

	if teamAStanding != nil && teamBStanding != nil {
		// Factor 1: Win %
		scoreA += teamAStanding.WinPercent * 35.0
		scoreB += teamBStanding.WinPercent * 35.0

		// Factor 2: Point differential (normalized)
		scoreA += math.Max(math.Min(float64(teamAStanding.PointDiff)*0.15, 20.0), -20.0)
		scoreB += math.Max(math.Min(float64(teamBStanding.PointDiff)*0.15, 20.0), -20.0)

		// Factor 3: Pythagorean expectation
		if teamAStanding.GamesPlayed > 0 {
			scoreA += (teamAStanding.PythagoreanWins / float64(teamAStanding.GamesPlayed)) * 25.0
		}
		if teamBStanding.GamesPlayed > 0 {
			scoreB += (teamBStanding.PythagoreanWins / float64(teamBStanding.GamesPlayed)) * 25.0
		}
	}

	// Factor 4: Historical H2H
	totalH2H := winsA + winsB
	if totalH2H > 0 {
		scoreA += (float64(winsA) / float64(totalH2H)) * 15.0
		scoreB += (float64(winsB) / float64(totalH2H)) * 15.0
	}

	var headline, detail string
	diffScore := scoreA - scoreB

	if teamAStanding != nil && teamBStanding != nil {
		if diffScore > 8.0 {
			headline = fmt.Sprintf("Ventaja analítica para %s", teamAStanding.FullName())
			detail = fmt.Sprintf("%s supera a %s en eficiencia neta (PPG %.1f vs %.1f defensivo) y un diferencial de %+d puntos. Su expectativa pitagórica (%.1f victorias) respalda su rendimiento como favorito.",
				teamAStanding.TeamName, teamBStanding.TeamName, teamAStanding.OffensivePPG, teamBStanding.DefensivePPG, teamAStanding.PointDiff, teamAStanding.PythagoreanWins)
		} else if diffScore < -8.0 {
			headline = fmt.Sprintf("Ventaja analítica para %s", teamBStanding.FullName())
			detail = fmt.Sprintf("%s muestra un perfil superior frente a %s (PPG %.1f vs %.1f defensivo) con un diferencial de %+d puntos. Su modelo de victorias esperadas (%.1f victorias) los posiciona con ventaja técnica.",
				teamBStanding.TeamName, teamAStanding.TeamName, teamBStanding.OffensivePPG, teamAStanding.DefensivePPG, teamBStanding.PointDiff, teamBStanding.PythagoreanWins)
		} else {
			headline = "Duelo balanceado de pronóstico reservado"
			detail = fmt.Sprintf("Paridad técnica absoluta entre %s (%d-%d) y %s (%d-%d). La diferencia en métricas avanzadas es mínima (%+.1f vs %+.1f victorias pitagóricas). Se prevé un juego cerrado de una sola posesión.",
				teamAStanding.TeamName, teamAStanding.Wins, teamAStanding.Losses, teamBStanding.TeamName, teamBStanding.Wins, teamBStanding.Losses, teamAStanding.PythagoreanDiff, teamBStanding.PythagoreanDiff)
		}
	}

	return &db.TeamH2HComparison{
		TeamA:              teamAStanding,
		TeamB:              teamBStanding,
		HistoricalMatchups: matchups,
		TeamAWins:          winsA,
		TeamBWins:          winsB,
		Ties:               ties,
		CommunityAdvantage: commAdv,
		VerdictHeadline:    headline,
		VerdictDetail:      detail,
	}
}
