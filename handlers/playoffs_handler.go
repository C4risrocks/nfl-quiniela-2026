package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

type PlayoffsHandler struct {
	repo       *db.Repository
	renderer   *Renderer
	seasonYear int
}

func NewPlayoffsHandler(repo *db.Repository, renderer *Renderer, seasonYear int) *PlayoffsHandler {
	return &PlayoffsHandler{
		repo:       repo,
		renderer:   renderer,
		seasonYear: seasonYear,
	}
}

type BracketTeamInfo struct {
	Seed         int    `json:"seed"`
	TeamID       int64  `json:"team_id"`
	TeamCode     string `json:"team_code"`
	TeamName     string `json:"team_name"`
	TeamCity     string `json:"team_city"`
	LogoURL      string `json:"logo_url"`
	PrimaryColor string `json:"primary_color"`
	Record       string `json:"record"`
	Conference   string `json:"conference"`
	Division     string `json:"division"`
}

type BracketMatchupInfo struct {
	MatchupKey string           `json:"matchup_key"`
	RoundName  string           `json:"round_name"`
	Conference string           `json:"conference"`
	HighSeed   *BracketTeamInfo `json:"high_seed"`
	LowSeed    *BracketTeamInfo `json:"low_seed"`
	RealGame   *db.Game         `json:"real_game,omitempty"`
}

type PlayoffBracketViewData struct {
	SeasonYear       int
	AFCSeeds         []*BracketTeamInfo
	NFCSeeds         []*BracketTeamInfo
	WildCardMatchups []*BracketMatchupInfo
	UserBracket      *db.UserPlayoffBracket
	IsPostseason     bool
	ActiveWeekNum    int
}

