package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestActiveWeekSelectionPrioritiesAndWindowBoundaries(t *testing.T) {
	base := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	type fixture struct {
		week    int
		status  string
		kickoff time.Time
	}
	cases := []struct {
		name              string
		now               time.Time
		active, completed int
		games             []fixture
		want              int
	}{
		{name: "explicit status precedes live games", now: base, active: 2, games: []fixture{{3, "in_progress", base}}, want: 2},
		{name: "live games precede window", now: base, games: []fixture{{3, "in_progress", base}, {2, "scheduled", base.Add(time.Hour)}}, want: 3},
		{name: "Monday recap holds until Tuesday CDMX", now: time.Date(2026, 10, 27, 3, 0, 0, 0, time.UTC), games: []fixture{{2, "final", time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC)}, {8, "scheduled", time.Date(2026, 10, 27, 4, 0, 0, 0, time.UTC)}}, want: 2},
		{name: "Tuesday midnight CDMX rolls to current window", now: time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC), games: []fixture{{2, "final", time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC)}, {8, "scheduled", time.Date(2026, 10, 27, 7, 0, 0, 0, time.UTC)}}, want: 8},
		{name: "window precedes monotonic scheduled fallback", now: base, games: []fixture{{8, "scheduled", base.Add(24 * time.Hour)}}, want: 8},
		{name: "window lower boundary inclusive", now: base, games: []fixture{{8, "scheduled", base.Add(48 * time.Hour)}}, want: 8},
		{name: "before window falls back without skipping", now: base, games: []fixture{{8, "scheduled", base.Add(48*time.Hour + time.Second)}}, want: 1},
		{name: "window upper boundary inclusive", now: base, games: []fixture{{8, "scheduled", base.Add(-24 * time.Hour)}}, want: 8},
		{name: "after window falls back without skipping", now: base, games: []fixture{{8, "scheduled", base.Add(-24*time.Hour - time.Second)}}, want: 1},
		{name: "overlapping windows choose earliest week", now: base, games: []fixture{{8, "scheduled", base.Add(time.Hour)}, {3, "scheduled", base.Add(2 * time.Hour)}}, want: 3},
		{name: "completed week excluded from window", now: base, completed: 8, games: []fixture{{8, "scheduled", base.Add(time.Hour)}}, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database, err := InitDB("sqlite", filepath.Join(t.TempDir(), "active.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			r := NewRepository(database)
			if err := SeedDatabase(r, "admin", "admin@example.invalid", "fictional-password", 2026); err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec("DELETE FROM games"); err != nil {
				t.Fatal(err)
			}
			season, err := r.GetActiveSeason(2026)
			if err != nil {
				t.Fatal(err)
			}
			if tc.active != 0 {
				week, err := r.GetWeekByNumber(season.ID, tc.active)
				if err != nil {
					t.Fatal(err)
				}
				if err := r.UpdateWeekStatus(week.ID, "active"); err != nil {
					t.Fatal(err)
				}
			}
			if tc.completed != 0 {
				week, err := r.GetWeekByNumber(season.ID, tc.completed)
				if err != nil {
					t.Fatal(err)
				}
				if err := r.UpdateWeekStatus(week.ID, "completed"); err != nil {
					t.Fatal(err)
				}
			}
			teams, err := r.ListTeams()
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range tc.games {
				week, err := r.GetWeekByNumber(season.ID, g.week)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := r.CreateManualGame(&Game{WeekID: week.ID, HomeTeamID: teams[0].ID, AwayTeamID: teams[1].ID, Status: g.status, KickoffTime: g.kickoff}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := r.getActiveWeekAt(season.ID, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			if got.WeekNumber != tc.want {
				t.Fatalf("active week=%d want=%d", got.WeekNumber, tc.want)
			}
		})
	}
}
