package chart

import (
	"fmt"
	"math"
	"sort"
)

// --- Input types ------------------------------------------------------------

type SankeyData struct {
	Nodes []SankeyNode `json:"nodes"`
	Links []SankeyLink `json:"links"`
}

type SankeyNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type SankeyLink struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Value  float64 `json:"value"`
}

// --- Output geometry --------------------------------------------------------

// Sankey uses a 1000x210 viewBox. The viewBox is wide and short on purpose:
// the dashboard row track grows to the SVG's aspect ratio, so a short viewBox
// gives a short panel that hugs the diagram instead of a tall one with dead
// space. Links are filled ribbons (closed polygons), not stroked centerlines,
// so their ends are flat and flush with the node bars.
type Sankey struct {
	Nodes []SankeyLayoutNode
	Links []SankeyLayoutLink
}

type SankeyLayoutNode struct {
	ID          string
	Label       string
	X, Y, W, H  float64
	Color       string
	Tip         string
	LabelX      float64
	LabelY      float64
	LabelAnchor string
}

type SankeyLayoutLink struct {
	PathD string
	Color string
	Tip   string
	// SrcID/DstID are the node IDs this link connects, so the client can
	// trace the connected chain when a node is hovered.
	SrcID string
	DstID string
}

// --- Internal ---------------------------------------------------------------

type sankeyInternal struct {
	id       string
	label    string
	tier     int
	value    float64
	y        float64
	h        float64
	inLinks  []int
	outLinks []int
}

// --- Tuning -----------------------------------------------------------------

const (
	sankeyVW          = 1000.0
	sankeyVH          = 210.0
	sankeyPadL        = 12.0
	sankeyPadR        = 12.0
	sankeyPadT        = 14.0
	sankeyPadB        = 14.0
	sankeyNodeW       = 6.0
	sankeyNodePad     = 14.0
	sankeyMinH        = 2.0 // floor for a node bar / link ribbon thickness
	sankeyIters       = 12  // barycenter relaxation passes
	sankeyStraightEps = 4.0 // |dy| below this draws a link as a level ribbon
)

// sankeyAngles are the allowed connector angles, in degrees. A link that is
// not level snaps UP to the smallest of these that is >= its natural angle.
// Snapping up (steeper, never shallower) guarantees the diagonal fits inside
// the horizontal gap, so the ribbon ends stay flush with the bars. The 90
// case renders as a right-angle (orthogonal) step.
var sankeyAngles = []float64{27.5, 45, 90}

// --- Layout -----------------------------------------------------------------

