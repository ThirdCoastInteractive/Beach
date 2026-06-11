// Command driftbottle is an Omegle-style anonymous stranger-chat: the hub/SSE
// showcase for the-beach. It is fully ephemeral — no database, no accounts, no
// chat persistence. The only state is in-memory: a per-session cookie identity,
// a matchmaking queue, and the live pairings. Two waiting strangers are paired
// onto a private hub topic; a "Next" button tears the pair down and re-queues.
//
// Run it:
//
//	go build -o bin/driftbottle ./examples/driftbottle
//	PORT=8080 ./bin/driftbottle        # or just ./bin/driftbottle to bind :8080
//
// Open http://localhost:8080/ in two browsers (or one normal + one private
// window, so they get distinct session cookies) and watch them get matched.
//
// Realtime model: every browser tab owns one SSE connection subscribed to
// exactly one hub topic — its own session topic "me:<sid>". Every UI transition
// for that session (moved to the waiting room, paired into a chat, a new
// message, the stranger left) is a fragment the server renders once and
// publishes to that topic. The matchmaker drives all transitions; the SSE loop
// the framework runs is a trivial one-topic subscribe. A chat message is
// rendered once per recipient and published to both partners' session topics.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/hub"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/ThirdCoastInteractive/the-beach/ui/driftwood"
	"github.com/a-h/templ"
)

// staticFS holds the two assets the page needs: the generated Stellar stylesheet
// (served at /static/css/stellar.css per framework convention) and the Datastar
// client the driftwood shell loads from /static/js/datastar.js.
//
//go:embed static
var staticFS embed.FS

const (
	// cookieName carries the anonymous session id. It is the only identity in the
	// whole app — there are no accounts. HttpOnly so page script can't read it.
	cookieName = "db_sid"

	// roomTarget is the DOM id of the single live region every state transition
	// morphs: the waiting room, the active chat, and the "stranger left" notice
	// all render into #room by its own id (outer morph).
	roomTarget = "room"

	// feedTarget is the DOM id of the message list inside an active chat. Each new
	// message re-renders the whole (small, rolling) feed and morphs #feed by id.
	feedTarget = "feed"

	// msgRateBurst / msgRateWindow rate-limit the composer per session: at most
	// msgRateBurst messages per msgRateWindow. A cheap moderation hook.
	msgRateBurst  = 8
	msgRateWindow = 10 * time.Second

	// maxMsgLen bounds a single message body; longer input is truncated. Keeps a
	// single SSE frame small and is a trivial abuse brake.
	maxMsgLen = 1000

	// feedCap bounds the rolling per-pair message buffer (ephemeral, never stored).
	feedCap = 100
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("driftbottle: static sub-fs: %v", err)
	}

	h := hub.New()

	app := beach.New(beach.Config{
		Service: "driftbottle",
		Static:  sub,
		Kit:     beach.KitStatic("driftwood"),
		Hub:     h,
		Logger:  logger,
	})

	srv := &server{
		hub:      h,
		log:      logger,
		sessions: map[string]*sessionState{},
		feeds:    map[string][]chatMessage{},
	}

	app.Page("/", srv.indexPage)
	app.Stream("/events", srv.eventsStream)
	app.Action("/start", srv.startAction)
	app.Action("/say", srv.sayAction)
	app.Action("/next", srv.nextAction)

	logger.Info("driftbottle: starting", "addr", addr,
		"routes", "GET / , GET /events , POST /start , POST /say , POST /next , GET /static/...")
	if err := app.Start(addr); err != nil {
		log.Fatalf("driftbottle: %v", err)
	}
}

// --- server state ------------------------------------------------------------

// server holds all in-memory chat state. Everything is ephemeral: lose the
// process, lose the conversations — which is the point.
type server struct {
	hub *hub.Hub
	log *slog.Logger

	mu       sync.Mutex
	waiting  string                   // session id parked in the lobby, or "" if none
	sessions map[string]*sessionState // every session that has connected
	feeds    map[string][]chatMessage // pair topic -> rolling message buffer
}

