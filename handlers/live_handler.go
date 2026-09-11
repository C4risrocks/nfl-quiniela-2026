package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"time"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

type LiveHandler struct {
	repo       *db.Repository
	renderer   *Renderer
	seasonYear int
}

func NewLiveHandler(repo *db.Repository, renderer *Renderer, seasonYear int) *LiveHandler {
	return &LiveHandler{
		repo:       repo,
		renderer:   renderer,
		seasonYear: seasonYear,
	}
}

type WhatIfGame struct {
	ID              int64  `json:"id"`
	AwayID          int64  `json:"away_id"`
	AwayCode        string `json:"away_code"`
	AwayName        string `json:"away_name"`
	AwayLogo        string `json:"away_logo"`
	HomeID          int64  `json:"home_id"`
	HomeCode        string `json:"home_code"`
	HomeName        string `json:"home_name"`
	HomeLogo        string `json:"home_logo"`
	Status          string `json:"status"`
	StatusDetail    string `json:"status_detail"`
	AwayScore       int    `json:"away_score"`
	HomeScore       int    `json:"home_score"`
	IsLocked        bool   `json:"is_locked"`
	UserPickedID    int64  `json:"user_picked_id"`
	ConsensusID     int64  `json:"consensus_id"`
	CurrentWinnerID int64  `json:"current_winner_id"`
	AwayPickPercent int    `json:"away_pick_pct"`
	HomePickPercent int    `json:"home_pick_pct"`
}

type WhatIfUser struct {
	UserID        int64           `json:"user_id"`
	Username      string          `json:"username"`
	AvatarURL     string          `json:"avatar_url"`
	IsCurrent     bool            `json:"is_current"`
	BasePoints    int             `json:"base_points"`
	CurrentPoints int             `json:"current_points"`
	Picks         map[int64]int64 `json:"picks"`
}

type WhatIfPayload struct {
	Games         []WhatIfGame `json:"games"`
	Users         []WhatIfUser `json:"users"`
	CurrentUserID int64        `json:"current_user_id"`
}

type LiveViewData struct {
	ActiveNav           string
	User                *db.User
	Season              *db.Season
	Weeks               []*db.Week
	SelectedWeek        *db.Week
	Games               []*db.Game
	FeaturedGame        *db.Game
	FeaturedStats       *db.GameCommunityStats
	FeaturedPicks       []*db.Pick
	LiveGamesCount      int
	FinalGamesCount     int
	UpcomingGamesCount  int
	UserConfirmedPoints int
	UserProvisionalPts  int
	CurrentTime         time.Time
	LockMode            string
	WhatIfJSON          template.JS
}

