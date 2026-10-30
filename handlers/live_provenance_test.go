package handlers

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"nfl-quiniela-2026/db"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLiveStatsPreservesPartialOfficialAndUnknownSummaries(t *testing.T) {
	repo, _, renderer, _, cleanup := setupTestApp(t)
	defer cleanup()
	season, _ := repo.GetActiveSeason(2026)
	week, _ := repo.GetWeekByNumber(season.ID, 3)
	kc, _ := repo.GetTeamByCode("KC")
	buf, _ := repo.GetTeamByCode("BUF")
	handler := NewLiveHandler(repo, renderer, nil, 2026)
	cases := []struct{ name, raw, badge string }{
		{"partial official", `{"source":"espn","has_stats":false,"has_leaders":false,"away_stats":{"team_code":"BUF"}}`, "Fuente de estadísticas: ESPN."},
		{"historical unknown", `{"has_stats":true,"away_stats":{"team_code":"BUF"},"home_stats":{"team_code":"KC"}}`, "Procedencia sin verificar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			game, err := repo.CreateManualGame(&db.Game{WeekID: week.ID, HomeTeamID: kc.ID, AwayTeamID: buf.ID, KickoffTime: time.Now().Add(-time.Hour), Status: "final"})
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.UpdateGameStatsJSON(game.ID, tc.raw); err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			router.Get("/games/{gameId}/stats", handler.GameStatsModal)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/games/"+strconv.FormatInt(game.ID, 10)+"/stats", nil))
			if rr.Code != 200 || !strings.Contains(rr.Body.String(), tc.badge) {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "Datos oficiales en vivo") || strings.Contains(rr.Body.String(), "ESPN FPI") {
				t.Fatal("summary claims unverified official provenance")
			}
			loaded, err := repo.GetGameByID(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.StatsJSON != tc.raw {
				t.Fatal("cached summary was replaced or polluted by simulation")
			}
			summary := loaded.DetailedSummary()
			if summary.HasPlayerStats || summary.VenueInfo != nil || summary.HasLeaders || summary.HasWinProb {
				t.Fatal("missing fields were filled with invented data")
			}
		})
	}
}
