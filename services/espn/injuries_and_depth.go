package espn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nfl-quiniela-2026/db"
)

// TeamESPNIDMap maps standard NFL team codes to ESPN team IDs
var TeamESPNIDMap = map[string]string{
	"ARI": "22",
	"ATL": "1",
	"BAL": "33",
	"BUF": "2",
	"CAR": "29",
	"CHI": "3",
	"CIN": "4",
	"CLE": "5",
	"DAL": "6",
	"DEN": "7",
	"DET": "8",
	"GB":  "9",
	"HOU": "34",
	"IND": "11",
	"JAX": "30",
	"KC":  "12",
	"LV":  "13",
	"LAC": "24",
	"LAR": "14",
	"MIA": "15",
	"MIN": "16",
	"NE":  "17",
	"NO":  "18",
	"NYG": "19",
	"NYJ": "20",
	"PHI": "21",
	"PIT": "23",
	"SF":  "25",
	"SEA": "26",
	"TB":  "27",
	"TEN": "10",
	"WSH": "28",
}

// ESPNIDToTeamCode maps ESPN team ID to standard team code
var ESPNIDToTeamCode = map[string]string{
	"22": "ARI",
	"1":  "ATL",
	"33": "BAL",
	"2":  "BUF",
	"29": "CAR",
	"3":  "CHI",
	"4":  "CIN",
	"5":  "CLE",
	"6":  "DAL",
	"7":  "DEN",
	"8":  "DET",
	"9":  "GB",
	"34": "HOU",
	"11": "IND",
	"30": "JAX",
	"12": "KC",
	"13": "LV",
	"24": "LAC",
	"14": "LAR",
	"15": "MIA",
	"16": "MIN",
	"17": "NE",
	"18": "NO",
	"19": "NYG",
	"20": "NYJ",
	"21": "PHI",
	"23": "PIT",
	"25": "SF",
	"26": "SEA",
	"27": "TB",
	"10": "TEN",
	"28": "WSH",
}

// TeamNameToCode maps ESPN team display names to standard codes
var TeamNameToCode = map[string]string{
	"Arizona Cardinals":     "ARI",
	"Atlanta Falcons":       "ATL",
	"Baltimore Ravens":      "BAL",
	"Buffalo Bills":         "BUF",
	"Carolina Panthers":     "CAR",
	"Chicago Bears":         "CHI",
	"Cincinnati Bengals":    "CIN",
	"Cleveland Browns":      "CLE",
	"Dallas Cowboys":        "DAL",
	"Denver Broncos":        "DEN",
	"Detroit Lions":         "DET",
	"Green Bay Packers":     "GB",
	"Houston Texans":        "HOU",
	"Indianapolis Colts":    "IND",
	"Jacksonville Jaguars":  "JAX",
	"Kansas City Chiefs":    "KC",
	"Las Vegas Raiders":     "LV",
	"Los Angeles Chargers":  "LAC",
	"Los Angeles Rams":      "LAR",
	"Miami Dolphins":        "MIA",
	"Minnesota Vikings":     "MIN",
	"New England Patriots":  "NE",
	"New Orleans Saints":    "NO",
	"New York Giants":       "NYG",
	"New York Jets":         "NYJ",
	"Philadelphia Eagles":   "PHI",
	"Pittsburgh Steelers":   "PIT",
	"San Francisco 49ers":   "SF",
	"Seattle Seahawks":      "SEA",
	"Tampa Bay Buccaneers":  "TB",
	"Tennessee Titans":      "TEN",
	"Washington Commanders": "WSH",
}

// GetESPNTeamID returns the ESPN ID for a team code
func GetESPNTeamID(teamCode string) string {
	norm := NormalizeTeamCode(teamCode)
	if id, ok := TeamESPNIDMap[norm]; ok {
		return id
	}
	return ""
}

// ESPN API Raw Types for Injuries
type espnInjuriesResponse struct {
	Timestamp string `json:"timestamp"`
	Status    string `json:"status"`
	Season    struct {
		Year int `json:"year"`
	} `json:"season"`
	Injuries []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Injuries    []struct {
			ID           string `json:"id"`
			Status       string `json:"status"` // "Out", "Questionable", "Doubtful", "Injured Reserve"
			ShortComment string `json:"shortComment"`
			LongComment  string `json:"longComment"`
			Date         string `json:"date"`
			Athlete      struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
				Jersey      string `json:"jersey"`
				Position    struct {
					Abbreviation string `json:"abbreviation"`
					DisplayName  string `json:"displayName"`
				} `json:"position"`
				Headshot struct {
					Href string `json:"href"`
				} `json:"headshot"`
			} `json:"athlete"`
		} `json:"injuries"`
	} `json:"injuries"`
}

