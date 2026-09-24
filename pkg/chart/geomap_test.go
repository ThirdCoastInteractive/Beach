package chart

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// regexpMustCompilePath matches one M/L path token ("L123.4 56.7").
func regexpMustCompilePath() *regexp.Regexp {
	return regexp.MustCompile(`([ML])([0-9.]+) ([0-9.]+)`)
}

// The generated Natural Earth data is an input contract for LayoutGeoMap:
// pin the properties the layout relies on so a bad regeneration fails loudly.
func TestGeoDataGenerated(t *testing.T) {
	if len(geoCountries) < 170 {
		t.Fatalf("expected ≥170 countries, got %d", len(geoCountries))
	}
	if len(geoUSStates) != 51 {
		t.Fatalf("expected 51 US states (incl. DC), got %d", len(geoUSStates))
	}
	if len(geoCities) < 200 {
		t.Fatalf("expected ≥200 cities, got %d", len(geoCities))
	}
	if len(geoUSInsetFrames) != 2 {
		t.Fatalf("expected AK + HI inset frames, got %d", len(geoUSInsetFrames))
	}

	// ISO_A2 is -99 for France and Norway in Natural Earth; the generator
	// must have recovered both via ISO_A2_EH.
	want := map[string]bool{"US": false, "FR": false, "NO": false, "GB": false}
	for _, c := range geoCountries {
		if _, ok := want[c.Code]; ok {
			want[c.Code] = true
		}
		if c.Path == "" {
			t.Errorf("country %q (%s) has empty path", c.Code, c.Name)
		}
		if strings.ContainsRune(c.Name, '\x00') {
			t.Errorf("country %q name not NUL-stripped: %q", c.Code, c.Name)
		}
	}
	for code, found := range want {
		if !found {
			t.Errorf("country %s missing from generated data", code)
		}
	}

	for _, s := range geoUSStates {
		if !strings.HasPrefix(s.Code, "US-") {
			t.Errorf("state code %q not ISO 3166-2", s.Code)
		}
		if s.Path == "" {
			t.Errorf("state %q has empty path", s.Code)
		}
	}

	for _, p := range []geoPlane{geoWorldPlane, geoUSConusPlane, geoUSAlaskaPlane, geoUSHawaiiPlane} {
		if p.W <= 0 || p.H <= 0 || p.Scale <= 0 {
			t.Fatalf("degenerate plane: %+v", p)
		}
	}

	// The world plane must cover the full projection extent — the poles and
	// the equatorial antimeridian — or the outline paints outside the
	// viewBox (the "horizontal line in the arctic" bug).
	for _, pt := range [][2]float64{{0, 90}, {0, -90}, {179.999, 0}, {-179.999, 0}} {
		x, y := geoWorldPlane.xy(pt[0], pt[1])
		if x < -0.5 || x > geoWorldPlane.W+0.5 || y < -0.5 || y > geoWorldPlane.H+0.5 {
			t.Errorf("projection extreme (%g,%g) outside world plane: %.1f,%.1f", pt[0], pt[1], x, y)
		}
	}

	// No country path may contain a map-spanning line segment. Natural Earth
	// has float-dust points (lon 180.00000000000006) that, wrapped naively,
	// flip to the opposite map edge and draw a chord across the arctic —
	// the projection wrap must tolerate them (the "errant arctic line" bug).
	segRe := regexpMustCompilePath()
	for _, c := range geoCountries {
		prevX := -1.0
		for _, m := range segRe.FindAllStringSubmatch(c.Path, -1) {
			x, _ := strconv.ParseFloat(m[2], 64)
			y, _ := strconv.ParseFloat(m[3], 64)
			// Antarctica's ring legitimately traverses the map bottom along
			// the −90° pole line; only chords away from the pole edges are
			// bugs.
			poleEdge := y > geoWorldPlane.H-12 || y < 12
			if m[1] == "L" && prevX >= 0 && math.Abs(x-prevX) > 500 && !poleEdge {
				t.Errorf("country %q has a map-spanning segment (Δx=%.0f at y=%.0f)", c.Code, math.Abs(x-prevX), y)
			}
			prevX = x
		}
	}

	// The source TopoJSON carries cp1252→UTF-8 double encoding; the
	// generator must have repaired it ("SÃ£o Paulo" → "São Paulo").
	sawSaoPaulo := false
	for _, c := range geoCities {
		if strings.Contains(c.Name, "Ã") {
			t.Errorf("city name still double-encoded: %q", c.Name)
		}
		if c.Name == "São Paulo" {
			sawSaoPaulo = true
		}
	}
	if !sawSaoPaulo {
		t.Error("São Paulo missing — mojibake repair may have mangled names")
	}
}

