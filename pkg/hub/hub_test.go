package hub

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

// recv waits up to a timeout for an event on c.
func recv(t *testing.T, c <-chan Event, timeout time.Duration) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-c:
		return ev, ok
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for event")
		return Event{}, false
	}
}

func TestEventRender(t *testing.T) {
	ev := Event{Bytes: []byte("hello")}
	var buf bytes.Buffer
	n, err := ev.Render(&buf)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if n != 5 || buf.String() != "hello" {
		t.Fatalf("got n=%d %q", n, buf.String())
	}
}

func TestEventModeDefaultIsMorph(t *testing.T) {
	// The zero value must be the default outer morph: no patch mode set, so the
	// flush path adds no Datastar option and existing publishers are unchanged.
	var ev Event
	if ev.Mode != PatchMorph {
		t.Fatalf("zero-value Mode = %d, want PatchMorph (%d)", ev.Mode, PatchMorph)
	}
	if got := ev.DatastarMode(); got != "" {
		t.Fatalf("default DatastarMode = %q, want \"\" (Datastar default morph)", got)
	}
}

func TestEventDatastarMode(t *testing.T) {
	cases := []struct {
		name string
		mode PatchMode
		want string
	}{
		{"morph", PatchMorph, ""},
		{"append", PatchAppend, "append"},
		{"prepend", PatchPrepend, "prepend"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := Event{Bytes: []byte("x"), Mode: tc.mode}
			if got := ev.DatastarMode(); got != tc.want {
				t.Fatalf("DatastarMode() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPublishCarriesModeAndTarget(t *testing.T) {
	h := New()
	sub := h.Subscribe("chat:room1")
	defer sub.Close()

	want := Event{Bytes: []byte("<li>line</li>"), Mode: PatchAppend, Target: "history"}
	h.Publish("chat:room1", want)

	got, ok := recv(t, sub.C, time.Second)
	if !ok {
		t.Fatal("channel closed unexpectedly")
	}
	if !bytes.Equal(got.Bytes, want.Bytes) {
		t.Fatalf("bytes: got %q want %q", got.Bytes, want.Bytes)
	}
	if got.Mode != PatchAppend {
		t.Fatalf("mode: got %d want PatchAppend", got.Mode)
	}
	if got.Target != "history" {
		t.Fatalf("target: got %q want %q", got.Target, "history")
	}
	if got.DatastarMode() != "append" {
		t.Fatalf("datastar mode: got %q want %q", got.DatastarMode(), "append")
	}
}

func TestSubscribePublish(t *testing.T) {
	h := New()
	sub := h.Subscribe("chat:room1")
	defer sub.Close()

	want := Event{Bytes: []byte("patch")}
	h.Publish("chat:room1", want)

	got, ok := recv(t, sub.C, time.Second)
	if !ok {
		t.Fatal("channel closed unexpectedly")
	}
	if !bytes.Equal(got.Bytes, want.Bytes) {
		t.Fatalf("got %q want %q", got.Bytes, want.Bytes)
	}
}

func TestTopicsListsSubscribedOnly(t *testing.T) {
	h := New()
	if got := h.Topics(); len(got) != 0 {
		t.Fatalf("empty hub Topics = %v", got)
	}

	a := h.Subscribe("track", "track:software")
	b := h.Subscribe("track?assignee=henry")
	got := h.Topics()
	if len(got) != 3 {
		t.Fatalf("Topics = %v, want 3", got)
	}
	seen := map[string]bool{}
	for _, tpc := range got {
		seen[tpc] = true
	}
	for _, want := range []string{"track", "track:software", "track?assignee=henry"} {
		if !seen[want] {
			t.Fatalf("Topics missing %q: %v", want, got)
		}
	}

	a.Close()
	got = h.Topics()
	if len(got) != 1 || got[0] != "track?assignee=henry" {
		t.Fatalf("after close Topics = %v", got)
	}
	b.Close()
	if got := h.Topics(); len(got) != 0 {
		t.Fatalf("after all close Topics = %v", got)
	}
}

func TestPublishUnknownTopicNoPanic(t *testing.T) {
	h := New()
	h.Publish("nobody-listening", Event{Bytes: []byte("x")}) // must not panic or block
}

func TestMultiTopicAndFanOut(t *testing.T) {
	h := New()
	a := h.Subscribe("t1", "t2")
	b := h.Subscribe("t2")
	defer a.Close()
	defer b.Close()

	h.Publish("t1", Event{Bytes: []byte("one")})
	if ev, _ := recv(t, a.C, time.Second); string(ev.Bytes) != "one" {
		t.Fatalf("a missed t1: %q", ev.Bytes)
	}

	// t2 fans out to both a and b.
	h.Publish("t2", Event{Bytes: []byte("two")})
	if ev, _ := recv(t, a.C, time.Second); string(ev.Bytes) != "two" {
		t.Fatalf("a missed t2: %q", ev.Bytes)
	}
	if ev, _ := recv(t, b.C, time.Second); string(ev.Bytes) != "two" {
		t.Fatalf("b missed t2: %q", ev.Bytes)
	}

	// b is not subscribed to t1: it must not receive it.
	h.Publish("t1", Event{Bytes: []byte("solo")})
	select {
	case ev := <-b.C:
		t.Fatalf("b got t1 event it shouldn't: %q", ev.Bytes)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDropOnFull(t *testing.T) {
	h := New()
	sub := h.Subscribe("flood")
	defer sub.Close()

	// Fill the buffer plus extra; extras beyond subBuffer are dropped, never block.
	total := subBuffer + 100
	for i := 0; i < total; i++ {
		h.Publish("flood", Event{Bytes: []byte{byte(i)}})
	}

	// Exactly subBuffer events should be buffered.
	count := 0
	for {
		select {
		case <-sub.C:
			count++
		default:
			if count != subBuffer {
				t.Fatalf("got %d buffered events, want %d", count, subBuffer)
			}
			return
		}
	}
}

func TestCloseStopsDelivery(t *testing.T) {
	h := New()
	sub := h.Subscribe("topic")
	sub.Close()

	// Channel must be closed.
	if _, ok := <-sub.C; ok {
		t.Fatal("expected closed channel after Close")
	}

	// Publishing after close must not panic (no send on closed channel).
	h.Publish("topic", Event{Bytes: []byte("late")})
}

func TestCloseIdempotent(t *testing.T) {
	h := New()
	sub := h.Subscribe("topic")
	sub.Close()
	sub.Close() // must not panic (double close)
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	h := New()
	const nSubs = 20
	const nMsgs = 200

	var wg sync.WaitGroup
	for i := 0; i < nSubs; i++ {
		s := h.Subscribe("hot")
		wg.Add(1)
		go func(s *Sub) {
			defer wg.Done()
			defer s.Close()
			deadline := time.After(2 * time.Second)
			for {
				select {
				case <-s.C: // drain; drop-on-full means we may not see all
				case <-deadline:
					return
				}
			}
		}(s)
	}

	// Concurrent publishers — exercise the RLock fan-out under the race detector.
	var pubWg sync.WaitGroup
	for p := 0; p < 4; p++ {
		pubWg.Add(1)
		go func() {
			defer pubWg.Done()
			for i := 0; i < nMsgs; i++ {
				h.Publish("hot", Event{Bytes: []byte("x")})
			}
		}()
	}
	pubWg.Wait()
	wg.Wait()
}

func TestTicker(t *testing.T) {
	h := New()
	sub := h.Subscribe("tick")
	defer sub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go h.Ticker(ctx, "tick", 10*time.Millisecond, func() (Event, bool) {
		return Event{Bytes: []byte("tock")}, true
	})

	if ev, _ := recv(t, sub.C, time.Second); string(ev.Bytes) != "tock" {
		t.Fatalf("ticker event %q", ev.Bytes)
	}
}

func TestTickerSkipWhenNotOk(t *testing.T) {
	h := New()
	sub := h.Subscribe("tick")
	defer sub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go h.Ticker(ctx, "tick", 5*time.Millisecond, func() (Event, bool) {
		return Event{}, false // never publish
	})

	select {
	case ev := <-sub.C:
		t.Fatalf("expected no events, got %q", ev.Bytes)
	case <-time.After(60 * time.Millisecond):
	}
}

func TestTickerStopsOnCancel(t *testing.T) {
	h := New()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		h.Ticker(ctx, "tick", time.Millisecond, func() (Event, bool) { return Event{}, false })
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Ticker did not return after cancel")
	}
}