// ESPN API Raw Types for Depth Charts
type espnDepthChartResponse struct {
	Timestamp  string `json:"timestamp"`
	DepthChart []struct {
		ID        string `json:"id"`
		Name      string `json:"name"` // "3WR 1TE", "Base 4-3 D", "Special Teams"
		Positions map[string]struct {
			Position struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				DisplayName  string `json:"displayName"`
				Abbreviation string `json:"abbreviation"`
			} `json:"position"`
			Athletes []struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
				ShortName   string `json:"shortName"`
				Jersey      string `json:"jersey"`
				Rank        int    `json:"rank"`
			} `json:"athletes"`
		} `json:"positions"`
	} `json:"depthchart"`
}

// FetchAllNFLInjuries retrieves the injury report for all 32 NFL teams from ESPN
func (c *Client) FetchAllNFLInjuries() (map[string][]*db.TeamInjury, error) {
	url := "https://site.api.espn.com/apis/site/v2/sports/football/nfl/injuries"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return c.getFallbackAllInjuries(), nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return c.getFallbackAllInjuries(), nil
	}
	defer resp.Body.Close()

	var raw espnInjuriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return c.getFallbackAllInjuries(), nil
	}

	result := make(map[string][]*db.TeamInjury)

	for _, teamInj := range raw.Injuries {
		code := ""
		if c, ok := ESPNIDToTeamCode[teamInj.ID]; ok {
			code = c
		} else if c, ok := TeamNameToCode[teamInj.DisplayName]; ok {
			code = c
		}
		if code == "" {
			continue
		}

		var list []*db.TeamInjury
		for _, item := range teamInj.Injuries {
			name := item.Athlete.DisplayName
			if name == "" {
				continue
			}

			status := item.Status
			if status == "" {
				status = "Questionable"
			}

			headshot := item.Athlete.Headshot.Href
			if headshot == "" && item.Athlete.ID != "" {
				headshot = fmt.Sprintf("https://a.espncdn.com/i/headshots/nfl/players/full/%s.png", item.Athlete.ID)
			}

			pos := item.Athlete.Position.Abbreviation
			if pos == "" {
				pos = item.Athlete.Position.DisplayName
			}

			comment := item.ShortComment
			if comment == "" {
				comment = item.LongComment
			}

			list = append(list, &db.TeamInjury{
				TeamCode:      code,
				AthleteESPNID: item.Athlete.ID,
				AthleteName:   name,
				Position:      pos,
				Jersey:        item.Athlete.Jersey,
				HeadshotURL:   headshot,
				Status:        status,
				Comment:       comment,
				InjuryDate:    item.Date,
			})
		}

		if len(list) > 0 {
			result[code] = list
		}
	}

	if len(result) == 0 {
		return c.getFallbackAllInjuries(), nil
	}

	return result, nil
}

// FetchTeamDepthChart retrieves official depth chart slots for a specific team code
func (c *Client) FetchTeamDepthChart(teamCode string) ([]*db.TeamDepthChartSlot, error) {
	normCode := NormalizeTeamCode(teamCode)
	espnID := GetESPNTeamID(normCode)
	if espnID == "" {
		return c.getFallbackDepthChart(normCode), nil
	}

	url := fmt.Sprintf("https://site.api.espn.com/apis/site/v2/sports/football/nfl/teams/%s/depthcharts", espnID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return c.getFallbackDepthChart(normCode), nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return c.getFallbackDepthChart(normCode), nil
	}
	defer resp.Body.Close()

	var raw espnDepthChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return c.getFallbackDepthChart(normCode), nil
	}

	var slots []*db.TeamDepthChartSlot

	for _, group := range raw.DepthChart {
		groupName := "Ofensiva"
		gLower := strings.ToLower(group.Name)
		if strings.Contains(gLower, "special") {
			groupName = "Equipos Especiales"
		} else if strings.Contains(gLower, "def") || strings.Contains(gLower, "4-3") || strings.Contains(gLower, "3-4") || strings.Contains(gLower, "nickel") {
			groupName = "Defensiva"
		}

		for _, posData := range group.Positions {
			pCode := posData.Position.Abbreviation
			if pCode == "" {
				pCode = posData.Position.Name
			}
			pName := posData.Position.DisplayName
			if pName == "" {
				pName = posData.Position.Name
			}

			for idx, ath := range posData.Athletes {
				rank := ath.Rank
				if rank <= 0 {
					rank = idx + 1
				}

				headshot := ""
				if ath.ID != "" {
					headshot = fmt.Sprintf("https://a.espncdn.com/i/headshots/nfl/players/full/%s.png", ath.ID)
				}

				slots = append(slots, &db.TeamDepthChartSlot{
					TeamCode:       normCode,
					FormationGroup: groupName,
					PositionCode:   pCode,
					PositionName:   pName,
					DepthRank:      rank,
					AthleteESPNID:  ath.ID,
					AthleteName:    ath.DisplayName,
					Jersey:         ath.Jersey,
					HeadshotURL:    headshot,
				})
			}
		}
	}

	if len(slots) == 0 {
		return c.getFallbackDepthChart(normCode), nil
	}

	return slots, nil
}

