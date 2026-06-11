package main

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"io"

	"github.com/ThirdCoastInteractive/the-beach/ecs"
	"github.com/a-h/templ"
)

// DOM ids the fragments morph into (outer morph by own id).
const (
	boardTarget = "bw-board"
	handTarget  = "bw-hand"
)

// boardSnapshot is a read-only copy of everything the board surface renders,
// built on the sim loop so the renderer never touches the live store off-loop.
type boardSnapshot struct {
	board   Board
	players []playerView
	owners  map[int]string // tile index -> owner token ("" = bank)
}

type playerView struct {
	Seat   int
	Name   string
	Token  string
	Square int
	Cash   int
	Spec   bool
}

// snapshotBoard reads the whole live slice into a plain value. Loop-goroutine
// only (called from a projection View).
func snapshotBoard(store *ecs.Store, seats []ecs.Entity, deeds map[int]ecs.Entity, gameEnt ecs.Entity) boardSnapshot {
	b, _ := ecs.Get[Board](store, gameEnt)
	snap := boardSnapshot{board: b, owners: make(map[int]string)}
	for i, e := range seats {
		p, _ := ecs.Get[Player](store, e)
		pos, _ := ecs.Get[Position](store, e)
		cash, _ := ecs.Get[Cash](store, e)
		snap.players = append(snap.players, playerView{
			Seat: i, Name: p.Name, Token: p.Token,
			Square: pos.Square, Cash: cash.Amount, Spec: p.Spec,
		})
	}
	for tile, de := range deeds {
		own, _ := ecs.Get[Ownership](store, de)
		if own.Owner != 0 {
			op, _ := ecs.Get[Player](store, own.Owner)
			snap.owners[tile] = op.Token
		}
	}
	return snap
}

// boardFragment renders the whole shared board surface: the tile ring, the log,
// and the standings. It carries id=boardTarget so every projection morphs it in
// place.
func boardFragment(snap boardSnapshot) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		var sb bytes.Buffer
		sb.WriteString(`<div id="` + boardTarget + `" class="bw-board">`)

		// Turn banner.
		turnName := "—"
		if snap.board.Turn < len(snap.players) {
			tp := snap.players[snap.board.Turn]
			turnName = tp.Token + " " + tp.Name
		}
		sb.WriteString(`<p class="bw-turn">Current turn: <strong>` + esc(turnName) + `</strong>`)
		if snap.board.LastD1 > 0 {
			fmt.Fprintf(&sb, ` <span class="bw-dice">last roll %d + %d</span>`, snap.board.LastD1, snap.board.LastD2)
		}
		sb.WriteString(`</p>`)

		// Tile ring as a grid of squares.
		sb.WriteString(`<div class="bw-tiles">`)
		for idx, t := range board {
			cls := "bw-tile"
			style := ""
			if t.Group != "" {
				style = `style="border-top:0.4rem solid ` + groupColor(t.Group) + `"`
			}
			sb.WriteString(`<div class="` + cls + `" ` + style + `>`)
			fmt.Fprintf(&sb, `<div class="bw-tile-name">%s</div>`, esc(t.Name))
			if t.Kind == kindProperty {
				if owner, ok := snap.owners[idx]; ok {
					fmt.Fprintf(&sb, `<div class="bw-tile-meta">owned %s</div>`, esc(owner))
				} else {
					fmt.Fprintf(&sb, `<div class="bw-tile-meta">$%d</div>`, t.Price)
				}
			}
			// Tokens standing on this tile.
			var here string
			for _, p := range snap.players {
				if !p.Spec && p.Square == idx {
					here += p.Token
				}
			}
			if here != "" {
				sb.WriteString(`<div class="bw-tile-here">` + here + `</div>`)
			}
			sb.WriteString(`</div>`)
		}
		sb.WriteString(`</div>`)

		// Standings.
		sb.WriteString(`<div class="bw-standings"><h3>Standings</h3><ul>`)
		for _, p := range snap.players {
			name := p.Name
			if p.Spec {
				fmt.Fprintf(&sb, `<li class="bw-open">%s <em>open seat</em></li>`, esc(name))
				continue
			}
			turnMark := ""
			if p.Seat == snap.board.Turn {
				turnMark = ` <span class="bw-toturn">▶</span>`
			}
			fmt.Fprintf(&sb, `<li>%s %s — $%d%s</li>`, p.Token, esc(name), p.Cash, turnMark)
		}
		sb.WriteString(`</ul></div>`)

		// Log (newest last).
		sb.WriteString(`<div class="bw-log"><h3>Game log</h3><ol>`)
		for _, ln := range snap.board.Log {
			sb.WriteString(`<li>` + esc(ln) + `</li>`)
		}
		sb.WriteString(`</ol></div>`)

		sb.WriteString(`</div>`)
		_, err := io.WriteString(w, sb.String())
		return err
	})
}

// handFragment renders one player's private panel: their cash, position, and
// the deeds they hold. Carries id=handTarget.
func handFragment(snap boardSnapshot, seat int) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		var sb bytes.Buffer
		sb.WriteString(`<div id="` + handTarget + `" class="bw-hand">`)
		if seat < 0 || seat >= len(snap.players) {
			sb.WriteString(`<p>Spectating. Join to get a hand.</p></div>`)
			_, err := io.WriteString(w, sb.String())
			return err
		}
		p := snap.players[seat]
		fmt.Fprintf(&sb, `<h3>%s %s</h3>`, p.Token, esc(p.Name))
		fmt.Fprintf(&sb, `<p class="bw-cash">Cash: <strong>$%d</strong></p>`, p.Cash)
		fmt.Fprintf(&sb, `<p>On: <strong>%s</strong></p>`, esc(board[p.Square].Name))

		sb.WriteString(`<h4>Your deeds</h4><ul class="bw-deeds">`)
		any := false
		for tile, owner := range snap.owners {
			if owner == p.Token {
				fmt.Fprintf(&sb, `<li>%s ($%d rent)</li>`, esc(board[tile].Name), board[tile].Rent)
				any = true
			}
		}
		if !any {
			sb.WriteString(`<li class="bw-muted">none yet</li>`)
		}
		sb.WriteString(`</ul></div>`)
		_, err := io.WriteString(w, sb.String())
		return err
	})
}

// renderToBytes renders a templ component to bytes for a hub.Event / projection.
func renderToBytes(c templ.Component) []byte {
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		return nil
	}
	return buf.Bytes()
}

func esc(s string) string { return html.EscapeString(s) }
