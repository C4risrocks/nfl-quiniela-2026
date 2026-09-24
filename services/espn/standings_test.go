package espn

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nfl-quiniela-2026/db"
)

const sampleESPNStandingsJSON = `{
  "uid": "s:20~l:28",
  "name": "NFL Standings",
  "season": {"year": 2026},
  "children": [
    {
      "id": "1",
      "name": "American Football Conference",
      "abbreviation": "AFC",
      "standings": {
        "entries": [
          {
            "team": {
              "id": "12",
              "abbreviation": "KC",
              "displayName": "Kansas City Chiefs",
              "location": "Kansas City",
              "logos": [{"href": "https://a.espncdn.com/i/teamlogos/nfl/500/kc.png"}]
            },
            "stats": [
              {"name": "wins", "displayValue": "3", "value": 3},
              {"name": "losses", "displayValue": "0", "value": 0},
              {"name": "ties", "displayValue": "0", "value": 0},
              {"name": "winPercent", "displayValue": "1.000", "value": 1.0},
              {"name": "pointsFor", "displayValue": "84", "value": 84},
              {"name": "pointsAgainst", "displayValue": "51", "value": 51},
              {"name": "differential", "displayValue": "+33", "value": 33},
              {"name": "streak", "displayValue": "W3", "value": 3},
              {"name": "Home", "displayValue": "2-0", "value": 0},
              {"name": "Road", "displayValue": "1-0", "value": 0},
              {"name": "divisionRecord", "displayValue": "1-0", "value": 0},
              {"name": "conferenceRecord", "displayValue": "2-0", "value": 0},
              {"name": "playoffSeed", "displayValue": "1", "value": 1},
              {"name": "gamesBehind", "displayValue": "-", "value": 0}
            ]
          },
          {
            "team": {
              "id": "2",
              "abbreviation": "BUF",
              "displayName": "Buffalo Bills",
              "location": "Buffalo",
              "logos": [{"href": "https://a.espncdn.com/i/teamlogos/nfl/500/buf.png"}]
            },
            "stats": [
              {"name": "wins", "displayValue": "2", "value": 2},
              {"name": "losses", "displayValue": "1", "value": 1},
              {"name": "ties", "displayValue": "0", "value": 0},
              {"name": "winPercent", "displayValue": "0.667", "value": 0.667},
              {"name": "pointsFor", "displayValue": "70", "value": 70},
              {"name": "pointsAgainst", "displayValue": "55", "value": 55},
              {"name": "differential", "displayValue": "+15", "value": 15},
              {"name": "streak", "displayValue": "W1", "value": 1},
              {"name": "Home", "displayValue": "1-0", "value": 0},
              {"name": "Road", "displayValue": "1-1", "value": 0},
              {"name": "divisionRecord", "displayValue": "1-0", "value": 0},
              {"name": "conferenceRecord", "displayValue": "2-1", "value": 0},
              {"name": "playoffSeed", "displayValue": "2", "value": 2},
              {"name": "gamesBehind", "displayValue": "1.0", "value": 1.0}
            ]
          }
        ]
      }
    },
    {
      "id": "2",
      "name": "National Football Conference",
      "abbreviation": "NFC",
      "standings": {
        "entries": [
          {
            "team": {
              "id": "25",
              "abbreviation": "SF",
              "displayName": "San Francisco 49ers",
              "location": "San Francisco",
              "logos": [{"href": "https://a.espncdn.com/i/teamlogos/nfl/500/sf.png"}]
            },
            "stats": [
              {"name": "wins", "displayValue": "2", "value": 2},
              {"name": "losses", "displayValue": "1", "value": 1},
              {"name": "ties", "displayValue": "0", "value": 0},
              {"name": "winPercent", "displayValue": "0.667", "value": 0.667},
              {"name": "pointsFor", "displayValue": "65", "value": 65},
              {"name": "pointsAgainst", "displayValue": "48", "value": 48},
              {"name": "differential", "displayValue": "+17", "value": 17},
              {"name": "streak", "displayValue": "L1", "value": 1},
              {"name": "Home", "displayValue": "1-1", "value": 0},
              {"name": "Road", "displayValue": "1-0", "value": 0},
              {"name": "divisionRecord", "displayValue": "1-0", "value": 0},
              {"name": "conferenceRecord", "displayValue": "1-1", "value": 0},
              {"name": "playoffSeed", "displayValue": "1", "value": 1},
              {"name": "gamesBehind", "displayValue": "-", "value": 0}
            ]
          }
        ]
      }
    }
  ]
}`

