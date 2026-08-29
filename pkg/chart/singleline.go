package chart

import (
	"fmt"
	"sort"
)

// singleline.go computes the geometry for an electrical single-line diagram: a
// tiered node-and-bus schematic where the vertical position is the electrical tier
// (supply at the top, load at the bottom), the horizontal position is the
// redundancy lane, and busbars are continuous horizontal runs that taps drop onto.
// It is pure geometry — no templ, no web, no domain types — the same contract every
// other layout in this package keeps. The renderer (SingleLineSVG in svg.templ)
// emits the markup; a caller (atlas) builds the SingleLineData from its rows and
// decorates each node with a status color + badge before layout.
//
// Unlike the Sankey (which emits flow ribbons sized by value), the single-line is a
// schematic: every node is a fixed-size glyph and every conductor is a thin
// orthogonal run. The bus-spine geometry the single-line needs — horizontal bus
// runs, tap drops, role-typed conductors, gear glyphs — is what this file adds on
// top of the layered-tier vocabulary the Sankey established.

// --- Input types ------------------------------------------------------------

// SingleLineData is the graph to lay out. Nodes and buses are placed by tier + lane;
// edges connect them. IDs are the caller's stable element ids (e.g. "sln-ups-123"),
// reused verbatim as SVG ids so a live patch can target a node in place.
type SingleLineData struct {
	Nodes []SingleLineNode
	Buses []SingleLineBus
	Edges []SingleLineEdge
}

// SingleLineNode is one drawn element (a feed, gear instance, PDU, panel, or rack).
// Tier is the vertical rank (smaller = closer to the supply); Lane is the redundancy
// lane ("a"/"b"/"c"/"other"). Color is the status color (a CSS var or token color);
// Glyph selects the symbol shape. Badge is the short live-state label (may be empty).
type SingleLineNode struct {
	ID        string
	Kind      string // feed, switchgear, transformer, ats, ups, genset, panel, floor_pdu, rpp, rack_pdu, rack
	Label     string
	Sub       string // a second line under the label (rating / headroom chip), may be empty
	Lane      string
	Tier      int
	Color     string // node outline / fill-accent color (status or lane color)
	Glyph     string // rect | circle | transformer | source
	Badge     string // live-state badge text, may be empty
	BadgeRole string // good | warn | bad | info | muted — drives the badge color in the templ
	Href      string // drill-in target, may be empty
	Tip       string // hover tooltip HTML
}

// SingleLineBus is a busbar: a horizontal conductor run. Lineup + Position group and
// order the bolted sections; Color is the lane color; Tier is the bus rank. The
// renderer joins same-lineup buses into one continuous run.
type SingleLineBus struct {
	ID       string
	Label    string
	Lineup   string
	Position int
	Lane     string
	Tier     int
	Color    string
	State    string
	Tip      string
}

// SingleLineEdge is a conductor between two endpoints. FromID/ToID reference a node
// ID or a bus ID; Role types the conductor (source/tap/feeder/tie/continuation/
// chain). Color is the lane color. The from/to ids are emitted as data attributes so
// the hover chain-trace can walk the graph.
type SingleLineEdge struct {
	FromID string
	ToID   string
	Role   string
	Color  string
	Tip    string
}

// --- Output geometry --------------------------------------------------------

// SingleLine is the laid-out diagram. Empty reports whether nothing was drawable
// (the caller renders an empty-state card instead of a blank SVG).
type SingleLine struct {
	VW, VH     float64
	Nodes      []SingleLineLayoutNode
	Buses      []SingleLineLayoutBus
	Conductors []SingleLineLayoutEdge
	Empty      bool
}

type SingleLineLayoutNode struct {
	ID             string
	Kind           string
	Label          string
	Sub            string
	X, Y, W, H     float64 // top-left + size of the glyph box
	CX, CY         float64 // center
	Color          string
	Glyph          string
	Badge          string
	BadgeRole      string
	Href           string
	Tip            string
	LabelY         float64 // baseline of the label text (below the glyph)
	SubY           float64
	BadgeX, BadgeY float64
}

type SingleLineLayoutBus struct {
	ID        string
	Label     string
	X1, X2, Y float64 // the run endpoints
	Color     string
	State     string
	Tip       string
	LabelX    float64
	LabelY    float64
}

type SingleLineLayoutEdge struct {
	PathD  string
	Role   string
	Color  string
	Tip    string
	FromID string
	ToID   string
	Dashed bool
}

// --- Tuning -----------------------------------------------------------------

const (
	slVW      = 1000.0
	slPadL    = 24.0
	slPadR    = 24.0
	slPadT    = 28.0
	slPadB    = 20.0
	slTierGap = 92.0 // vertical distance between adjacent occupied tiers
	slNodeW   = 116.0
	slNodeH   = 34.0
	slLaneGap = 18.0 // horizontal gap between nodes sharing a tier
	slBusH    = 6.0  // visual thickness band a bus reserves
	slLabelDY = 13.0 // label baseline below the glyph box
	slSubDY   = 25.0
)

