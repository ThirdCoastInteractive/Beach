package chart

import (
	"encoding/json"
	"strings"
	"testing"
)

// shapeByCode finds a projected country by ISO code (ok=false when it
// was clipped away entirely).
func shapeByCode(g Globe, code string) (GlobeShape, bool) {
	for _, s := range g.Shapes {
		if s.Code == code {
			return s, true
		}
	}
	return GlobeShape{}, false
}

// TestGlobeFarSideClipped: with the camera at (0°, 0°), geography more
// than 90° of arc away is on the far side. Germany (~10°E) is visible;
// New Zealand (~172°E) is not.
func TestGlobeFarSideClipped(t *testing.T) {
	g := LayoutGlobe(GlobeData{})
	if _, ok := shapeByCode(g, "DE"); !ok {
		t.Fatalf("Germany should be visible from camera (0,0)")
	}
	if s, ok := shapeByCode(g, "NZ"); ok {
		t.Fatalf("New Zealand should be clipped from camera (0,0), got path %q", s.Path)
	}

	// The projection primitive agrees: lon 170 is invisible, lon 10 is not.
	cam := newGlobeCam(0, 0, 490, 500, 500)
	if _, _, cosc := cam.project(170, 0); cosc > 0 {
		t.Fatalf("lon 170 from subpoint (0,0): cosc = %v, want <= 0", cosc)
	}
	if _, _, cosc := cam.project(10, 0); cosc <= 0 {
		t.Fatalf("lon 10 from subpoint (0,0): cosc = %v, want > 0", cosc)
	}
	if p := globePath(cam, [][2]float64{{170, -5}, {175, 0}, {170, 5}}, true); p != "" {
		t.Fatalf("far-side ring should produce an empty path, got %q", p)
	}
}

// TestGlobeFraming: disc radius is 490·Zoom and the disc center sits at
// (CenterX·1000, CenterY·1000); zero CenterX/CenterY default to 0.5.
func TestGlobeFraming(t *testing.T) {
	g := LayoutGlobe(GlobeData{Zoom: 2, CenterX: 1, CenterY: 0.25})
	if g.R != "980.0" || g.CX != "1000.0" || g.CY != "250.0" {
		t.Fatalf("zoom 2 @ (1, 0.25): got R=%s CX=%s CY=%s, want 980.0/1000.0/250.0", g.R, g.CX, g.CY)
	}
	if g.ViewBox != "0 0 1000 1000" {
		t.Fatalf("viewBox = %q", g.ViewBox)
	}

	d := LayoutGlobe(GlobeData{})
	if d.R != "490.0" || d.CX != "500.0" || d.CY != "500.0" {
		t.Fatalf("defaults: got R=%s CX=%s CY=%s, want 490.0/500.0/500.0", d.R, d.CX, d.CY)
	}
}

// TestGlobeThemed: valued regions get oklch ramp fills and tooltips;
// unvalued countries keep the neutral fill with no tooltip markup loss.
func TestGlobeThemed(t *testing.T) {
	g := LayoutGlobe(GlobeData{
		Style:   "themed",
		Unit:    "ms",
		Regions: []GeoRegion{{Code: "DE", Value: 1}, {Code: "FR", Value: 5}},
	})
	if g.Style != "themed" {
		t.Fatalf("style normalized to %q", g.Style)
	}
	de, ok := shapeByCode(g, "DE")
	if !ok {
		t.Fatalf("DE missing")
	}
	if !de.HasData || !strings.HasPrefix(de.Fill, "oklch(") {
		t.Fatalf("DE fill = %q hasData=%v, want oklch ramp fill", de.Fill, de.HasData)
	}
	if !strings.Contains(de.Tip, "Germany") || !strings.Contains(de.Tip, "1 ms") {
		t.Fatalf("DE tip = %q, want name + formatted value", de.Tip)
	}
	// A visible country with no data keeps the neutral fill + name tip.
	es, ok := shapeByCode(g, "ES")
	if !ok {
		t.Fatalf("ES missing")
	}
	if es.HasData || es.Fill != geoNoDataFill {
		t.Fatalf("ES fill = %q hasData=%v, want neutral no-data fill", es.Fill, es.HasData)
	}
	if !strings.Contains(es.Tip, "Spain") {
		t.Fatalf("ES tip = %q", es.Tip)
	}
}