func (h *PlayoffsHandler) buildBracketData(user *db.User) (*PlayoffBracketViewData, error) {
	standings, _ := h.repo.GetSeasonStandings(h.seasonYear)

	var afcSeeds, nfcSeeds []*BracketTeamInfo
	if standings != nil {
		for _, conf := range standings.Conferences {
			if conf.Conference == "AFC" {
				for _, s := range conf.Teams {
					rec := fmt.Sprintf("%d-%d", s.Wins, s.Losses)
					if s.Ties > 0 {
						rec = fmt.Sprintf("%d-%d-%d", s.Wins, s.Losses, s.Ties)
					}
					afcSeeds = append(afcSeeds, &BracketTeamInfo{
						Seed:         s.ConferenceSeed,
						TeamID:       s.TeamID,
						TeamCode:     s.TeamCode,
						TeamName:     s.TeamName,
						TeamCity:     s.TeamCity,
						LogoURL:      s.LogoURL,
						PrimaryColor: s.PrimaryColor,
						Record:       rec,
						Conference:   s.Conference,
						Division:     s.Division,
					})
					if len(afcSeeds) == 7 {
						break
					}
				}
			} else if conf.Conference == "NFC" {
				for _, s := range conf.Teams {
					rec := fmt.Sprintf("%d-%d", s.Wins, s.Losses)
					if s.Ties > 0 {
						rec = fmt.Sprintf("%d-%d-%d", s.Wins, s.Losses, s.Ties)
					}
					nfcSeeds = append(nfcSeeds, &BracketTeamInfo{
						Seed:         s.ConferenceSeed,
						TeamID:       s.TeamID,
						TeamCode:     s.TeamCode,
						TeamName:     s.TeamName,
						TeamCity:     s.TeamCity,
						LogoURL:      s.LogoURL,
						PrimaryColor: s.PrimaryColor,
						Record:       rec,
						Conference:   s.Conference,
						Division:     s.Division,
					})
					if len(nfcSeeds) == 7 {
						break
					}
				}
			}
		}
	}

	// Fallback mock seeds if database standings are unpopulated
	if len(afcSeeds) < 7 {
		afcFallback := []struct {
			seed int
			code string
			name string
			city string
			logo string
			rec  string
		}{
			{1, "KC", "Chiefs", "Kansas City", "https://a.espncdn.com/i/teamlogos/nfl/500/kc.png", "14-3"},
			{2, "BUF", "Bills", "Buffalo", "https://a.espncdn.com/i/teamlogos/nfl/500/buf.png", "13-4"},
			{3, "BAL", "Ravens", "Baltimore", "https://a.espncdn.com/i/teamlogos/nfl/500/bal.png", "12-5"},
			{4, "HOU", "Texans", "Houston", "https://a.espncdn.com/i/teamlogos/nfl/500/hou.png", "10-7"},
			{5, "LAC", "Chargers", "Los Angeles", "https://a.espncdn.com/i/teamlogos/nfl/500/lac.png", "11-6"},
			{6, "PIT", "Steelers", "Pittsburgh", "https://a.espncdn.com/i/teamlogos/nfl/500/pit.png", "10-7"},
			{7, "DEN", "Broncos", "Denver", "https://a.espncdn.com/i/teamlogos/nfl/500/den.png", "9-8"},
		}
		afcSeeds = nil
		for _, f := range afcFallback {
			afcSeeds = append(afcSeeds, &BracketTeamInfo{
				Seed:       f.seed,
				TeamCode:   f.code,
				TeamName:   f.name,
				TeamCity:   f.city,
				LogoURL:    f.logo,
				Record:     f.rec,
				Conference: "AFC",
			})
		}
	}

	if len(nfcSeeds) < 7 {
		nfcFallback := []struct {
			seed int
			code string
			name string
			city string
			logo string
			rec  string
		}{
			{1, "DET", "Lions", "Detroit", "https://a.espncdn.com/i/teamlogos/nfl/500/det.png", "15-2"},
			{2, "PHI", "Eagles", "Philadelphia", "https://a.espncdn.com/i/teamlogos/nfl/500/phi.png", "13-4"},
			{3, "TB", "Buccaneers", "Tampa Bay", "https://a.espncdn.com/i/teamlogos/nfl/500/tb.png", "10-7"},
			{4, "LAR", "Rams", "Los Angeles", "https://a.espncdn.com/i/teamlogos/nfl/500/lar.png", "10-7"},
			{5, "MIN", "Vikings", "Minnesota", "https://a.espncdn.com/i/teamlogos/nfl/500/min.png", "12-5"},
			{6, "WAS", "Commanders", "Washington", "https://a.espncdn.com/i/teamlogos/nfl/500/was.png", "11-6"},
			{7, "GB", "Packers", "Green Bay", "https://a.espncdn.com/i/teamlogos/nfl/500/gb.png", "10-7"},
		}
		nfcSeeds = nil
		for _, f := range nfcFallback {
			nfcSeeds = append(nfcSeeds, &BracketTeamInfo{
				Seed:       f.seed,
				TeamCode:   f.code,
				TeamName:   f.name,
				TeamCity:   f.city,
				LogoURL:    f.logo,
				Record:     f.rec,
				Conference: "NFC",
			})
		}
	}

	// Build official Wild Card initial matchups (2v7, 3v6, 4v5)
	var wcMatchups []*BracketMatchupInfo
	if len(afcSeeds) >= 7 {
		wcMatchups = append(wcMatchups, &BracketMatchupInfo{
			MatchupKey: "AFC_2v7",
			RoundName:  "Wild Card AFC",
			Conference: "AFC",
			HighSeed:   afcSeeds[1], // Seed 2
			LowSeed:    afcSeeds[6], // Seed 7
		})
		wcMatchups = append(wcMatchups, &BracketMatchupInfo{
			MatchupKey: "AFC_3v6",
			RoundName:  "Wild Card AFC",
			Conference: "AFC",
			HighSeed:   afcSeeds[2], // Seed 3
			LowSeed:    afcSeeds[5], // Seed 6
		})
		wcMatchups = append(wcMatchups, &BracketMatchupInfo{
			MatchupKey: "AFC_4v5",
			RoundName:  "Wild Card AFC",
			Conference: "AFC",
			HighSeed:   afcSeeds[3], // Seed 4
			LowSeed:    afcSeeds[4], // Seed 5
		})
	}

	if len(nfcSeeds) >= 7 {
		wcMatchups = append(wcMatchups, &BracketMatchupInfo{
			MatchupKey: "NFC_2v7",
			RoundName:  "Wild Card NFC",
			Conference: "NFC",
			HighSeed:   nfcSeeds[1], // Seed 2
			LowSeed:    nfcSeeds[6], // Seed 7
		})
		wcMatchups = append(wcMatchups, &BracketMatchupInfo{
			MatchupKey: "NFC_3v6",
			RoundName:  "Wild Card NFC",
			Conference: "NFC",
			HighSeed:   nfcSeeds[2], // Seed 3
			LowSeed:    nfcSeeds[5], // Seed 6
		})
		wcMatchups = append(wcMatchups, &BracketMatchupInfo{
			MatchupKey: "NFC_4v5",
			RoundName:  "Wild Card NFC",
			Conference: "NFC",
			HighSeed:   nfcSeeds[3], // Seed 4
			LowSeed:    nfcSeeds[4], // Seed 5
		})
	}

	var userBracket *db.UserPlayoffBracket
	if user != nil {
		userBracket, _ = h.repo.GetUserPlayoffBracket(user.ID, h.seasonYear)
	}

	activeWeekNum := 8
	season, err := h.repo.GetActiveSeason(h.seasonYear)
	if err == nil && season != nil {
		aw, _ := h.repo.GetActiveWeek(season.ID)
		if aw != nil {
			activeWeekNum = aw.WeekNumber
		}
	}

	return &PlayoffBracketViewData{
		SeasonYear:       h.seasonYear,
		AFCSeeds:         afcSeeds,
		NFCSeeds:         nfcSeeds,
		WildCardMatchups: wcMatchups,
		UserBracket:      userBracket,
		IsPostseason:     activeWeekNum >= 19,
		ActiveWeekNum:    activeWeekNum,
	}, nil
}