// laneOrder ranks the redundancy lanes left-to-right. 'a' leftmost, 'other' last.
var slLaneOrder = map[string]int{"a": 0, "b": 1, "c": 2, "other": 3, "": 4}

// LayoutSingleLine places the graph. The pipeline:
//
//  1. collapse the sparse tier ranks to dense rows (a chain that skips genset/
//     transformer does not leave vertical gaps);
//  2. assign each node a row (its tier) and, within the row, a column ordered by
//     lane then label — so the redundancy lanes read as vertical columns;
//  3. lay buses as horizontal runs at their tier row, one run per (lineup) group
//     spanning its sections in position order;
//  4. route each conductor as an orthogonal drop between its endpoints (node center
//     or bus attach point), dashed for tie/continuation couplers.
//
// Layout is fully derived from tier + lane + lineup; nothing is stored.
func LayoutSingleLine(data SingleLineData) SingleLine {
	if len(data.Nodes) == 0 && len(data.Buses) == 0 {
		return SingleLine{VW: slVW, VH: 160, Empty: true}
	}

	// --- 1. dense tier rows. Gather every occupied tier across nodes + buses. ----
	tierSet := map[int]bool{}
	for _, n := range data.Nodes {
		tierSet[n.Tier] = true
	}
	for _, b := range data.Buses {
		tierSet[b.Tier] = true
	}
	tiers := make([]int, 0, len(tierSet))
	for t := range tierSet {
		tiers = append(tiers, t)
	}
	sort.Ints(tiers)
	rowOf := make(map[int]int, len(tiers))
	for i, t := range tiers {
		rowOf[t] = i
	}
	vh := slPadT + float64(len(tiers))*slTierGap + slPadB
	if vh < 160 {
		vh = 160
	}

	out := SingleLine{VW: slVW, VH: vh}
	plotW := slVW - slPadL - slPadR

	// --- 2. nodes: per row, order by lane then label, spread across the width. ---
	byRow := map[int][]int{} // row -> node indices
	for i, n := range data.Nodes {
		r := rowOf[n.Tier]
		byRow[r] = append(byRow[r], i)
	}
	nodeCenter := make(map[string][2]float64, len(data.Nodes)) // id -> (cx, cy)
	for r, idxs := range byRow {
		sort.SliceStable(idxs, func(a, b int) bool {
			na, nb := data.Nodes[idxs[a]], data.Nodes[idxs[b]]
			la, lb := slLaneOrder[na.Lane], slLaneOrder[nb.Lane]
			if la != lb {
				return la < lb
			}
			return na.Label < nb.Label
		})
		n := len(idxs)
		// Center the row's glyphs across the plot, evenly spaced.
		span := float64(n)*slNodeW + float64(n-1)*slLaneGap
		startX := slPadL + (plotW-span)/2
		if startX < slPadL {
			startX = slPadL
		}
		y := slPadT + float64(r)*slTierGap
		for k, ni := range idxs {
			node := data.Nodes[ni]
			x := startX + float64(k)*(slNodeW+slLaneGap)
			cx := x + slNodeW/2
			cy := y + slNodeH/2
			nodeCenter[node.ID] = [2]float64{cx, cy}
			out.Nodes = append(out.Nodes, SingleLineLayoutNode{
				ID: node.ID, Kind: node.Kind, Label: node.Label, Sub: node.Sub,
				X: x, Y: y, W: slNodeW, H: slNodeH, CX: cx, CY: cy,
				Color: node.Color, Glyph: glyphOrDefault(node.Glyph),
				Badge: node.Badge, BadgeRole: node.BadgeRole, Href: node.Href, Tip: node.Tip,
				LabelY: y + slNodeH + slLabelDY, SubY: y + slNodeH + slSubDY,
				BadgeX: x + slNodeW, BadgeY: y - 4,
			})
		}
	}

	// --- 3. buses: one run per lineup group at its tier row. ---------------------
	// Group buses by (tier, lineup); a continuation lineup spans its sections in
	// position order as one run. A standalone bus (no lineup) is its own run.
	type busGroupKey struct {
		tier   int
		lineup string
	}
	groups := map[busGroupKey][]int{}
	var groupOrder []busGroupKey
	for i, b := range data.Buses {
		key := busGroupKey{tier: b.Tier, lineup: b.Lineup}
		if b.Lineup == "" {
			key.lineup = "__bus_" + b.ID // standalone: unique key so it is its own run
		}
		if _, seen := groups[key]; !seen {
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], i)
	}
	// Place bus groups across the width per row, stacking multiple groups in a row.
	busRowCount := map[int]int{}
	for _, key := range groupOrder {
		busRowCount[rowOf[key.tier]]++
	}
	busRowPlaced := map[int]int{}
	busAttach := make(map[string][2]float64, len(data.Buses)) // bus id -> (cx, y) for tap routing
	for _, key := range groupOrder {
		members := groups[key]
		sort.SliceStable(members, func(a, b int) bool {
			return data.Buses[members[a]].Position < data.Buses[members[b]].Position
		})
		r := rowOf[key.tier]
		y := slPadT + float64(r)*slTierGap + slNodeH/2
		count := busRowCount[r]
		slot := busRowPlaced[r]
		busRowPlaced[r]++
		// Each group gets an equal horizontal slice of the plot; the run fills 86%
		// of its slice, centered, so multiple lineups in a row read as separate runs.
		sliceW := plotW / float64(count)
		runW := sliceW * 0.86
		x1 := slPadL + float64(slot)*sliceW + (sliceW-runW)/2
		x2 := x1 + runW
		color := data.Buses[members[0]].Color
		// Each section's attach point is its slice of the run, so a tap drops onto
		// the section it is on.
		for j, bi := range members {
			b := data.Buses[bi]
			segW := runW / float64(len(members))
			segCX := x1 + segW*float64(j) + segW/2
			busAttach[b.ID] = [2]float64{segCX, y}
		}
		label := data.Buses[members[0]].Label
		if key.lineup != "" && len(members) > 1 {
			label = key.lineup
		}
		out.Buses = append(out.Buses, SingleLineLayoutBus{
			ID: data.Buses[members[0]].ID, Label: label,
			X1: x1, X2: x2, Y: y, Color: color, State: data.Buses[members[0]].State,
			Tip: data.Buses[members[0]].Tip, LabelX: x1, LabelY: y - 8,
		})
	}

	// --- 4. conductors: orthogonal drops between endpoints. ----------------------
	endpoint := func(id string) ([2]float64, bool) {
		if p, ok := nodeCenter[id]; ok {
			return p, true
		}
		if p, ok := busAttach[id]; ok {
			return p, true
		}
		return [2]float64{}, false
	}
	for _, e := range data.Edges {
		from, ok1 := endpoint(e.FromID)
		to, ok2 := endpoint(e.ToID)
		if !ok1 || !ok2 {
			continue
		}
		out.Conductors = append(out.Conductors, SingleLineLayoutEdge{
			PathD:  orthPath(from[0], from[1], to[0], to[1]),
			Role:   e.Role,
			Color:  e.Color,
			Tip:    e.Tip,
			FromID: e.FromID,
			ToID:   e.ToID,
			Dashed: e.Role == "tie" || e.Role == "continuation",
		})
	}

	out.Empty = len(out.Nodes) == 0 && len(out.Buses) == 0
	return out
}

