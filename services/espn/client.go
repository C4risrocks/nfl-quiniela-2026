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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,es;q=0.8")

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

	kickoffTime, err := time.Parse(time.RFC3339, event.Date)
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
	}, nil
}