// sessionState is one anonymous stranger. partner is the session id of the
// person they're chatting with ("" when waiting/idle). msgTimes is a sliding
// window of recent send timestamps for rate limiting.
type sessionState struct {
	id       string
	partner  string
	last     string // most recent ex-partner, to avoid an instant rematch on Next
	msgTimes []time.Time
}

// chatMessage is one line in a pairing's ephemeral feed.
type chatMessage struct {
	Body string
	From string // sender session id (rewritten to "me"/"them" before rendering)
}

// meTopic is the single hub topic a session's SSE connection subscribes to.
func meTopic(sid string) string { return "me:" + sid }

// pairTopic is the stable private identity for a pairing: "pair:<a>:<b>" with
// the two session ids in sorted order, so both sides compute the same name. It
// keys the rolling feed buffer and shows up in logs.
func pairTopic(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return "pair:" + a + ":" + b
}

// state returns the sessionState for id, creating it on first sight. Caller
// holds s.mu.
func (s *server) state(id string) *sessionState {
	st := s.sessions[id]
	if st == nil {
		st = &sessionState{id: id}
		s.sessions[id] = st
	}
	return st
}

// --- session identity --------------------------------------------------------

// sid reads the session id from the request cookie, minting a new one if absent.
// This is app-level identity: the framework's session.Store needs a database,
// and driftbottle deliberately has none, so the anonymous principal is just this
// cookie. The bool reports whether a new id was minted (the page handler sets the
// cookie on the response so subsequent action/stream requests carry it).
func sid(c *beach.Ctx) (string, bool) {
	if ck, err := c.R.Cookie(cookieName); err == nil && ck.Value != "" {
		return ck.Value, false
	}
	return newID(), true
}