// GroupDepthChartByFormation transforms flat depth slots into hierarchical formations with Starters and Backups
func GroupDepthChartByFormation(slots []*db.TeamDepthChartSlot) []*db.TeamDepthChartFormation {
	if len(slots) == 0 {
		return nil
	}

	// Map: FormationGroup -> PositionCode -> []*TeamDepthChartSlot
	groupOrder := []string{"Ofensiva", "Defensiva", "Equipos Especiales"}
	grouped := make(map[string]map[string][]*db.TeamDepthChartSlot)
	posNameMap := make(map[string]string)

	for _, g := range groupOrder {
		grouped[g] = make(map[string][]*db.TeamDepthChartSlot)
	}

	for _, s := range slots {
		g := s.FormationGroup
		if _, ok := grouped[g]; !ok {
			grouped[g] = make(map[string][]*db.TeamDepthChartSlot)
		}
		grouped[g][s.PositionCode] = append(grouped[g][s.PositionCode], s)
		posNameMap[s.PositionCode] = s.PositionName
	}

	// Preferred position order for clean NFL display
	offenseOrder := []string{"QB", "RB", "FB", "WR", "TE", "LT", "LG", "C", "RG", "RT"}
	defenseOrder := []string{"LDE", "LDT", "NT", "RDT", "RDE", "WLB", "MLB", "SLB", "LCB", "SS", "FS", "RCB", "NB"}
	specialOrder := []string{"PK", "P", "H", "PR", "KR", "LS"}

	var formations []*db.TeamDepthChartFormation

	for _, gName := range groupOrder {
		posMap := grouped[gName]
		if len(posMap) == 0 {
			continue
		}

		var preferred []string
		switch gName {
		case "Ofensiva":
			preferred = offenseOrder
		case "Defensiva":
			preferred = defenseOrder
		case "Equipos Especiales":
			preferred = specialOrder
		}

		var orderedPositions []*db.TeamDepthChartPosition
		seen := make(map[string]bool)

		// Add preferred order first
		for _, pCode := range preferred {
			if sList, ok := posMap[pCode]; ok && len(sList) > 0 {
				seen[pCode] = true
				pos := buildPosition(pCode, posNameMap[pCode], sList)
				orderedPositions = append(orderedPositions, pos)
			}
		}

		// Add remaining positions
		for pCode, sList := range posMap {
			if !seen[pCode] && len(sList) > 0 {
				pos := buildPosition(pCode, posNameMap[pCode], sList)
				orderedPositions = append(orderedPositions, pos)
			}
		}

		formations = append(formations, &db.TeamDepthChartFormation{
			GroupName: gName,
			Positions: orderedPositions,
		})
	}

	return formations
}

func buildPosition(pCode, pName string, sList []*db.TeamDepthChartSlot) *db.TeamDepthChartPosition {
	pos := &db.TeamDepthChartPosition{
		PositionCode: pCode,
		PositionName: pName,
	}
	for _, slot := range sList {
		if slot.DepthRank == 1 {
			pos.Starters = append(pos.Starters, slot)
		} else {
			pos.Backups = append(pos.Backups, slot)
		}
	}
	return pos
}

