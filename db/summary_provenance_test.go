package db

import "testing"

func TestSummaryProvenancePreservesHistoricalData(t *testing.T) {
	cases := []struct {
		name, raw string
		source    StatsSource
	}{
		{"legacy unknown", `{"has_stats":true,"venue_info":{"name":"Historical venue"},"has_simulated_win_prob":true}`, StatsSourceUnknown},
		{"legacy simulation", `{"has_stats":true,"is_simulated_fallback":true}`, StatsSourceSimulated},
		{"official", `{"source":"espn","has_stats":true}`, StatsSourceESPN},
		{"simulation", `{"source":"simulated","has_stats":true}`, StatsSourceSimulated},
		{"unrecognized", `{"source":"unrecognized","has_stats":true}`, StatsSourceUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			game := &Game{StatsJSON: tc.raw}
			summary := game.DetailedSummary()
			if summary == nil || summary.Source != tc.source || summary.EffectiveSource() != tc.source {
				t.Fatalf("summary source = %#v, want %s", summary, tc.source)
			}
			if !summary.HasStats || game.StatsJSON != tc.raw {
				t.Fatal("historical cached data was modified")
			}
		})
	}
}
