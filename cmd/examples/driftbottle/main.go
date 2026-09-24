// Command driftbottle is an Omegle-style anonymous stranger-chat: the hub/SSE
// fan-out benchmark for Beach. The LIVE matchmaking and feed stay in memory
// on the hot path (internal/chat) — a per-session cookie identity, a matchmaking
// queue, and the live pairings — because driftbottle measures fan-out and
// nothing may sit in the path of a message. Two waiting strangers are paired
// onto a private hub topic; a "Next" button tears the pair down and re-queues.
//
// Persistence and analytics live entirely OFF the hot path (internal/analytics),
// and both are required to boot: Postgres archives transcripts (a background
// writer drains a buffered channel of pairings and messages — never read on the
// hot path) and ClickHouse holds the event firehose (one fire-and-forget row per
// session, queue, pairing, message, and teardown), aggregated by the public
// /stats page. A missing Postgres or ClickHouse DSN aborts boot rather than
// degrading.
//
// Run it as a container stack (the app, its Postgres, and its ClickHouse):
//
//	cd examples/driftbottle
//	cp .env.example .env
//	docker compose up                  # → http://localhost:8080
//
// Run outside compose with both DSNs set:
//
//	DATABASE_URL=postgres://user:pass@localhost/driftbottle \
//	CLICKHOUSE_DSN=clickhouse://default:@localhost:9000/driftbottle \
//	  go run ./examples/driftbottle
//
// Open http://localhost:8080/ in two browsers (or one normal + one private
// window, so they get distinct session cookies) and watch them get matched.
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"log/slog"
	"os"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/ch"
	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/specimen"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/driftbottle/internal/analytics"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/driftbottle/internal/chat"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/driftbottle/internal/sockets"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed static
var staticFS embed.FS

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Load and validate config first: a missing Postgres or ClickHouse DSN aborts
	// here with the full list, before anything serves. There is no database-less
	// mode — driftbottle archives transcripts and runs a firehose, both required.
	conf := loadConfig()
	addr := ":" + conf.Port

	ctx := context.Background()

	// Postgres: the transcript ARCHIVE (off the hot path). Required — connect and
	// migrate or die loudly. The live wall never reads these tables; the analytics
	// writer drains the persist channel into them.
	pool, err := pg.Pool(ctx, conf.DSN)
	if err != nil {
		log.Fatalf("driftbottle: database connect failed: %v", err)
	}
	if err := pg.Migrate(ctx, pool, migrationsFS); err != nil {
		log.Fatalf("driftbottle: migrate failed: %v", err)
	}

	// ClickHouse: the event firehose (off the hot path). Required — a bad DSN or
	// migration is fatal; there is no analytics-off mode.
	conn, err := ch.Connect(ctx, conf.CHDSN)
	if err != nil {
		log.Fatalf("driftbottle: clickhouse connect failed: %v", err)
	}
	if err := ch.Migrate(ctx, conn, analytics.Migrations()); err != nil {
		log.Fatalf("driftbottle: clickhouse migrate failed: %v", err)
	}

	h := hub.New()

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("driftbottle: static sub-fs: %v", err)
	}

	app := beach.New(beach.Config{
		Service: "driftbottle",
		Hub:     h,
		Static:  static,
		Logger:  logger,
	})

	// Off-the-hot-path sink: the firehose batcher + the transcript archive writer.
	anl := analytics.New(pool, conn)
	anl.Run(ctx)

	// The live chat server: in-memory matchmaking + fan-out, firing events at anl.
	srv := chat.New(h, logger, anl)
	srv.Routes(app)

	// The WebSocket demo surface (RFC 05): /sockets + /ws/echo + /ws/tick — the
	// non-hypermedia channel next door to the SSE fan-out.
	sockets.Routes(app)

	// The public analytics page (internal/analytics): aggregates only, no auth —
	// driftbottle is anonymous. Each widget is a deferred fragment route.
	app.Page("/stats", anl.StatsPage)
	app.Page("/stats/sessions", analytics.Widget("anl-sessions", anl.Sessions))
	app.Page("/stats/pairings", analytics.Widget("anl-pairings", anl.Pairings))
	app.Page("/stats/messages", analytics.Widget("anl-messages", anl.Messages))
	// The living component/chart showcase, mounted on every example.
	app.Page("/specimen", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: specimen.Page()}, nil
	})

	logger.Info("driftbottle: starting", "addr", addr,
		"routes", "GET / , GET /events , POST /start , POST /say , POST /next , GET /stats , GET /sockets , GET /ws/echo , GET /ws/tick , GET /specimen , GET /static/...")
	if err := app.Start(addr); err != nil {
		log.Fatalf("driftbottle: %v", err)
	}
}
