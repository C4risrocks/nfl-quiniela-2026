package espn

import (
	"testing"

	"nfl-quiniela-2026/db"
)

func TestInjuriesAndDepthMapping(t *testing.T) {
	// Verify Team ID mappings for key franchises
	cases := []struct {
		code   string
		espnID string
	}{
		{"KC", "12"},
		{"SF", "25"},
		{"BAL", "33"},
		{"DAL", "6"},
		{"PHI", "21"},
		{"BUF", "2"},
		{"DET", "8"},
		{"WSH", "28"},
		{"WAS", "28"}, // Alias
		{"JAX", "30"},
		{"JAC", "30"}, // Alias
	}

	for _, c := range cases {
		id := GetESPNTeamID(c.code)
		if id != c.espnID {
			t.Errorf("expected team %s to map to ESPN ID %s, got %s", c.code, c.espnID, id)
		}
	}
}

func TestInjuriesFallbackAndGrouping(t *testing.T) {
	client := NewClient()

	// 1. Test Fallback Injuries
	fallInjuries := client.getFallbackAllInjuries()
	if len(fallInjuries) == 0 {
		t.Fatal("expected non-empty fallback injuries")
	}

	kcInjuries, ok := fallInjuries["KC"]
	if !ok || len(kcInjuries) == 0 {
		t.Fatal("expected KC injuries in fallback")
	}

	foundRice := false
	for _, inj := range kcInjuries {
		if inj.AthleteName == "Rashee Rice" {
			foundRice = true
			if inj.Status != "Injured Reserve" {
				t.Errorf("expected Rashee Rice status to be Injured Reserve, got %s", inj.Status)
			}
		}
	}
	if !foundRice {
		t.Error("expected Rashee Rice in KC injuries")
	}

	// 2. Test Fallback Depth Chart
	kcDepth := client.getFallbackDepthChart("KC")
	if len(kcDepth) == 0 {
		t.Fatal("expected non-empty KC depth chart")
	}

	// 3. Test GroupDepthChartByFormation
	formations := GroupDepthChartByFormation(kcDepth)
	if len(formations) == 0 {
		t.Fatal("expected non-empty formations")
	}

	foundOffense := false
	for _, f := range formations {
		if f.GroupName == "Ofensiva" {
			foundOffense = true
			if len(f.Positions) == 0 {
				t.Error("expected positions in Ofensiva")
			}

			// First position in offense should be QB
			if f.Positions[0].PositionCode != "QB" {
				t.Errorf("expected first offense position to be QB, got %s", f.Positions[0].PositionCode)
			}
			if len(f.Positions[0].Starters) == 0 || f.Positions[0].Starters[0].AthleteName != "Patrick Mahomes" {
				t.Errorf("expected Patrick Mahomes to be starter QB")
			}
		}
	}
	if !foundOffense {
		t.Error("expected Ofensiva formation group")
	}
}

func TestDepthChartCrossReference(t *testing.T) {
	// Verify that injury status attaches cleanly to depth chart slot
	slot := &db.TeamDepthChartSlot{
		TeamCode:       "KC",
		FormationGroup: "Ofensiva",
		PositionCode:   "WR",
		PositionName:   "Wide Receiver",
		DepthRank:      1,
		AthleteName:    "Rashee Rice",
	}

	inj := &db.TeamInjury{
		TeamCode:    "KC",
		AthleteName: "Rashee Rice",
		Status:      "Injured Reserve",
		Comment:     "Knee",
	}

	slot.InjuryStatus = inj

	if slot.InjuryStatus == nil || slot.InjuryStatus.Status != "Injured Reserve" {
		t.Fatalf("expected attached injury status Injured Reserve")
	}
}
