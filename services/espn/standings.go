package espn

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nfl-quiniela-2026/db"
)

type standingsCacheEntry struct {
	data      *db.SeasonStandings
	expiresAt time.Time
}

type scheduleCacheEntry struct {
	items     []*db.TeamScheduleItem
	expiresAt time.Time
}

var (
	standingsCacheMu sync.Mutex
	standingsCache   = make(map[int]standingsCacheEntry)

	scheduleCacheMu sync.Mutex
	scheduleCache   = make(map[string]scheduleCacheEntry)
)

// ESPNStandingsRawResponse structures for decoding site.api.espn.com/apis/v2/sports/football/nfl/standings
type ESPNStandingsRawResponse struct {
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Season   struct {
		Year int `json:"year"`
	} `json:"season"`
	Children []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Abbreviation string `json:"abbreviation"`
		Standings    struct {
			Entries []struct {
				Team struct {
					ID           string `json:"id"`
					Abbreviation string `json:"abbreviation"`
					DisplayName  string `json:"displayName"`
					Location     string `json:"location"`
					Logos        []struct {
						Href string `json:"href"`
					} `json:"logos"`
				} `json:"team"`
				Stats []struct {
					Name         string  `json:"name"`
					DisplayName  string  `json:"displayName"`
					DisplayValue string  `json:"displayValue"`
					Value        float64 `json:"value"`
				} `json:"stats"`
			} `json:"entries"`
		} `json:"standings"`
	} `json:"children"`
}

// ESPNTeamScheduleRawResponse for decoding team schedules from ESPN
type ESPNTeamScheduleRawResponse struct {
	Team struct {
		ID           string `json:"id"`
		Abbreviation string `json:"abbreviation"`
		DisplayName  string `json:"displayName"`
	} `json:"team"`
	Events []struct {
		ID           string `json:"id"`
		Date         string `json:"date"`
		Name         string `json:"name"`
		ShortName    string `json:"shortName"`
		Competitions []struct {
			ID         string `json:"id"`
			Date       string `json:"date"`
			Attendance int    `json:"attendance"`
			Broadcast  string `json:"broadcast"`
			Status     struct {
				Type struct {
					Completed   bool   `json:"completed"`
					Description string `json:"description"`
					Detail      string `json:"detail"`
					ShortDetail string `json:"shortDetail"`
					State       string `json:"state"`
				} `json:"type"`
			} `json:"status"`
			Competitors []struct {
				ID       string `json:"id"`
				HomeAway string `json:"homeAway"`
				Score    struct {
					Value        float64 `json:"value"`
					DisplayValue string  `json:"displayValue"`
				} `json:"score"`
				Winner bool `json:"winner"`
				Team   struct {
					ID           string `json:"id"`
					Abbreviation string `json:"abbreviation"`
					DisplayName  string `json:"displayName"`
					Location     string `json:"location"`
					Logo         string `json:"logo"`
				} `json:"team"`
			} `json:"competitors"`
		} `json:"competitions"`
		Week struct {
			Number int `json:"number"`
		} `json:"week"`
	} `json:"events"`
}