func TestLayoutGeoMapWorld(t *testing.T) {
	g := LayoutGeoMap(GeoMapData{
		Regions: []GeoRegion{
			{Code: "us", Value: 900}, // case-insensitive
			{Code: "FR", Value: 100},
			{Code: "ZZ", Value: 50}, // unknown: ignored
		},
		Unit: "min",
	})

	if g.ViewBox == "" || !strings.HasPrefix(g.ViewBox, "0 0 ") {
		t.Fatalf("bad viewBox %q", g.ViewBox)
	}
	if len(g.Shapes) != len(geoCountries) {
		t.Fatalf("expected all %d countries drawn, got %d", len(geoCountries), len(g.Shapes))
	}

	var withData int
	var usFill, frFill string
	for i, s := range g.Shapes {
		if s.HasData {
			withData++
			if !strings.HasPrefix(s.Fill, "oklch(") {
				t.Errorf("data shape fill %q not on the OKLCH ramp", s.Fill)
			}
			switch geoCountries[i].Code {
			case "US":
				usFill = s.Fill
				if !strings.Contains(s.Tip, "900 min") {
					t.Errorf("US tip missing value: %q", s.Tip)
				}
			case "FR":
				frFill = s.Fill
			}
		} else {
			if s.Fill != geoNoDataFill {
				t.Errorf("no-data shape fill = %q", s.Fill)
			}
			if s.Tip == "" {
				t.Error("no-data shape should still carry a name tooltip")
			}
		}
	}
	if withData != 2 {
		t.Fatalf("expected 2 valued shapes (unknown ZZ dropped), got %d", withData)
	}
	if usFill == frFill {
		t.Error("min and max values mapped to the same ramp color")
	}

	if !g.Legend.Show {
		t.Fatal("legend hidden despite valued regions")
	}
	if g.Legend.MinLabel != "100 min" || g.Legend.MaxLabel != "900 min" {
		t.Errorf("legend labels = %q / %q", g.Legend.MinLabel, g.Legend.MaxLabel)
	}
	if len(g.Legend.Swatches) < 2 {
		t.Errorf("legend swatches = %d", len(g.Legend.Swatches))
	}
}

// The default world basemap: 10°/30° minor/major meridians (minors skip the
// majors' positions), 15° parallels minus the equator, and the named lines.
func TestGeoMapDefaultBasemap(t *testing.T) {
	g := LayoutGeoMap(GeoMapData{})

	if g.Outline == "" || !strings.HasSuffix(g.Outline, "Z") {
		t.Errorf("outline missing or unclosed: %.40q", g.Outline)
	}
	if len(g.Meridians) != 24 {
		t.Errorf("expected 24 minor meridians (35 minus 11 majors), got %d", len(g.Meridians))
	}
	if len(g.MajorMeridians) != 11 {
		t.Errorf("expected 11 major meridians, got %d", len(g.MajorMeridians))
	}
	if len(g.Parallels) != 10 {
		t.Errorf("expected 10 minor parallels (11 minus equator), got %d", len(g.Parallels))
	}
	if g.Equator == "" {
		t.Error("equator missing")
	}
	if len(g.Tropics) != 2 || len(g.PolarCircles) != 2 {
		t.Errorf("named parallels: tropics=%d polar=%d", len(g.Tropics), len(g.PolarCircles))
	}
	for _, m := range g.Meridians {
		if !strings.HasPrefix(m, "M") || strings.HasSuffix(m, "Z") {
			t.Errorf("graticule line malformed (must be open path): %.40q", m)
		}
	}
	if len(g.Insets) != 0 {
		t.Error("world map should have no inset frames")
	}
}

