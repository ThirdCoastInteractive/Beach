package chart

import (
	"sort"
	"testing"
)

// specimenSankey mirrors the power-flow data on the component specimen page:
// utility feeds -> ATS -> UPS -> Hall PDUs.
func specimenSankey() SankeyData {
	return SankeyData{
		Nodes: []SankeyNode{
			{ID: "utility-a", Label: "Utility A"},
			{ID: "utility-b", Label: "Utility B"},
			{ID: "ats-1", Label: "ATS-1"},
			{ID: "ats-2", Label: "ATS-2"},
			{ID: "ups-1", Label: "UPS-1"},
			{ID: "ups-2", Label: "UPS-2"},
			{ID: "ups-3", Label: "UPS-3"},
			{ID: "pdu-hall-a", Label: "Hall A PDUs"},
			{ID: "pdu-hall-b", Label: "Hall B PDUs"},
			{ID: "pdu-hall-c", Label: "Hall C PDUs"},
		},
		Links: []SankeyLink{
			{Source: "utility-a", Target: "ats-1", Value: 500},
			{Source: "utility-a", Target: "ats-2", Value: 300},
			{Source: "utility-b", Target: "ats-1", Value: 200},
			{Source: "utility-b", Target: "ats-2", Value: 400},
			{Source: "ats-1", Target: "ups-1", Value: 400},
			{Source: "ats-1", Target: "ups-2", Value: 300},
			{Source: "ats-2", Target: "ups-2", Value: 200},
			{Source: "ats-2", Target: "ups-3", Value: 500},
			{Source: "ups-1", Target: "pdu-hall-a", Value: 400},
			{Source: "ups-2", Target: "pdu-hall-a", Value: 200},
			{Source: "ups-2", Target: "pdu-hall-b", Value: 300},
			{Source: "ups-3", Target: "pdu-hall-b", Value: 200},
			{Source: "ups-3", Target: "pdu-hall-c", Value: 300},
		},
	}
}

// columnsTopToBottom groups laid-out nodes by their column (X) and returns the
// labels in each column read top to bottom, left column first.
func columnsTopToBottom(s Sankey) [][]string {
	byX := map[int][]SankeyLayoutNode{}
	var xs []int
	for _, n := range s.Nodes {
		xi := int(n.X + 0.5)
		if _, ok := byX[xi]; !ok {
			xs = append(xs, xi)
		}
		byX[xi] = append(byX[xi], n)
	}
	sort.Ints(xs)
	out := make([][]string, 0, len(xs))
	for _, xi := range xs {
		col := byX[xi]
		sort.Slice(col, func(a, b int) bool { return col[a].Y < col[b].Y })
		labels := make([]string, len(col))
		for i, n := range col {
			labels[i] = n.Label
		}
		out = append(out, labels)
	}
	return out
}

// Connected nodes should land across from one another. In particular the
// relaxation must lift UPS-1 to the top of its column -- it feeds only Hall A,
// which is fed from the top of the previous column -- rather than leaving it at
// the bottom where a raw value-descending sort puts it (400 < 500, 500).
func TestSankeyColumnOrder(t *testing.T) {
	got := columnsTopToBottom(LayoutSankey(specimenSankey()))
	want := [][]string{
		{"Utility A", "Utility B"},
		{"ATS-1", "ATS-2"},
		{"UPS-1", "UPS-2", "UPS-3"},
		{"Hall A PDUs", "Hall B PDUs", "Hall C PDUs"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d columns, want %d (%v)", len(got), len(want), got)
	}
	for ci := range want {
		if len(got[ci]) != len(want[ci]) {
			t.Fatalf("column %d: got %v, want %v", ci, got[ci], want[ci])
		}
		for ri := range want[ci] {
			if got[ci][ri] != want[ci][ri] {
				t.Errorf("column %d order = %v, want %v", ci, got[ci], want[ci])
				break
			}
		}
	}
}

// Links are filled ribbons, not stroked centerlines: every path is a closed
// polygon (ends with Z), so the ends are flat and flush with the bars.
func TestSankeyLinksAreClosedPolygons(t *testing.T) {
	s := LayoutSankey(specimenSankey())
	if len(s.Links) != 13 {
		t.Fatalf("got %d links, want 13", len(s.Links))
	}
	for i, l := range s.Links {
		if l.PathD == "" || l.PathD[len(l.PathD)-1] != 'Z' {
			t.Errorf("link %d is not a closed polygon: %q", i, l.PathD)
		}
		if l.Color == "" {
			t.Errorf("link %d has no color", i)
		}
	}
}

// Every node bar must stay inside the plot's vertical bounds after relaxation
// and collision resolution.
func TestSankeyNodesWithinBounds(t *testing.T) {
	s := LayoutSankey(specimenSankey())
	const top, bottom = sankeyPadT, sankeyVH - sankeyPadB
	for _, n := range s.Nodes {
		if n.Y < top-0.5 || n.Y+n.H > bottom+0.5 {
			t.Errorf("node %q out of bounds: y=%.1f h=%.1f (plot %.1f..%.1f)", n.Label, n.Y, n.H, top, bottom)
		}
	}
}

func TestSankeyEmptyInput(t *testing.T) {
	if got := LayoutSankey(SankeyData{}); len(got.Nodes) != 0 || len(got.Links) != 0 {
		t.Errorf("empty input should produce empty layout, got %+v", got)
	}
}

// The client traces the connected chain on hover from node ids and link
// endpoints, so every node must carry its id and every link its src/dst.
func TestSankeyChainIdentifiers(t *testing.T) {
	s := LayoutSankey(specimenSankey())
	ids := map[string]bool{}
	for _, n := range s.Nodes {
		if n.ID == "" {
			t.Errorf("node %q has no ID", n.Label)
		}
		ids[n.ID] = true
	}
	for i, l := range s.Links {
		if !ids[l.SrcID] || !ids[l.DstID] {
			t.Errorf("link %d endpoints %q->%q not both real node ids", i, l.SrcID, l.DstID)
		}
	}
}
