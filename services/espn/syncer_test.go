package espn

import (
	"encoding/json"
	"testing"
	"time"

	"nfl-quiniela-2026/services/events"
)

func TestSyncerBroadcastAlert(t *testing.T) {
	broker := events.NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	syncer := &Syncer{
		broker: broker,
	}

	alert := LiveAlert{
		Type:      "game_start",
		Title:     "¡Kickoff!",
		Message:   "KC @ BAL ha iniciado",
		GameID:    42,
		AwayCode:  "KC",
		HomeCode:  "BAL",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	syncer.broadcastAlert(alert)

	select {
	case ev := <-ch:
		if ev.Name != "live-alert" {
			t.Fatalf("Expected event name 'live-alert', got '%s'", ev.Name)
		}
		var received LiveAlert
		if err := json.Unmarshal([]byte(ev.Data), &received); err != nil {
			t.Fatalf("Failed to unmarshal alert payload: %v", err)
		}
		if received.GameID != 42 || received.Type != "game_start" || received.AwayCode != "KC" {
			t.Fatalf("Unexpected alert content: %+v", received)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timed out waiting for live-alert event")
	}
}

func TestWeekUpdatedPayloadFormat(t *testing.T) {
	broker := events.NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	type gameUpdateInfo struct {
		ID           int64  `json:"id"`
		Status       string `json:"status"`
		StatusDetail string `json:"status_detail"`
		HomeScore    int    `json:"home_score"`
		AwayScore    int    `json:"away_score"`
	}

	gameUpdates := []gameUpdateInfo{
		{
			ID:           101,
			Status:       "in_progress",
			StatusDetail: "2Q 05:42",
			HomeScore:    14,
			AwayScore:    10,
		},
		{
			ID:           102,
			Status:       "final",
			StatusDetail: "Final",
			HomeScore:    24,
			AwayScore:    27,
		},
	}

	weekPayload, err := json.Marshal(map[string]interface{}{
		"week_num": 1,
		"games":    gameUpdates,
	})
	if err != nil {
		t.Fatalf("Failed to marshal week payload: %v", err)
	}

	broker.Broadcast("week-updated", string(weekPayload))

	select {
	case ev := <-ch:
		if ev.Name != "week-updated" {
			t.Fatalf("Expected event name 'week-updated', got '%s'", ev.Name)
		}
		var received struct {
			WeekNum int              `json:"week_num"`
			Games   []gameUpdateInfo `json:"games"`
		}
		if err := json.Unmarshal([]byte(ev.Data), &received); err != nil {
			t.Fatalf("Failed to unmarshal week-updated payload: %v", err)
		}
		if received.WeekNum != 1 {
			t.Fatalf("Expected week_num 1, got %d", received.WeekNum)
		}
		if len(received.Games) != 2 {
			t.Fatalf("Expected 2 games, got %d", len(received.Games))
		}
		if received.Games[0].ID != 101 || received.Games[0].HomeScore != 14 || received.Games[0].AwayScore != 10 {
			t.Fatalf("Game 101 data mismatch: %+v", received.Games[0])
		}
		if received.Games[1].ID != 102 || received.Games[1].Status != "final" {
			t.Fatalf("Game 102 data mismatch: %+v", received.Games[1])
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timed out waiting for week-updated event")
	}
}