// LayoutSankey lays out a Sankey diagram. The pipeline follows the Sugiyama
// framework for layered graph drawing (the same approach d3-sankey uses):
//
//  1. assign every node to a tier (longest path from a source);
//  2. order nodes within each tier and align them vertically by iterating a
//     barycenter relaxation -- each node is pulled toward the weighted-average
//     position of its neighbours, then collisions are resolved. This is what
//     straightens the flows: a node ends up across from the nodes it connects
//     to (so ATS-1, UPS-1 and Hall A line up) instead of being sorted by raw
//     value, which is what stranded UPS-1 at the bottom;
//  3. stack each node's links by the position of the node at the far end, to
//     minimise crossings where ribbons meet a bar;
//  4. emit every link as a filled, angular ribbon with flat, flush ends.
func LayoutSankey(data SankeyData) Sankey {
	if len(data.Nodes) == 0 || len(data.Links) == 0 {
		return Sankey{}
	}

	nodeIdx := make(map[string]int, len(data.Nodes))
	nodes := make([]sankeyInternal, len(data.Nodes))
	for i, n := range data.Nodes {
		nodeIdx[n.ID] = i
		nodes[i] = sankeyInternal{id: n.ID, label: n.Label}
	}

	for i, l := range data.Links {
		si, ok1 := nodeIdx[l.Source]
		ti, ok2 := nodeIdx[l.Target]
		if !ok1 || !ok2 {
			continue
		}
		nodes[si].outLinks = append(nodes[si].outLinks, i)
		nodes[ti].inLinks = append(nodes[ti].inLinks, i)
	}
	// A node's flow is the larger of what enters and what leaves it.
	for i := range nodes {
		var in, out float64
		for _, li := range nodes[i].inLinks {
			in += data.Links[li].Value
		}
		for _, li := range nodes[i].outLinks {
			out += data.Links[li].Value
		}
		nodes[i].value = math.Max(in, out)
	}

	// --- 1. Tier assignment: longest path from a source. --------------------
	for changed, guard := true, 0; changed && guard <= len(nodes); guard++ {
		changed = false
		for _, l := range data.Links {
			si, ti := nodeIdx[l.Source], nodeIdx[l.Target]
			if nodes[ti].tier <= nodes[si].tier {
				nodes[ti].tier = nodes[si].tier + 1
				changed = true
			}
		}
	}
	maxTier := 0
	for _, n := range nodes {
		if n.tier > maxTier {
			maxTier = n.tier
		}
	}
	tiers := make([][]int, maxTier+1)
	for i, n := range nodes {
		tiers[n.tier] = append(tiers[n.tier], i)
	}
	// Initial order: largest flow first. The relaxation below reorders as
	// needed; this just gives it a stable, sensible starting point.
	for _, tier := range tiers {
		sort.SliceStable(tier, func(a, b int) bool {
			return nodes[tier[a]].value > nodes[tier[b]].value
		})
	}

	// --- vertical scale: one thickness-per-unit shared by all tiers. --------
	const plotH = sankeyVH - sankeyPadT - sankeyPadB
	const plotW = sankeyVW - sankeyPadL - sankeyPadR
	ky := math.Inf(1)
	for _, tier := range tiers {
		var sum float64
		for _, ni := range tier {
			sum += nodes[ni].value
		}
		if sum <= 0 {
			continue
		}
		avail := plotH - sankeyNodePad*float64(len(tier)-1)
		if avail < sankeyMinH {
			avail = sankeyMinH
		}
		if k := avail / sum; k < ky {
			ky = k
		}
	}
	if math.IsInf(ky, 1) || ky <= 0 {
		ky = 1
	}

	// Initial y: stack each tier, vertically centred in the plot.
	for _, tier := range tiers {
		var sum float64
		for _, ni := range tier {
			sum += nodes[ni].value
		}
		colH := sum*ky + sankeyNodePad*float64(len(tier)-1)
		y := sankeyPadT + (plotH-colH)/2
		for _, ni := range tier {
			h := nodes[ni].value * ky
			if h < sankeyMinH {
				h = sankeyMinH
			}
			nodes[ni].y = y
			nodes[ni].h = h
			y += h + sankeyNodePad
		}
	}

	center := func(i int) float64 { return nodes[i].y + nodes[i].h/2 }

	// resolveColumn re-sorts a tier by current center and packs nodes so they
	// keep nodePad spacing and stay inside the plot. The sort is what turns a
	// nudge from align() into an actual reordering.
	resolveColumn := func(tier []int) {
		sort.SliceStable(tier, func(a, b int) bool { return center(tier[a]) < center(tier[b]) })
		y := sankeyPadT
		for _, ni := range tier {
			if nodes[ni].y < y {
				nodes[ni].y = y
			}
			y = nodes[ni].y + nodes[ni].h + sankeyNodePad
		}
		// If the stack overran the bottom, push it back up.
		y = sankeyPadT + plotH
		for k := len(tier) - 1; k >= 0; k-- {
			ni := tier[k]
			if nodes[ni].y+nodes[ni].h > y {
				nodes[ni].y = y - nodes[ni].h
			}
			if nodes[ni].y < sankeyPadT {
				nodes[ni].y = sankeyPadT
			}
			y = nodes[ni].y - sankeyNodePad
		}
	}

	// align pulls each node toward the weighted-average center of its
	// neighbours on one side -- sources when forward, targets when backward.
	align := func(tier []int, forward bool, alpha float64) {
		for _, ni := range tier {
			links := nodes[ni].inLinks
			if !forward {
				links = nodes[ni].outLinks
			}
			var num, den float64
			for _, li := range links {
				other := nodeIdx[data.Links[li].Target]
				if forward {
					other = nodeIdx[data.Links[li].Source]
				}
				num += center(other) * data.Links[li].Value
				den += data.Links[li].Value
			}
			if den > 0 {
				nodes[ni].y += (num/den - center(ni)) * alpha
			}
		}
	}

	// --- 2. Barycenter relaxation: order within tiers + vertical alignment. -
	alpha := 1.0
	for it := 0; it < sankeyIters; it++ {
		alpha *= 0.99
		for t := maxTier - 1; t >= 0; t-- { // right-to-left: align to targets
			align(tiers[t], false, alpha)
			resolveColumn(tiers[t])
		}
		for t := 1; t <= maxTier; t++ { // left-to-right: align to sources
			align(tiers[t], true, alpha)
			resolveColumn(tiers[t])
		}
	}

	// --- 3. Order each node's links by the far node's position. -------------
	for i := range nodes {
		out := nodes[i].outLinks
		sort.SliceStable(out, func(a, b int) bool {
			return center(nodeIdx[data.Links[out[a]].Target]) < center(nodeIdx[data.Links[out[b]].Target])
		})
		in := nodes[i].inLinks
		sort.SliceStable(in, func(a, b int) bool {
			return center(nodeIdx[data.Links[in[a]].Source]) < center(nodeIdx[data.Links[in[b]].Source])
		})
	}

	// --- 4. Emit geometry. ---------------------------------------------------
	tierSpacing := 0.0
	if maxTier > 0 {
		tierSpacing = (plotW - sankeyNodeW) / float64(maxTier)
	}
	nodeX := func(i int) float64 { return sankeyPadL + float64(nodes[i].tier)*tierSpacing }

	out := Sankey{}
	for i, n := range nodes {
		x := nodeX(i)
		color := ColorVar(i)
		labelX := x + sankeyNodeW + 6
		anchor := "start"
		if n.tier == maxTier {
			labelX = x - 6
			anchor = "end"
		}
		tip := BuildTipHTML(color, n.label, []TipRow{
			{Label: "flow", Value: fmt.Sprintf("%.0f", n.value)},
		})
		out.Nodes = append(out.Nodes, SankeyLayoutNode{
			ID:          n.id,
			Label:       n.label,
			X:           x,
			Y:           n.y,
			W:           sankeyNodeW,
			H:           n.h,
			Color:       color,
			Tip:         tip,
			LabelX:      labelX,
			LabelY:      n.y + n.h/2,
			LabelAnchor: anchor,
		})
	}

	// Per-link thickness and stacked attach positions at each bar.
	linkH := make([]float64, len(data.Links))
	for li, l := range data.Links {
		h := l.Value * ky
		if h < sankeyMinH {
			h = sankeyMinH
		}
		linkH[li] = h
	}
	srcTop := make([]float64, len(data.Links))
	tgtTop := make([]float64, len(data.Links))
	for i := range nodes {
		off := nodes[i].y
		for _, li := range nodes[i].outLinks {
			srcTop[li] = off
			off += linkH[li]
		}
		off = nodes[i].y
		for _, li := range nodes[i].inLinks {
			tgtTop[li] = off
			off += linkH[li]
		}
	}

	for li, l := range data.Links {
		si, ti := nodeIdx[l.Source], nodeIdx[l.Target]
		sx := nodeX(si) + sankeyNodeW
		tx := nodeX(ti)
		out.Links = append(out.Links, SankeyLayoutLink{
			PathD: sankeyRibbon(sx, srcTop[li], tx, tgtTop[li], linkH[li]),
			Color: ColorVar(si),
			Tip: BuildTipHTML("", l.Source+" → "+l.Target,
				[]TipRow{{Label: "flow", Value: fmt.Sprintf("%.0f", l.Value)}}),
			SrcID: l.Source,
			DstID: l.Target,
		})
	}

	return out
}