func TestFetchNFLStandingsParsing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleESPNStandingsJSON))
	}))
	defer ts.Close()

	client := NewClient()
	client.httpClient = ts.Client()
	client.standingsURL = ts.URL

	// Clear cache for isolated test
	standingsCacheMu.Lock()
	standingsCache = make(map[int]standingsCacheEntry)
	standingsCacheMu.Unlock()

	teamMap := map[string]*db.Team{
		"KC": {
			ID:             1,
			Code:           "KC",
			Name:           "Chiefs",
			City:           "Kansas City",
			PrimaryColor:   "#E31837",
			SecondaryColor: "#FFB81C",
			Conference:     "AFC",
			Division:       "West",
		},
		"BUF": {
			ID:             2,
			Code:           "BUF",
			Name:           "Bills",
			City:           "Buffalo",
			PrimaryColor:   "#00338D",
			SecondaryColor: "#C60C30",
			Conference:     "AFC",
			Division:       "East",
		},
		"SF": {
			ID:             3,
			Code:           "SF",
			Name:           "49ers",
			City:           "San Francisco",
			PrimaryColor:   "#AA0000",
			SecondaryColor: "#B3995D",
			Conference:     "NFC",
			Division:       "West",
		},
	}

	standings, err := client.FetchNFLStandings(2026, 2026, teamMap)
	if err != nil {
		t.Fatalf("unexpected error fetching standings: %v", err)
	}

	if standings == nil {
		t.Fatal("expected non-nil standings")
	}

	if standings.Year != 2026 || !standings.IsCurrent {
		t.Errorf("expected year 2026 isCurrent true, got year %d isCurrent %v", standings.Year, standings.IsCurrent)
	}

	if len(standings.Conferences) != 2 {
		t.Errorf("expected 2 conferences, got %d", len(standings.Conferences))
	}

	// Verify top team in AFC is KC
	afc := standings.Conferences[0]
	if afc.Conference != "AFC" {
		t.Errorf("expected AFC first, got %s", afc.Conference)
	}
	if len(afc.Teams) < 2 {
		t.Fatalf("expected at least 2 teams in AFC, got %d", len(afc.Teams))
	}
	topAFC := afc.Teams[0]
	if topAFC.TeamCode != "KC" {
		t.Errorf("expected top AFC team KC, got %s", topAFC.TeamCode)
	}
	if topAFC.Wins != 3 || topAFC.Losses != 0 {
		t.Errorf("expected KC 3-0, got %d-%d", topAFC.Wins, topAFC.Losses)
	}
	if topAFC.OffensivePPG != 28.0 {
		t.Errorf("expected KC 28.0 PPG (84/3), got %.1f", topAFC.OffensivePPG)
	}

	// Verify Summary
	if standings.Summary == nil {
		t.Fatal("expected non-nil dashboard summary")
	}
	if standings.Summary.TopRecordTeam.TeamCode != "KC" {
		t.Errorf("expected TopRecordTeam KC, got %s", standings.Summary.TopRecordTeam.TeamCode)
	}
	if standings.Summary.BestStreakTeam.TeamCode != "KC" {
		t.Errorf("expected BestStreakTeam KC, got %s", standings.Summary.BestStreakTeam.TeamCode)
	}

	// Verify Division mapping for BUF in AFC East
	var foundDivTeam *db.TeamStanding
	for _, div := range standings.Divisions {
		if div.Name == "AFC Este" {
			for _, tm := range div.Teams {
				if tm.TeamCode == "BUF" {
					foundDivTeam = tm
					break
				}
			}
		}
	}
	if foundDivTeam == nil {
		t.Errorf("expected BUF to be placed in AFC Este division")
	} else if foundDivTeam.Rank != 1 {
		t.Errorf("expected BUF to have rank 1 in AFC Este, got %d", foundDivTeam.Rank)
	}
}

const sampleScheduleJSON = `{
  "team": {
    "id": "12",
    "abbreviation": "KC",
    "displayName": "Kansas City Chiefs"
  },
  "events": [
    {
      "id": "401547417",
      "date": "2026-09-08T00:20Z",
      "name": "Baltimore Ravens at Kansas City Chiefs",
      "week": {"number": 1},
      "competitions": [
        {
          "id": "401547417",
          "date": "2026-09-08T00:20Z",
          "broadcast": "NBC",
          "status": {
            "type": {
              "completed": true,
              "description": "Final",
              "detail": "Final",
              "state": "post"
            }
          },
          "competitors": [
            {
              "id": "12",
              "homeAway": "home",
              "score": {"displayValue": "27", "value": 27},
              "winner": true,
              "team": {"id": "12", "abbreviation": "KC", "displayName": "Kansas City Chiefs", "location": "Kansas City"}
            },
            {
              "id": "33",
              "homeAway": "away",
              "score": {"displayValue": "20", "value": 20},
              "winner": false,
              "team": {"id": "33", "abbreviation": "BAL", "displayName": "Baltimore Ravens", "location": "Baltimore"}
            }
          ]
        }
      ]
    },
    {
      "id": "401547418",
      "date": "2026-09-15T20:25Z",
      "name": "Cincinnati Bengals at Kansas City Chiefs",
      "week": {"number": 2},
      "competitions": [
        {
          "id": "401547418",
          "date": "2026-09-15T20:25Z",
          "broadcast": "CBS",
          "status": {
            "type": {
              "completed": false,
              "description": "Scheduled",
              "detail": "Sun, Sep 15 - 4:25 PM",
              "state": "pre"
            }
          },
          "competitors": [
            {
              "id": "12",
              "homeAway": "home",
              "score": {"displayValue": "", "value": 0},
              "winner": false,
              "team": {"id": "12", "abbreviation": "KC", "displayName": "Kansas City Chiefs", "location": "Kansas City"}
            },
            {
              "id": "4",
              "homeAway": "away",
              "score": {"displayValue": "", "value": 0},
              "winner": false,
              "team": {"id": "4", "abbreviation": "CIN", "displayName": "Cincinnati Bengals", "location": "Cincinnati"}
            }
          ]
        }
      ]
    }
  ]
}`

