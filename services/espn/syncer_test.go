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
