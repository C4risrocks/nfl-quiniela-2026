package espn

// ESPNScoreboardResponse represents the JSON response from ESPN NFL scoreboard API
type ESPNScoreboardResponse struct {
	Leagues []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Calendar []struct {
			Label      string `json:"label"`
			Value      string `json:"value"`
			StartDate  string `json:"startDate"`
			EndDate    string `json:"endDate"`
			Entries    []struct {
				Label     string `json:"label"`
				Value     string `json:"value"`
				StartDate string `json:"startDate"`
				EndDate   string `json:"endDate"`
			} `json:"entries"`
		} `json:"calendar"`
	} `json:"leagues"`
	Season struct {
		Type int `json:"type"`
		Year int `json:"year"`
	} `json:"season"`
	Week struct {
		Number int `json:"number"`
	} `json:"week"`
	Events []ESPNEvent `json:"events"`
}

type ESPNEvent struct {
	ID           string           `json:"id"`
	Date         string           `json:"date"`
	Name         string           `json:"name"`
	ShortName    string           `json:"shortName"`
	Competitions []ESPNCompetition `json:"competitions"`
	Status       ESPNEventStatus  `json:"status"`
}

type ESPNCompetition struct {
	ID            string           `json:"id"`
	Date          string           `json:"date"`
	Competitors   []ESPNCompetitor `json:"competitors"`
	Broadcast     string           `json:"broadcast"`
	GeoBroadcasts []struct {
		Media struct {
			ShortName string `json:"shortName"`
		} `json:"media"`
	} `json:"geoBroadcasts"`
	Situation *struct {
		DownDistanceText      string `json:"downDistanceText"`
		ShortDownDistanceText string `json:"shortDownDistanceText"`
		Possession            string `json:"possession"`
		IsRedZone             bool   `json:"isRedZone"`
		LastPlay              struct {
			Text string `json:"text"`
		} `json:"lastPlay"`
	} `json:"situation"`
}

type ESPNLinescore struct {
	Value float64 `json:"value"`
}

type ESPNCompetitor struct {
	ID         string          `json:"id"`
	HomeAway   string          `json:"homeAway"` // "home" or "away"
	Score      string          `json:"score"`
	Winner     *bool           `json:"winner"`
	Team       ESPNTeam        `json:"team"`
	Linescores []ESPNLinescore `json:"linescores"`
}

type ESPNTeam struct {
	ID           string `json:"id"`
	Abbreviation string `json:"abbreviation"`
	DisplayName  string `json:"displayName"`
	ShortDisplayName string `json:"shortDisplayName"`
	Location     string `json:"location"`
	Logo         string `json:"logo"`
}

type ESPNEventStatus struct {
	Clock        float64 `json:"clock"`
	DisplayClock string  `json:"displayClock"`
	Period       int     `json:"period"`
	Type         struct {
		ID          string `json:"id"`
		Name        string `json:"name"` // STATUS_SCHEDULED, STATUS_IN_PROGRESS, STATUS_FINAL, STATUS_HALFTIME
		State       string `json:"state"` // "pre", "in", "post"
		Completed   bool   `json:"completed"`
		Description string `json:"description"`
		Detail      string `json:"detail"`
		ShortDetail string `json:"shortDetail"`
	} `json:"type"`
}

// ESPNSummaryResponse represents the JSON response from ESPN NFL game summary API
type ESPNSummaryResponse struct {
	Boxscore struct {
		Teams []struct {
			Team struct {
				ID           string `json:"id"`
				Abbreviation string `json:"abbreviation"`
				DisplayName  string `json:"displayName"`
				Logo         string `json:"logo"`
			} `json:"team"`
			Statistics []struct {
				Name         string `json:"name"`
				DisplayValue string `json:"displayValue"`
				Label        string `json:"label"`
			} `json:"statistics"`
		} `json:"teams"`
	} `json:"boxscore"`
	ScoringPlays []struct {
		ID     string `json:"id"`
		Type   struct {
			Text string `json:"text"`
		} `json:"type"`
		Text      string `json:"text"`
		AwayScore int    `json:"awayScore"`
		HomeScore int    `json:"homeScore"`
		Period    struct {
			Number int `json:"number"`
		} `json:"period"`
		Clock struct {
			DisplayValue string `json:"displayValue"`
		} `json:"clock"`
		Team struct {
			ID           string `json:"id"`
			Abbreviation string `json:"abbreviation"`
			Logo         string `json:"logo"`
		} `json:"team"`
	} `json:"scoringPlays"`
}