func (h *PlayoffsHandler) ShowPlayoffs(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	data, err := h.buildBracketData(user)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error building playoffs: %v", err), http.StatusInternalServerError)
		return
	}

	h.renderer.RenderPage(w, "playoffs.html", map[string]interface{}{
		"User":        user,
		"ActiveNav":   "playoffs",
		"BracketData": data,
	})
}

func (h *PlayoffsHandler) PlayoffsBracketPartial(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	data, err := h.buildBracketData(user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.renderer.RenderPartial(w, "playoffs_bracket.html", map[string]interface{}{
		"User":        user,
		"BracketData": data,
	})
}

func (h *PlayoffsHandler) SaveBracketPrediction(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Debes iniciar sesión para guardar tu bracket", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	wcPicks := r.FormValue("wild_card_picks")
	divPicks := r.FormValue("divisional_picks")
	confPicks := r.FormValue("conference_picks")
	sbPick := r.FormValue("super_bowl_pick")
	tiebreakerStr := r.FormValue("tiebreaker_points")
	tiebreakerPts, _ := strconv.Atoi(tiebreakerStr)

	// Fallback JSON normalization
	if wcPicks == "" {
		wcPicks = "{}"
	}
	if divPicks == "" {
		divPicks = "{}"
	}
	if confPicks == "" {
		confPicks = "{}"
	}
	if sbPick == "" {
		sbPick = "{}"
	}

	bracket := &db.UserPlayoffBracket{
		UserID:           user.ID,
		SeasonYear:       h.seasonYear,
		WildCardPicks:    wcPicks,
		DivisionalPicks:  divPicks,
		ConferencePicks:  confPicks,
		SuperBowlPick:    sbPick,
		TiebreakerPoints: tiebreakerPts,
	}

	if err := h.repo.SaveUserPlayoffBracket(bracket); err != nil {
		http.Error(w, fmt.Sprintf("Error saving bracket: %v", err), http.StatusInternalServerError)
		return
	}

	// Check if request expects JSON
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "¡Tu Bracket de Postemporada ha sido guardado exitosamente!",
		})
		return
	}

	// HTMX badge / toast feedback
	w.Header().Set("HX-Trigger", `{"showToast":{"message":"¡Tu Bracket de Postemporada ha sido guardado con éxito!","type":"success"}}`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<span class="inline-flex items-center text-xs font-semibold text-emerald-400 bg-emerald-500/10 border border-emerald-500/30 px-3 py-1.5 rounded-xl"><i class="fa-solid fa-check mr-1.5"></i> Bracket Guardado</span>`))
}
