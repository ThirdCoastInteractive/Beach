// Package web is boardwalk's HTTP surface: the page/action handlers, the SSE
// stream handlers, and the templ views they render. Handlers hang off the Server
// struct so they share the game (live model + Snapshot); they take *beach.Ctx
// like any beach handler. main constructs the Server and wires the routes.
package web

import (
	"net/http"
	"strconv"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/boardwalk/internal/game"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"
)

// seatCookie names the cookie that ties a browser to its claimed seat. With no
// database, this is the whole "session" — a small integer seat index. A browser
// without it is a spectator.
const seatCookie = "bw_seat"

// RaceTopic is the hub topic the cash-race ticker publishes frames to and the
// /race stream subscribes to. The race is main-level wiring (a ticker), not a
// sim projection, but the race stream surface lives here with the other streams.
const RaceTopic = "race"

// Server holds the handler dependencies: the game (live model + Snapshot).
// Handlers are methods only to share these.
type Server struct {
	game *game.Game
}

// NewServer builds the HTTP surface over the live game.
func NewServer(g *game.Game) *Server { return &Server{game: g} }

// seatFrom reads the browser's claimed seat from its cookie, or -1 (spectator).
func seatFrom(c *beach.Ctx) int {
	ck, err := c.R.Cookie(seatCookie)
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(ck.Value)
	if err != nil || n < 0 || n >= game.MaxSeats {
		return -1
	}
	return n
}

// IndexPage renders the full board page on navigation and patches the board on a
// Datastar request. The page opens three SSE streams on load: the shared board,
// the private hand (if seated), and the cash race.
func (s *Server) IndexPage(c *beach.Ctx) (beach.View, error) {
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

// JoinAction claims a seat for this browser. It Asks the sim to claim the next
// open seat, sets the seat cookie, and patches a data-init element that re-opens
// the hand stream so the controls switch to "Roll" for the new seat.
func (s *Server) JoinAction(c *beach.Ctx) (beach.Patches, error) {
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

// RollAction rolls for this browser's seat. The Ask returns the outcome; the
// board/hand projections push the visible state, so the action itself only needs
// to surface a toast on an invalid move (not your turn, open seat).
func (s *Server) RollAction(c *beach.Ctx) (beach.Patches, error) {
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

// BoardStream is the shared board surface, subscribed by every connection
// (spectators included — it is unauthenticated). Catch-up renders the current
// board immediately so a fresh connection is correct before the next projection.
func (s *Server) BoardStream(c *beach.Ctx) (beach.Sub, error) {
	snap, err := s.game.Snapshot(c.Context())
	if err != nil {
		return beach.Sub{}, err
	}
	return beach.Sub{
		Topics: []string{game.BoardTopic},
		CatchUp: func(_ string, p beach.Patcher) error {
			return p.Patch(beach.Patch{Fragment: boardFragment(snap), Target: boardTarget})
		},
	}, nil
}

// HandStream is this browser's private hand on its user:<seat> topic. A
// spectator (no seat) subscribes to nothing and just gets the spectator
// catch-up frame.
func (s *Server) HandStream(c *beach.Ctx) (beach.Sub, error) {
	seat := seatFrom(c)
	snap, err := s.game.Snapshot(c.Context())
	if err != nil {
		return beach.Sub{}, err
	}
	var topics []string
	if seat >= 0 {
		topics = []string{game.UserTopic(seat)}
	}
	return beach.Sub{
		Topics: topics,
		CatchUp: func(_ string, p beach.Patcher) error {
			return p.Patch(beach.Patch{Fragment: handFragment(snap, seat), Target: handTarget})
		},
	}, nil
}

// RaceStream is the shared cash-race surface. Catch-up renders the current
// standings immediately; afterwards the ticker's published frames keep the
// chart moving (the ticker dedupes, so this catch-up is what a fresh
// connection sees until the standings change).
func (s *Server) RaceStream(c *beach.Ctx) (beach.Sub, error) {
	snap, err := s.game.Snapshot(c.Context())
	if err != nil {
		return beach.Sub{}, err
	}
	return beach.Sub{
		Topics: []string{RaceTopic},
		CatchUp: func(_ string, p beach.Patcher) error {
			return p.Patch(beach.Patch{Fragment: raceFragment(raceLayout(snap)), Target: raceTarget})
		},
	}, nil
}

// StatsPage renders the public analytics page (the deferred widget grid).
func (s *Server) StatsPage(c *beach.Ctx) (beach.View, error) {
	return beach.View{Page: statsPage()}, nil
}
