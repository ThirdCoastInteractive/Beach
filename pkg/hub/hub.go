// Package hub is an in-memory topic fan-out for realtime SSE surfaces.
//
// Every realtime surface — event-driven chat, periodic dashboard refresh,
// per-user notifications — runs on one concept: topics. A subscriber registers
// interest in one or more topics and receives every Event published to any of
// them on its own buffered channel.
//
// Render once per topic, write per connection: a publisher renders a fragment to
// bytes once and the hub fans those shared bytes out to every subscriber. The
// per-subscription channel is buffered (64) and drops on full — a slow client
// reconciles via a since-cursor catch-up query on reconnect. Liveness on the push
// path beats completeness; completeness is the catch-up query's job.
//
// The core has no Postgres dependency. Producers (HTTP handlers after a mutation,
// pg.Listen listeners, sim projections) call Publish or run a Ticker.
package hub

import (
	"context"
	"io"
	"sync"
	"time"
)

// subBuffer is the per-subscription channel capacity. Writes that would block
// because the buffer is full are dropped (see Publish).
const subBuffer = 64

// PatchMode selects how a fanned-out Event's fragment is merged into the DOM. It
// mirrors the Datastar element-patch modes; the zero value is the default outer
// morph by the fragment's own id, so existing publishers are unchanged.
//
// A high-volume incremental feed (e.g. driftbottle chat) sets PatchAppend or
// PatchPrepend so each message inserts a single line into a container instead of
// re-rendering the whole rolling history and morphing the container every time.
type PatchMode uint8

const (
	// PatchMorph is the default: morph the target element by its own id.
	PatchMorph PatchMode = iota
	// PatchAppend inserts the fragment as the target's last child.
	PatchAppend
	// PatchPrepend inserts the fragment as the target's first child.
	PatchPrepend
)

// datastarMode maps a PatchMode to the corresponding Datastar element-patch mode
// string. The zero value (PatchMorph) maps to "", which the flush path reads as
// "use Datastar's default outer morph" — so it adds no patch option. The non-zero
// values return the Datastar mode strings ("append", "prepend") verbatim.
func (m PatchMode) datastarMode() string {
	switch m {
	case PatchAppend:
		return "append"
	case PatchPrepend:
		return "prepend"
	default:
		return ""
	}
}

// Event is a pre-rendered patch fanned out to subscribers. The bytes are rendered
// once by the publisher; the hub shares them across every connection. Render
// writes those bytes to a single connection's writer (e.g. its gzip stream).
type Event struct {
	Bytes []byte

	// Mode selects how the fragment is merged into the DOM when the event is
	// flushed to SSE subscribers. The zero value (PatchMorph) keeps the default
	// outer-morph-by-id behavior, so existing publishers need no change. Set
	// PatchAppend or PatchPrepend on an incremental feed to insert a single line
	// into a container rather than re-rendering and morphing the container.
	Mode PatchMode

	// Target is the DOM id (without the leading '#') the fragment patches into.
	// It is required when Mode is append/prepend — the insert needs a container
	// to insert into — and ignored for the default morph, which targets the
	// fragment's own id.
	Target string
}

// DatastarMode reports the Datastar element-patch mode string for this event's
// Mode: "" for the default outer morph (add no patch option), or "append" /
// "prepend" for an incremental insert. The SSE flush path maps this to the
// Datastar patch option when writing the event to a subscriber's stream.
func (e Event) DatastarMode() string { return e.Mode.datastarMode() }

// Render writes the pre-rendered bytes to w. It is called once per subscriber
// connection so each connection's own writer (compressor) sees the shared bytes.
func (e Event) Render(w io.Writer) (int, error) {
	return w.Write(e.Bytes)
}

// Sub is a subscription handle. C delivers events for any of the subscribed
// topics. Close removes the subscription from the hub and closes C; it is safe to
// call more than once.
type Sub struct {
	// C delivers events. It is buffered (64) and drops on full.
	C <-chan Event

	hub    *Hub
	c      chan Event
	topics []string

	closeMu sync.Mutex // guards closed; held by Close while removing+closing
	closed  bool
}

// Close unsubscribes from all topics and closes C. Safe to call multiple times.
func (s *Sub) Close() {
	s.hub.remove(s)
}

// Hub is an in-memory topic fan-out. The zero value is not usable; call New.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[*Sub]struct{} // topic -> set of subscriptions
}

// New returns a ready Hub.
func New() *Hub {
	return &Hub{subs: make(map[string]map[*Sub]struct{})}
}

// Subscribe registers interest in the given topics and returns a Sub. The caller
// must Close the Sub when done. Subscribing to zero topics yields a Sub that
// never receives events (but can still be closed).
func (h *Hub) Subscribe(topics ...string) *Sub {
	c := make(chan Event, subBuffer)
	s := &Sub{C: c, hub: h, c: c, topics: topics}

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, t := range topics {
		set := h.subs[t]
		if set == nil {
			set = make(map[*Sub]struct{})
			h.subs[t] = set
		}
		set[s] = struct{}{}
	}
	return s
}

// Topics returns the topic names that currently have at least one subscriber.
// The order is unspecified. Publishers use this to render once per live
// interest instead of guessing which filters are open.
func (h *Hub) Topics() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.subs))
	for t := range h.subs {
		out = append(out, t)
	}
	return out
}

// Publish fans ev out to every subscriber of topic. A subscriber whose buffer is
// full has the event dropped rather than blocking the publisher; that client
// catches up via its since-cursor on reconnect.
func (h *Hub) Publish(topic string, ev Event) {
	h.mu.RLock()
	// Snapshot subscribers so the non-blocking sends happen outside the hub lock.
	set := h.subs[topic]
	targets := make([]*Sub, 0, len(set))
	for s := range set {
		targets = append(targets, s)
	}
	h.mu.RUnlock()

	for _, s := range targets {
		s.send(ev)
	}
}

// send delivers ev to s without blocking, dropping on a full buffer. The per-sub
// closeMu serializes against Close so we never send on a closed channel.
func (s *Sub) send(ev Event) {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.c <- ev:
	default: // buffer full: drop on full
	}
}

// remove unsubscribes s from all of its topics and closes its channel exactly
// once. Called by Sub.Close.
func (h *Hub) remove(s *Sub) {
	h.mu.Lock()
	for _, t := range s.topics {
		set := h.subs[t]
		if set == nil {
			continue
		}
		delete(set, s)
		if len(set) == 0 {
			delete(h.subs, t)
		}
	}
	h.mu.Unlock()

	// Mark closed (blocking out any in-flight send) before closing the channel.
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.c)
	}
}

// Ticker publishes the Event produced by produce to topic every interval until
// ctx is cancelled. produce is called on each tick; returning ok=false skips that
// tick's publish (e.g. nothing changed). Ticker blocks until ctx is done, so
// callers typically run it in a goroutine.
func (h *Hub) Ticker(ctx context.Context, topic string, interval time.Duration, produce func() (Event, bool)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if ev, ok := produce(); ok {
				h.Publish(topic, ev)
			}
		}
	}
}