func TestFetchTeamSchedule(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleScheduleJSON))
	}))
	defer ts.Close()

	client := NewClient()
	client.httpClient = ts.Client()
	client.scheduleURL = ts.URL

	scheduleCacheMu.Lock()
	scheduleCache = make(map[string]scheduleCacheEntry)
	scheduleCacheMu.Unlock()

	teamMap := map[string]*db.Team{
		"KC":  {ID: 1, Code: "KC", Name: "Chiefs", City: "Kansas City"},
		"BAL": {ID: 5, Code: "BAL", Name: "Ravens", City: "Baltimore"},
		"CIN": {ID: 6, Code: "CIN", Name: "Bengals", City: "Cincinnati"},
	}

	items, err := client.FetchTeamSchedule("KC", 2026, teamMap)
	if err != nil {
		t.Fatalf("unexpected error fetching team schedule: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 schedule items, got %d", len(items))
	}

	// Game 1: KC vs BAL (Completed, W)
	g1 := items[0]
	if g1.WeekNumber != 1 {
		t.Errorf("expected week 1, got %d", g1.WeekNumber)
	}
	if !g1.IsHome {
		t.Errorf("expected KC to be home team")
	}
	if g1.OpponentCode != "BAL" {
		t.Errorf("expected opponent BAL, got %s", g1.OpponentCode)
	}
	if g1.Result != "W" {
		t.Errorf("expected result W, got %s", g1.Result)
	}
	if g1.TeamScore == nil || *g1.TeamScore != 27 {
		t.Errorf("expected KC score 27, got %v", g1.TeamScore)
	}
	if g1.OpponentScore == nil || *g1.OpponentScore != 20 {
		t.Errorf("expected BAL score 20, got %v", g1.OpponentScore)
	}

	// Game 2: KC vs CIN (Scheduled)
	g2 := items[1]
	if g2.WeekNumber != 2 {
		t.Errorf("expected week 2, got %d", g2.WeekNumber)
	}
	if g2.Result != "scheduled" {
		t.Errorf("expected result scheduled, got %s", g2.Result)
	}
}

func TestLiveESPN2025(t *testing.T) {
	client := NewClient()
	for _, year := range []int{2025, 2024, 2023, 2022} {
		standings, err := client.FetchNFLStandings(year, 2026, nil)
		if err != nil {
			t.Errorf("year %d err: %v", year, err)
			continue
		}
		if standings == nil || len(standings.League) != 32 {
			t.Errorf("year %d: expected 32 teams, got %d", year, len(standings.League))
		} else {
			if len(standings.Divisions) != 8 {
				t.Errorf("year %d: expected 8 divisions, got %d", year, len(standings.Divisions))
			}
			for _, div := range standings.Divisions {
				if len(div.Teams) != 4 {
					t.Errorf("year %d division %s: expected 4 teams, got %d", year, div.Name, len(div.Teams))
				}
			}
			t.Logf("year %d: successfully fetched 32 teams across 8 divisions (Top: %s %d-%d)", 
				year, standings.League[0].TeamName, standings.League[0].Wins, standings.League[0].Losses)
		}
	}

	// Also verify schedule retrieval for past season (2025 KC)
	sched, err := client.FetchTeamSchedule("KC", 2025, nil)
	if err != nil {
		t.Fatalf("fetch 2025 KC schedule err: %v", err)
	}
	if len(sched) == 0 {
		t.Fatalf("expected non-empty schedule for 2025 KC")
	}
	t.Logf("2025 KC schedule items: %d (Game 1: vs %s, Result: %s)", len(sched), sched[0].OpponentCode, sched[0].Result)
}


