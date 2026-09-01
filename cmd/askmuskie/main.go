// Command askmuskie is askmuskie.com: a shared, server-authoritative pond where
// a talking Muskie roams, takes questions from live visitors, and speaks wisdom
// in a retro klattsch voice. There is no LLM and no database — quotes come from
// a flat file (data/quotes.txt), and the whole live world (the Muskie's
// position, the speak queue, who's present, and the food) lives in memory in the
// pond sim. A restart resets the pond.
//
// Run it:
//
//	go build -o bin/askmuskie ./cmd/askmuskie
//	PORT=8080 ./bin/askmuskie
//
// Then open http://localhost:8080/ in two browsers to see the shared pond.
//
// This file is wiring: load the quotes + pronunciation dictionary, start the
// pond sim, and serve the page, the realtime SSE snapshot stream, and the ask /
// drop-food commands. Markup is in views.templ; the client island in
// static/js/muskie.js.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/chart"
	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
	"github.com/ThirdCoastInteractive/Beach/pkg/pg"

	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/authx"
	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/chat"
	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/oracle"
	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/pond"
	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/quotes"
	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/speech"
	"github.com/ThirdCoastInteractive/Beach/cmd/askmuskie/internal/stats"
)

//go:embed static
var staticFS embed.FS

//go:embed data/quotes.txt
var quotesData []byte

//go:embed migrations/*.sql
var migrationsFS embed.FS

