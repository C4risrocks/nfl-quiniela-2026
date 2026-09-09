package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"nfl-quiniela-2026/services/events"
)

type EventsHandler struct {
	broker *events.Broker
}

func NewEventsHandler(broker *events.Broker) *EventsHandler {
	return &EventsHandler{
		broker: broker,
	}
}

func (h *EventsHandler) StreamLiveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Crucial for Traefik / Nginx reverse proxies

	ch := h.broker.Subscribe()
	defer h.broker.Unsubscribe(ch)

	// Send initial connected event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\": \"ready\"}\n\n")
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			// Format multiline SSE data properly
			lines := strings.Split(ev.Data, "\n")
			fmt.Fprintf(w, "event: %s\n", ev.Name)
			for _, line := range lines {
				fmt.Fprintf(w, "data: %s\n", line)
			}
			fmt.Fprint(w, "\n")
			flusher.Flush()
		}
	}
}
