//go:build integration

package db_test

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/scoring"
)

// Every PostgreSQL run owns a fresh schema; never reset a caller's existing tables.
func integrationDatabase(t *testing.T, dialect string) *db.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "repository.sqlite")
	if dialect == "postgres" {
		dsn = os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			if os.Getenv("REQUIRE_POSTGRES_TESTS") == "1" {
				t.Fatal("TEST_POSTGRES_DSN is required")
			}
			t.Skip("set TEST_POSTGRES_DSN to run PostgreSQL integration tests")
		}
		control, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		schema := fmt.Sprintf("quiniela_test_%d", time.Now().UnixNano())
		if _, err := control.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
			control.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			defer control.Close()
			if _, err := control.Exec(`DROP SCHEMA "` + schema + `" CASCADE`); err != nil {
				t.Errorf("cleanup schema: %v", err)
			}
		})
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dsn = u.String()
	}
	database, err := db.InitDB(dialect, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestRepositoryDialects(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			database := integrationDatabase(t, dialect)
			r := db.NewRepository(database)
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(db.SeedDatabase(r, "integration_admin", "admin@example.invalid", "fictional-password", 2026))
			teams, err := r.ListTeams()
			must(err)
			if len(teams) != 32 {
				t.Fatalf("teams=%d", len(teams))
			}
			season, err := r.GetActiveSeason(2026)
			must(err)
			weeks, err := r.ListWeeks(season.ID)
			must(err)
			if len(weeks) != 22 {
				t.Fatalf("weeks=%d", len(weeks))
			}
			user, err := r.CreateUser("integration_player", "player@example.invalid", "fakehash", "player")
			must(err)
			other, err := r.CreateUser("other_player", "other@example.invalid", "fakehash", "player")
			must(err)
			assertUser := func(verified, beta bool) {
				t.Helper()
				u, err := r.GetUserByID(user.ID)
				must(err)
				if u.EmailVerified != verified || u.IsBetaTester != beta {
					t.Fatalf("verified/beta=%v/%v want=%v/%v", u.EmailVerified, u.IsBetaTester, verified, beta)
				}
			}
			for _, value := range []bool{true, false} {
				must(r.SetUserEmailVerified(user.ID, value))
				must(r.SetUserBetaTester(user.ID, value))
				assertUser(value, value)
			}
			ids := []int64{user.ID, other.ID}
			for _, value := range []bool{true, false} {
				must(r.BulkSetUserVerification(ids, value))
				must(r.BulkSetUserBeta(ids, value))
				assertUser(value, value)
				o, err := r.GetUserByID(other.ID)
				must(err)
				if o.EmailVerified != value || o.IsBetaTester != value {
					t.Fatal("bulk update missed second user")
				}
			}
			for _, value := range []bool{true, false} {
				passwordHash := ""
				if value {
					passwordHash = "changedhash"
				}
				must(r.AdminUpdateUser(user.ID, user.Username, user.Email, "player", value, value, nil, "integration bio", passwordHash))
				assertUser(value, value)
			}
			must(r.UpdateUserPreferences(user.ID, "", nil, false, "bio", "", false, false, false))
			u, err := r.GetUserByID(user.ID)
			must(err)
			if u.NotifyEmail || u.NotifyKickoff || u.NotifyRecap || u.NotifyPush {
				t.Fatal("notification preferences were not persisted")
			}
			must(r.SetUserEmailVerified(user.ID, true))
			for _, value := range []bool{false, true} {
				must(r.UpsertFeatureFlag(&db.FeatureFlag{Key: "integration_flag", Name: "Flag", AccessLevel: "all", IsBeta: value}))
				f, err := r.GetFeatureFlag("integration_flag")
				must(err)
				if f.IsBeta != value {
					t.Fatal("flag bool round trip")
				}
				must(r.UpdateFeatureFlag("integration_flag", "all", !value))
				f, err = r.GetFeatureFlag("integration_flag")
				must(err)
				if f.IsBeta == value {
					t.Fatal("flag update failed")
				}
			}
			flags, err := r.ListFeatureFlags()
			must(err)
			if len(flags) < 9 {
				t.Fatalf("missing seed flags: %d", len(flags))
			}
			first, err := r.CreateInAppNotification(user.ID, "First", "Message", "/picks", "test")
			must(err)
			second, err := r.CreateInAppNotification(user.ID, "Second", "Message", "/picks", "test")
			must(err)
			unread, err := r.GetUnreadNotificationsCount(user.ID)
			must(err)
			if unread != 2 {
				t.Fatalf("unread=%d", unread)
			}
			must(r.MarkNotificationAsRead(first.ID, other.ID))
			unread, err = r.GetUnreadNotificationsCount(user.ID)
			must(err)
			if unread != 2 {
				t.Fatal("notification ownership violated")
			}
			must(r.MarkNotificationAsRead(first.ID, user.ID))
			unread, err = r.GetUnreadNotificationsCount(user.ID)
			must(err)
			if unread != 1 {
				t.Fatalf("unread=%d", unread)
			}
			must(r.MarkAllNotificationsAsRead(user.ID))
			unread, err = r.GetUnreadNotificationsCount(user.ID)
			must(err)
			if unread != 0 {
				t.Fatalf("unread=%d", unread)
			}
			notes, err := r.ListUserNotifications(user.ID, 10)
			must(err)
			if len(notes) != 2 || !notes[0].IsRead || !notes[1].IsRead || second.ID == first.ID {
				t.Fatal("notifications round trip failed")
			}
			week, err := r.GetWeekByNumber(season.ID, 2)
			must(err)
			home, err := r.GetTeamByCode("KC")
			must(err)
			away, err := r.GetTeamByCode("BAL")
			must(err)
			hs, as := 24, 17
			game, err := r.CreateManualGame(&db.Game{WeekID: week.ID, HomeTeamID: home.ID, AwayTeamID: away.ID, KickoffTime: time.Now().Add(-time.Hour), HomeScore: &hs, AwayScore: &as, Status: "final", IsTiebreaker: true})
			must(err)
			pick, err := r.SavePick(user.ID, game.ID, &home.ID, &hs, &as)
			must(err)
			must(scoring.NewCalculator(r).CalculateWeekScores(week.ID))
			pick, err = r.GetUserPickForGame(user.ID, game.ID)
			must(err)
			if pick.IsCorrect == nil || !*pick.IsCorrect || pick.PointsEarned+pick.BonusPoints != 17 {
				t.Fatalf("bad scored pick: %+v", pick)
			}
			stats, err := r.GetUserStats(user.ID)
			must(err)
			if stats.TotalPicks != 1 || stats.CorrectPicks != 1 || stats.TotalPoints != 17 {
				t.Fatalf("bad profile stats: %+v", stats)
			}
			_, err = r.GetAdvancedUserStats(user.ID)
			must(err)
			weekly, err := r.GetWeeklyLeaderboard(week.ID)
			must(err)
			if len(weekly) != 1 || weekly[0].TotalPoints != 17 || !weekly[0].TiebreakerWinnerCorrect {
				t.Fatalf("weekly=%+v", weekly)
			}
			_, err = r.GetSeasonLeaderboard(season.ID)
			must(err)
			community, err := r.GetTeamCommunityStats(home.ID, 2026)
			must(err)
			if community.WinningPicks != 1 {
				t.Fatalf("community stats=%+v", community)
			}
			for _, available := range []bool{false, true} {
				must(r.SaveGameForecast(&db.GameForecast{GameID: game.ID, PredictedWinnerID: home.ID, ESPNAvailable: available}))
				forecast, err := r.GetGameForecast(game.ID)
				must(err)
				if forecast.ESPNAvailable != available {
					t.Fatal("forecast bool round trip")
				}
			}
			for _, year := range []int{2023, 2026} {
				must(r.SaveSeasonStandings(&db.SeasonStandings{Year: year, League: []*db.TeamStanding{{TeamID: home.ID, TeamCode: home.Code, Conference: home.Conference, Division: home.Division}}}))
				standings, err := r.GetSeasonStandings(year)
				must(err)
				if standings == nil || len(standings.League) != 1 || standings.League[0].IsSuperBowlChampion != (year == 2023) {
					t.Fatal("standings bool round trip")
				}
			}
			// Prepared statements must also rebind inside transactions.
			must(r.SavePlayoffProbabilities([]*db.TeamPlayoffProbability{{SeasonYear: 2026, WeekNumber: 2, TeamID: home.ID, TeamCode: home.Code, IsFavorite: true}}))
			probabilities, err := r.GetPlayoffProbabilities(2026, 2)
			must(err)
			if len(probabilities) != 1 || !probabilities[0].IsFavorite {
				t.Fatal("playoff bool round trip")
			}
			must(r.AdminDeleteUser(other.ID))
			_, err = r.GetUserByID(other.ID)
			if err == nil {
				t.Fatal("individual delete did not remove user")
			}
			must(r.BulkDeleteUsers([]int64{user.ID}))
			_, err = r.GetUserByID(user.ID)
			if err == nil {
				t.Fatal("bulk delete did not remove user")
			}
		})
	}
}

func TestRepositoryActiveWindowDialects(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			database := integrationDatabase(t, dialect)
			r := db.NewRepository(database)
			if err := db.SeedDatabase(r, "admin", "admin@example.invalid", "fictional-password", 2026); err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec("DELETE FROM games"); err != nil {
				t.Fatal(err)
			}
			s, err := r.GetActiveSeason(2026)
			if err != nil {
				t.Fatal(err)
			}
			week, err := r.GetWeekByNumber(s.ID, 8)
			if err != nil {
				t.Fatal(err)
			}
			teams, err := r.ListTeams()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.CreateManualGame(&db.Game{WeekID: week.ID, HomeTeamID: teams[0].ID, AwayTeamID: teams[1].ID, Status: "scheduled", KickoffTime: time.Now().Add(24 * time.Hour)}); err != nil {
				t.Fatal(err)
			}
			active, err := r.GetActiveWeek(s.ID)
			if err != nil {
				t.Fatal(err)
			}
			if active.WeekNumber != 8 {
				t.Fatalf("window selected=%d want8 (fallback would incorrectly return week1)", active.WeekNumber)
			}
		})
	}
}
