package espn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nfl-quiniela-2026/db"
)

const ESPNScoreboardURL = "https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard"

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: ESPNScoreboardURL,
	}
}

// FetchWeekScoreboard fetches games for a specific season, week, and seasonType (2 = regular season, 3 = postseason)
func (c *Client) FetchWeekScoreboard(year, weekNum, seasonType int) (*ESPNScoreboardResponse, error) {
	url := fmt.Sprintf("%s?dates=%d&seasontype=%d&week=%d", c.baseURL, year, seasonType, weekNum)
	
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating espn request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching espn scoreboard: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("espn api returned status: %d", resp.StatusCode)
	}

	var sb ESPNScoreboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&sb); err != nil {
		return nil, fmt.Errorf("decoding espn response: %w", err)
	}

	return &sb, nil
}

// NormalizeTeamCode handles team abbreviation differences between ESPN and standard codes
func NormalizeTeamCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	switch code {
	case "WAS", "WSH":
		return "WSH"
	case "JAC", "JAX":
		return "JAX"
	case "LA", "LAR":
		return "LAR"
	default:
		return code
	}
}

// MapESPNEventToGame maps an ESPN Event to our DB Game model
func MapESPNEventToGame(event *ESPNEvent, weekID int64, teamMap map[string]*db.Team) (*db.Game, error) {
	if len(event.Competitions) == 0 || len(event.Competitions[0].Competitors) < 2 {
		return nil, fmt.Errorf("invalid competition data for event: %s", event.ID)
	}

	comp := event.Competitions[0]
	var homeComp, awayComp *ESPNCompetitor
	for i := range comp.Competitors {
		c := &comp.Competitors[i]
		if c.HomeAway == "home" {
			homeComp = c
		} else {
			awayComp = c
		}
	}

	if homeComp == nil || awayComp == nil {
		return nil, fmt.Errorf("missing home or away competitor for event: %s", event.ID)
	}

	homeCode := NormalizeTeamCode(homeComp.Team.Abbreviation)
	awayCode := NormalizeTeamCode(awayComp.Team.Abbreviation)

	homeTeam, existsHome := teamMap[homeCode]
	awayTeam, existsAway := teamMap[awayCode]

	if !existsHome {
		return nil, fmt.Errorf("unknown home team code: %s", homeCode)
	}
	if !existsAway {
		return nil, fmt.Errorf("unknown away team code: %s", awayCode)
	}

	kickoffTime, err := ParseKickoffTime(event.Date)
	if err != nil {
		kickoffTime = time.Now()
	}

	status := "scheduled"
	state := strings.ToLower(event.Status.Type.State)
	if event.Status.Type.Completed || state == "post" {
		status = "final"
	} else if state == "in" {
		status = "in_progress"
	}

	var homeScore, awayScore *int
	if homeComp.Score != "" {
		if s, err := strconv.Atoi(homeComp.Score); err == nil {
			homeScore = &s
		}
	}
	if awayComp.Score != "" {
		if s, err := strconv.Atoi(awayComp.Score); err == nil {
			awayScore = &s
		}
	}

	statusDetail := event.Status.Type.ShortDetail
	if statusDetail == "" {
		statusDetail = event.Status.Type.Detail
	}

	// Broadcast channel detection
	broadcast := comp.Broadcast
	if broadcast == "" && len(comp.GeoBroadcasts) > 0 {
		broadcast = comp.GeoBroadcasts[0].Media.ShortName
	}

	// Game situation / Down & Distance
	situation := ""
	if comp.Situation != nil {
		if comp.Situation.DownDistanceText != "" {
			situation = comp.Situation.DownDistanceText
		} else if comp.Situation.ShortDownDistanceText != "" {
			situation = comp.Situation.ShortDownDistanceText
		} else if comp.Situation.LastPlay.Text != "" {
			situation = comp.Situation.LastPlay.Text
		}
	}

	// Quarter linescores
	linescoresJSON := ""
	if len(awayComp.Linescores) > 0 || len(homeComp.Linescores) > 0 {
		matrix := struct {
			Away []string `json:"away"`
			Home []string `json:"home"`
		}{
			Away: make([]string, len(awayComp.Linescores)),
			Home: make([]string, len(homeComp.Linescores)),
		}
		for i, ls := range awayComp.Linescores {
			matrix.Away[i] = fmt.Sprintf("%.0f", ls.Value)
		}
		for i, ls := range homeComp.Linescores {
			matrix.Home[i] = fmt.Sprintf("%.0f", ls.Value)
		}
		if b, err := json.Marshal(matrix); err == nil {
			linescoresJSON = string(b)
		}
	}

	return &db.Game{
		WeekID:       weekID,
		ESPNGameID:   event.ID,
		HomeTeamID:   homeTeam.ID,
		AwayTeamID:   awayTeam.ID,
		KickoffTime:  kickoffTime,
		HomeScore:    homeScore,
		AwayScore:    awayScore,
		Status:       status,
		StatusDetail: statusDetail,
		Broadcast:    broadcast,
		Situation:    situation,
		Linescores:   linescoresJSON,
	}, nil
}

// ParseKickoffTime parses ISO8601 strings from ESPN which may omit seconds (e.g. 2026-09-10T00:20Z)
func ParseKickoffTime(dateStr string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04Z",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse kickoff date: %s", dateStr)
}
