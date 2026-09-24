package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/espn"
)

func setupTestTeamStats(t *testing.T) (*TeamStatsHandler, func()) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	espnClient := espn.NewClient()
	handler := NewTeamStatsHandler(repo, renderer, espnClient, 2026)
	return handler, cleanup
}

func TestTeamStatsHandler_ShowTeams(t *testing.T) {
	handler, cleanup := setupTestTeamStats(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/teams?season=2026&view=division", nil)
	rr := httptest.NewRecorder()

	handler.ShowTeams(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if len(body) == 0 {
		t.Fatal("expected non-empty response body")
	}

	expectedStrings := []string{
		"Estadísticas de Equipos",
		"American Football Conference",
		"National Football Conference",
		"Por División",
		"Playoffs",
	}
	for _, exp := range expectedStrings {
		if !strings.Contains(body, exp) {
			t.Errorf("expected body to contain %q", exp)
		}
	}
}

func TestTeamStatsHandler_TeamsTablePartial(t *testing.T) {
	handler, cleanup := setupTestTeamStats(t)
	defer cleanup()

	tests := []struct {
		view     string
		expected []string
	}{
		{
			view:     "division",
			expected: []string{"AFC Este", "NFC", "División"},
		},
		{
			view:     "conference",
			expected: []string{"Línea de Clasificación a Playoffs", "Seed", "Comodín"},
		},
		{
			view:     "league",
			expected: []string{"Tabla General", "32 Equipos"},
		},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/teams/table?season=2026&view="+tt.view, nil)
		rr := httptest.NewRecorder()

		handler.TeamsTablePartial(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("view %s: expected status 200 OK, got %d. Body: %s", tt.view, rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, exp := range tt.expected {
			if !strings.Contains(body, exp) {
				t.Errorf("view %s: expected body to contain %q", tt.view, exp)
			}
		}
	}
}

func TestTeamStatsHandler_TeamDetailAndModal(t *testing.T) {
	handler, cleanup := setupTestTeamStats(t)
	defer cleanup()

	r := chi.NewRouter()
	r.Get("/teams/{code}", handler.ShowTeamDetail)
	r.Get("/teams/{code}/modal", handler.TeamDetailModal)

	// Valid team KC
	req := httptest.NewRequest(http.MethodGet, "/teams/KC?season=2026", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for KC detail, got %d. Body: %s", rr.Code, rr.Body.String())
	}
	bodyDetail := rr.Body.String()
	for _, exp := range []string{"Volver a Estadísticas", "Chiefs", "Calendario Oficial", "Récord"} {
		if !strings.Contains(bodyDetail, exp) {
			t.Errorf("KC detail: expected body to contain %q", exp)
		}
	}

	// Valid team KC modal
	reqModal := httptest.NewRequest(http.MethodGet, "/teams/KC/modal?season=2026", nil)
	rrModal := httptest.NewRecorder()
	r.ServeHTTP(rrModal, reqModal)

	if rrModal.Code != http.StatusOK {
		t.Errorf("expected 200 for KC modal, got %d. Body: %s", rrModal.Code, rrModal.Body.String())
	}
	bodyModal := rrModal.Body.String()
	for _, exp := range []string{"Ver Ficha Completa del Equipo", "Cerrar", "Chiefs"} {
		if !strings.Contains(bodyModal, exp) {
			t.Errorf("KC modal: expected body to contain %q", exp)
		}
	}

	// Invalid team
	reqInvalid := httptest.NewRequest(http.MethodGet, "/teams/INVALID_CODE", nil)
	rrInvalid := httptest.NewRecorder()
	r.ServeHTTP(rrInvalid, reqInvalid)

	if rrInvalid.Code != http.StatusNotFound {
		t.Errorf("expected 404 for invalid team, got %d", rrInvalid.Code)
	}
}

func TestTeamStatsHandler_PastSeasons(t *testing.T) {
	handler, cleanup := setupTestTeamStats(t)
	defer cleanup()

	r := chi.NewRouter()
	r.Get("/teams", handler.ShowTeams)
	r.Get("/teams/table", handler.TeamsTablePartial)
	r.Get("/teams/{code}", handler.ShowTeamDetail)
	r.Get("/teams/{code}/modal", handler.TeamDetailModal)

	// 1. /teams?season=2025 (Page)
	req := httptest.NewRequest(http.MethodGet, "/teams?season=2025&view=division", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for 2025 /teams, got %d. Body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Temporada 2025") {
		t.Errorf("expected body to contain 'Temporada 2025'")
	}
	if !strings.Contains(body, "AFC Este") {
		t.Errorf("expected body to contain division 'AFC Este'")
	}

	// 2. /teams/table?season=2025&view=conference
	reqConf := httptest.NewRequest(http.MethodGet, "/teams/table?season=2025&view=conference", nil)
	rrConf := httptest.NewRecorder()
	r.ServeHTTP(rrConf, reqConf)

	if rrConf.Code != http.StatusOK {
		t.Fatalf("expected 200 for 2025 conference table, got %d", rrConf.Code)
	}
	bodyConf := rrConf.Body.String()
	if !strings.Contains(bodyConf, "Línea de Clasificación a Playoffs") {
		t.Errorf("expected conference table to contain playoff cutoff line")
	}

	// 3. /teams/KC?season=2025 (Detail with schedule)
	reqDetail := httptest.NewRequest(http.MethodGet, "/teams/KC?season=2025", nil)
	rrDetail := httptest.NewRecorder()
	r.ServeHTTP(rrDetail, reqDetail)

	if rrDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 for 2025 KC detail, got %d", rrDetail.Code)
	}
	bodyDetail := rrDetail.Body.String()
	if !strings.Contains(bodyDetail, "Chiefs") || !strings.Contains(bodyDetail, "Temporada 2025") {
		t.Errorf("expected KC 2025 detail to contain Chiefs and Temporada 2025")
	}

	// 4. /teams/KC/modal?season=2025 (Modal)
	reqModal := httptest.NewRequest(http.MethodGet, "/teams/KC/modal?season=2025", nil)
	rrModal := httptest.NewRecorder()
	r.ServeHTTP(rrModal, reqModal)

	if rrModal.Code != http.StatusOK {
		t.Fatalf("expected 200 for 2025 KC modal, got %d", rrModal.Code)
	}
	bodyModal := rrModal.Body.String()
	if !strings.Contains(bodyModal, "Chiefs") {
		t.Errorf("expected KC 2025 modal to contain Chiefs")
	}
}

func TestTeamStatsHandler_SuperBowlChampionsAndPersistence(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()

	// Pre-seed DB standings for 2023 (Chiefs won Super Bowl LVIII)
	kc, err := repo.GetTeamByCode("KC")
	if err != nil || kc == nil {
		t.Fatalf("KC team not found: %v", err)
	}
	sf, err := repo.GetTeamByCode("SF")
	if err != nil || sf == nil {
		t.Fatalf("SF team not found: %v", err)
	}

	kcStanding := &db.TeamStanding{
		TeamID:              kc.ID,
		TeamCode:            "KC",
		TeamName:            "Chiefs",
		TeamCity:            "Kansas City",
		LogoURL:             kc.LogoURL,
		Conference:          "AFC",
		Division:            "West",
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
		SuperBowlTitle:      "Super Bowl LVIII (25-22 vs SF)",
	}

	sfStanding := &db.TeamStanding{
		TeamID:              sf.ID,
		TeamCode:            "SF",
		TeamName:            "49ers",
		TeamCity:            "San Francisco",
		LogoURL:             sf.LogoURL,
		Conference:          "NFC",
		Division:            "West",
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

	standings2023 := &db.SeasonStandings{
		Year: 2023,
		League: []*db.TeamStanding{
			sfStanding,
			kcStanding,
		},
		Conferences: []*db.ConferenceStandings{
			{Name: "American Football Conference", Conference: "AFC", Teams: []*db.TeamStanding{kcStanding}},
			{Name: "National Football Conference", Conference: "NFC", Teams: []*db.TeamStanding{sfStanding}},
		},
		Divisions: []*db.DivisionStandings{
			{Name: "AFC Oeste", Conference: "AFC", Division: "West", Teams: []*db.TeamStanding{kcStanding}},
			{Name: "NFC Oeste", Conference: "NFC", Division: "West", Teams: []*db.TeamStanding{sfStanding}},
		},
		Summary: &db.SeasonDashboardSummary{
			TopRecordTeam:     sfStanding,
			TopOffenseTeam:    sfStanding,
			TopDefenseTeam:    kcStanding,
			SuperBowlChampion: kcStanding,
		},
	}

	if err := repo.SaveSeasonStandings(standings2023); err != nil {
		t.Fatalf("SaveSeasonStandings failed: %v", err)
	}

	// Pre-seed team schedule for KC 2023
	kcScore := 25
	sfScore := 22
	testSchedule := []*db.TeamScheduleItem{
		{
			WeekNumber:       22,
			KickoffFormatted: "11 feb 2024",
			OpponentCode:     "SF",
			OpponentName:     "49ers",
			IsHome:           false,
			TeamScore:        &kcScore,
			OpponentScore:    &sfScore,
			Result:           "W",
			StatusDetail:     "Super Bowl LVIII",
		},
	}
	if err := repo.SaveTeamSchedule("KC", 2023, testSchedule); err != nil {
		t.Fatalf("SaveTeamSchedule failed: %v", err)
	}

	// Use an ESPN client pointing to an unreachable dummy URL to prove pure offline DB usage
	offlineClient := espn.NewClientWithCustomURL("http://127.0.0.1:9", "http://127.0.0.1:9")
	handler := NewTeamStatsHandler(repo, renderer, offlineClient, 2026)

	r := chi.NewRouter()
	r.Get("/teams", handler.ShowTeams)
	r.Get("/teams/table", handler.TeamsTablePartial)
	r.Get("/teams/{code}", handler.ShowTeamDetail)
	r.Get("/teams/{code}/modal", handler.TeamDetailModal)

	// 1. Check /teams?season=2023 (Loads from DB without hitting ESPN)
	req := httptest.NewRequest(http.MethodGet, "/teams?season=2023&view=division", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from DB standings, got %d. Body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, exp := range []string{"Campeón Super Bowl", "Chiefs", "Super Bowl LVIII", "Campeón"} {
		if !strings.Contains(body, exp) {
			t.Errorf("expected body to contain %q", exp)
		}
	}

	// 2. Check /teams/table?season=2023&view=conference
	reqConf := httptest.NewRequest(http.MethodGet, "/teams/table?season=2023&view=conference", nil)
	rrConf := httptest.NewRecorder()
	r.ServeHTTP(rrConf, reqConf)

	if rrConf.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for conference table, got %d", rrConf.Code)
	}
	if !strings.Contains(rrConf.Body.String(), "Campeón") {
		t.Errorf("expected conference table to contain 'Campeón' badge")
	}

	// 3. Check /teams/table?season=2023&view=league
	reqLeague := httptest.NewRequest(http.MethodGet, "/teams/table?season=2023&view=league", nil)
	rrLeague := httptest.NewRecorder()
	r.ServeHTTP(rrLeague, reqLeague)

	if rrLeague.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for league table, got %d", rrLeague.Code)
	}
	if !strings.Contains(rrLeague.Body.String(), "Campeón") {
		t.Errorf("expected league table to contain 'Campeón' badge")
	}

	// 4. Check /teams/KC?season=2023 (Team Detail from DB)
	reqDetail := httptest.NewRequest(http.MethodGet, "/teams/KC?season=2023", nil)
	rrDetail := httptest.NewRecorder()
	r.ServeHTTP(rrDetail, reqDetail)

	if rrDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for KC detail, got %d", rrDetail.Code)
	}
	bodyDetail := rrDetail.Body.String()
	if !strings.Contains(bodyDetail, "Campeón de Super Bowl") || !strings.Contains(bodyDetail, "LVIII") {
		t.Errorf("expected KC detail to show Super Bowl champion badge")
	}

	// 5. Check /teams/KC/modal?season=2023 (Modal from DB)
	reqModal := httptest.NewRequest(http.MethodGet, "/teams/KC/modal?season=2023", nil)
	rrModal := httptest.NewRecorder()
	r.ServeHTTP(rrModal, reqModal)

	if rrModal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for KC modal, got %d", rrModal.Code)
	}
	bodyModal := rrModal.Body.String()
	if !strings.Contains(bodyModal, "Super Bowl LVIII") {
		t.Errorf("expected KC modal to show Super Bowl LVIII badge")
	}
}



