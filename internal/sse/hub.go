// Package sse implements a small in-process pub/sub hub for Server-Sent Events.
package sse

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is a single SSE frame.
type Event struct {
	ID   int64
	Type string
	Data []byte
}

type subscriber struct {
	ch chan Event
}

// Hub fans out events to per-request and global subscribers, retaining a small
// replay buffer so late subscribers (and reconnects) receive the full trace.
type Hub struct {
	mu      sync.RWMutex
	seq     int64
	subs    map[string]map[*subscriber]struct{}
	buffers map[string][]Event
	order   []string
	maxBuf  int
	maxKeys int
	closed  bool
}

// NewHub builds a hub with bounded replay buffers.
func NewHub() *Hub {
	return &Hub{
		subs:    map[string]map[*subscriber]struct{}{},
		buffers: map[string][]Event{},
		maxBuf:  500,
		maxKeys: 256,
	}
}

// Subscribe registers a subscriber for a key ("" is the global feed). It
// returns the channel, the replay of events newer than lastID, and an
// unsubscribe function.
func (h *Hub) Subscribe(key string, lastID int64) (<-chan Event, []Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sub := &subscriber{ch: make(chan Event, 256)}
	if h.subs[key] == nil {
		h.subs[key] = map[*subscriber]struct{}{}
	}
	h.subs[key][sub] = struct{}{}
	var past []Event
	for _, e := range h.buffers[key] {
		if e.ID > lastID {
			past = append(past, e)
		}
	}
	unsub := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if set, ok := h.subs[key]; ok {
			delete(set, sub)
			if len(set) == 0 {
				delete(h.subs, key)
			}
		}
	}
	return sub.ch, past, unsub
}

// Publish appends an event to the replay buffer and fans it out. Delivery to
// slow subscribers is non-blocking to protect the engine.
func (h *Hub) Publish(key, eventType string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.seq++
	ev := Event{ID: h.seq, Type: eventType, Data: payload}
	if _, known := h.buffers[key]; !known {
		h.order = append(h.order, key)
	}
	h.buffers[key] = append(h.buffers[key], ev)
	if len(h.buffers[key]) > h.maxBuf {
		h.buffers[key] = h.buffers[key][len(h.buffers[key])-h.maxBuf:]
	}
	for len(h.order) > h.maxKeys {
		oldest := h.order[0]
		h.order = h.order[1:]
		delete(h.buffers, oldest)
	}
	subs := make([]*subscriber, 0, len(h.subs[key]))
	for s := range h.subs[key] {
		subs = append(subs, s)
	}
	h.mu.Unlock()

	for _, s := range subs {
		select {
		case s.ch <- ev:
		default:
		}
	}
}

// Close unblocks subscribers and marks the hub closed.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for _, set := range h.subs {
		for s := range set {
			close(s.ch)
		}
	}
	h.subs = map[string]map[*subscriber]struct{}{}
}

// Now returns the current UTC time in RFC3339 (helper for event payloads).
func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
