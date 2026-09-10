package db

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func SeedDatabase(repo *Repository, adminUser, adminEmail, adminPass string, seasonYear int) error {
	log.Println("[DB] Seeding default database data...")

	// 1. Seed Scoring Settings (only if not already set)
	if val, _ := repo.GetSetting("scoring_mode", ""); val == "" {
		_ = repo.SetSetting("scoring_mode", "weighted")
	}
	if val, _ := repo.GetSetting("winner_points", ""); val == "" {
		_ = repo.SetSetting("winner_points", "10")
	}
	if val, _ := repo.GetSetting("exact_score_bonus", ""); val == "" {
		_ = repo.SetSetting("exact_score_bonus", "5")
	}
	if val, _ := repo.GetSetting("margin_bonus", ""); val == "" {
		_ = repo.SetSetting("margin_bonus", "2")
	}
	if val, _ := repo.GetSetting("lock_mode", ""); val == "" {
		_ = repo.SetSetting("lock_mode", "per_game")
	}

	// 2. Seed Default Admin User if no admin exists
	admin, err := repo.GetUserByUsername(adminUser)
	if err != nil || admin == nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hashing admin password: %w", err)
		}
		_, err = repo.CreateUser(adminUser, adminEmail, string(hash), "admin")
		if err != nil {
			log.Printf("[DB] Note on admin creation: %v", err)
		} else {
			log.Printf("[DB] Created default admin account: %s", adminUser)
		}
	}

	// 3. Seed 32 NFL Teams
	teams := []Team{
		// AFC East
		{Code: "BUF", Name: "Bills", City: "Buffalo", Conference: "AFC", Division: "East", PrimaryColor: "#00338D", SecondaryColor: "#C60C30", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/buf.png"},
		{Code: "MIA", Name: "Dolphins", City: "Miami", Conference: "AFC", Division: "East", PrimaryColor: "#008E97", SecondaryColor: "#FC4C02", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/mia.png"},
		{Code: "NE", Name: "Patriots", City: "New England", Conference: "AFC", Division: "East", PrimaryColor: "#002244", SecondaryColor: "#C60C30", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/ne.png"},
		{Code: "NYJ", Name: "Jets", City: "New York", Conference: "AFC", Division: "East", PrimaryColor: "#125740", SecondaryColor: "#000000", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/nyj.png"},
		// AFC North
		{Code: "BAL", Name: "Ravens", City: "Baltimore", Conference: "AFC", Division: "North", PrimaryColor: "#241773", SecondaryColor: "#000000", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/bal.png"},
		{Code: "CIN", Name: "Bengals", City: "Cincinnati", Conference: "AFC", Division: "North", PrimaryColor: "#FB4F14", SecondaryColor: "#000000", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/cin.png"},
		{Code: "CLE", Name: "Browns", City: "Cleveland", Conference: "AFC", Division: "North", PrimaryColor: "#311D00", SecondaryColor: "#FF3C00", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/cle.png"},
		{Code: "PIT", Name: "Steelers", City: "Pittsburgh", Conference: "AFC", Division: "North", PrimaryColor: "#FFB612", SecondaryColor: "#101820", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/pit.png"},
		// AFC South
		{Code: "HOU", Name: "Texans", City: "Houston", Conference: "AFC", Division: "South", PrimaryColor: "#03202F", SecondaryColor: "#A71930", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/hou.png"},
		{Code: "IND", Name: "Colts", City: "Indianapolis", Conference: "AFC", Division: "South", PrimaryColor: "#002C5F", SecondaryColor: "#A2AAAD", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/ind.png"},
		{Code: "JAX", Name: "Jaguars", City: "Jacksonville", Conference: "AFC", Division: "South", PrimaryColor: "#006778", SecondaryColor: "#D7A22A", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/jax.png"},
		{Code: "TEN", Name: "Titans", City: "Tennessee", Conference: "AFC", Division: "South", PrimaryColor: "#0C2340", SecondaryColor: "#4B92DB", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/ten.png"},
		// AFC West
		{Code: "DEN", Name: "Broncos", City: "Denver", Conference: "AFC", Division: "West", PrimaryColor: "#FB4F14", SecondaryColor: "#002244", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/den.png"},
		{Code: "KC", Name: "Chiefs", City: "Kansas City", Conference: "AFC", Division: "West", PrimaryColor: "#E31837", SecondaryColor: "#FFB81C", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/kc.png"},
		{Code: "LV", Name: "Raiders", City: "Las Vegas", Conference: "AFC", Division: "West", PrimaryColor: "#000000", SecondaryColor: "#A5ACAF", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/lv.png"},
		{Code: "LAC", Name: "Chargers", City: "Los Angeles", Conference: "AFC", Division: "West", PrimaryColor: "#0080C6", SecondaryColor: "#FFC20E", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/lac.png"},

		// NFC East
		{Code: "DAL", Name: "Cowboys", City: "Dallas", Conference: "NFC", Division: "East", PrimaryColor: "#003594", SecondaryColor: "#041E42", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/dal.png"},
		{Code: "NYG", Name: "Giants", City: "New York", Conference: "NFC", Division: "East", PrimaryColor: "#0B2265", SecondaryColor: "#A71930", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/nyg.png"},
		{Code: "PHI", Name: "Eagles", City: "Philadelphia", Conference: "NFC", Division: "East", PrimaryColor: "#004C54", SecondaryColor: "#A5ACAF", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/phi.png"},
		{Code: "WSH", Name: "Commanders", City: "Washington", Conference: "NFC", Division: "East", PrimaryColor: "#5A1414", SecondaryColor: "#FFB612", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/wsh.png"},
		// NFC North
		{Code: "CHI", Name: "Bears", City: "Chicago", Conference: "NFC", Division: "North", PrimaryColor: "#0B162A", SecondaryColor: "#C83803", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/chi.png"},
		{Code: "DET", Name: "Lions", City: "Detroit", Conference: "NFC", Division: "North", PrimaryColor: "#0076B6", SecondaryColor: "#B0B7BC", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/det.png"},
		{Code: "GB", Name: "Packers", City: "Green Bay", Conference: "NFC", Division: "North", PrimaryColor: "#203731", SecondaryColor: "#FFB612", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/gb.png"},
		{Code: "MIN", Name: "Vikings", City: "Minnesota", Conference: "NFC", Division: "North", PrimaryColor: "#4F2683", SecondaryColor: "#FFC62F", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/min.png"},
		// NFC South
		{Code: "ATL", Name: "Falcons", City: "Atlanta", Conference: "NFC", Division: "South", PrimaryColor: "#A71930", SecondaryColor: "#000000", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/atl.png"},
		{Code: "CAR", Name: "Panthers", City: "Carolina", Conference: "NFC", Division: "South", PrimaryColor: "#0085CA", SecondaryColor: "#101820", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/car.png"},
		{Code: "NO", Name: "Saints", City: "New Orleans", Conference: "NFC", Division: "South", PrimaryColor: "#D3BC8D", SecondaryColor: "#101820", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/no.png"},
		{Code: "TB", Name: "Buccaneers", City: "Tampa Bay", Conference: "NFC", Division: "South", PrimaryColor: "#D50A0A", SecondaryColor: "#0A0A08", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/tb.png"},
		// NFC West
		{Code: "ARI", Name: "Cardinals", City: "Arizona", Conference: "NFC", Division: "West", PrimaryColor: "#97233F", SecondaryColor: "#000000", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/ari.png"},
		{Code: "LAR", Name: "Rams", City: "Los Angeles", Conference: "NFC", Division: "West", PrimaryColor: "#003594", SecondaryColor: "#FFA300", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/lar.png"},
		{Code: "SF", Name: "49ers", City: "San Francisco", Conference: "NFC", Division: "West", PrimaryColor: "#AA0000", SecondaryColor: "#B3995D", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/sf.png"},
		{Code: "SEA", Name: "Seahawks", City: "Seattle", Conference: "NFC", Division: "West", PrimaryColor: "#002244", SecondaryColor: "#69BE28", LogoURL: "https://a.espncdn.com/i/teamlogos/nfl/500/sea.png"},
	}

	for _, team := range teams {
		t := team
		if err := repo.UpsertTeam(&t); err != nil {
			log.Printf("[DB] Error upserting team %s: %v", team.Code, err)
		}
	}

	// 4. Seed Season and 18 Weeks + Postseason
	season, err := repo.GetActiveSeason(seasonYear)
	if err != nil {
		return fmt.Errorf("getting active season: %w", err)
	}

	existingWeeks, err := repo.ListWeeks(season.ID)
	if err == nil && len(existingWeeks) == 0 {
		log.Printf("[DB] Seeding 18 regular season weeks and playoffs for season %d...", seasonYear)
		for i := 1; i <= 18; i++ {
			name := fmt.Sprintf("Semana %d", i)
			_, _ = repo.CreateWeek(season.ID, i, name)
		}
		// Postseason
		_, _ = repo.CreateWeek(season.ID, 19, "Wild Card")
		_, _ = repo.CreateWeek(season.ID, 20, "Divisional")
		_, _ = repo.CreateWeek(season.ID, 21, "Campeonato de Conferencia")
		_, _ = repo.CreateWeek(season.ID, 22, "Super Bowl LXI")
	}

	// 5. Seed official matchups for Week 1 (and purge outdated dummy seed games)
	weeks, _ := repo.ListWeeks(season.ID)
	if len(weeks) > 0 {
		week1 := weeks[0]
		// Purge any legacy placeholder games from previous seed versions
		_ = repo.DeletePlaceholderSeedGames(week1.ID)

		g1, _ := repo.ListGamesByWeek(week1.ID)
		if len(g1) == 0 {
			log.Printf("[DB] Seeding official 2026 Week 1 NFL matchups...")
			matchups := []struct {
				awayCode, homeCode string
				espnID             string
				kickoffStr         string
				isTiebreaker       bool
			}{
				{"NE", "SEA", "401872656", "2026-09-10T00:20:00Z", false},
				{"SF", "LAR", "401872657", "2026-09-11T00:35:00Z", false},
				{"TB", "CIN", "401872925", "2026-09-13T17:00:00Z", false},
				{"NO", "DET", "401872923", "2026-09-13T17:00:00Z", false},
				{"NYJ", "TEN", "401872924", "2026-09-13T17:00:00Z", false},
				{"BAL", "IND", "401872659", "2026-09-13T17:00:00Z", false},
				{"ATL", "PIT", "401872658", "2026-09-13T17:00:00Z", false},
				{"CHI", "CAR", "401872661", "2026-09-13T17:00:00Z", false},
				{"CLE", "JAX", "401872922", "2026-09-13T17:00:00Z", false},
				{"BUF", "HOU", "401872660", "2026-09-13T17:00:00Z", false},
				{"MIA", "LV", "401872928", "2026-09-13T20:25:00Z", false},
				{"GB", "MIN", "401872927", "2026-09-13T20:25:00Z", false},
				{"WSH", "PHI", "401872929", "2026-09-13T20:25:00Z", false},
				{"ARI", "LAC", "401872926", "2026-09-13T20:25:00Z", false},
				{"DAL", "NYG", "401872930", "2026-09-14T00:20:00Z", false},
				{"DEN", "KC", "401872931", "2026-09-15T00:15:00Z", true},
			}

			for _, m := range matchups {
				homeTeam, _ := repo.GetTeamByCode(m.homeCode)
				awayTeam, _ := repo.GetTeamByCode(m.awayCode)
				t, err := time.Parse(time.RFC3339, m.kickoffStr)
				if err != nil {
					t = time.Now()
				}
				if homeTeam != nil && awayTeam != nil {
					_, _ = repo.CreateManualGame(&Game{
						WeekID:       week1.ID,
						ESPNGameID:   m.espnID,
						HomeTeamID:   homeTeam.ID,
						AwayTeamID:   awayTeam.ID,
						KickoffTime:  t,
						Status:       "scheduled",
						StatusDetail: "Programado",
						IsTiebreaker: m.isTiebreaker,
					})
				}
			}
		}
	}

	log.Println("[DB] Seeding completed successfully.")
	return nil
}
