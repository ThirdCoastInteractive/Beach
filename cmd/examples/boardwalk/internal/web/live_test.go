package web

// boardwalk is the framework's live-updates showcase, so it is also where the
// pause has to hold. The board redraws on every player's action; WCAG 2.2.2 asks
// that auto-updating information can be stopped, and a control that stopped one
// of the three streams while the other two kept moving would read as a broken
// page rather than a paused one.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/boardwalk/internal/game"
	"github.com/ThirdCoastInteractive/Beach/pkg/prefs"
)

// render draws the whole index page under the given preferences.
func render(t *testing.T, ctx context.Context) string {
	t.Helper()
	snap := game.Snapshot{}
	var b bytes.Buffer
	if err := indexView(snap, -1, raceLayout(snap)).Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestPausingStopsAllThreeStreams(t *testing.T) {
	// The count is the assertion that matters: three streams, three openers.
	// (templ escapes the quotes inside the attribute, so the path is the needle
	// rather than the @get expression a call site writes.)
	live := render(t, context.Background())
	if n := strings.Count(live, "data-init"); n != 3 {
		t.Fatalf("a live board opens %d streams, want 3", n)
	}
	for _, s := range []string{"/board", "/hand", "/race"} {
		if !strings.Contains(live, s) {
			t.Errorf("a live board never opens %s", s)
		}
	}

	paused := render(t, prefs.With(context.Background(), prefs.Prefs{LiveUpdates: false, AutoDismiss: true}))
	if n := strings.Count(paused, "data-init"); n != 0 {
		t.Errorf("a paused board still opens %d stream(s)", n)
	}

	// The control itself has to survive the pause, or there is no way back.
	if !strings.Contains(paused, "dw-live-btn") {
		t.Error("the pause removed its own resume control")
	}
}

// TestReopeningAHandRespectsThePause covers the path an action takes rather than
// a page load: claiming a seat patches a fresh hand stream into the page, which
// would otherwise re-open a connection the visitor had just closed.
func TestReopeningAHandRespectsThePause(t *testing.T) {
	var b bytes.Buffer
	ctx := prefs.With(context.Background(), prefs.Prefs{LiveUpdates: false, AutoDismiss: true})
	if err := reopenHand().Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(b.String(), "data-init") {
		t.Errorf("claiming a seat re-opened a stream behind a paused visitor's back: %s", b.String())
	}
}
