package db

import (
	"os"
	"testing"
)

func TestInjuriesAndDepthChartPersistence(t *testing.T) {
	testDBPath := "test_injuries.db"
	cleanup := func() {
		_ = os.Remove(testDBPath)
		_ = os.Remove("db/" + testDBPath)
		_ = os.Remove("db/" + testDBPath + "-shm")
		_ = os.Remove("db/" + testDBPath + "-wal")
	}
	cleanup()
	defer cleanup()

	database, err := InitDB("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	defer database.Close()

	repo := NewRepository(database)

	// 1. Verify initially empty
	injuriesInit, err := repo.GetTeamInjuries("KC")
	if err != nil {
		t.Fatalf("GetTeamInjuries error: %v", err)
	}
	if len(injuriesInit) != 0 {
		t.Errorf("expected 0 injuries initially, got %d", len(injuriesInit))
	}

	depthInit, err := repo.GetTeamDepthChart("KC")
	if err != nil {
		t.Fatalf("GetTeamDepthChart error: %v", err)
	}
	if len(depthInit) != 0 {
		t.Errorf("expected 0 depth chart slots initially, got %d", len(depthInit))
	}

	// 2. Insert test injuries for KC and SF
	kcInjuries := []*TeamInjury{
		{
			TeamCode:      "KC",
			AthleteESPNID: "4241474",
			AthleteName:   "Rashee Rice",
			Position:      "WR",
			Jersey:        "4",
			HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/4241474.png",
			Status:        "Injured Reserve",
			Comment:       "knee injury, out for regular season",
			InjuryDate:    "2026-10-01T12:00Z",
		},
		{
			TeamCode:      "KC",
			AthleteESPNID: "4428633",
			AthleteName:   "Kenneth Walker III",
			Position:      "RB",
			Jersey:        "9",
			HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/4428633.png",
			Status:        "Questionable",
			Comment:       "oblique strain, limited practice",
			InjuryDate:    "2026-10-04T18:00Z",
		},
	}

	sfInjuries := []*TeamInjury{
		{
			TeamCode:      "SF",
			AthleteESPNID: "4361529",
			AthleteName:   "Christian McCaffrey",
			Position:      "RB",
			Jersey:        "23",
			HeadshotURL:   "https://a.espncdn.com/i/headshots/nfl/players/full/4361529.png",
			Status:        "Out",
			Comment:       "calf/Achilles tendinitis",
			InjuryDate:    "2026-10-03T15:00Z",
		},
	}

	if err := repo.SaveTeamInjuries("KC", kcInjuries); err != nil {
		t.Fatalf("SaveTeamInjuries KC failed: %v", err)
	}
	if err := repo.SaveTeamInjuries("SF", sfInjuries); err != nil {
		t.Fatalf("SaveTeamInjuries SF failed: %v", err)
	}

	// 3. Query KC injuries and verify sorting (Out / Doubtful / Questionable / IR)
	kcQueried, err := repo.GetTeamInjuries("KC")
	if err != nil {
		t.Fatalf("GetTeamInjuries failed: %v", err)
	}
	if len(kcQueried) != 2 {
		t.Fatalf("expected 2 injuries for KC, got %d", len(kcQueried))
	}
	// Questionable should come before Injured Reserve based on ORDER BY CASE
	if kcQueried[0].AthleteName != "Kenneth Walker III" || kcQueried[0].Status != "Questionable" {
		t.Errorf("expected first KC injury to be Kenneth Walker III (Questionable), got %s (%s)", kcQueried[0].AthleteName, kcQueried[0].Status)
	}

	// 4. Query GetAllInjuries
	allInjuries, err := repo.GetAllInjuries()
	if err != nil {
		t.Fatalf("GetAllInjuries failed: %v", err)
	}
	if len(allInjuries) != 3 {
		t.Fatalf("expected 3 total injuries across league, got %d", len(allInjuries))
	}
	// First should be SF McCaffrey because status is 'Out' (rank 1)
	if allInjuries[0].AthleteName != "Christian McCaffrey" || allInjuries[0].Status != "Out" {
		t.Errorf("expected first overall injury to be Christian McCaffrey (Out), got %s (%s)", allInjuries[0].AthleteName, allInjuries[0].Status)
	}

	// 5. Test Depth Chart Persistence
	kcDepth := []*TeamDepthChartSlot{
		{
			TeamCode:       "KC",
			FormationGroup: "Ofensiva",
			PositionCode:   "QB",
			PositionName:   "Quarterback",
			DepthRank:      1,
			AthleteESPNID:  "3139477",
			AthleteName:    "Patrick Mahomes",
			Jersey:         "15",
			HeadshotURL:    "https://a.espncdn.com/i/headshots/nfl/players/full/3139477.png",
		},
		{
			TeamCode:       "KC",
			FormationGroup: "Ofensiva",
			PositionCode:   "QB",
			PositionName:   "Quarterback",
			DepthRank:      2,
			AthleteESPNID:  "4362887",
			AthleteName:    "Justin Fields",
			Jersey:         "2",
			HeadshotURL:    "https://a.espncdn.com/i/headshots/nfl/players/full/4362887.png",
		},
		{
			TeamCode:       "KC",
			FormationGroup: "Defensiva",
			PositionCode:   "LDT",
			PositionName:   "Left Defensive Tackle",
			DepthRank:      1,
			AthleteESPNID:  "3045353",
			AthleteName:    "Chris Jones",
			Jersey:         "95",
			HeadshotURL:    "https://a.espncdn.com/i/headshots/nfl/players/full/3045353.png",
		},
	}

	if err := repo.SaveTeamDepthChart("KC", kcDepth); err != nil {
		t.Fatalf("SaveTeamDepthChart failed: %v", err)
	}

	kcDepthQueried, err := repo.GetTeamDepthChart("KC")
	if err != nil {
		t.Fatalf("GetTeamDepthChart failed: %v", err)
	}
	if len(kcDepthQueried) != 3 {
		t.Fatalf("expected 3 depth chart slots for KC, got %d", len(kcDepthQueried))
	}
	if kcDepthQueried[0].AthleteName != "Patrick Mahomes" || kcDepthQueried[0].DepthRank != 1 {
		t.Errorf("expected starter QB Patrick Mahomes, got %s rank %d", kcDepthQueried[0].AthleteName, kcDepthQueried[0].DepthRank)
	}
}
