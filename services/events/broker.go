package events

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type Event struct {
	Name string // e.g. "game-updated", "leaderboard-updated", "ping"
	Data string // Payload / HTML snippet
}

type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan Event]struct{}),
	}
}

// Subscribe creates a buffered event channel for a connected client
func (b *Broker) Subscribe() chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 64)
	b.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes and closes a client channel safely
func (b *Broker) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.subscribers[ch]; exists {
		delete(b.subscribers, ch)
		close(ch)
	}
}

// Broadcast sends an event to all active subscribers without blocking
func (b *Broker) Broadcast(name, data string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ev := Event{Name: name, Data: data}
	for ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
			// Client channel full or slow consumer, drop to prevent backpressure
		}
	}
}

// BroadcastGameUpdate notifies all connected clients that a game score or clock has updated
func (b *Broker) BroadcastGameUpdate(gameID int64, htmlCard string) {
	eventName := fmt.Sprintf("game-%d", gameID)
	b.Broadcast(eventName, htmlCard)
	// Also trigger generic week refresh if needed
	b.Broadcast("game-updated", fmt.Sprintf(`{"game_id": %d}`, gameID))
}

// BroadcastLeaderboardUpdate signals that standings have changed and should refresh
func (b *Broker) BroadcastLeaderboardUpdate() {
	b.Broadcast("leaderboard-updated", `{"action": "refresh"}`)
}

// StartHeartbeat sends periodic ping events every interval to keep SSE streams alive through reverse proxies
func (b *Broker) StartHeartbeat(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.Broadcast("ping", fmt.Sprintf(`{"timestamp": "%s"}`, time.Now().UTC().Format(time.RFC3339)))
			}
		}
	}()
	log.Printf("[Events] SSE Heartbeat broker started (interval %v).", interval)
}
