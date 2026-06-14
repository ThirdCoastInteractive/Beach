// Command boardwalk is a sim/ecs + persistence + analytics showcase of
// the-beach's stack: a Monopoly-style real-estate game where the whole live
// slice — player positions, cash, turn order, property ownership — lives in an
// ecs.Store owned by a single-writer sim loop, mutated by commands, and pushed
// to every watcher over SSE via hub projections.
//
// The live model stays purely in-memory: the game runs in the ecs.Store/sim,
// fast and lock-free. Two persistence lanes sit beside it, and BOTH are required
// to boot (a missing DSN aborts rather than degrading):
//
//   - Postgres holds a periodic CBOR snapshot of the store (saveLoop, every 15s)
//     and is restored on boot, so a restart resumes the game mid-play.
//   - ClickHouse holds the action firehose — one append-only row per join, roll,
//     buy, rent, tax, chance, and pass-GO — aggregated into the public /stats page.
//
// Run it as a container stack — the app, its Postgres, and its ClickHouse:
//
//	cd examples/boardwalk
//	cp .env.example .env
//	docker compose up                     # → http://localhost:8080
//
// Or outside compose with both DSNs set:
//
//	DATABASE_URL=postgres://beach:beach@localhost:5432/boardwalk?sslmode=disable \
//	CLICKHOUSE_DSN=clickhouse://beach:beach@localhost:9000/beach \
//	go run ./examples/boardwalk
//
// Then open http://localhost:8080/. The board renders immediately (spectator
// stream, unauthenticated). Click "Take a seat" to join, then "Roll" on your
// turn. Every roll is a beach Action -> sim Ask: it rolls two dice off the
// deterministic sim PRNG, moves, resolves the tile (buy/rent/tax/chance), and
// the Board projection fans the fresh board to every connection; a per-player
// Cash projection updates your private hand on your own user:<seat> topic.
//
// The cash race card is the server-animated chart pattern: a hub ticker
// re-renders the bar-race fragment from the live standings once a second and
// publishes it when it changed; Datastar morphs the SVG in place and CSS
// transitions tween the bars. /specimen serves the framework's living
// component showcase; /stats serves the analytics dashboard.
package main

import (
	"bytes"
	"context"
	"embed"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ch"
	"github.com/ThirdCoastInteractive/the-beach/pkg/hub"
	"github.com/ThirdCoastInteractive/the-beach/pkg/pg"
	"github.com/ThirdCoastInteractive/the-beach/pkg/sim"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ui/driftwood"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ui/specimen"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// seatCookie names the cookie that ties a browser to its claimed seat. With no
// database, this is the whole "session" — a small integer seat index. A browser
// without it is a spectator.
const seatCookie = "bw_seat"

// raceTopic is the hub topic the cash-race ticker publishes frames to and the
// /race stream subscribes to. The race is main-level wiring (a ticker), not a
// sim projection, so the topic lives here rather than in game.go.
const raceTopic = "race"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Load and validate config first: a missing Postgres or ClickHouse DSN aborts
	// here with the list, before anything serves. There is no database-less mode —
	// both stores are required to boot.
	conf := loadConfig()
	addr := ":" + conf.Port

	// Register component types under their stable schema ids before anything
	// touches the store — snapshot restore (ecs.Load, below) resolves saved
	// columns by schema id and must find the types already registered.
	registerComponents()

	// A signal-aware context so a clean shutdown (Ctrl-C / SIGTERM) cancels the
	// game loop, the race ticker, and the save loop together.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	h := hub.New()

	// Postgres is required: it holds the periodic snapshot of the live store and
	// is restored on boot. Each step dies loudly rather than degrading.
	pool := pg.MustPool(ctx, conf.DSN)
	if err := pg.Migrate(ctx, pool, migrationsFS); err != nil {
		log.Fatalf("boardwalk: migrate failed: %v", err)
	}
	snap := snapStore{pool: pool}

	// ClickHouse is required: the action firehose behind /stats. A bad DSN or
	// migration is fatal — there is no analytics-off mode.
	conn, err := ch.Connect(ctx, conf.CHDSN)
	if err != nil {
		log.Fatalf("boardwalk: clickhouse connect failed: %v", err)
	}
	if err := ch.Migrate(ctx, conn, chMigrations()); err != nil {
		log.Fatalf("boardwalk: clickhouse migrate failed: %v", err)
	}
	events := ch.NewBatcher[event](conn, "boardwalk_events", ch.Batch{})

	// Restore the live store from the latest snapshot if one exists; a fresh
	// database (no snapshot row) starts a new game. The sim hosts the game.
	// ProjectRate is bumped so a roll's board update fans out promptly; the game
	// is event-driven (rolls), not continuous, so the tick rate only bounds how
	// fast a command is picked up.
	blob, err := snap.load(ctx)
	if err != nil {
		log.Fatalf("boardwalk: snapshot load failed: %v", err)
	}
	simCfg := sim.Config{TickRate: 20, ProjectRate: 20, Hub: h}
	if blob != nil {
		restored, rerr := sim.RestoreStore(blob)
		if rerr != nil {
			log.Fatalf("boardwalk: snapshot restore failed: %v", rerr)
		}
		simCfg.Store = restored
		logger.Info("boardwalk: restored from snapshot")
	}
	s := sim.New(simCfg)
	game := NewGame(s, events)

	app := beach.New(beach.Config{
		Service: "boardwalk",
		Hub:     h,
		Logger:  logger,
	})

	srv := &server{game: game, chConn: conn}

	app.Page("/", srv.indexPage)
	app.Action("/join", srv.joinAction)
	app.Action("/roll", srv.rollAction)
	app.Stream("/board", srv.boardStream) // shared board (spectators included)
	app.Stream("/hand", srv.handStream)   // per-player private hand
	app.Stream("/race", srv.raceStream)   // shared cash race

	// The public analytics page (analytics.go): the dashboard plus its deferred
	// widget fragments. The rolls/tiles widgets aggregate the ClickHouse firehose;
	// the cash widget reads the live store off the loop.
	app.Page("/stats", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: statsPage()}, nil
	})
	app.Page("/stats/rolls", anlWidget("anl-rolls", srv.anlRolls))
	app.Page("/stats/tiles", anlWidget("anl-tiles", srv.anlTiles))
	app.Page("/stats/cash", anlWidget("anl-cash", srv.anlCash))

	// The living component showcase, mounted so the example doubles as a kit
	// browser.
	app.Page("/specimen", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: specimen.Page()}, nil
	})

	go game.Run(ctx)

	// Keep the durable copy current: snapshot the live store to Postgres every
	// 15s so a restart resumes the game mid-play.
	go saveLoop(ctx, game, snap)

	// The race ticker is the chart's animation engine: each second it snapshots
	// the game, re-lays-out the cash race, and publishes the rendered fragment —
	// but only when the standings actually changed, so idle tables stay silent.
	// The client does no animation work: Datastar morphs the new SVG in and the
	// chart CSS transitions tween the geometry.
	var lastRace []byte
	go h.Ticker(ctx, raceTopic, time.Second, func() (hub.Event, bool) {
		bs, err := game.Snapshot(ctx)
		if err != nil {
			return hub.Event{}, false
		}
		frame := renderToBytes(raceFragment(raceLayout(bs)))
		if frame == nil || bytes.Equal(frame, lastRace) {
			return hub.Event{}, false
		}
		lastRace = frame
		return hub.Event{Bytes: frame}, true
	})

	logger.Info("boardwalk: starting", "addr", addr,
		"routes", "GET / , POST /join , POST /roll , GET /board , GET /hand , GET /race , GET /stats , GET /specimen , GET /static/...")
	if err := app.Start(addr); err != nil {
		log.Fatalf("boardwalk: %v", err)
	}
}

