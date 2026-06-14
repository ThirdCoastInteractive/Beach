package main

// The small data-shaping helpers views.templ leans on. Markup lives in
// views.templ; these stay Go so the templates read as plain markup.

import (
	"fmt"
	"sort"

	"github.com/ThirdCoastInteractive/the-beach/pkg/chart"
	"github.com/a-h/templ"
)

// DOM ids the fragments morph into (outer morph by own id).
const (
	boardTarget = "bw-board"
	handTarget  = "bw-hand"
	raceTarget  = "bw-race"
)

// turnName labels the current turn for the board banner.
func turnName(snap boardSnapshot) string {
	if snap.board.Turn < len(snap.players) {
		tp := snap.players[snap.board.Turn]
		return tp.Token + " " + tp.Name
	}
	return "—"
}

// tokensOn concatenates the tokens of the players standing on tile idx.
func tokensOn(snap boardSnapshot, idx int) string {
	var here string
	for _, p := range snap.players {
		if !p.Spec && p.Square == idx {
			here += p.Token
		}
	}
	return here
}

// deedsOf lists the deeds a token holds, ordered by tile index — snap.owners
// is a map, and a stable order keeps successive SSE morphs of the hand quiet.
func deedsOf(snap boardSnapshot, token string) []string {
	var tiles []int
	for tile, owner := range snap.owners {
		if owner == token {
			tiles = append(tiles, tile)
		}
	}
	sort.Ints(tiles)
	out := make([]string, len(tiles))
	for i, tile := range tiles {
		out[i] = fmt.Sprintf("%s (%s rent)", board[tile].Name, dollars(board[tile].Rent))
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
// group's series token. groupColor's output is a fixed token string from a
// closed switch (no user input reaches it), so bypassing templ's CSS property
// sanitizer — which predates var() — is safe.
func groupAccent(group string) templ.SafeCSS {
	return templ.SafeCSS("border-top:0.4rem solid " + groupColor(group))
}

// raceLayout builds one cash-race frame from a board snapshot: every seat is a
// contestant, its value the seat's cash. Seat labels never change, so
// LayoutBarRace's label-derived element ids stay put across frames and the
// client tweens the bar geometry between SSE patches.
func raceLayout(snap boardSnapshot) chart.BarRaceLayout {
	bars := make([]chart.Datum, 0, len(snap.players))
	for _, p := range snap.players {
		bars = append(bars, chart.Datum{Label: p.Token + " " + p.Name, Value: float64(p.Cash)})
	}
	return chart.LayoutBarRace(chart.BarRaceInput{
		Title: "Cash race",
		Bars:  bars,
		Width: 520,
		RowH:  44,
	})
}
