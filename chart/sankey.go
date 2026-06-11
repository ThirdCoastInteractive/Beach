package chart

import (
	"io"
)

// SankeyLink is a weighted flow from one node index to another.
type SankeyLink struct {
	Source, Target int
	Value          float64
}

// SankeyInput configures a Sankey flow diagram. Nodes must form a DAG; this
// implementation assigns each node to a column by longest-path depth.
type SankeyInput struct {
	Title  string
	Nodes  []string
	Links  []SankeyLink
	Width  float64 // defaults to 640
	Height float64 // defaults to 360
}

// SankeyLayout is the geometry for a Sankey diagram.
type SankeyLayout struct {
	W, H    float64
	Title   string
	Nodes   []sankeyNode
	Ribbons []sankeyRibbon
}

type sankeyNode struct {
	Box   Box
	Color string
	Label string
}

type sankeyRibbon struct {
	D     string
	Color string
	Tip   string
}

// LayoutSankey computes geometry for a Sankey diagram.
func LayoutSankey(in SankeyInput) SankeyLayout {
	w := in.Width
	if w == 0 {
		w = 640
	}
	h := in.Height
	if h == 0 {
		h = 360
	}
	lay := SankeyLayout{W: w, H: h, Title: in.Title}
	n := len(in.Nodes)
	if n == 0 {
		return lay
	}
	// Column = longest path depth from any source.
	depth := make([]int, n)
	for iter := 0; iter < n; iter++ {
		changed := false
		for _, l := range in.Links {
			if l.Source < 0 || l.Target < 0 || l.Source >= n || l.Target >= n {
				continue
			}
			if depth[l.Source]+1 > depth[l.Target] {
				depth[l.Target] = depth[l.Source] + 1
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	maxDepth := 0
	for _, d := range depth {
		if d > maxDepth {
			maxDepth = d
		}
	}
	// Node throughput = max(in, out).
	inSum := make([]float64, n)
	outSum := make([]float64, n)
	for _, l := range in.Links {
		if l.Source >= 0 && l.Source < n {
			outSum[l.Source] += l.Value
		}
		if l.Target >= 0 && l.Target < n {
			inSum[l.Target] += l.Value
		}
	}
	through := make([]float64, n)
	for i := range through {
		through[i] = inSum[i]
		if outSum[i] > through[i] {
			through[i] = outSum[i]
		}
		if through[i] == 0 {
			through[i] = 1
		}
	}
	// Group nodes by column, total throughput per column for scaling.
	cols := make([][]int, maxDepth+1)
	for i := 0; i < n; i++ {
		cols[depth[i]] = append(cols[depth[i]], i)
	}
	pad := 16.0
	nodeW := 14.0
	gap := 8.0
	colX := func(d int) float64 {
		if maxDepth == 0 {
			return pad
		}
		return pad + float64(d)/float64(maxDepth)*(w-2*pad-nodeW)
	}
	// Place nodes vertically per column scaled to fit height.
	nodeBox := make([]Box, n)
	for d := 0; d <= maxDepth; d++ {
		colNodes := cols[d]
		total := 0.0
		for _, ni := range colNodes {
			total += through[ni]
		}
		if total == 0 {
			total = 1
		}
		avail := h - 2*pad - gap*float64(len(colNodes)-1)
		if avail < 1 {
			avail = 1
		}
		y := pad
		for _, ni := range colNodes {
			bh := through[ni] / total * avail
			nodeBox[ni] = Box{X: colX(d), Y: y, W: nodeW, H: bh}
			y += bh + gap
		}
	}
	for i := 0; i < n; i++ {
		lay.Nodes = append(lay.Nodes, sankeyNode{
			Box: nodeBox[i], Color: QualitativeVar(i), Label: in.Nodes[i],
		})
	}
	// Track running offsets on each node's edges for ribbon stacking.
	outOff := make(map[int]float64)
	inOff := make(map[int]float64)
	for _, l := range in.Links {
		if l.Source < 0 || l.Target < 0 || l.Source >= n || l.Target >= n {
			continue
		}
		sb := nodeBox[l.Source]
		tb := nodeBox[l.Target]
		sh := l.Value / through[l.Source] * sb.H
		th := l.Value / through[l.Target] * tb.H
		sy0 := sb.Y + outOff[l.Source]
		ty0 := tb.Y + inOff[l.Target]
		outOff[l.Source] += sh
		inOff[l.Target] += th
		x0 := sb.Right()
		x1 := tb.X
		mx := (x0 + x1) / 2
		// Two cubic curves forming a filled ribbon.
		d := "M" + fnum(x0) + "," + fnum(sy0) +
			" C" + fnum(mx) + "," + fnum(sy0) + " " + fnum(mx) + "," + fnum(ty0) + " " + fnum(x1) + "," + fnum(ty0) +
			" L" + fnum(x1) + "," + fnum(ty0+th) +
			" C" + fnum(mx) + "," + fnum(ty0+th) + " " + fnum(mx) + "," + fnum(sy0+sh) + " " + fnum(x0) + "," + fnum(sy0+sh) +
			" Z"
		lay.Ribbons = append(lay.Ribbons, sankeyRibbon{
			D: d, Color: QualitativeVar(l.Source),
			Tip: in.Nodes[l.Source] + " → " + in.Nodes[l.Target] + ": " + fnum(l.Value),
		})
	}
	return lay
}

// SankeySVG renders the Sankey diagram described by lay.
func SankeySVG(w io.Writer, lay SankeyLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, r := range lay.Ribbons {
		s.p(`<g>`)
		s.title(r.Tip)
		s.path(r.D, r.Color, "", `fill-opacity="0.35"`)
		s.p(`</g>`)
	}
	for _, nd := range lay.Nodes {
		s.p(`<g>`)
		s.title(nd.Label)
		s.rect(nd.Box, nd.Color, `rx="1"`)
		s.p(`</g>`)
		s.text(nd.Box.X+nd.Box.W+4, nd.Box.Y+nd.Box.H/2+3, "start", "chart-label", nd.Label)
	}
	s.close()
	return s.Err()
}