func (h *LiveHandler) buildLiveData(r *http.Request) (*LiveViewData, error) {
	currentUser := auth.GetUserFromContext(r.Context())
	now := time.Now()

	season, err := h.repo.GetActiveSeason(h.seasonYear)
	if err != nil {
		return nil, err
	}

	weeks, err := h.repo.ListWeeks(season.ID)
	if err != nil {
		return nil, err
	}

	// Select requested week or default to first active / scheduled week
	var selectedWeek *db.Week
	weekParam := r.URL.Query().Get("week")
	if weekParam != "" {
		if wNum, err := strconv.Atoi(weekParam); err == nil {
			for _, w := range weeks {
				if w.WeekNumber == wNum {
					selectedWeek = w
					break
				}
			}
		}
	}

	if selectedWeek == nil {
		for _, w := range weeks {
			if w.Status == "active" {
				selectedWeek = w
				break
			}
		}
	}
	if selectedWeek == nil && len(weeks) > 0 {
		selectedWeek = weeks[0]
	}

	scoringCfg, _ := h.repo.GetScoringConfig()
	lockMode := "per_game"
	if scoringCfg != nil {
		lockMode = scoringCfg.LockMode
	}

	var games []*db.Game
	if selectedWeek != nil {
		games, _ = h.repo.ListGamesByWeek(selectedWeek.ID)
	}

	// Load user picks if logged in
	userPicksMap := make(map[int64]*db.Pick)
	if currentUser != nil && selectedWeek != nil {
		picks, _ := h.repo.GetUserPicksForWeek(currentUser.ID, selectedWeek.ID)
		for _, p := range picks {
			userPicksMap[p.GameID] = p
		}
	}

	var liveCount, finalCount, upcomingCount int
	var confirmedPoints, provisionalPoints int

	for _, g := range games {
		if p, ok := userPicksMap[g.ID]; ok {
			g.UserPick = p
		}

		// Community pick counts
		if stats, err := h.repo.GetGameCommunityStats(g.ID); err == nil && stats != nil {
			g.HomePickCount = stats.HomePicksCount
			g.AwayPickCount = stats.AwayPicksCount
			g.TotalPicks = stats.TotalPicks
		}

		switch g.Status {
		case "in_progress":
			liveCount++
			if g.UserPick != nil && g.UserPick.IsProvisionalWinner(g) {
				provisionalPoints++
			}
		case "final":
			finalCount++
			if g.UserPick != nil && g.UserPick.HasCorrectWinner() {
				confirmedPoints += g.UserPick.TotalPoints()
			}
		default:
			upcomingCount++
		}
	}

	// Select featured game for Matchcast
	var featuredGame *db.Game
	// 1. First in_progress game
	for _, g := range games {
		if g.Status == "in_progress" {
			featuredGame = g
			break
		}
	}
	// 2. Otherwise, first tiebreaker game
	if featuredGame == nil {
		for _, g := range games {
			if g.IsTiebreaker {
				featuredGame = g
				break
			}
		}
	}
	// 3. Otherwise, upcoming game closest to current time
	if featuredGame == nil {
		for _, g := range games {
			if g.Status != "final" {
				featuredGame = g
				break
			}
		}
	}
	// 4. Default to first game
	if featuredGame == nil && len(games) > 0 {
		featuredGame = games[0]
	}

	var featuredStats *db.GameCommunityStats
	var featuredPicks []*db.Pick
	if featuredGame != nil {
		featuredStats, _ = h.repo.GetGameCommunityStats(featuredGame.ID)
		featuredPicks, _ = h.repo.ListPicksForGame(featuredGame.ID)
		// Sort picks alphabetically by username
		sort.Slice(featuredPicks, func(i, j int) bool {
			if featuredPicks[i].User == nil || featuredPicks[j].User == nil {
				return false
			}
			return featuredPicks[i].User.Username < featuredPicks[j].User.Username
		})
	}

	// ----------------------------------------------------
	// Build What-If Simulation Payload
	// ----------------------------------------------------
	var firstKickoff *time.Time
	for _, g := range games {
		if firstKickoff == nil || g.KickoffTime.Before(*firstKickoff) {
			t := g.KickoffTime
			firstKickoff = &t
		}
	}

	whatIfGames := make([]WhatIfGame, 0, len(games))
	gameLockedMap := make(map[int64]bool)
	gameFinalWinnerMap := make(map[int64]int64)
	gameLiveLeaderMap := make(map[int64]int64)

	for _, g := range games {
		isLocked := g.IsGameOrWeekLocked(now, lockMode, firstKickoff)
		gameLockedMap[g.ID] = isLocked

		var currentWinnerID int64
		var awayScoreVal, homeScoreVal int
		if g.AwayScore != nil {
			awayScoreVal = *g.AwayScore
		}
		if g.HomeScore != nil {
			homeScoreVal = *g.HomeScore
		}

		if g.Status == "final" {
			if awayScoreVal > homeScoreVal {
				currentWinnerID = g.AwayTeamID
				gameFinalWinnerMap[g.ID] = g.AwayTeamID
			} else if homeScoreVal > awayScoreVal {
				currentWinnerID = g.HomeTeamID
				gameFinalWinnerMap[g.ID] = g.HomeTeamID
			}
		} else if g.Status == "in_progress" {
			if awayScoreVal > homeScoreVal {
				currentWinnerID = g.AwayTeamID
				gameLiveLeaderMap[g.ID] = g.AwayTeamID
			} else if homeScoreVal > awayScoreVal {
				currentWinnerID = g.HomeTeamID
				gameLiveLeaderMap[g.ID] = g.HomeTeamID
			}
		}

		var consensusID int64 = g.HomeTeamID
		if g.AwayPickCount > g.HomePickCount {
			consensusID = g.AwayTeamID
		}

		var userPickedID int64
		if g.UserPick != nil && g.UserPick.PickedTeamID != nil {
			userPickedID = *g.UserPick.PickedTeamID
		}

		var awayCode, awayName, awayLogo string
		if g.AwayTeam != nil {
			awayCode = g.AwayTeam.Code
			awayName = g.AwayTeam.Name
			awayLogo = g.AwayTeam.LogoURL
		}
		var homeCode, homeName, homeLogo string
		if g.HomeTeam != nil {
			homeCode = g.HomeTeam.Code
			homeName = g.HomeTeam.Name
			homeLogo = g.HomeTeam.LogoURL
		}

		whatIfGames = append(whatIfGames, WhatIfGame{
			ID:              g.ID,
			AwayID:          g.AwayTeamID,
			AwayCode:        awayCode,
			AwayName:        awayName,
			AwayLogo:        awayLogo,
			HomeID:          g.HomeTeamID,
			HomeCode:        homeCode,
			HomeName:        homeName,
			HomeLogo:        homeLogo,
			Status:          g.Status,
			StatusDetail:    g.StatusDetail,
			AwayScore:       awayScoreVal,
			HomeScore:       homeScoreVal,
			IsLocked:        isLocked,
			UserPickedID:    userPickedID,
			ConsensusID:     consensusID,
			CurrentWinnerID: currentWinnerID,
			AwayPickPercent: g.AwayPickPercent(),
			HomePickPercent: g.HomePickPercent(),
		})
	}

	var currentUserID int64
	if currentUser != nil {
		currentUserID = currentUser.ID
	}

	var whatIfUsers []WhatIfUser
	if selectedWeek != nil {
		allUserPicks, _ := h.repo.GetAllUsersPicksForWeek(selectedWeek.ID)
		userSeen := make(map[int64]bool)

		for _, up := range allUserPicks {
			userSeen[up.UserID] = true
			isCurrent := up.UserID == currentUserID
			basePts := 0
			currentPts := 0
			filteredPicks := make(map[int64]int64)

			for gID, pickedTeamID := range up.Picks {
				if winTeam, ok := gameFinalWinnerMap[gID]; ok && winTeam == pickedTeamID {
					basePts++
					currentPts++
				}
				if leadTeam, ok := gameLiveLeaderMap[gID]; ok && leadTeam == pickedTeamID {
					currentPts++
				}
				if gameLockedMap[gID] || isCurrent {
					filteredPicks[gID] = pickedTeamID
				}
			}

			whatIfUsers = append(whatIfUsers, WhatIfUser{
				UserID:        up.UserID,
				Username:      up.Username,
				AvatarURL:     up.AvatarURL,
				IsCurrent:     isCurrent,
				BasePoints:    basePts,
				CurrentPoints: currentPts,
				Picks:         filteredPicks,
			})
		}

		if currentUser != nil && !userSeen[currentUser.ID] {
			whatIfUsers = append(whatIfUsers, WhatIfUser{
				UserID:        currentUser.ID,
				Username:      currentUser.Username,
				AvatarURL:     currentUser.AvatarURL,
				IsCurrent:     true,
				BasePoints:    0,
				CurrentPoints: 0,
				Picks:         make(map[int64]int64),
			})
		}
	}

	payload := WhatIfPayload{
		Games:         whatIfGames,
		Users:         whatIfUsers,
		CurrentUserID: currentUserID,
	}

	var whatIfJSON template.JS
	if jsonBytes, err := json.Marshal(payload); err == nil {
		whatIfJSON = template.JS(jsonBytes)
	}

	return &LiveViewData{
		ActiveNav:           "live",
		User:                currentUser,
		Season:              season,
		Weeks:               weeks,
		SelectedWeek:        selectedWeek,
		Games:               games,
		FeaturedGame:        featuredGame,
		FeaturedStats:       featuredStats,
		FeaturedPicks:       featuredPicks,
		LiveGamesCount:      liveCount,
		FinalGamesCount:     finalCount,
		UpcomingGamesCount:  upcomingCount,
		UserConfirmedPoints: confirmedPoints,
		UserProvisionalPts:  provisionalPoints,
		CurrentTime:         now,
		LockMode:            lockMode,
		WhatIfJSON:          whatIfJSON,
	}, nil
}

func (h *LiveHandler) ShowLive(w http.ResponseWriter, r *http.Request) {
	data, err := h.buildLiveData(r)
	if err != nil {
		http.Error(w, "Error cargando Game Center en vivo", http.StatusInternalServerError)
		return
	}

	h.renderer.RenderPage(w, "live.html", data)
}

func (h *LiveHandler) LiveContent(w http.ResponseWriter, r *http.Request) {
	data, err := h.buildLiveData(r)
	if err != nil {
		http.Error(w, "Error actualizando Game Center", http.StatusInternalServerError)
		return
	}

	h.renderer.RenderPartial(w, "live_content.html", data)
}
