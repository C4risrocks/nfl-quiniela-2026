package db

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestStandingsPersistence_SaveAndGet(t *testing.T) {
	testDBPath := "test_standings_persistence.db"
	defer os.Remove(testDBPath)

	database, err := InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)

	// Initially, HasSeasonStandings should return false
	has, err := repo.HasSeasonStandings(2023)
	if err != nil {
		t.Fatalf("HasSeasonStandings error: %v", err)
	}
	if has {
		t.Errorf("expected HasSeasonStandings to be false before saving")
	}

	// Create test season standings for 2023
	kcStanding := &TeamStanding{
		TeamCode:            "KC",
		TeamName:            "Chiefs",
		TeamCity:            "Kansas City",
		LogoURL:             "https://a.espncdn.com/i/teamlogos/nfl/500/kc.png",
		Conference:          "AFC",
		Division:            "AFC Oeste",
		Wins:                11,
		Losses:              6,
		Ties:                0,
		WinPercent:          0.647,
		WinPercentFormatted: ".647",
		PointsFor:           371,
		PointsAgainst:       294,
		PointDiff:           77,
		Streak:              "W2",
		HomeRecord:          "5-4",
		AwayRecord:          "6-2",
		ConfRecord:          "8-4",
		DivisionRecord:      "4-2",
		Rank:                1,
		ConferenceSeed:      3,
		IsSuperBowlChampion: true,
		SuperBowlTitle:      "LVIII",
	}

	sfStanding := &TeamStanding{
		TeamCode:            "SF",
		TeamName:            "49ers",
		TeamCity:            "San Francisco",
		LogoURL:             "https://a.espncdn.com/i/teamlogos/nfl/500/sf.png",
		Conference:          "NFC",
		Division:            "NFC Oeste",
		Wins:                12,
		Losses:              5,
		Ties:                0,
		WinPercent:          0.706,
		WinPercentFormatted: ".706",
		PointsFor:           491,
		PointsAgainst:       298,
		PointDiff:           193,
		Streak:              "L1",
		HomeRecord:          "5-3",
		AwayRecord:          "7-2",
		ConfRecord:          "10-2",
		DivisionRecord:      "5-1",
		Rank:                1,
		ConferenceSeed:      1,
		IsSuperBowlChampion: false,
	}

	standings := &SeasonStandings{
		Year: 2023,
		League: []*TeamStanding{
			sfStanding,
			kcStanding,
		},
		Conferences: []*ConferenceStandings{
			{
				Name:       "American Football Conference",
				Conference: "AFC",
				Teams:      []*TeamStanding{kcStanding},
			},
			{
				Name:       "National Football Conference",
				Conference: "NFC",
				Teams:      []*TeamStanding{sfStanding},
			},
		},
		Divisions: []*DivisionStandings{
			{
				Name:       "AFC Oeste",
				Conference: "AFC",
				Division:   "West",
				Teams:      []*TeamStanding{kcStanding},
			},
			{
				Name:       "NFC Oeste",
				Conference: "NFC",
				Division:   "West",
				Teams:      []*TeamStanding{sfStanding},
			},
		},
		Summary: &SeasonDashboardSummary{
			TopRecordTeam:     sfStanding,
			TopOffenseTeam:    sfStanding,
			TopDefenseTeam:    kcStanding,
			SuperBowlChampion: kcStanding,
		},
	}

	// Save to DB
	if err := repo.SaveSeasonStandings(standings); err != nil {
		t.Fatalf("SaveSeasonStandings failed: %v", err)
	}

	// Now HasSeasonStandings should return true
	has, err = repo.HasSeasonStandings(2023)
	if err != nil {
		t.Fatalf("HasSeasonStandings error: %v", err)
	}
	if !has {
		t.Errorf("expected HasSeasonStandings to be true after saving")
	}

	// Retrieve from DB
	loaded, err := repo.GetSeasonStandings(2023)
	if err != nil {
		t.Fatalf("GetSeasonStandings failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil loaded standings")
	}

	if loaded.Year != 2023 {
		t.Errorf("expected Year 2023, got %d", loaded.Year)
	}
	if len(loaded.League) != 2 {
		t.Fatalf("expected 2 teams in League, got %d", len(loaded.League))
	}

	// Verify KC data
	var loadedKC *TeamStanding
	for _, team := range loaded.League {
		if team.TeamCode == "KC" {
			loadedKC = team
		}
	}
	if loadedKC == nil {
		t.Fatal("KC standing not found in loaded league")
	}
	if !loadedKC.IsSuperBowlChampion {
		t.Errorf("expected KC IsSuperBowlChampion to be true")
	}
	if !strings.Contains(loadedKC.SuperBowlTitle, "LVIII") {
		t.Errorf("expected KC SuperBowlTitle to contain LVIII, got %s", loadedKC.SuperBowlTitle)
	}
	if loadedKC.Wins != 11 || loadedKC.Losses != 6 {
		t.Errorf("expected KC 11-6, got %d-%d", loadedKC.Wins, loadedKC.Losses)
	}
	if loadedKC.PointsFor != 371 || loadedKC.PointsAgainst != 294 {
		t.Errorf("expected KC 371/294, got %d/%d", loadedKC.PointsFor, loadedKC.PointsAgainst)
	}

	// Verify Summary
	if loaded.Summary == nil {
		t.Fatal("expected loaded Summary to be non-nil")
	}
	if loaded.Summary.SuperBowlChampion == nil {
		t.Fatal("expected loaded Summary.SuperBowlChampion to be non-nil")
	}
	if loaded.Summary.SuperBowlChampion.TeamCode != "KC" {
		t.Errorf("expected Summary.SuperBowlChampion KC, got %s", loaded.Summary.SuperBowlChampion.TeamCode)
	}
}

