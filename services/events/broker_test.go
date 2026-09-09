package events

import (
	"context"
	"testing"
	"time"
)

func TestBrokerSubscribeAndBroadcast(t *testing.T) {
	b := NewBroker()

	ch1 := b.Subscribe()
	ch2 := b.Subscribe()

	b.Broadcast("test-event", "hello world")

	select {
	case ev := <-ch1:
		if ev.Name != "test-event" || ev.Data != "hello world" {
			t.Errorf("Unexpected event in ch1: %+v", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for event in ch1")
	}

	select {
	case ev := <-ch2:
		if ev.Name != "test-event" || ev.Data != "hello world" {
			t.Errorf("Unexpected event in ch2: %+v", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for event in ch2")
	}

	// Unsubscribe ch1
	b.Unsubscribe(ch1)

	b.Broadcast("after-unsub", "ping")

	// ch2 should receive
	select {
	case ev := <-ch2:
		if ev.Name != "after-unsub" {
			t.Errorf("Unexpected event: %s", ev.Name)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for event in ch2")
	}

	// ch1 is closed
	_, ok := <-ch1
	if ok {
		t.Error("Expected ch1 to be closed")
	}

	// Heartbeat test
	ctx, cancel := context.WithCancel(context.Background())
	b.StartHeartbeat(ctx, 50*time.Millisecond)
	select {
	case ev := <-ch2:
		if ev.Name != "ping" {
			t.Errorf("Expected ping event, got %s", ev.Name)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for heartbeat ping")
	}
	cancel()
}