// cookieName carries the anonymous visitor id. It is the only identity — there
// are no accounts. The page embeds the same id so the island knows which visitor
// in the world snapshot is "me".
const cookieName = "am_sid"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// The Muskie only knows what the flat file says; an empty repertoire is broken.
	qs, err := quotes.Parse(bytes.NewReader(quotesData))
	if err != nil {
		log.Fatalf("askmuskie: parse quotes: %v", err)
	}
	wisdom := quotes.NewSet(qs)
	if wisdom.Len() == 0 {
		log.Fatal("askmuskie: data/quotes.txt yielded no quotes")
	}

	// The embedded pronunciation dictionary is what lets klattsch speak English.
	voice, err := speech.Default()
	if err != nil {
		log.Fatalf("askmuskie: load pronunciation dictionary: %v", err)
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("askmuskie: static sub-fs: %v", err)
	}

	// The hub fans the pond's world snapshots out to every SSE client.
	h := hub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Per-account stats recorder + chat service exist only with a database.
	var statsRec pond.Recorder
	var chatSvc *chat.Chat

	cfg := beach.Config{
		Service: "askmuskie",
		Static:  sub,
		Logger:  logger,
		// three.js loads the model's embedded texture through a blob: URL, so widen
		// connect-src/img-src to allow blob:.
		CSP: beach.CSPDefault().AllowConnect("blob:").AllowImg("blob:"),
	}

	// Optional database: with DATABASE_URL set, accounts (chat/stats/leaderboard)
	// turn on; without it, askmuskie runs the anonymous pond exactly as before.
	release := os.Getenv("RELEASE") != ""
	var ah *authHandlers
	var pool *pgxpool.Pool
	authEnabled := false
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		p, err := pg.Pool(ctx, dsn)
		if err != nil {
			log.Fatalf("askmuskie: database connect failed: %v", err)
		}
		pool = p
		a := authx.New(pool, release)
		if err := a.Migrate(ctx, migrationsFS); err != nil {
			log.Fatalf("askmuskie: migrate failed: %v", err)
		}
		cfg.Sessions = a.Sessions
		cfg.Principals = a.PrincipalResolver
		ah = &authHandlers{auth: a, pool: pool}
		authEnabled = true
		st := stats.New(pool)
		go st.Run(ctx)
		statsRec = st
		chatSvc = chat.New(h, pool, st)
		logger.Info("askmuskie: accounts enabled (DATABASE_URL set)")
	}

	// Optional LLM oracle: with MUSKIE_LLM_URL set, the Muskie's lines are voiced
	// by a local model (an Ollama sidecar) from live state + the asker's message;
	// without it, he stays on the flat-file quotes. The model is a *content
	// author*, never exposed to raw user input without the output cap + filter in
	// the oracle package; messages are gated to logged-in visitors below.
	var orc pond.Oracle
	if url := os.Getenv("MUSKIE_LLM_URL"); url != "" {
		model := os.Getenv("MUSKIE_LLM_MODEL")
		if model == "" {
			model = "qwen3:4b"
		}
		ocfg := oracle.Config{BaseURL: url, Model: model}
		if pool != nil {
			ocfg.Stats = func(ctx context.Context, accountID int64) (fed, wisdom, chats int) {
				_ = pool.QueryRow(ctx,
					`SELECT fed, wisdom, chats FROM askmuskie_players WHERE user_id = $1`,
					accountID).Scan(&fed, &wisdom, &chats)
				return
			}
		}
		if c := oracle.New(ocfg); c != nil { // guard the typed-nil interface pitfall
			orc = c
			logger.Info("askmuskie: oracle enabled (MUSKIE_LLM_URL set)", "model", model)
		}
	}

	// The pond sim owns all live state on its own goroutine.
	world := pond.New(h, wisdom, voice, statsRec, orc)
	go world.Run(ctx)

	app := beach.New(cfg)

	// Cache-busted asset URLs (content-hashed ?v=) so island/CSS changes are picked
	// up across deploys despite the immutable static cache headers.
	jsURL := app.AssetURL("js/muskie.js")
	cssURL := app.AssetURL("css/askmuskie.css")

	app.Page("/", func(c *beach.Ctx) (beach.View, error) {
		sid := readSID(c.R)
		if sid == "" {
			sid = newID()
			c.SetCookie(sidCookie(sid))
		}
		username := ""
		if p, ok := c.Principal(); ok && p != nil {
			username = p.Username
		}
		// Leaderboard charts are rendered into a modal in the page (server-side
		// SVG); built here so they're current as of page load.
		var lbWisdom, lbFed, lbChats chart.BarChart
		if authEnabled && pool != nil {
			ctx := c.Context()
			lbWisdom = barChart(topBy(ctx, pool, "wisdom"), " quotes")
			lbFed = barChart(topBy(ctx, pool, "fed"), " fed")
			lbChats = barChart(topBy(ctx, pool, "chats"), " msgs")
		}
		return beach.View{Page: document(sid, jsURL, cssURL, authEnabled, username,
			authErrMessage(c.Query("ae")), c.Query("at"), lbWisdom, lbFed, lbChats)}, nil
	})

	// Realtime world snapshots: a raw JSON SSE stream (outside the Datastar
	// discipline) fed by the pond's hub topic. The connection is the visitor's
	// presence — join on open, leave on close.
	app.Raw(http.MethodGet, "/pond/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		sid := readSID(r)
		if sid == "" {
			sid = newID()
			http.SetCookie(w, sidCookie(sid))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		acct, name, shape, bc, sk := resolveVisitor(r, ah)
		world.Join(sid, acct, name, shape, bc, sk)
		defer world.Leave(sid)

		// One subscription, the shared hot/cold/moves frames + this visitor's own
		// self frame, plus chat. Each frame is self-describing JSON (a "k" tag), so
		// the loop just writes whatever arrives.
		topics := []string{pond.Topic, pond.ColdTopic, pond.MovesTopic, pond.SelfTopic(sid)}
		if chatSvc != nil {
			topics = append(topics, chat.Topic)
		}
		s := h.Subscribe(topics...)
		defer s.Close()

		// Open the stream immediately so the client's onopen fires; replay chat backlog.
		_, _ = w.Write([]byte(": connected\n\n"))
		if chatSvc != nil {
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(chatSvc.BacklogFrame())
			_, _ = w.Write([]byte("\n\n"))
		}
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, ok := <-s.C:
				if !ok {
					return
				}
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(ev.Bytes)
				_, _ = w.Write([]byte("\n\n"))
				flusher.Flush()
			}
		}
	})

	app.Raw(http.MethodPost, "/pond/ask", func(w http.ResponseWriter, r *http.Request) {
		sid := readSID(r)
		if sid == "" {
			http.Error(w, "no session", http.StatusBadRequest)
			return
		}
		var body struct {
			Skip bool   `json:"skip"`
			Msg  string `json:"msg"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body) // empty/!json body → skip=false
		// An untrusted message is only accepted from a logged-in (accountable,
		// rate-limited) visitor; anonymous asks still get state-aware wisdom.
		msg := body.Msg
		if p, ok := auth.PrincipalFrom(r.Context()); !ok || p == nil || p.UserID == 0 {
			msg = ""
		}
		world.Ask(sid, body.Skip, msg)
		w.WriteHeader(http.StatusNoContent)
	})

	app.Raw(http.MethodPost, "/pond/food", func(w http.ResponseWriter, r *http.Request) {
		sid := readSID(r)
		if sid == "" {
			http.Error(w, "no session", http.StatusBadRequest)
			return
		}
		var body struct {
			X float64 `json:"x"`
			Z float64 `json:"z"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		world.DropFood(sid, body.X, body.Z)
		w.WriteHeader(http.StatusNoContent)
	})

	app.Raw(http.MethodPost, "/pond/move", func(w http.ResponseWriter, r *http.Request) {
		sid := readSID(r)
		if sid == "" {
			http.Error(w, "no session", http.StatusBadRequest)
			return
		}
		var body struct {
			X float64 `json:"x"`
			Z float64 `json:"z"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		world.Move(sid, body.X, body.Z)
		w.WriteHeader(http.StatusNoContent)
	})

	// Account routes exist only when a database is wired. The auth forms are
	// plain POSTs (native submit → 303 redirect), CSP-clean — see auth.go.
	if authEnabled {
		app.Raw(http.MethodPost, "/auth/register", ah.register)
		app.Raw(http.MethodPost, "/auth/login", ah.login)
		app.Raw(http.MethodPost, "/auth/logout", ah.logout)
		app.Page("/tos", ah.tos)

		// Save a logged-in visitor's avatar appearance, and reflect it live.
		app.Raw(http.MethodPost, "/me/avatar", func(w http.ResponseWriter, r *http.Request) {
			acct, _, _, _, _ := resolveVisitor(r, ah)
			if acct == 0 {
				http.Error(w, "login required", http.StatusUnauthorized)
				return
			}
			var b struct {
				Shape string `json:"shape"`
				BC    int    `json:"bc"`
				SK    int    `json:"sk"`
			}
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			_, _ = ah.pool.Exec(r.Context(),
				`UPDATE askmuskie_players SET shape=$2, body_color=$3, skin_color=$4, updated_at=now() WHERE user_id=$1`,
				acct, b.Shape, b.BC, b.SK)
			world.SetAvatar(acct, b.Shape, b.BC, b.SK)
			w.WriteHeader(http.StatusNoContent)
		})

		// Chat: logged-in (= ToS-accepted) only; rate-limited inside the service.
		app.Raw(http.MethodPost, "/chat", func(w http.ResponseWriter, r *http.Request) {
			acct, name, _, _, _ := resolveVisitor(r, ah)
			if acct == 0 {
				http.Error(w, "login required", http.StatusUnauthorized)
				return
			}
			var b struct {
				Text string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			chatSvc.Post(acct, name, b.Text)
			w.WriteHeader(http.StatusNoContent)
		})
	}

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	logger.Info("askmuskie: starting", "addr", addr, "quotes", wisdom.Len(),
		"routes", "GET / , GET /pond/stream , POST /pond/ask , POST /pond/food , GET /static/...")
	if err := app.Start(addr); err != nil {
		log.Fatalf("askmuskie: %v", err)
	}
}

// readSID returns the visitor id from the request cookie, or "" if absent.
func readSID(r *http.Request) string {
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	return ""
}

// sidCookie builds the visitor-id cookie. HttpOnly: page script reads the id from
// the embedded data attribute, never from the cookie.
func sidCookie(id string) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	}
}

// newID returns a random 128-bit hex id.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// resolveVisitor maps a request to its pond identity: the logged-in account
// (id + username) and saved avatar appearance, or anon zero/defaults (bc/sk -1,
// which the pond derives from the session id). ah is nil when there is no DB.
func resolveVisitor(r *http.Request, ah *authHandlers) (accountID int64, name, shape string, bc, sk int) {
	bc, sk = -1, -1
	if ah == nil {
		return
	}
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok || p == nil || p.UserID == 0 {
		return
	}
	accountID, name = p.UserID, p.Username
	var s string
	var b, k int
	if err := ah.pool.QueryRow(r.Context(),
		`SELECT shape, body_color, skin_color FROM askmuskie_players WHERE user_id = $1`,
		accountID).Scan(&s, &b, &k); err == nil {
		shape, bc, sk = s, b, k
	}
	return
}
