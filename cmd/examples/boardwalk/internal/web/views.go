package web

// The small data-shaping helpers views.templ leans on. Markup lives in
// views.templ; these stay Go so the templates read as plain markup.

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/boardwalk/internal/game"
	"github.com/ThirdCoastInteractive/Beach/pkg/chart"
	"github.com/a-h/templ"
)

// DOM ids the fragments morph into (outer morph by own id).
const (
	boardTarget = "bw-board"
	handTarget  = "bw-hand"
	raceTarget  = "bw-race"
)

// renderToBytes renders a templ component to bytes for a hub.Event / projection.
func renderToBytes(c templ.Component) []byte {
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		return nil
	}
	return buf.Bytes()
}

// RenderBoard renders the shared board fragment to bytes. The sim's board
// projection (game package) renders through this hook, injected at construction
// so game never imports web.
func RenderBoard(snap game.Snapshot) []byte { return renderToBytes(boardFragment(snap)) }

// RenderHand renders a seat's private hand fragment to bytes. The sim's per-seat
// Cash projection renders through this hook.
func RenderHand(snap game.Snapshot, seat int) []byte { return renderToBytes(handFragment(snap, seat)) }

// RenderRace renders the cash-race fragment to bytes. The race ticker in main
// re-renders and publishes this each second.
func RenderRace(snap game.Snapshot) []byte { return renderToBytes(raceFragment(raceLayout(snap))) }

// turnName labels the current turn for the board banner.
func turnName(snap game.Snapshot) string {
	if snap.Board.Turn < len(snap.Players) {
		tp := snap.Players[snap.Board.Turn]
		return tp.Token + " " + tp.Name
	}
	return "—"
}

// tokensOn concatenates the tokens of the players standing on tile idx.
func tokensOn(snap game.Snapshot, idx int) string {
	var here string
	for _, p := range snap.Players {
		if !p.Spec && p.Square == idx {
			here += p.Token
		}
	}
	return here
}

// deedsOf lists the deeds a token holds, ordered by tile index — snap.Owners
// is a map, and a stable order keeps successive SSE morphs of the hand quiet.
func deedsOf(snap game.Snapshot, token string) []string {
	var tiles []int
	for tile, owner := range snap.Owners {
		if owner == token {
			tiles = append(tiles, tile)
		}
	}
	sort.Ints(tiles)
	out := make([]string, len(tiles))
	for i, tile := range tiles {
		out[i] = fmt.Sprintf("%s (%s rent)", game.Tiles[tile].Name, dollars(game.Tiles[tile].Rent))
	}
	return out
}

// dollars formats a cash amount with the sign outside the $ (chance and rent
// can push a balance negative).
func dollars(n int) string {
	if n < 0 {
		return fmt.Sprintf("-$%d", -n)
	}
	return fmt.Sprintf("$%d", n)
}

// groupAccent is a tile's per-instance group swatch: a border-top in the
// group's series token. game.GroupColor's output is a fixed token string from a
// closed switch (no user input reaches it), so bypassing templ's CSS property
// sanitizer — which predates var() — is safe.
func groupAccent(group string) templ.SafeCSS {
	return templ.SafeCSS("border-top:0.4rem solid " + game.GroupColor(group))
}

// raceLayout builds one cash-race frame from a board snapshot: every seat is a
// contestant, its value the seat's cash. Seat labels never change, so
// LayoutBarRace's label-derived element ids stay put across frames and the
// client tweens the bar geometry between SSE patches.
func raceLayout(snap game.Snapshot) chart.BarRaceLayout {
	bars := make([]chart.Datum, 0, len(snap.Players))
	for _, p := range snap.Players {
		bars = append(bars, chart.Datum{Label: p.Token + " " + p.Name, Value: float64(p.Cash)})
	}
	return chart.LayoutBarRace(chart.BarRaceInput{
		Title: "Cash race",
		Bars:  bars,
		Width: 520,
		RowH:  44,
	})
}