// FetchNFLStandings retrieves official NFL standings from ESPN for a given year with caching
func (c *Client) FetchNFLStandings(year int, currentYear int, teamMap map[string]*db.Team) (*db.SeasonStandings, error) {
	standingsCacheMu.Lock()
	if entry, ok := standingsCache[year]; ok && time.Now().Before(entry.expiresAt) {
		standingsCacheMu.Unlock()
		return entry.data, nil
	}
	standingsCacheMu.Unlock()

	baseURL := c.standingsURL
	if baseURL == "" {
		baseURL = "https://site.api.espn.com/apis/v2/sports/football/nfl/standings"
	}
	url := fmt.Sprintf("%s?season=%d", baseURL, year)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating espn standings request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "NFL-Quiniela-2026/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching espn standings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("espn standings api returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading espn standings response: %w", err)
	}

	var raw ESPNStandingsRawResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing espn standings json: %w", err)
	}

	isCurrent := (year == currentYear)
	standings := &db.SeasonStandings{
		Year:        year,
		IsCurrent:   isCurrent,
		League:      make([]*db.TeamStanding, 0, 32),
		Conferences: make([]*db.ConferenceStandings, 0, 2),
		Divisions:   make([]*db.DivisionStandings, 0, 8),
	}

	var allTeams []*db.TeamStanding

	for _, confChild := range raw.Children {
		confName := confChild.Name
		confAbbr := "AFC"
		if strings.Contains(strings.ToUpper(confName), "NATIONAL") || strings.ToUpper(confChild.Abbreviation) == "NFC" {
			confAbbr = "NFC"
		}

		confGroup := &db.ConferenceStandings{
			Conference: confAbbr,
			Name:       confName,
			Teams:      make([]*db.TeamStanding, 0, 16),
		}

		for _, entry := range confChild.Standings.Entries {
			code := NormalizeTeamCode(entry.Team.Abbreviation)

			// Map stat attributes
			statsMap := make(map[string]string)
			for _, st := range entry.Stats {
				statsMap[st.Name] = st.DisplayValue
			}

			wins, _ := strconv.Atoi(statsMap["wins"])
			losses, _ := strconv.Atoi(statsMap["losses"])
			ties, _ := strconv.Atoi(statsMap["ties"])
			ptsFor, _ := strconv.Atoi(statsMap["pointsFor"])
			ptsAgainst, _ := strconv.Atoi(statsMap["pointsAgainst"])
			pointDiff, _ := strconv.Atoi(statsMap["differential"])
			if pointDiff == 0 {
				pointDiff, _ = strconv.Atoi(statsMap["pointDifferential"])
			}

			winPct := 0.0
			if wpStr := statsMap["winPercent"]; wpStr != "" {
				if wp, err := strconv.ParseFloat(wpStr, 64); err == nil {
					winPct = wp
				}
			} else if (wins + losses + ties) > 0 {
				winPct = float64(wins) / float64(wins+losses+ties)
			}

			seed, _ := strconv.Atoi(statsMap["playoffSeed"])
			gamesPlayed := wins + losses + ties
			var offPPG, defPPG float64
			if gamesPlayed > 0 {
				offPPG = float64(ptsFor) / float64(gamesPlayed)
				defPPG = float64(ptsAgainst) / float64(gamesPlayed)
			}

			streak := statsMap["streak"]
			if streak == "" {
				streak = "-"
			}

			homeRec := statsMap["Home"]
			if homeRec == "" {
				homeRec = "0-0"
			}
			awayRec := statsMap["Road"]
			if awayRec == "" {
				awayRec = "0-0"
			}
			divRec := statsMap["divisionRecord"]
			if divRec == "" {
				divRec = statsMap["vs. Div."]
			}
			confRec := statsMap["conferenceRecord"]
			if confRec == "" {
				confRec = statsMap["vs. Conf."]
			}
			gb := statsMap["gamesBehind"]
			if gb == "" {
				gb = "-"
			}

			logoURL := ""
			if len(entry.Team.Logos) > 0 {
				logoURL = entry.Team.Logos[0].Href
			}

			teamCity := entry.Team.Location
			teamName := entry.Team.DisplayName
			if teamCity == "" {
				teamCity = entry.Team.DisplayName
			}

			primaryColor := "#000000"
			secondaryColor := "#FFFFFF"
			teamDivision := ""
			var teamID int64

			if localTeam, exists := teamMap[code]; exists && localTeam != nil {
				teamID = localTeam.ID
				teamCity = localTeam.City
				teamName = localTeam.Name
				logoURL = localTeam.LogoURL
				primaryColor = localTeam.PrimaryColor
				secondaryColor = localTeam.SecondaryColor
				teamDivision = localTeam.Division
				confAbbr = localTeam.Conference
			}

			ts := &db.TeamStanding{
				TeamID:              teamID,
				TeamCode:            code,
				TeamName:            teamName,
				TeamCity:            teamCity,
				LogoURL:             logoURL,
				PrimaryColor:        primaryColor,
				SecondaryColor:      secondaryColor,
				Conference:          confAbbr,
				Division:            teamDivision,
				ConferenceSeed:      seed,
				Wins:                wins,
				Losses:              losses,
				Ties:                ties,
				WinPercent:          winPct,
				WinPercentFormatted: fmt.Sprintf("%.3f", winPct),
				GamesPlayed:         gamesPlayed,
				PointsFor:           ptsFor,
				PointsAgainst:       ptsAgainst,
				PointDiff:           pointDiff,
				OffensivePPG:        offPPG,
				DefensivePPG:        defPPG,
				Streak:              streak,
				HomeRecord:          homeRec,
				AwayRecord:          awayRec,
				DivisionRecord:      divRec,
				ConfRecord:          confRec,
				GamesBehind:         gb,
			}

			confGroup.Teams = append(confGroup.Teams, ts)
			allTeams = append(allTeams, ts)
		}

		// Sort conference teams by seed
		sort.Slice(confGroup.Teams, func(i, j int) bool {
			if confGroup.Teams[i].ConferenceSeed != confGroup.Teams[j].ConferenceSeed {
				if confGroup.Teams[i].ConferenceSeed == 0 {
					return false
				}
				if confGroup.Teams[j].ConferenceSeed == 0 {
					return true
				}
				return confGroup.Teams[i].ConferenceSeed < confGroup.Teams[j].ConferenceSeed
			}
			if confGroup.Teams[i].WinPercent != confGroup.Teams[j].WinPercent {
				return confGroup.Teams[i].WinPercent > confGroup.Teams[j].WinPercent
			}
			return confGroup.Teams[i].PointDiff > confGroup.Teams[j].PointDiff
		})

		standings.Conferences = append(standings.Conferences, confGroup)
	}

	// Group into 8 NFL divisions
	divisionsList := []struct {
		conf string
		div  string
		name string
	}{
		{"AFC", "East", "AFC Este"},
		{"AFC", "North", "AFC Norte"},
		{"AFC", "South", "AFC Sur"},
		{"AFC", "West", "AFC Oeste"},
		{"NFC", "East", "NFC Este"},
		{"NFC", "North", "NFC Norte"},
		{"NFC", "South", "NFC Sur"},
		{"NFC", "West", "NFC Oeste"},
	}

	for _, dInfo := range divisionsList {
		divGroup := &db.DivisionStandings{
			Name:       dInfo.name,
			Conference: dInfo.conf,
			Division:   dInfo.div,
			Teams:      make([]*db.TeamStanding, 0, 4),
		}

		for _, t := range allTeams {
			if strings.EqualFold(t.Conference, dInfo.conf) && strings.EqualFold(t.Division, dInfo.div) {
				divGroup.Teams = append(divGroup.Teams, t)
			}
		}

		// Sort division teams by record and point diff
		sort.Slice(divGroup.Teams, func(i, j int) bool {
			if divGroup.Teams[i].WinPercent != divGroup.Teams[j].WinPercent {
				return divGroup.Teams[i].WinPercent > divGroup.Teams[j].WinPercent
			}
			if divGroup.Teams[i].Wins != divGroup.Teams[j].Wins {
				return divGroup.Teams[i].Wins > divGroup.Teams[j].Wins
			}
			return divGroup.Teams[i].PointDiff > divGroup.Teams[j].PointDiff
		})

		// Assign ranks 1 to 4 within division
		for rIdx, dt := range divGroup.Teams {
			dt.Rank = rIdx + 1
		}

		standings.Divisions = append(standings.Divisions, divGroup)
	}

	// Sort whole league (all 32 teams)
	sort.Slice(allTeams, func(i, j int) bool {
		if allTeams[i].WinPercent != allTeams[j].WinPercent {
			return allTeams[i].WinPercent > allTeams[j].WinPercent
		}
		if allTeams[i].Wins != allTeams[j].Wins {
			return allTeams[i].Wins > allTeams[j].Wins
		}
		return allTeams[i].PointDiff > allTeams[j].PointDiff
	})
	standings.League = allTeams

	// Build Dashboard Summary
	standings.Summary = buildDashboardSummary(allTeams)

	// Cache result
	ttl := 24 * time.Hour
	if isCurrent {
		ttl = 5 * time.Minute
	}
	standingsCacheMu.Lock()
	standingsCache[year] = standingsCacheEntry{
		data:      standings,
		expiresAt: time.Now().Add(ttl),
	}
	standingsCacheMu.Unlock()

	return standings, nil
}

