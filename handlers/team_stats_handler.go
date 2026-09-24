package handlers

import (
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

	h.renderer.RenderPage(w, "teams.html", map[string]interface{}{
		"ActiveNav":        "teams",
		"User":             user,
		"Standings":        standings,
		"SelectedSeason":   seasonYear,
		"SelectedView":     viewMode,
		"AvailableSeasons": defaultAvailableSeasons,
		"CurrentSeason":    h.seasonYear,
	})
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
