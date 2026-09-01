package chart

import (
	"fmt"
	"math"
	"sort"
)

// --- Input types ------------------------------------------------------------

type BundleData struct {
	Nodes []BundleNode `json:"nodes"`
	Links []BundleLink `json:"links"`
}

type BundleNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"`
}

type BundleLink struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Value  float64 `json:"value,omitempty"`
}

// --- Output geometry --------------------------------------------------------

// Bundle uses a 200x200 viewBox centered at (100,100).
type Bundle struct {
	Nodes []BundleLayoutNode
	Links []BundleLayoutLink
	Arcs  []BundleArc
}

type BundleLayoutNode struct {
	X          string
	Y          string
	Label      string
	LabelX     string
	LabelY     string
	Rotate     string
	Color      string
	Tip        string
	TextAnchor string
	NodeID     string
	Group      string
}

type BundleLayoutLink struct {
	PathD   string
	Color   string
	Opacity string
	SrcID   string
	DstID   string
}

type BundleArc struct {
	PathD string
	Color string
	Label string
}

// --- Layout -----------------------------------------------------------------

func LayoutBundle(data BundleData) Bundle {
	n := len(data.Nodes)
	if n == 0 {
		return Bundle{}
	}

	const (
		cx     = 100.0
		cy     = 100.0
		outerR = 42.0
		innerR = 38.0
		labelR = 47.0
		arcR   = 44.0
	)

	// Group nodes and sort within groups.
	groups := make(map[string][]int)
	var groupOrder []string
	for i, node := range data.Nodes {
		if _, ok := groups[node.Group]; !ok {
			groupOrder = append(groupOrder, node.Group)
		}
		groups[node.Group] = append(groups[node.Group], i)
	}
	sort.Strings(groupOrder)

	// Flatten into radial order.
	ordered := make([]int, 0, n)
	for _, g := range groupOrder {
		ordered = append(ordered, groups[g]...)
	}

	// Assign angles.
	angles := make([]float64, n)
	nodeAngle := make(map[string]float64, n)
	for idx, ni := range ordered {
		a := (float64(idx) / float64(n)) * 2 * math.Pi
		angles[ni] = a
		nodeAngle[data.Nodes[ni].ID] = a
	}

	// Group color map.
	groupColor := make(map[string]string, len(groupOrder))
	for gi, g := range groupOrder {
		groupColor[g] = ColorVar(gi)
	}

	polarXY := func(r, a float64) (float64, float64) {
		return cx + r*math.Cos(a-math.Pi/2), cy + r*math.Sin(a-math.Pi/2)
	}

	out := Bundle{}

	// Group arcs.
	for _, g := range groupOrder {
		members := groups[g]
		if len(members) == 0 {
			continue
		}
		minA := angles[members[0]]
		maxA := angles[members[0]]
		for _, mi := range members {
			a := angles[mi]
			if a < minA {
				minA = a
			}
			if a > maxA {
				maxA = a
			}
		}
		step := (2 * math.Pi) / float64(n)
		a0 := minA - step*0.4
		a1 := maxA + step*0.4

		x0, y0 := polarXY(arcR, a0)
		x1, y1 := polarXY(arcR, a1)
		large := 0
		if a1-a0 > math.Pi {
			large = 1
		}
		d := fmt.Sprintf("M %.2f %.2f A %.2f %.2f 0 %d 1 %.2f %.2f", x0, y0, arcR, arcR, large, x1, y1)

		out.Arcs = append(out.Arcs, BundleArc{
			PathD: d,
			Color: groupColor[g],
			Label: g,
		})
	}

	// Nodes.
	for i, node := range data.Nodes {
		a := angles[i]
		x, y := polarXY(innerR, a)
		lx, ly := polarXY(labelR, a)

		deg := (a - math.Pi/2) * 180 / math.Pi
		textAnchor := "start"
		rotate := fmt.Sprintf("rotate(%.1f %.2f %.2f)", deg, lx, ly)
		if a > math.Pi {
			textAnchor = "end"
			rotate = fmt.Sprintf("rotate(%.1f %.2f %.2f)", deg+180, lx, ly)
		}

		color := groupColor[node.Group]
		tip := BuildTipHTML(color, node.Label, []TipRow{{Label: "group", Value: node.Group}})

		out.Nodes = append(out.Nodes, BundleLayoutNode{
			X:          fmt.Sprintf("%.2f", x),
			Y:          fmt.Sprintf("%.2f", y),
			Label:      node.Label,
			LabelX:     fmt.Sprintf("%.2f", lx),
			LabelY:     fmt.Sprintf("%.2f", ly),
			Rotate:     rotate,
			Color:      color,
			Tip:        tip,
			TextAnchor: textAnchor,
			NodeID:     node.ID,
			Group:      node.Group,
		})
	}

	// Links -- quadratic bezier through the center point.
	const tension = 0.85
	for _, link := range data.Links {
		srcA, ok1 := nodeAngle[link.Source]
		tgtA, ok2 := nodeAngle[link.Target]
		if !ok1 || !ok2 {
			continue
		}

		sx, sy := polarXY(innerR, srcA)
		tx, ty := polarXY(innerR, tgtA)

		// Control point: lerp between midpoint and center.
		mx := (sx + tx) / 2
		my := (sy + ty) / 2
		cpx := mx + (cx-mx)*tension
		cpy := my + (cy-my)*tension

		d := fmt.Sprintf("M %.2f %.2f Q %.2f %.2f %.2f %.2f", sx, sy, cpx, cpy, tx, ty)

		// Use source node's group color.
		srcGroup := ""
		for _, node := range data.Nodes {
			if node.ID == link.Source {
				srcGroup = node.Group
				break
			}
		}
		color := groupColor[srcGroup]

		out.Links = append(out.Links, BundleLayoutLink{
			PathD:   d,
			Color:   color,
			Opacity: "0.25",
			SrcID:   link.Source,
			DstID:   link.Target,
		})
	}

	return out
}
