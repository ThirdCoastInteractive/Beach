// Command boardwalk is a sim/ecs + persistence + analytics showcase of
// Beach's stack: a Monopoly-style real-estate game where the whole live
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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/boardwalk/internal/analytics"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/boardwalk/internal/game"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/boardwalk/internal/store"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/boardwalk/internal/web"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/ch"
	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
	"github.com/ThirdCoastInteractive/Beach/pkg/sim"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/specimen"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

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
	game.RegisterComponents()

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
	snap := store.Snap{Pool: pool}

	// ClickHouse is required: the action firehose behind /stats. A bad DSN or
	// migration is fatal — there is no analytics-off mode.
	conn, err := ch.Connect(ctx, conf.CHDSN)
	if err != nil {
		log.Fatalf("boardwalk: clickhouse connect failed: %v", err)
	}
	if err := ch.Migrate(ctx, conn, analytics.Migrations()); err != nil {
		log.Fatalf("boardwalk: clickhouse migrate failed: %v", err)
	}
	events := ch.NewBatcher[analytics.Event](conn, "boardwalk_events", ch.Batch{})

	// Restore the live store from the latest snapshot if one exists; a fresh
	// database (no snapshot row) starts a new game. The sim hosts the game.
	// ProjectRate is bumped so a roll's board update fans out promptly; the game
	// is event-driven (rolls), not continuous, so the tick rate only bounds how
	// fast a command is picked up.
	blob, err := snap.Load(ctx)
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

	// The game's projections render through the web view hooks; injecting them
	// keeps internal/game free of any web import.
	g := game.NewGame(s, events, game.Renderers{
		Board: web.RenderBoard,
		Hand:  web.RenderHand,
	})

	app := beach.New(beach.Config{
		Service: "boardwalk",
		Hub:     h,
		Logger:  logger,
	})

	srv := web.NewServer(g)

	app.Page("/", srv.IndexPage)
	app.Action("/join", srv.JoinAction)
	app.Action("/roll", srv.RollAction)
	app.Stream("/board", srv.BoardStream) // shared board (spectators included)
	app.Stream("/hand", srv.HandStream)   // per-player private hand
	app.Stream("/race", srv.RaceStream)   // shared cash race

	// The public analytics page (internal/analytics): the dashboard plus its
	// deferred widget fragments. The rolls/tiles widgets aggregate the ClickHouse
	// firehose; the cash widget reads the live store off the loop, fed by CashBars.
	stats := &analytics.Stats{
		Conn: conn,
		CashBars: func(c *beach.Ctx) ([]analytics.CashBar, error) {
			snap, err := g.Snapshot(c.Context())
			if err != nil {
				return nil, err
			}
			bars := make([]analytics.CashBar, 0, len(snap.Players))
			for _, p := range snap.Players {
				if p.Spec {
					continue
				}
				bars = append(bars, analytics.CashBar{Label: p.Token + " " + p.Name, Cash: p.Cash})
			}
			return bars, nil
		},
	}
	app.Page("/stats", srv.StatsPage)
	app.Page("/stats/rolls", analytics.Widget("anl-rolls", stats.Rolls))
	app.Page("/stats/tiles", analytics.Widget("anl-tiles", stats.Tiles))
	app.Page("/stats/cash", analytics.Widget("anl-cash", stats.Cash))

	// The living component showcase, mounted so the example doubles as a kit
	// browser.
	app.Page("/specimen", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: specimen.Page()}, nil
	})

	go g.Run(ctx)

	// Keep the durable copy current: snapshot the live store to Postgres every
	// 15s so a restart resumes the game mid-play.
	go saveLoop(ctx, g, snap)

	// The race ticker is the chart's animation engine: each second it snapshots
	// the game, re-lays-out the cash race, and publishes the rendered fragment —
	// but only when the standings actually changed, so idle tables stay silent.
	// The client does no animation work: Datastar morphs the new SVG in and the
	// chart CSS transitions tween the geometry.
	var lastRace []byte
	go h.Ticker(ctx, web.RaceTopic, time.Second, func() (hub.Event, bool) {
		bs, err := g.Snapshot(ctx)
		if err != nil {
			return hub.Event{}, false
		}
		frame := web.RenderRace(bs)
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

// saveLoop persists a fresh snapshot every 15s until ctx is cancelled. The live
// game is the in-memory store; this loop keeps Postgres's durable copy current
// so a restart resumes mid-game. A save failure is logged-by-return — the loop
// keeps trying on the next tick rather than tearing down the game.
func saveLoop(ctx context.Context, g *game.Game, snap store.Snap) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			data, tick, err := g.SaveSnapshot(ctx)
			if err != nil {
				continue
			}
			_ = snap.Save(ctx, data, tick)
		}
	}
}