// --- Ribbon geometry --------------------------------------------------------

// sankeyRibbon returns the SVG path for one filled link ribbon. The ribbon
// leaves the source bar horizontally, turns once through a snapped angle, and
// enters the target bar horizontally, so both ends are flat and flush with the
// bars. Thickness (h) is measured vertically and preserved end to end, which
// is what keeps stacked ribbons from overlapping where they meet a bar.
//
//	sx,sTop ── source attach: top of this link's slice of the source bar
//	tx,tTop ── target attach: top of this link's slice of the target bar
func sankeyRibbon(sx, sTop, tx, tTop, h float64) string {
	dx := tx - sx
	dy := tTop - sTop
	if dx <= 0 || math.Abs(dy) < sankeyStraightEps {
		return sankeyQuad(sx, sTop, tx, tTop, h) // level (or degenerate): straight
	}

	natural := math.Atan2(math.Abs(dy), dx) * 180 / math.Pi
	angle := sankeyAngles[len(sankeyAngles)-1]
	for _, a := range sankeyAngles {
		if a >= natural {
			angle = a
			break
		}
	}
	if angle >= 90 {
		return sankeyElbow(sx, sTop, tx, tTop, h)
	}

	// Put all the vertical travel in a middle diagonal at the snapped angle;
	// equal horizontal stubs at each end absorb the slack and keep the ends
	// flush and horizontal.
	dxMid := math.Abs(dy) / math.Tan(angle*math.Pi/180)
	if dxMid > dx {
		dxMid = dx
	}
	stub := (dx - dxMid) / 2
	x2, x3 := sx+stub, tx-stub
	// Walk the outline: top edge left-to-right, down the target end, bottom
	// edge right-to-left, up the source end (Z).
	return fmt.Sprintf(
		"M %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f Z",
		sx, sTop, x2, sTop, x3, tTop, tx, tTop,
		tx, tTop+h, x3, tTop+h, x2, sTop+h, sx, sTop+h,
	)
}

// sankeyQuad is a straight (level) ribbon: a quadrilateral joining the two bar
// faces directly. Used when the ends sit at (nearly) the same height.
func sankeyQuad(sx, sTop, tx, tTop, h float64) string {
	return fmt.Sprintf("M %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f Z",
		sx, sTop, tx, tTop, tx, tTop+h, sx, sTop+h)
}

// sankeyElbow renders a 90-degree right-angle step: horizontal out of the
// source, a vertical run (width = h) at the midpoint, horizontal into the
// target. Used for drops too steep for a 45-degree diagonal.
func sankeyElbow(sx, sTop, tx, tTop, h float64) string {
	xMid := (sx + tx) / 2
	half := h / 2
	if lim := (tx - sx) / 2; half > lim {
		half = lim // keep the vertical run inside the column gap
	}
	if tTop >= sTop { // step down
		return fmt.Sprintf(
			"M %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f Z",
			sx, sTop, xMid+half, sTop, xMid+half, tTop, tx, tTop,
			tx, tTop+h, xMid-half, tTop+h, xMid-half, sTop+h, sx, sTop+h,
		)
	}
	// step up
	return fmt.Sprintf(
		"M %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f Z",
		sx, sTop, xMid-half, sTop, xMid-half, tTop, tx, tTop,
		tx, tTop+h, xMid+half, tTop+h, xMid+half, sTop+h, sx, sTop+h,
	)
}
