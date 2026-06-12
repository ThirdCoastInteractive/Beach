// Command boardwalk is a database-less, in-memory showcase of the-beach's
// sim/ecs stack: a Monopoly-style real-estate game where the whole live slice —
// player positions, cash, turn order, property ownership — lives in an ecs.Store
// owned by a single-writer sim loop, mutated by commands, and pushed to every
// watcher over SSE via hub projections.
//
// Run it:
//
//	go build -o bin/boardwalk ./examples/boardwalk
//	PORT=8080 ./bin/boardwalk        # or just ./bin/boardwalk to bind :8080
//
// Then open http://localhost:8080/. The board renders immediately (spectator
// stream, unauthenticated). Click "Take a seat" to join, then "Roll" on your
// turn. Every roll is a beach Action -> sim Ask: it rolls two dice off the
// deterministic sim PRNG, moves, resolves the tile (buy/rent/tax/chance), and
// the Board projection fans the fresh board to every connection; a per-player
// Cash projection updates your private hand on your own user:<seat> topic.
//
// No database is required to play — persistence lanes are documented in
// docs/architecture/08-sim.md but the runnable demo is pure in-memory.
package main

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/hub"
	"github.com/ThirdCoastInteractive/the-beach/sim"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/ThirdCoastInteractive/the-beach/ui/driftwood"
	"github.com/a-h/templ"
)

// seatCookie names the cookie that ties a browser to its claimed seat. With no
// database, this is the whole "session" — a small integer seat index. A browser
// without it is a spectator.
const seatCookie = "bw_seat"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	h := hub.New()

	// The sim hosts the game. ProjectRate is bumped so a roll's board update fans
	// out promptly; the game is event-driven (rolls), not continuous, so the tick
	// rate only bounds how fast a command is picked up.
	s := sim.New(sim.Config{
		TickRate:    20,
		ProjectRate: 20,
		Hub:         h,
	})
	game := NewGame(s)

	app := beach.New(beach.Config{
		Service: "boardwalk",
		Kit:     beach.KitStatic("driftwood"),
		Hub:     h,
		Logger:  logger,
	})

	srv := &server{game: game}

	app.Page("/", srv.indexPage)
	app.Action("/join", srv.joinAction)
	app.Action("/roll", srv.rollAction)
	app.Stream("/board", srv.boardStream) // shared board (spectators included)
	app.Stream("/hand", srv.handStream)   // per-player private hand

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go game.Run(ctx)

	logger.Info("boardwalk: starting", "addr", addr,
		"routes", "GET / , POST /join , POST /roll , GET /board , GET /hand , GET /static/...")
	if err := app.Start(addr); err != nil {
		log.Fatalf("boardwalk: %v", err)
	}
}

// server holds the handler dependencies (the game). Handlers are methods only to
// share the single game pointer; they take *beach.Ctx like any beach handler.
type server struct {
	game *Game
}

// seatFrom reads the browser's claimed seat from its cookie, or -1 (spectator).
func seatFrom(c *beach.Ctx) int {
	ck, err := c.R.Cookie(seatCookie)
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(ck.Value)
	if err != nil || n < 0 || n >= maxSeats {
		return -1
	}
	return n
}

// indexPage renders the full board page on navigation and patches the board on a
// Datastar request. The page opens two SSE streams on load: the shared board and
// (if seated) the private hand.
func (s *server) indexPage(c *beach.Ctx) (beach.View, error) {
	kit := c.Kit()
	seat := seatFrom(c)

	snap, err := s.game.Snapshot(c.Context())
	if err != nil {
		return beach.View{}, err
	}

	heading := kit.Layout().PageHeading(ui.PageHeadingProps{
		Title:    "Boardwalk",
		Subtitle: "A sim/ecs real-estate game. Roll, move, buy, pay rent — live over SSE.",
	})

	boardCard := kit.Layout().Card(ui.CardProps{
		Surface: ui.SurfacePanel,
		Heading: "Board",
	}, boardFragment(snap))

	handCard := kit.Layout().Card(ui.CardProps{
		Surface: ui.SurfaceWell,
		Heading: "Your hand",
	}, handFragment(snap, seat))

	// Open the streams on load: board always; hand always (it self-resolves the
	// spectator case server-side from the cookie).
	body := kit.Layout().Container(ui.ContainerProps{
		Attrs: datastar.Attrs{datastar.Init("@get('/board'); @get('/hand')")},
	},
		heading,
		sinks(), // stable #bw-toast and #bw-reload morph targets
		kit.Layout().Grid(ui.GridProps{Cols: 2}, boardCard, handCard),
	)

	page := driftwood.Page(driftwood.PageProps{
		Title:       "Boardwalk — the-beach",
		Description: "A database-less sim/ecs board game over SSE.",
	}, body)

	return beach.View{
		Page:     page,
		Fragment: boardFragment(snap),
		Target:   boardTarget,
	}, nil
}