// TestGlobeWire: wireframe shapes carry no fill, and the graticule is
// forced on even when not requested.
func TestGlobeWire(t *testing.T) {
	g := LayoutGlobe(GlobeData{Style: "wire"})
	if len(g.Shapes) == 0 {
		t.Fatalf("no shapes")
	}
	for _, s := range g.Shapes {
		if s.Fill != "none" {
			t.Fatalf("wire shape %s fill = %q, want none", s.Name, s.Fill)
		}
		if s.Tip != "" {
			t.Fatalf("wire shape %s should carry no tooltip", s.Name)
		}
	}
	if len(g.Graticule) == 0 {
		t.Fatalf("wire style must force the graticule on")
	}
	if !g.Payload.Graticule {
		t.Fatalf("payload must report the forced graticule")
	}
}

// TestGlobeGraticuleCount: from subpoint (0°, 0°) the visible 15° grid
// is 11 meridians (Δλ strictly inside ±90°) + 11 parallels (-75°..75°,
// each partially visible).
func TestGlobeGraticuleCount(t *testing.T) {
	g := LayoutGlobe(GlobeData{Graticule: true})
	if len(g.Graticule) != 22 {
		t.Fatalf("graticule paths = %d, want 22 (11 meridians + 11 parallels)", len(g.Graticule))
	}
	// Solid without the flag draws none.
	if n := len(LayoutGlobe(GlobeData{}).Graticule); n != 0 {
		t.Fatalf("unrequested graticule drew %d paths", n)
	}
}

// TestGlobePayload: the JSONScript payload marshals with the keys
// globe.js reads, resolved so the client needs no Go knowledge.
func TestGlobePayload(t *testing.T) {
	g := LayoutGlobe(GlobeData{
		Style:       "themed",
		Unit:        "ms",
		Regions:     []GeoRegion{{Code: "us", Value: 10}, {Code: "JP", Value: 90}},
		RotateSpeed: 4,
		Orbit:       true,
		Graticule:   true,
		Lon0:        -30,
		Lat0:        15,
		Zoom:        1.5,
		CenterX:     0.8,
		CenterY:     0.2,
	})
	if !g.Animate {
		t.Fatalf("RotateSpeed > 0 must set Animate")
	}
	raw, err := json.Marshal(g.Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{
		"style", "regions", "ramp", "min", "max", "unit",
		"lon0", "lat0", "zoom", "centerX", "centerY",
		"rotateSpeed", "orbit", "graticule", "src",
	} {
		if _, ok := m[k]; !ok {
			t.Fatalf("payload missing key %q in %s", k, raw)
		}
	}
	if m["src"] != "/static/geo/world-geo.json" {
		t.Fatalf("src = %v", m["src"])
	}
	regions, _ := m["regions"].(map[string]any)
	if regions["US"] != 10.0 || regions["JP"] != 90.0 {
		t.Fatalf("regions = %v, want case-normalized codes", m["regions"])
	}
	if m["min"] != 10.0 || m["max"] != 90.0 {
		t.Fatalf("min/max = %v/%v", m["min"], m["max"])
	}
	ramp, _ := m["ramp"].([]any)
	if len(ramp) != 16 {
		t.Fatalf("ramp swatches = %d, want 16", len(ramp))
	}
	for _, c := range ramp {
		if !strings.HasPrefix(c.(string), "oklch(") {
			t.Fatalf("ramp swatch %v is not oklch", c)
		}
	}

	// A static solid globe does not animate and omits the choropleth keys.
	s := LayoutGlobe(GlobeData{})
	if s.Animate {
		t.Fatalf("static globe must not set Animate")
	}
	if s.Payload.Regions != nil || s.Payload.Ramp != nil {
		t.Fatalf("solid payload should omit regions/ramp")
	}
}

// TestGlobeShadeUnique: each layout gets its own gradient id so several
// solid globes can share a page.
func TestGlobeShadeUnique(t *testing.T) {
	a := LayoutGlobe(GlobeData{})
	b := LayoutGlobe(GlobeData{})
	if a.ShadeID == "" || a.ShadeID == b.ShadeID {
		t.Fatalf("shade ids must be unique per instance: %q vs %q", a.ShadeID, b.ShadeID)
	}
	if !strings.HasPrefix(a.ShadeID, "globe-shade-") {
		t.Fatalf("shade id = %q", a.ShadeID)
	}
}
