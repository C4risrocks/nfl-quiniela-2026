package espn

import (
	"testing"
	"time"

	"nfl-quiniela-2026/db"
)

func TestMapESPNEventToGame(t *testing.T) {
	teamMap := map[string]*db.Team{
		"KC":  {ID: 1, Code: "KC", Name: "Chiefs", City: "Kansas City"},
		"BAL": {ID: 2, Code: "BAL", Name: "Ravens", City: "Baltimore"},
		"WSH": {ID: 3, Code: "WSH", Name: "Commanders", City: "Washington"},
	}

	event := &ESPNEvent{
		ID:   "401671785",
		Date: "2026-09-05T00:20:00Z",
		Name: "Baltimore Ravens at Kansas City Chiefs",
		Status: ESPNEventStatus{
			Type: struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				State       string `json:"state"`
				Completed   bool   `json:"completed"`
				Description string `json:"description"`
				Detail      string `json:"detail"`
				ShortDetail string `json:"shortDetail"`
			}{
				Name:        "STATUS_FINAL",
				State:       "post",
				Completed:   true,
				ShortDetail: "Final",
			},
		},
		Competitions: []ESPNCompetition{
			{
				ID: "401671785",
				Competitors: []ESPNCompetitor{
					{
						HomeAway: "home",
						Score:    "27",
						Team: ESPNTeam{
							Abbreviation: "KC",
							DisplayName:  "Kansas City Chiefs",
						},
					},
					{
						HomeAway: "away",
						Score:    "20",
						Team: ESPNTeam{
							Abbreviation: "BAL",
							DisplayName:  "Baltimore Ravens",
						},
					},
				},
			},
		},
	}

	game, err := MapESPNEventToGame(event, 10, teamMap)
	if err != nil {
		t.Fatalf("Failed to map ESPN event: %v", err)
	}

	if game.HomeTeamID != 1 {
		t.Errorf("Expected home team ID 1 (KC), got %d", game.HomeTeamID)
	}
	if game.AwayTeamID != 2 {
		t.Errorf("Expected away team ID 2 (BAL), got %d", game.AwayTeamID)
	}
	if game.Status != "final" {
		t.Errorf("Expected status final, got %s", game.Status)
	}
	if game.HomeScore == nil || *game.HomeScore != 27 {
		t.Errorf("Expected home score 27, got %v", game.HomeScore)
	}
	if game.AwayScore == nil || *game.AwayScore != 20 {
		t.Errorf("Expected away score 20, got %v", game.AwayScore)
	}
	if game.KickoffTime.UTC().Format(time.RFC3339) != "2026-09-05T00:20:00Z" {
		t.Errorf("Expected kickoff time 2026-09-05T00:20:00Z, got %v", game.KickoffTime)
	}
}

func TestNormalizeTeamCode(t *testing.T) {
	if NormalizeTeamCode("WAS") != "WSH" {
		t.Errorf("Expected WSH, got %s", NormalizeTeamCode("WAS"))
	}
	if NormalizeTeamCode("JAC") != "JAX" {
		t.Errorf("Expected JAX, got %s", NormalizeTeamCode("JAC"))
	}
	if NormalizeTeamCode("KC") != "KC" {
		t.Errorf("Expected KC, got %s", NormalizeTeamCode("KC"))
	}
}

func TestParseKickoffTime(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"2026-09-10T00:20Z", "2026-09-10T00:20:00Z"},
		{"2026-09-10T00:20:00Z", "2026-09-10T00:20:00Z"},
		{"2026-09-13T17:00:00Z", "2026-09-13T17:00:00Z"},
		{"2026-09-13 17:00:00", "2026-09-13T17:00:00Z"},
	}

	for _, c := range cases {
		parsed, err := ParseKickoffTime(c.input)
		if err != nil {
			t.Fatalf("Failed to parse %s: %v", c.input, err)
		}
		if parsed.UTC().Format(time.RFC3339) != c.expected {
			t.Errorf("For input %s, expected %s, got %s", c.input, c.expected, parsed.UTC().Format(time.RFC3339))
		}
	}
}

func TestLiveFetchWeekScoreboard(t *testing.T) {
	client := NewClient()
	sb, err := client.FetchWeekScoreboard(2026, 1, 2)
	if err != nil {
		t.Fatalf("Live ESPN fetch failed: %v", err)
	}
	if len(sb.Events) == 0 {
		t.Fatalf("Expected events for Week 1 2026, got 0")
	}
	t.Logf("Successfully fetched %d ESPN events for 2026 Week 1", len(sb.Events))
}