func buildDashboardSummary(teams []*db.TeamStanding) *db.SeasonDashboardSummary {
	if len(teams) == 0 {
		return nil
	}
	summary := &db.SeasonDashboardSummary{
		TopRecordTeam:  teams[0],
		TopOffenseTeam: teams[0],
		TopDefenseTeam: teams[0],
		BestStreakTeam: teams[0],
	}

	maxPF := -1
	minPA := 99999
	bestStreakWins := -1

	for _, t := range teams {
		if t.PointsFor > maxPF {
			maxPF = t.PointsFor
			summary.TopOffenseTeam = t
		}
		if t.GamesPlayed > 0 && t.PointsAgainst < minPA {
			minPA = t.PointsAgainst
			summary.TopDefenseTeam = t
		}
		if strings.HasPrefix(t.Streak, "W") {
			wCount, _ := strconv.Atoi(strings.TrimPrefix(t.Streak, "W"))
			if wCount > bestStreakWins {
				bestStreakWins = wCount
				summary.BestStreakTeam = t
			}
		}
	}

	return summary
}

// FetchTeamSchedule retrieves schedule and match results for a team from ESPN with caching
func (c *Client) FetchTeamSchedule(teamCode string, year int, teamMap map[string]*db.Team) ([]*db.TeamScheduleItem, error) {
	normCode := NormalizeTeamCode(teamCode)
	cacheKey := fmt.Sprintf("%s_%d", normCode, year)

	scheduleCacheMu.Lock()
	if entry, ok := scheduleCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		scheduleCacheMu.Unlock()
		return entry.items, nil
	}
	scheduleCacheMu.Unlock()

	baseURL := c.scheduleURL
	if baseURL == "" {
		baseURL = "https://site.api.espn.com/apis/site/v2/sports/football/nfl/teams/%s/schedule"
	}
	var url string
	if strings.Contains(baseURL, "%s") {
		url = fmt.Sprintf(baseURL+"?season=%d", strings.ToLower(normCode), year)
	} else {
		url = fmt.Sprintf("%s?team=%s&season=%d", baseURL, strings.ToLower(normCode), year)
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating schedule request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "NFL-Quiniela-2026/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching team schedule: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("espn schedule api returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading espn schedule response: %w", err)
	}

	var raw ESPNTeamScheduleRawResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing espn schedule json: %w", err)
	}

	items := make([]*db.TeamScheduleItem, 0, len(raw.Events))

	for _, ev := range raw.Events {
		if len(ev.Competitions) == 0 || len(ev.Competitions[0].Competitors) < 2 {
			continue
		}
		comp := ev.Competitions[0]
		var myComp, oppComp *struct {
			ID       string `json:"id"`
			HomeAway string `json:"homeAway"`
			Score    struct {
				Value        float64 `json:"value"`
				DisplayValue string  `json:"displayValue"`
			} `json:"score"`
			Winner bool `json:"winner"`
			Team   struct {
				ID           string `json:"id"`
				Abbreviation string `json:"abbreviation"`
				DisplayName  string `json:"displayName"`
				Location     string `json:"location"`
				Logo         string `json:"logo"`
			} `json:"team"`
		}

		for i := range comp.Competitors {
			cObj := &comp.Competitors[i]
			if NormalizeTeamCode(cObj.Team.Abbreviation) == normCode {
				myComp = cObj
			} else {
				oppComp = cObj
			}
		}

		if myComp == nil || oppComp == nil {
			continue
		}

		isHome := (myComp.HomeAway == "home")
		oppCode := NormalizeTeamCode(oppComp.Team.Abbreviation)
		oppName := oppComp.Team.DisplayName
		oppCity := oppComp.Team.Location
		oppLogo := oppComp.Team.Logo

		if locOpp, ok := teamMap[oppCode]; ok && locOpp != nil {
			oppName = locOpp.Name
			oppCity = locOpp.City
			oppLogo = locOpp.LogoURL
		}

		kickoff, err := ParseKickoffTime(ev.Date)
		if err != nil {
			kickoff = time.Now()
		}

		var homeScore, awayScore, teamScore, oppScore *int
		result := "scheduled"

		if myComp.Score.DisplayValue != "" {
			if s, err := strconv.Atoi(myComp.Score.DisplayValue); err == nil {
				teamScore = &s
			}
		}
		if oppComp.Score.DisplayValue != "" {
			if s, err := strconv.Atoi(oppComp.Score.DisplayValue); err == nil {
				oppScore = &s
			}
		}

		if isHome {
			homeScore = teamScore
			awayScore = oppScore
		} else {
			homeScore = oppScore
			awayScore = teamScore
		}

		state := strings.ToLower(comp.Status.Type.State)
		if comp.Status.Type.Completed || state == "post" {
			if teamScore != nil && oppScore != nil {
				if *teamScore > *oppScore {
					result = "W"
				} else if *teamScore < *oppScore {
					result = "L"
				} else {
					result = "T"
				}
			}
		} else if state == "in" {
			result = "in_progress"
		}

		broadcast := comp.Broadcast
		statusDetail := comp.Status.Type.ShortDetail
		if statusDetail == "" {
			statusDetail = comp.Status.Type.Detail
		}

		weekNum := ev.Week.Number
		if weekNum == 0 && len(items) > 0 {
			weekNum = len(items) + 1
		} else if weekNum == 0 {
			weekNum = 1
		}

		items = append(items, &db.TeamScheduleItem{
			WeekNumber:       weekNum,
			KickoffTime:      kickoff,
			KickoffFormatted: kickoff.Format("02/01 15:04"),
			OpponentCode:     oppCode,
			OpponentName:     oppName,
			OpponentCity:     oppCity,
			OpponentLogo:     oppLogo,
			IsHome:           isHome,
			HomeScore:        homeScore,
			AwayScore:        awayScore,
			TeamScore:        teamScore,
			OpponentScore:    oppScore,
			Result:           result,
			StatusDetail:     statusDetail,
			Broadcast:        broadcast,
		})
	}

	scheduleCacheMu.Lock()
	scheduleCache[cacheKey] = scheduleCacheEntry{
		items:     items,
		expiresAt: time.Now().Add(6 * time.Hour),
	}
	scheduleCacheMu.Unlock()

	return items, nil
}
