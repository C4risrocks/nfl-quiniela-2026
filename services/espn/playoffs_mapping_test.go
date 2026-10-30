package espn

import (
	"testing"
)

func TestMapQuinielaWeekToESPNWeek(t *testing.T) {
	tests := []struct {
		weekNum    int
		wantESPNWk int
		wantType   int
	}{
		{weekNum: 1, wantESPNWk: 1, wantType: 2},
		{weekNum: 9, wantESPNWk: 9, wantType: 2},
		{weekNum: 18, wantESPNWk: 18, wantType: 2},
		{weekNum: 19, wantESPNWk: 1, wantType: 3}, // Wild Card
		{weekNum: 20, wantESPNWk: 2, wantType: 3}, // Divisional
		{weekNum: 21, wantESPNWk: 3, wantType: 3}, // Conference Championship
		{weekNum: 22, wantESPNWk: 5, wantType: 3}, // Super Bowl (skipping week 4 Pro Bowl)
	}

	for _, tt := range tests {
		gotWk, gotType := MapQuinielaWeekToESPNWeek(tt.weekNum)
		if gotWk != tt.wantESPNWk || gotType != tt.wantType {
			t.Errorf("MapQuinielaWeekToESPNWeek(%d) = (espnWk: %d, type: %d), want (espnWk: %d, type: %d)",
				tt.weekNum, gotWk, gotType, tt.wantESPNWk, tt.wantType)
		}
	}
}