// A zero-value Basemap turns every layer off; a custom one draws only what
// the caller asked for.
func TestGeoMapCustomBasemap(t *testing.T) {
	off := LayoutGeoMap(GeoMapData{Basemap: &GeoBasemap{}})
	if off.Outline != "" || len(off.Meridians) != 0 || len(off.MajorMeridians) != 0 ||
		len(off.Parallels) != 0 || off.Equator != "" || len(off.Tropics) != 0 || len(off.PolarCircles) != 0 {
		t.Error("zero-value basemap should disable every layer")
	}

	only := LayoutGeoMap(GeoMapData{Basemap: &GeoBasemap{Equator: true}})
	if only.Equator == "" || len(only.Meridians) != 0 || only.Outline != "" {
		t.Error("custom basemap should draw exactly the requested layers")
	}
}

func TestLayoutGeoMapUSStates(t *testing.T) {
	g := LayoutGeoMap(GeoMapData{
		Level: GeoLevelUSStates,
		Regions: []GeoRegion{
			{Code: "MN", Value: 10},    // bare postal shorthand
			{Code: "US-TX", Value: 20}, // full ISO 3166-2
		},
	})
	if len(g.Shapes) != 51 {
		t.Fatalf("expected 51 state shapes, got %d", len(g.Shapes))
	}
	var matched int
	for _, s := range g.Shapes {
		if s.HasData {
			matched++
		}
	}
	if matched != 2 {
		t.Fatalf("expected MN and US-TX both matched, got %d", matched)
	}

	world := LayoutGeoMap(GeoMapData{})
	if g.ViewBox == world.ViewBox {
		t.Error("US composite should have its own viewBox")
	}
	if len(g.Insets) != 2 {
		t.Fatalf("expected 2 inset window frames, got %d", len(g.Insets))
	}
	if g.Outline != "" {
		t.Error("US default basemap should not draw the projection outline")
	}
	if len(g.Meridians) == 0 || len(g.Parallels) == 0 {
		t.Error("US default basemap should draw a graticule")
	}
}

// Alaska and Hawaii live in inset windows: points there must land inside
// their frames, not at their raw geographic positions.
func TestGeoMapUSInsetProjection(t *testing.T) {
	g := LayoutGeoMap(GeoMapData{
		Level: GeoLevelUSStates,
		Points: []GeoPoint{
			{Name: "Honolulu", Lon: -157.86, Lat: 21.31, Value: 10},
			{Name: "Anchorage", Lon: -149.9, Lat: 61.22, Value: 10},
			{Name: "Juneau", Lon: -134.42, Lat: 58.3, Value: 10},
			{Name: "Chicago", Lon: -87.63, Lat: 41.88, Value: 10},
			// The far Aleutians are simplified out of the 110m data, so the
			// Alaska window doesn't reach them; points there clip away,
			// same as d3's albersUsa.
			{Name: "Attu", Lon: 172.9, Lat: 52.85, Value: 10},
		},
	})
	if len(g.Points) != 4 {
		t.Fatalf("expected 4 projected points (Attu clipped), got %d", len(g.Points))
	}

	planes := map[string]geoPlane{
		"Honolulu": geoUSHawaiiPlane, "Anchorage": geoUSAlaskaPlane,
		"Juneau": geoUSAlaskaPlane, "Chicago": geoUSConusPlane,
	}
	names := []string{"Honolulu", "Anchorage", "Juneau", "Chicago"}
	for i, p := range g.Points {
		want := planes[names[i]]
		x, errX := strconv.ParseFloat(p.CX, 64)
		y, errY := strconv.ParseFloat(p.CY, 64)
		if errX != nil || errY != nil {
			t.Fatalf("bad point coords %q,%q", p.CX, p.CY)
		}
		if !want.contains(x, y) {
			t.Errorf("%s at (%.1f,%.1f) outside its plane window %+v", names[i], x, y, want)
		}
	}
}

func TestLayoutGeoMapEmptyIsBasemap(t *testing.T) {
	g := LayoutGeoMap(GeoMapData{})
	if len(g.Shapes) == 0 {
		t.Fatal("empty data should still render landmass")
	}
	if g.Legend.Show {
		t.Error("legend shown with no values")
	}
	for _, s := range g.Shapes {
		if s.HasData {
			t.Fatal("no shape should claim data")
		}
	}
}