// server holds the handler dependencies: the game (live model + Snapshot) and
// the ClickHouse connection the /stats widgets query. Handlers are methods only
// to share these; they take *beach.Ctx like any beach handler.
type server struct {
	game   *Game
	chConn ch.Conn
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
// Datastar request. The page opens three SSE streams on load: the shared board,
// the private hand (if seated), and the cash race.
func (s *server) indexPage(c *beach.Ctx) (beach.View, error) {
	seat := seatFrom(c)

	snap, err := s.game.Snapshot(c.Context())
	if err != nil {
		return beach.View{}, err
	}

	return beach.View{
		Page:     indexView(snap, seat, raceLayout(snap)),
		Fragment: boardFragment(snap),
		Target:   boardTarget,
	}, nil
}

// joinAction claims a seat for this browser. It Asks the sim to claim the next
// open seat, sets the seat cookie, and patches a data-init element that re-opens
// the hand stream so the controls switch to "Roll" for the new seat.
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
			Fragment: driftwood.Toast(driftwood.ToastProps{
				Role: driftwood.RoleWarn, Title: "Table full", Message: "All seats are taken. Spectate for now.",
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

// rollAction rolls for this browser's seat. The Ask returns the outcome; the
// board/hand projections push the visible state, so the action itself only needs
// to surface a toast on an invalid move (not your turn, open seat).
func (s *server) rollAction(c *beach.Ctx) (beach.Patches, error) {
	seat := seatFrom(c)
	if seat < 0 {
		return beach.Patches{{
			Fragment: driftwood.Toast(driftwood.ToastProps{
				Role: driftwood.RoleInfo, Title: "Spectating", Message: "Take a seat to roll.",
			}),
			Target: "bw-toast", Mode: beach.PatchInner,
		}}, nil
	}
	res, err := s.game.Roll(c.Context(), seat)
	if err != nil {
		return nil, err
	}
	role := driftwood.RoleGood
	title := "Rolled"
	if !res.OK {
		role = driftwood.RoleWarn
		title = "Hold on"
	}
	return beach.Patches{{
		Fragment: driftwood.Toast(driftwood.ToastProps{Role: role, Title: title, Message: res.Msg}),
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

// raceStream is the shared cash-race surface. Catch-up renders the current
// standings immediately; afterwards the ticker's published frames keep the
// chart moving (the ticker dedupes, so this catch-up is what a fresh
// connection sees until the standings change).
func (s *server) raceStream(c *beach.Ctx) (beach.Sub, error) {
	snap, err := s.game.Snapshot(c.Context())
	if err != nil {
		return beach.Sub{}, err
	}
	return beach.Sub{
		Topics: []string{raceTopic},
		CatchUp: func(_ string, p beach.Patcher) error {
			return p.Patch(beach.Patch{Fragment: raceFragment(raceLayout(snap)), Target: raceTarget})
		},
	}, nil
}