func setSIDCookie(c *beach.Ctx, id string) {
	http.SetCookie(c.W, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

// newID returns a random 128-bit hex id for a session.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// short trims an id for log lines.
func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// --- matchmaking -------------------------------------------------------------

// enqueue parks id in the lobby, or — if someone is already waiting — pairs the
// two and pushes a fresh chat view to both over their me: topics. Idempotent: a
// double-click or reconnect while already waiting/paired just re-renders the
// current view. Caller must NOT hold s.mu.
func (s *server) enqueue(id string) {
	s.mu.Lock()
	st := s.state(id)

	if st.partner != "" { // already chatting
		s.mu.Unlock()
		s.publish(id, chatFragment())
		return
	}
	if s.waiting == id { // already waiting
		s.mu.Unlock()
		s.publish(id, waitingFragment())
		return
	}
	// Pair only if there's a waiter who isn't us and isn't the person we just left
	// (avoid an instant rematch when only two strangers are online). Otherwise park.
	// The avoid is one-shot: parking here consumes the "last partner" guard so a
	// later attempt can rematch the same pair if no one else has shown up.
	other := s.waiting
	if other == "" || other == st.last || s.state(other).last == id {
		s.waiting = id
		st.last = ""
		s.mu.Unlock()
		s.publish(id, waitingFragment())
		return
	}

	// Pair the two waiting strangers.
	s.waiting = ""
	st.partner = other
	st.last = ""
	os := s.state(other)
	os.partner = id
	os.last = ""
	pt := pairTopic(id, other)
	delete(s.feeds, pt) // fresh conversation
	s.mu.Unlock()

	s.log.Info("driftbottle: paired", "pair", pt, "a", short(id), "b", short(other))
	s.publish(id, chatFragment())
	s.publish(other, chatFragment())
}

// teardown ends id's current pairing: clears both sides, notifies the stranger,
// re-queues the stranger (so they aren't stranded), and drops the feed buffer.
// It does NOT re-queue id — the caller decides. Caller must NOT hold s.mu.
func (s *server) teardown(id string) {
	s.mu.Lock()
	st := s.state(id)
	partner := st.partner
	if partner == "" {
		if s.waiting == id { // was only waiting: vacate the lobby
			s.waiting = ""
		}
		s.mu.Unlock()
		return
	}
	st.partner = ""
	st.last = partner
	if ps := s.sessions[partner]; ps != nil {
		ps.partner = ""
		ps.last = id
	}
	delete(s.feeds, pairTopic(id, partner))
	s.mu.Unlock()

	s.log.Info("driftbottle: unpaired", "a", short(id), "b", short(partner))
	s.publish(partner, endedFragment())
	s.enqueue(partner)
}

// rateOK reports whether id may send now, recording the send if so. Sliding
// window: drop timestamps older than the window, allow if under burst. Caller
// must NOT hold s.mu.
func (s *server) rateOK(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state(id)
	cutoff := time.Now().Add(-msgRateWindow)
	kept := st.msgTimes[:0]
	for _, t := range st.msgTimes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	st.msgTimes = kept
	if len(st.msgTimes) >= msgRateBurst {
		return false
	}
	st.msgTimes = append(st.msgTimes, time.Now())
	return true
}

// partnerOf returns id's current partner ("" if none). Caller must NOT hold s.mu.
func (s *server) partnerOf(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state(id).partner
}

// broadcast appends body to the pairing's rolling feed and fans the re-rendered
// feed to both sides (own/other highlighting flips per side). Caller must NOT
// hold s.mu.
func (s *server) broadcast(senderID, partner, body string) {
	pt := pairTopic(senderID, partner)

	s.mu.Lock()
	buf := append(s.feeds[pt], chatMessage{Body: body, From: senderID})
	if len(buf) > feedCap {
		buf = buf[len(buf)-feedCap:]
	}
	s.feeds[pt] = buf
	snapshot := make([]chatMessage, len(buf))
	copy(snapshot, buf)
	s.mu.Unlock()

	s.publish(senderID, feedFragment(forViewer(snapshot, senderID)))
	s.publish(partner, feedFragment(forViewer(snapshot, partner)))
}

// forViewer returns a copy of msgs with From rewritten to "me"/"them" for the
// given viewer, so the same buffer renders correctly on each side.
func forViewer(msgs []chatMessage, viewer string) []chatMessage {
	out := make([]chatMessage, len(msgs))
	for i, m := range msgs {
		from := "them"
		if m.From == viewer {
			from = "me"
		}
		out[i] = chatMessage{Body: m.Body, From: from}
	}
	return out
}

// publish renders frag to bytes once and fans it to id's session topic.
func (s *server) publish(id string, frag templ.Component) {
	s.hub.Publish(meTopic(id), hub.Event{Bytes: renderToBytes(frag)})
}

// --- handlers ----------------------------------------------------------------

// indexPage renders the full document on navigation and patches just the #room
// region on a Datastar request. The page opens the SSE stream on load; the
// initial #room is the lobby's "find a stranger" call to action. New visitors
// get their session cookie set here.
func (s *server) indexPage(c *beach.Ctx) (beach.View, error) {
	id, fresh := sid(c)
	if fresh {
		setSIDCookie(c, id)
	}

	body := s.kitContainer(c, lobbyFragment())
	page := driftwood.Page(driftwood.PageProps{
		Title:       "driftbottle — talk to a stranger",
		Description: "Anonymous, ephemeral stranger chat. No accounts, no history.",
	}, body)

	return beach.View{
		Page:     page,
		Fragment: lobbyFragment(),
		Target:   roomTarget,
	}, nil
}

// kitContainer wraps a room fragment in the page heading + container that opens
// the SSE stream on load. Shared by the navigation render.
func (s *server) kitContainer(c *beach.Ctx, room templ.Component) templ.Component {
	kit := c.Kit()
	heading := kit.Layout().PageHeading(ui.PageHeadingProps{
		Title:    "driftbottle",
		Subtitle: "You're about to talk to a complete stranger. Be kind.",
	})
	// data-on-load opens the per-session SSE stream the instant the page is
	// interactive, so server-driven transitions land without any user action.
	return kit.Layout().Container(ui.ContainerProps{
		Attrs: datastar.Attrs{datastar.Init("@get('/events')")},
	}, heading, room)
}

// eventsStream is the per-session SSE subscription: subscribe to exactly the
// caller's me: topic and replay one catch-up frame reflecting their current
// state (lobby / waiting / chatting), so a reconnect lands in the right place.
func (s *server) eventsStream(c *beach.Ctx) (beach.Sub, error) {
	id, _ := sid(c)
	return beach.Sub{
		Topics: []string{meTopic(id)},
		CatchUp: func(_ string, p beach.Patcher) error {
			return p.Patch(beach.Patch{Fragment: s.currentRoom(id), Target: roomTarget})
		},
	}, nil
}

// currentRoom renders the room fragment matching id's current state, for the SSE
// catch-up replay on (re)connect.
func (s *server) currentRoom(id string) templ.Component {
	s.mu.Lock()
	st := s.state(id)
	partner := st.partner
	waiting := s.waiting == id
	var feed []chatMessage
	if partner != "" {
		buf := s.feeds[pairTopic(id, partner)]
		feed = forViewer(append([]chatMessage(nil), buf...), id)
	}
	s.mu.Unlock()

	switch {
	case partner != "":
		return chatFragmentWith(feed)
	case waiting:
		return waitingFragment()
	default:
		return lobbyFragment()
	}
}

// startAction puts the caller into matchmaking from the lobby (the "Talk to a
// stranger" button) or skips to a new partner from the ended notice.
func (s *server) startAction(c *beach.Ctx) (beach.Patches, error) {
	id, _ := sid(c)
	s.enqueue(id)
	return nil, nil // the resulting view is pushed over SSE, not returned here
}

// sayAction posts a message to the caller's current partner. It enforces the
// per-session rate limit and the word filter, then broadcasts to both feeds over
// SSE. The composer input is cleared via a data-signals patch in the response.
func (s *server) sayAction(c *beach.Ctx) (beach.Patches, error) {
	id, _ := sid(c)

	partner := s.partnerOf(id)
	if partner == "" {
		// Not in a chat (race with a teardown): bounce them back to their room.
		return beach.Patches{{Fragment: s.currentRoom(id), Target: roomTarget}}, nil
	}

	body := clean(c.FormValue("msg"))
	if body == "" {
		return clearComposer(), nil
	}
	if !s.rateOK(id) {
		// Soft moderation: drop the message, nudge the sender, clear the box.
		return beach.Patches{
			{Fragment: noticeFragment("Slow down — you're sending too fast."), Target: "notice"},
			composerClearPatch(),
		}, nil
	}

	s.broadcast(id, partner, body)
	return clearComposer(), nil
}

// nextAction tears down the current pairing and immediately re-queues the
// caller, so "Next" finds a fresh stranger.
func (s *server) nextAction(c *beach.Ctx) (beach.Patches, error) {
	id, _ := sid(c)
	s.teardown(id)
	s.enqueue(id)
	return nil, nil // both the new view (caller) and the ended notice (ex-partner) go over SSE
}

// --- moderation hooks --------------------------------------------------------

// clean trims, length-caps, and word-filters a message body. The filter is a
// deliberately tiny demo blocklist — the hook is the point, not the list.
func clean(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxMsgLen {
		s = s[:maxMsgLen]
	}
	return filterWords(s)
}

// blocklist is the demo word filter. Replace with a real moderation source.
var blocklist = []string{"badword", "slur1", "slur2"}

// filterWords masks any blocklisted term (case-insensitive, substring) with
// asterisks. Crude on purpose — it shows where moderation plugs in.
func filterWords(s string) string {
	low := strings.ToLower(s)
	for _, w := range blocklist {
		for {
			i := strings.Index(low, w)
			if i < 0 {
				break
			}
			s = s[:i] + strings.Repeat("*", len(w)) + s[i+len(w):]
			low = low[:i] + strings.Repeat("*", len(w)) + low[i+len(w):]
		}
	}
	return s
}

// --- composer response patches ----------------------------------------------

// clearComposer returns the patch set that empties the message input after a
// send (a data-signals patch clearing the bound "msg" signal).
func clearComposer() beach.Patches {
	return beach.Patches{composerClearPatch()}
}

func composerClearPatch() beach.Patch {
	return beach.Patch{Signals: map[string]any{"msg": ""}}
}

// renderToBytes renders a templ component to bytes for a hub.Event. The hub fans
// these pre-rendered bytes to every subscriber; the framework's stream loop
// PatchElements them, morphing the target element by its id.
func renderToBytes(c templ.Component) []byte {
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		return nil
	}
	return buf.Bytes()
}
