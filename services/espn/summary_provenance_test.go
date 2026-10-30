package espn

import (
	"io"
	"net/http"
	"nfl-quiniela-2026/db"
	"strings"
	"testing"
)

type provenanceTransport func(*http.Request) (*http.Response, error)

func (f provenanceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchedSummaryHasOfficialProvenanceWithoutSyntheticDetails(t *testing.T) {
	client := NewClient()
	client.httpClient = &http.Client{Transport: provenanceTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"boxscore":{"teams":[{"team":{"abbreviation":"BUF"},"statistics":[{"name":"totalYards","displayValue":"101"}]},{"team":{"abbreviation":"KC"},"statistics":[{"name":"totalYards","displayValue":"202"}]}]}}`)), Header: make(http.Header)}, nil
	})}
	summary, err := client.FetchGameSummary("provenance-official-unit-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Source != db.StatsSourceESPN || !summary.HasStats {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.HasWinProb || summary.HasLeaders || summary.VenueInfo != nil || summary.HasPlayerStats {
		t.Fatal("missing feed fields must stay absent rather than fabricated")
	}
}

func TestFallbackSummaryIsExplicitlySimulated(t *testing.T) {
	summary := GenerateRealisticSummary(&db.Game{ID: 101, HomeTeam: &db.Team{Code: "KC"}, AwayTeam: &db.Team{Code: "BUF"}})
	if summary == nil || summary.Source != db.StatsSourceSimulated || summary.EffectiveSource() != db.StatsSourceSimulated {
		t.Fatalf("unmarked simulated summary: %#v", summary)
	}
}
