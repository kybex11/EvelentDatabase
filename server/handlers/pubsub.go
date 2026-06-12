package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// PubSubHandler serves:
//
//	POST /api/pubsub/<channel>            publish {"message":"..."} -> {"receivers":N}
//	GET  /api/pubsub/<channel>/subscribe  Server-Sent Events stream of messages
func PubSubHandler(w http.ResponseWriter, r *http.Request) {
	broker := database.PubSub()
	segs := pathSegs(r.URL.Path, "/api/pubsub/")
	if len(segs) == 0 {
		http.Error(w, "channel required", http.StatusBadRequest)
		return
	}
	channel := segs[0]

	// Subscribe: GET /api/pubsub/<channel>/subscribe (SSE)
	if len(segs) == 2 && segs[1] == "subscribe" && r.Method == http.MethodGet {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		sub := broker.Subscribe(channel)
		defer broker.Unsubscribe(channel, sub.ID)

		// initial comment so clients know the stream is open
		fmt.Fprintf(w, ": subscribed to %s\n\n", channel)
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub.C:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(msg, "\n", "\\n"))
				flusher.Flush()
			}
		}
	}

	// Publish: POST /api/pubsub/<channel>
	if len(segs) == 1 && r.Method == http.MethodPost {
		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		n := broker.Publish(channel, req.Message)
		writeJSON(w, map[string]interface{}{"receivers": n})
		return
	}

	// Subscriber count: GET /api/pubsub/<channel>
	if len(segs) == 1 && r.Method == http.MethodGet {
		writeJSON(w, map[string]interface{}{
			"channel":     channel,
			"subscribers": broker.NumSubscribers(channel),
		})
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}
