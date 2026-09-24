package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

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