// Fallback Embedded Data (Resilience when offline or network fails)
func (c *Client) getFallbackAllInjuries() map[string][]*db.TeamInjury {
	nowStr := time.Now().UTC().Format(time.RFC3339)
	return map[string][]*db.TeamInjury{
		"KC": {
			{
				TeamCode:      "KC",
				AthleteESPNID: "4241474",
				AthleteName:   "Rashee Rice",
				Position:      "WR",
				Jersey:        "4",
				HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/4241474.png",
				Status:        "Injured Reserve",
				Comment:       "Cirugía de rodilla tras choque en semana 4, baja prolongada.",
				InjuryDate:    nowStr,
			},
			{
				TeamCode:      "KC",
				AthleteESPNID: "3051390",
				AthleteName:   "Marquise Brown",
				Position:      "WR",
				Jersey:        "5",
				HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/3051390.png",
				Status:        "Injured Reserve",
				Comment:       "Lesión en la articulación esternoclavicular.",
				InjuryDate:    nowStr,
			},
			{
				TeamCode:      "KC",
				AthleteESPNID: "4039014",
				AthleteName:   "Clyde Edwards-Helaire",
				Position:      "RB",
				Jersey:        "25",
				HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/4039014.png",
				Status:        "Questionable",
				Comment:       "Enfermedad / Lista de no lesiones de fútbol americano.",
				InjuryDate:    nowStr,
			},
		},
		"SF": {
			{
				TeamCode:      "SF",
				AthleteESPNID: "4361529",
				AthleteName:   "Christian McCaffrey",
				Position:      "RB",
				Jersey:        "23",
				HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/4361529.png",
				Status:        "Injured Reserve",
				Comment:       "Tendinitis bilateral en el tendón de Aquiles.",
				InjuryDate:    nowStr,
			},
			{
				TeamCode:      "SF",
				AthleteESPNID: "3122976",
				AthleteName:   "Deebo Samuel",
				Position:      "WR",
				Jersey:        "1",
				HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/3122976.png",
				Status:        "Questionable",
				Comment:       "Distensión en la pantorrilla, participación limitada.",
				InjuryDate:    nowStr,
			},
		},
		"BAL": {
			{
				TeamCode:      "BAL",
				AthleteESPNID: "4240763",
				AthleteName:   "Isaiah Likely",
				Position:      "TE",
				Jersey:        "80",
				HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/4240763.png",
				Status:        "Questionable",
				Comment:       "Molestia en el tobillo.",
				InjuryDate:    nowStr,
			},
		},
		"DAL": {
			{
				TeamCode:      "DAL",
				AthleteESPNID: "4362628",
				AthleteName:   "Micah Parsons",
				Position:      "LB",
				Jersey:        "11",
				HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/4362628.png",
				Status:        "Questionable",
				Comment:       "Esguince de tobillo alto, día a día.",
				InjuryDate:    nowStr,
			},
			{
				TeamCode:      "DAL",
				AthleteESPNID: "3051389",
				AthleteName:   "DeMarcus Lawrence",
				Position:      "DE",
				Jersey:        "90",
				HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/3051389.png",
				Status:        "Injured Reserve",
				Comment:       "Lesión en el pie, fuera 4-8 semanas.",
				InjuryDate:    nowStr,
			},
		},
		"MIA": {
			{
				TeamCode:      "MIA",
				AthleteESPNID: "4241479",
				AthleteName:   "Tua Tagovailoa",
				Position:      "QB",
				Jersey:        "1",
				HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/4241479.png",
				Status:        "Injured Reserve",
				Comment:       "Protocolo de conmoción cerebral.",
				InjuryDate:    nowStr,
			},
		},
	}
}

