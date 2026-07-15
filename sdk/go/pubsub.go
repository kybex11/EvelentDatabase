package sdk

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strings"
)

// PubSubService is a client for the database's in-process publish/subscribe
// broker, exposed over HTTP (publish) and Server-Sent Events (subscribe).
type PubSubService struct {
	http *HTTPClient
}

// Publish sends message to channel and returns how many subscribers received it.
func (s *PubSubService) Publish(channel, message string) (int, error) {
	var out struct {
		Receivers int `json:"receivers"`
	}
	err := s.http.DoJSON(http.MethodPost, "/api/pubsub/"+url.PathEscape(channel),
		map[string]interface{}{"message": message}, &out)
	return out.Receivers, err
}

// NumSubscribers returns the current subscriber count for channel.
func (s *PubSubService) NumSubscribers(channel string) (int, error) {
	var out struct {
		Subscribers int `json:"subscribers"`
	}
	err := s.http.DoJSON(http.MethodGet, "/api/pubsub/"+url.PathEscape(channel), nil, &out)
	return out.Subscribers, err
}

// Subscription is a live SSE subscription. Read messages from Messages until it
// is closed; call Close (or cancel the context) to stop.
type Subscription struct {
	Messages <-chan string
	cancel   context.CancelFunc
}

// Close ends the subscription.
func (sub *Subscription) Close() { sub.cancel() }

// Subscribe opens an SSE stream for channel. Messages are delivered on the
// returned subscription's Messages channel until the context is cancelled, the
// connection drops, or Close is called.
func (s *PubSubService) Subscribe(ctx context.Context, channel string) (*Subscription, error) {
	ctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.http.URL("/api/pubsub/"+url.PathEscape(channel)+"/subscribe"), nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	s.http.applyAuth(req)
	resp, err := s.http.HTTPClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		cancel()
		return nil, &APIError{Status: resp.StatusCode, StatusText: resp.Status}
	}

	out := make(chan string, 64)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			msg := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			msg = strings.ReplaceAll(msg, "\\n", "\n")
			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	return &Subscription{Messages: out, cancel: cancel}, nil
}
