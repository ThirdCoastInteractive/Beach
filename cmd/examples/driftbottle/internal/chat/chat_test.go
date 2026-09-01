package chat

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
)

// newTestServer builds a Server wired to a fresh hub, with logging discarded and
// no analytics sink (Track and the archive ops become no-ops).
func newTestServer() *Server {
	return New(hub.New(), slog.New(slog.NewTextHandler(discard{}, nil)), nil)
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// recv waits up to a short timeout for one hub event's bytes, failing otherwise.
func recv(t *testing.T, sub *hub.Sub) string {
	t.Helper()
	select {
	case ev, ok := <-sub.C:
		if !ok {
			t.Fatal("subscription closed before an event arrived")
		}
		return string(ev.Bytes)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a hub event")
		return ""
	}
}

// TestMatchmakingPairsTwoStrangers is the core fan-out test: two sessions
// subscribe to their own me: topics (as their SSE connections would), then
// enqueue. The second enqueue must pair them, pushing a chat view to BOTH.
func TestMatchmakingPairsTwoStrangers(t *testing.T) {
	s := newTestServer()
	a, b := "aaaa1111", "bbbb2222"

	subA := s.hub.Subscribe(meTopic(a))
	defer subA.Close()
	subB := s.hub.Subscribe(meTopic(b))
	defer subB.Close()

	// First stranger parks in the lobby (waiting), no pairing yet.
	s.enqueue(a)
	if got := recv(t, subA); !strings.Contains(got, "Waiting for a match") {
		t.Fatalf("first enqueue should publish the waiting view, got: %q", got)
	}
	if p := s.partnerOf(a); p != "" {
		t.Fatalf("a should have no partner while waiting, got %q", p)
	}

	// Second stranger arrives: both get paired into a chat view.
	s.enqueue(b)
	gotA, gotB := recv(t, subA), recv(t, subB)
	for who, got := range map[string]string{"a": gotA, "b": gotB} {
		if !strings.Contains(got, "Connected to a stranger") {
			t.Fatalf("%s should receive the chat view after pairing, got: %q", who, got)
		}
		if !strings.Contains(got, `id="`+feedTarget+`"`) {
			t.Fatalf("%s chat view should contain the feed region, got: %q", who, got)
		}
	}

	if s.partnerOf(a) != b || s.partnerOf(b) != a {
		t.Fatalf("a and b should be partners: a.partner=%q b.partner=%q",
			s.partnerOf(a), s.partnerOf(b))
	}
}

// TestMessageFansOutToBothSides checks a sent message reaches both feeds, with
// own/other framing flipped per recipient.
func TestMessageFansOutToBothSides(t *testing.T) {
	s := newTestServer()
	a, b := "aaaa1111", "bbbb2222"

	subA := s.hub.Subscribe(meTopic(a))
	defer subA.Close()
	subB := s.hub.Subscribe(meTopic(b))
	defer subB.Close()

	s.enqueue(a)
	recv(t, subA) // waiting
	s.enqueue(b)
	recv(t, subA) // chat view
	recv(t, subB) // chat view

	s.broadcast(a, b, "hello stranger")

	gotA := recv(t, subA)
	gotB := recv(t, subB)
	if !strings.Contains(gotA, "hello stranger") || !strings.Contains(gotB, "hello stranger") {
		t.Fatalf("both sides should see the message; a=%q b=%q", gotA, gotB)
	}
	// The sender's own line carries the is-own class; the receiver's the is-other.
	if !strings.Contains(gotA, "is-own") {
		t.Fatalf("sender's feed should mark the line as own, got: %q", gotA)
	}
	if !strings.Contains(gotB, "is-other") {
		t.Fatalf("receiver's feed should mark the line as other, got: %q", gotB)
	}
}

// TestNextTearsDownAndRequeues verifies "Next": the caller is unpaired and the
// stranger is both told the chat ended and re-queued as the new waiter.
func TestNextTearsDownAndRequeues(t *testing.T) {
	s := newTestServer()
	a, b := "aaaa1111", "bbbb2222"

	subB := s.hub.Subscribe(meTopic(b))
	defer subB.Close()

	s.enqueue(a)
	s.enqueue(b) // a and b are now paired

	// a hits Next: teardown + re-queue a.
	s.teardown(a)
	s.enqueue(a)

	// b should have received the "stranger left" notice (the first event after
	// pairing on subB; pairing's own chat-view event also lands — drain until we
	// see the ended notice).
	sawEnded := false
	for i := 0; i < 3 && !sawEnded; i++ {
		if strings.Contains(recv(t, subB), "The stranger left") {
			sawEnded = true
		}
	}
	if !sawEnded {
		t.Fatal("partner should have been notified the stranger left")
	}

	// After teardown, a is re-queued; b was re-queued by teardown. They should not
	// still be partnered.
	if s.partnerOf(a) == b || s.partnerOf(b) == a {
		t.Fatalf("a and b should no longer be partners after Next")
	}
}

// TestRateLimit confirms the per-session burst limiter eventually blocks.
func TestRateLimit(t *testing.T) {
	s := newTestServer()
	id := "rate1234"
	allowed := 0
	for i := 0; i < msgRateBurst+3; i++ {
		if s.rateOK(id) {
			allowed++
		}
	}
	if allowed != msgRateBurst {
		t.Fatalf("rate limiter should allow exactly %d in a burst, allowed %d", msgRateBurst, allowed)
	}
}

// TestWordFilter checks the moderation hook masks blocklisted terms.
func TestWordFilter(t *testing.T) {
	got := clean("you are a BADWORD friend")
	if strings.Contains(strings.ToLower(got), "badword") {
		t.Fatalf("blocklisted term should be masked, got: %q", got)
	}
	if !strings.Contains(got, "*******") {
		t.Fatalf("masked term should be asterisks, got: %q", got)
	}
}

// TestCurrentRoomDefaultsToLobby confirms currentRoom renders a lobby for an
// unknown session — the home view renders without a database.
func TestCurrentRoomDefaultsToLobby(t *testing.T) {
	s := newTestServer()
	frag := s.currentRoom("never-seen")
	var sb strings.Builder
	if err := frag.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(sb.String(), "Talk to a stranger") {
		t.Fatalf("unknown session should land in the lobby, got: %q", sb.String())
	}
}