// orthPath routes a conductor from (x1,y1) to (x2,y2) as an orthogonal run: a
// vertical drop to the midpoint y, a horizontal traverse, then a vertical drop to
// the target. A same-column run draws a straight vertical line. The mid-y elbow
// keeps the schematic readable when tiers are stacked.
func orthPath(x1, y1, x2, y2 float64) string {
	if x1 == x2 {
		return fmt.Sprintf("M %.1f %.1f L %.1f %.1f", x1, y1, x2, y2)
	}
	if y1 == y2 {
		return fmt.Sprintf("M %.1f %.1f L %.1f %.1f", x1, y1, x2, y2)
	}
	midY := (y1 + y2) / 2
	return fmt.Sprintf("M %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f",
		x1, y1, x1, midY, x2, midY, x2, y2)
}

// glyphOrDefault maps an empty glyph to "rect".
func glyphOrDefault(g string) string {
	if g == "" {
		return "rect"
	}
	return g
}

// SingleLineViewBox returns the "0 0 W H" viewBox string for a laid-out diagram.
func SingleLineViewBox(s SingleLine) string {
	return fmt.Sprintf("0 0 %.0f %.0f", s.VW, s.VH)
}

// BadgeColorVar maps a badge role to its status-token CSS variable. An unknown or
// empty role falls back to the muted foreground. Every value resolves to a real
// @theme token (no ghost tokens): good/warn/bad/info and fg-muted.
func BadgeColorVar(role string) string {
	switch role {
	case "good":
		return "var(--color-good)"
	case "warn":
		return "var(--color-warn)"
	case "bad":
		return "var(--color-bad)"
	case "info":
		return "var(--color-info)"
	default:
		return "var(--color-fg-muted)"
	}
}

// TransformerGlyphCY1 / CY2 give the two winding-circle centers for a transformer
// glyph centered on a node, so the renderer draws the two overlapping circles
// without arithmetic in the template.
func TransformerCircleR() float64 { return slNodeH * 0.32 }