func TestTeamSchedulePersistence_SaveAndGet(t *testing.T) {
	testDBPath := "test_schedule_persistence.db"
	defer os.Remove(testDBPath)

	database, err := InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)

	// Initially not found
	sched, err := repo.GetTeamSchedule("KC", 2024)
	if err != nil {
		t.Fatalf("GetTeamSchedule error: %v", err)
	}
	if sched != nil {
		t.Errorf("expected nil schedule initially")
	}

	teamScore := 27
	oppScore := 20
	testSchedule := []*TeamScheduleItem{
		{
			WeekNumber:       1,
			KickoffTime:      time.Now(),
			KickoffFormatted: "5 sep 2024",
			OpponentCode:     "BAL",
			OpponentName:     "Ravens",
			OpponentLogo:     "https://a.espncdn.com/i/teamlogos/nfl/500/bal.png",
			IsHome:           true,
			TeamScore:        &teamScore,
			OpponentScore:    &oppScore,
			Result:           "W",
			StatusDetail:     "Final",
		},
	}

	if err := repo.SaveTeamSchedule("KC", 2024, testSchedule); err != nil {
		t.Fatalf("SaveTeamSchedule failed: %v", err)
	}

	loadedSched, err := repo.GetTeamSchedule("KC", 2024)
	if err != nil {
		t.Fatalf("GetTeamSchedule after save failed: %v", err)
	}
	if loadedSched == nil || len(loadedSched) != 1 {
		t.Fatalf("expected 1 loaded schedule item, got %v", loadedSched)
	}

	item := loadedSched[0]
	if item.OpponentCode != "BAL" || item.Result != "W" {
		t.Errorf("unexpected loaded schedule item: %+v", item)
	}
	if item.TeamScore == nil || *item.TeamScore != 27 {
		t.Errorf("expected TeamScore 27, got %v", item.TeamScore)
	}
}

func TestKnownSuperBowlChampions(t *testing.T) {
	tests := []struct {
		year     int
		expected string
		hasChamp bool
	}{
		{year: 2022, expected: "KC", hasChamp: true},
		{year: 2023, expected: "KC", hasChamp: true},
		{year: 2024, expected: "PHI", hasChamp: true},
		{year: 2025, expected: "SEA", hasChamp: true},
		{year: 2026, expected: "", hasChamp: false},
	}

	for _, tt := range tests {
		champ, ok := GetSuperBowlChampion(tt.year)
		if ok != tt.hasChamp {
			t.Errorf("year %d: expected hasChamp=%v, got %v", tt.year, tt.hasChamp, ok)
		}
		if ok && champ.TeamCode != tt.expected {
			t.Errorf("year %d: expected champ %s, got %s", tt.year, tt.expected, champ.TeamCode)
		}
	}
}