func (c *Client) getFallbackDepthChart(teamCode string) []*db.TeamDepthChartSlot {
	switch teamCode {
	case "KC":
		return []*db.TeamDepthChartSlot{
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "QB", PositionName: "Quarterback", DepthRank: 1, AthleteName: "Patrick Mahomes", Jersey: "15", AthleteESPNID: "3139477", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3139477.png"},
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "QB", PositionName: "Quarterback", DepthRank: 2, AthleteName: "Carson Wentz", Jersey: "11", AthleteESPNID: "2573079", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/2573079.png"},
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "RB", PositionName: "Running Back", DepthRank: 1, AthleteName: "Kareem Hunt", Jersey: "29", AthleteESPNID: "3051890", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3051890.png"},
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "RB", PositionName: "Running Back", DepthRank: 2, AthleteName: "Samaje Perine", Jersey: "34", AthleteESPNID: "3116385", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3116385.png"},
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "WR", PositionName: "Wide Receiver", DepthRank: 1, AthleteName: "Xavier Worthy", Jersey: "1", AthleteESPNID: "4685382", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4685382.png"},
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "WR", PositionName: "Wide Receiver", DepthRank: 2, AthleteName: "JuJu Smith-Schuster", Jersey: "9", AthleteESPNID: "3120348", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3120348.png"},
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "TE", PositionName: "Tight End", DepthRank: 1, AthleteName: "Travis Kelce", Jersey: "87", AthleteESPNID: "15847", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/15847.png"},
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "TE", PositionName: "Tight End", DepthRank: 2, AthleteName: "Noah Gray", Jersey: "83", AthleteESPNID: "4241470", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4241470.png"},
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "LT", PositionName: "Left Tackle", DepthRank: 1, AthleteName: "Kingsley Suamataia", Jersey: "76", AthleteESPNID: "4430737"},
			{TeamCode: "KC", FormationGroup: "Ofensiva", PositionCode: "C", PositionName: "Center", DepthRank: 1, AthleteName: "Creed Humphrey", Jersey: "52", AthleteESPNID: "4241475"},
			{TeamCode: "KC", FormationGroup: "Defensiva", PositionCode: "LDE", PositionName: "Left Defensive End", DepthRank: 1, AthleteName: "George Karlaftis", Jersey: "56", AthleteESPNID: "4429013"},
			{TeamCode: "KC", FormationGroup: "Defensiva", PositionCode: "LDT", PositionName: "Left Defensive Tackle", DepthRank: 1, AthleteName: "Chris Jones", Jersey: "95", AthleteESPNID: "3045353", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3045353.png"},
			{TeamCode: "KC", FormationGroup: "Defensiva", PositionCode: "MLB", PositionName: "Middle Linebacker", DepthRank: 1, AthleteName: "Nick Bolton", Jersey: "32", AthleteESPNID: "4361523"},
			{TeamCode: "KC", FormationGroup: "Defensiva", PositionCode: "LCB", PositionName: "Left Cornerback", DepthRank: 1, AthleteName: "Trent McDuffie", Jersey: "22", AthleteESPNID: "4426515"},
			{TeamCode: "KC", FormationGroup: "Defensiva", PositionCode: "FS", PositionName: "Free Safety", DepthRank: 1, AthleteName: "Justin Reid", Jersey: "20", AthleteESPNID: "3915509"},
			{TeamCode: "KC", FormationGroup: "Equipos Especiales", PositionCode: "PK", PositionName: "Place Kicker", DepthRank: 1, AthleteName: "Harrison Butker", Jersey: "7", AthleteESPNID: "3055899"},
			{TeamCode: "KC", FormationGroup: "Equipos Especiales", PositionCode: "P", PositionName: "Punter", DepthRank: 1, AthleteName: "Matt Araiza", Jersey: "14", AthleteESPNID: "4361757"},
		}
	default:
		// Generic default starters
		return []*db.TeamDepthChartSlot{
			{TeamCode: teamCode, FormationGroup: "Ofensiva", PositionCode: "QB", PositionName: "Quarterback", DepthRank: 1, AthleteName: "Titular QB", Jersey: "1"},
			{TeamCode: teamCode, FormationGroup: "Ofensiva", PositionCode: "QB", PositionName: "Quarterback", DepthRank: 2, AthleteName: "Suplente QB", Jersey: "2"},
			{TeamCode: teamCode, FormationGroup: "Ofensiva", PositionCode: "RB", PositionName: "Running Back", DepthRank: 1, AthleteName: "Titular RB", Jersey: "20"},
			{TeamCode: teamCode, FormationGroup: "Ofensiva", PositionCode: "WR", PositionName: "Wide Receiver", DepthRank: 1, AthleteName: "Titular WR1", Jersey: "11"},
			{TeamCode: teamCode, FormationGroup: "Ofensiva", PositionCode: "TE", PositionName: "Tight End", DepthRank: 1, AthleteName: "Titular TE", Jersey: "88"},
			{TeamCode: teamCode, FormationGroup: "Defensiva", PositionCode: "DE", PositionName: "Defensive End", DepthRank: 1, AthleteName: "Titular DE", Jersey: "99"},
			{TeamCode: teamCode, FormationGroup: "Defensiva", PositionCode: "MLB", PositionName: "Middle Linebacker", DepthRank: 1, AthleteName: "Titular MLB", Jersey: "54"},
			{TeamCode: teamCode, FormationGroup: "Defensiva", PositionCode: "CB", PositionName: "Cornerback", DepthRank: 1, AthleteName: "Titular CB", Jersey: "24"},
			{TeamCode: teamCode, FormationGroup: "Equipos Especiales", PositionCode: "PK", PositionName: "Kicker", DepthRank: 1, AthleteName: "Titular K", Jersey: "9"},
		}
	}
}