func TestGeoMapCitiesAndPoints(t *testing.T) {
	g := LayoutGeoMap(GeoMapData{
		ShowCities: true, // world default: rank 0 megacities
		Points: []GeoPoint{
			{Name: "Chicago", Lon: -87.63, Lat: 41.88, Value: 500},
			{Name: "Tiny", Lon: 2.35, Lat: 48.85, Value: 5},
		},
	})
	if len(g.Cities) == 0 {
		t.Fatal("no reference cities marked")
	}
	if len(g.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(g.Points))
	}
	if g.Points[0].R == g.Points[1].R {
		t.Error("bubble radius should scale with value")
	}
	for _, c := range g.Cities {
		if c.Label == "" || c.CX == "" {
			t.Errorf("malformed city %+v", c)
		}
	}

	// US level keeps only US cities.
	us := LayoutGeoMap(GeoMapData{Level: GeoLevelUSStates, ShowCities: true})
	if len(us.Cities) == 0 {
		t.Fatal("no US cities at us-states level")
	}
	for _, c := range us.Cities {
		if !strings.Contains(c.Tip, "United States") {
			t.Errorf("non-US city %q on US map", c.Label)
		}
	}
}

// Interactive flag flows through to the layout; DrillAction formats a
// per-shape Datastar action with each shape's own region code, for both
// valued and no-data shapes; an empty DrillAction leaves Drill blank.
func TestGeoMapInteractiveAndDrill(t *testing.T) {
	g := LayoutGeoMap(GeoMapData{
		Interactive: true,
		DrillAction: "@get('/api/geo/%s')",
		Regions:     []GeoRegion{{Code: "US", Value: 5}},
	})

	if !g.Interactive {
		t.Error("Interactive flag did not flow to GeoMap output")
	}

	var sawUS, sawNoDataDrill bool
	for _, s := range g.Shapes {
		if s.Code == "" {
			continue // uncoded shapes carry no drill
		}
		want := "@get('/api/geo/" + s.Code + "')"
		if s.Drill != want {
			t.Errorf("shape %s Drill = %q, want %q", s.Code, s.Drill, want)
		}
		if s.Code == "US" {
			sawUS = true
			if !s.HasData {
				t.Error("US should be a valued shape")
			}
		}
		if !s.HasData {
			sawNoDataDrill = true // no-data shapes are drillable too
		}
	}
	if !sawUS {
		t.Error("US shape missing from interactive map")
	}
	if !sawNoDataDrill {
		t.Error("expected no-data shapes to also receive a Drill action")
	}
}

// Every shape carries its source Code; with no DrillAction none get a Drill,
// and the map is non-interactive by default.
func TestGeoMapShapeCodesNoDrill(t *testing.T) {
	g := LayoutGeoMap(GeoMapData{
		Regions: []GeoRegion{{Code: "US", Value: 5}},
	})

	if g.Interactive {
		t.Error("map should be non-interactive without Interactive set")
	}

	var coded int
	for _, s := range g.Shapes {
		if s.Drill != "" {
			t.Errorf("shape %s got a Drill with no DrillAction: %q", s.Code, s.Drill)
		}
		if s.Code != "" {
			coded++
		}
	}
	// The world basemap codes essentially every country; require the bulk of
	// them to prove Code is populated from the source shapes.
	if coded < len(g.Shapes)/2 {
		t.Errorf("only %d/%d shapes carry a Code", coded, len(g.Shapes))
	}
}

// Named ramps restyle the fills; unknown names fall back to the single-hue
// default.
func TestGeoMapRamps(t *testing.T) {
	base := GeoMapData{Regions: []GeoRegion{{Code: "US", Value: 1}, {Code: "FR", Value: 9}}}

	fills := func(d GeoMapData) []string {
		var out []string
		for _, s := range LayoutGeoMap(d).Shapes {
			if s.HasData {
				out = append(out, s.Fill)
			}
		}
		return out
	}

	def := fills(base)
	ember := base
	ember.Ramp = "ember"
	emberFills := fills(ember)
	if def[0] == emberFills[0] && def[1] == emberFills[1] {
		t.Error("ember ramp should differ from the default")
	}

	unknown := base
	unknown.Ramp = "does-not-exist"
	if u := fills(unknown); u[0] != def[0] || u[1] != def[1] {
		t.Error("unknown ramp should fall back to the default")
	}

	for name := range geoRamps {
		d := base
		d.Ramp = name
		for _, f := range fills(d) {
			if !strings.HasPrefix(f, "oklch(") {
				t.Errorf("ramp %s produced non-OKLCH fill %q", name, f)
			}
		}
	}
}
