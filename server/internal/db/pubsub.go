package db

import "sync"

// Broker is a lightweight in-process publish/subscribe hub. Subscribers receive
// messages published to a channel after they subscribe. It is independent of
// the key/value store and has no persistence — it is a live message bus.
type Broker struct {
	mu     sync.RWMutex
	subs   map[string]map[int64]chan string
	nextID int64
	buffer int
}

// NewBroker builds a broker. bufferSize is the per-subscriber channel buffer;
// when a subscriber's buffer is full, messages to it are dropped (slow-consumer
// protection) rather than blocking the publisher.
func NewBroker(bufferSize int) *Broker {
	if bufferSize < 1 {
		bufferSize = 16
	}
	return &Broker{
		subs:   make(map[string]map[int64]chan string),
		buffer: bufferSize,
	}
}

// Subscription is a handle to a live subscription.
type Subscription struct {
	ID      int64
	Channel string
	C       <-chan string
}

// Subscribe registers interest in a channel and returns a subscription whose
// C delivers published messages until Unsubscribe is called.
func (b *Broker) Subscribe(channel string) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	ch := make(chan string, b.buffer)
	if b.subs[channel] == nil {
		b.subs[channel] = make(map[int64]chan string)
	}
	b.subs[channel][id] = ch
	return &Subscription{ID: id, Channel: channel, C: ch}
}

// Unsubscribe removes a subscription and closes its channel.
func (b *Broker) Unsubscribe(channel string, id int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if m, ok := b.subs[channel]; ok {
		if ch, ok := m[id]; ok {
			close(ch)
			delete(m, id)
		}
		if len(m) == 0 {
			delete(b.subs, channel)
		}
	}
}

// Publish delivers message to every current subscriber of channel and returns
// the number of subscribers it reached. Delivery is non-blocking: a subscriber
// whose buffer is full is skipped.
func (b *Broker) Publish(channel, message string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	delivered := 0
	for _, ch := range b.subs[channel] {
		select {
		case ch <- message:
			delivered++
		default:
			// slow consumer: drop rather than block the publisher
		}
	}
	return delivered
}

// Channels returns the list of channels that currently have subscribers.
func (b *Broker) Channels() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, len(b.subs))
	for c := range b.subs {
		out = append(out, c)
	}
	return out
}

// NumSubscribers returns how many subscribers a channel currently has.
func (b *Broker) NumSubscribers(channel string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[channel])
}