// joinAction claims a seat for this browser. It Asks the sim to claim the next
// open seat, sets the seat cookie, and reloads so the controls switch to "Roll"
// and the hand stream opens for the new seat.
func (s *server) joinAction(c *beach.Ctx) (beach.Patches, error) {
	if seat := seatFrom(c); seat >= 0 {
		return beach.Patches{}, nil // already seated
	}
	seat, err := s.game.Join(c.Context(), "")
	if err != nil {
		return nil, err
	}
	if seat < 0 {
		// Table full — surface a toast.
		return beach.Patches{{
			Fragment: c.Kit().Feedback().Toast(ui.ToastProps{
				Role: ui.RoleWarn, Title: "Table full", Message: "All seats are taken. Spectate for now.",
			}),
			Target: "bw-toast", Mode: beach.PatchInner,
		}}, nil
	}
	http.SetCookie(c.W, &http.Cookie{
		Name:     seatCookie,
		Value:    strconv.Itoa(seat),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	// Re-open the hand stream so it resubscribes to user:<seat> with the cookie we
	// just set: the new stream's catch-up immediately renders the seated hand and
	// the Roll control. Patching a data-init element fires @get('/hand') again —
	// no page reload, no inline script (the CSP forbids it).
	return beach.Patches{{
		Fragment: reopenHand(),
		Target:   "bw-reopen",
		Mode:     beach.PatchInner,
	}}, nil
}

// sinks renders the two always-present morph targets: a toast region for action
// feedback and the #bw-reopen sink the join action patches a data-init element
// into to re-open the hand stream. Keeping them in the initial document means
// action patches always have a stable target.
func sinks() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<div id="bw-toast" class="bw-toast-region" aria-live="polite"></div><div id="bw-reopen" hidden></div>`)
		return err
	})
}

// reopenHand renders a data-init element that re-fires @get('/hand'). Patched
// into #bw-reopen after a seat claim, it makes Datastar open a fresh hand stream
// that resubscribes to user:<seat> with the just-set cookie.
func reopenHand() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<div data-init="@get('/hand')"></div>`)
		return err
	})
}

// rollAction rolls for this browser's seat. The Ask returns the outcome; the
// board/hand projections push the visible state, so the action itself only needs
// to surface a toast on an invalid move (not your turn, open seat).
func (s *server) rollAction(c *beach.Ctx) (beach.Patches, error) {
	seat := seatFrom(c)
	if seat < 0 {
		return beach.Patches{{
			Fragment: c.Kit().Feedback().Toast(ui.ToastProps{
				Role: ui.RoleInfo, Title: "Spectating", Message: "Take a seat to roll.",
			}),
			Target: "bw-toast", Mode: beach.PatchInner,
		}}, nil
	}
	res, err := s.game.Roll(c.Context(), seat)
	if err != nil {
		return nil, err
	}
	role := ui.RoleGood
	title := "Rolled"
	if !res.OK {
		role = ui.RoleWarn
		title = "Hold on"
	}
	return beach.Patches{{
		Fragment: c.Kit().Feedback().Toast(ui.ToastProps{Role: role, Title: title, Message: res.Msg}),
		Target:   "bw-toast", Mode: beach.PatchInner,
	}}, nil
}

// boardStream is the shared board surface, subscribed by every connection
// (spectators included — it is unauthenticated). Catch-up renders the current
// board immediately so a fresh connection is correct before the next projection.
func (s *server) boardStream(c *beach.Ctx) (beach.Sub, error) {
	snap, err := s.game.Snapshot(c.Context())
	if err != nil {
		return beach.Sub{}, err
	}
	return beach.Sub{
		Topics: []string{boardTopic},
		CatchUp: func(_ string, p beach.Patcher) error {
			return p.Patch(beach.Patch{Fragment: boardFragment(snap), Target: boardTarget})
		},
	}, nil
}

// handStream is this browser's private hand on its user:<seat> topic. A
// spectator (no seat) subscribes to nothing and just gets the spectator
// catch-up frame.
func (s *server) handStream(c *beach.Ctx) (beach.Sub, error) {
	seat := seatFrom(c)
	snap, err := s.game.Snapshot(c.Context())
	if err != nil {
		return beach.Sub{}, err
	}
	var topics []string
	if seat >= 0 {
		topics = []string{userTopic(seat)}
	}
	return beach.Sub{
		Topics: topics,
		CatchUp: func(_ string, p beach.Patcher) error {
			return p.Patch(beach.Patch{Fragment: handFragment(snap, seat), Target: handTarget})
		},
	}, nil
}
