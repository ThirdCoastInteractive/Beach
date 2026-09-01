package chart

import (
	"fmt"
	"math"
	"strings"
)

// --- Input types ------------------------------------------------------------

// GeoLevel selects which shape set a geo map draws.
type GeoLevel string

const (
	// GeoLevelWorld draws all countries on an Equal Earth world plane
	// centered on the prime meridian. Region codes are ISO 3166-1 alpha-2
	// ("US", "GB").
	GeoLevelWorld GeoLevel = "world"
	// GeoLevelUSStates draws the 50 US states + DC with Alaska and Hawaii
	// in traditional inset windows below the conterminous map. Region codes
	// are ISO 3166-2 ("US-MN") or bare postal codes ("MN").
	GeoLevelUSStates GeoLevel = "us-states"
)

// GeoMapData describes an Equal Earth choropleth map. Shapes with no
// matching region render as neutral landmass, so a GeoMapData with no
// Regions is still a valid basemap.
type GeoMapData struct {
	Level   GeoLevel    `json:"level,omitempty"` // default GeoLevelWorld
	Regions []GeoRegion `json:"regions"`
	Points  []GeoPoint  `json:"points,omitempty"`
	// ShowCities marks reference cities from the embedded Natural Earth
	// gazetteer (rank-filtered via MaxCityRank).
	ShowCities bool `json:"showCities,omitempty"`
	// MaxCityRank is the largest Natural Earth SCALERANK (0 = megacity,
	// 8 = minor) still shown when ShowCities is set. Default 0 for world,
	// 2 for US states.
	MaxCityRank int    `json:"maxCityRank,omitempty"`
	Unit        string `json:"unit,omitempty"`
	// Ramp names a fill color ramp ("ocean", "forest", "ember", "royal",
	// "mono"). Empty uses the single-hue default driven by Hue.
	Ramp string  `json:"ramp,omitempty"`
	Hue  float64 `json:"hue,omitempty"` // default-ramp OKLCH hue, default 250
	// Basemap overrides the graticule/outline layers. Nil takes the level's
	// defaults; a zero-value &GeoBasemap{} turns every layer off.
	Basemap *GeoBasemap `json:"basemap,omitempty"`
	// Interactive enables the client-side zoom/pan module
	// (geomap-interact.js): wheel zoom, drag pan, double-click reset, and
	// overlay +/−/reset controls. It sets GeoMap.Interactive, which the
	// templ layer turns into the data-geomap-interact hook on the svg.
	Interactive bool `json:"interactive,omitempty"`
	// DrillAction is a Datastar action template applied per shape, with a
	// single %s replaced by the shape's region code — e.g.
	// "@get('/api/geo/%s')". Build the template with the pkg/datastar
	// helpers (datastar.Get(...) etc.) rather than by hand; only the code
	// substitution happens here. When set, every shape with a non-empty
	// code (data or no-data alike) gets a formatted GeoMapShape.Drill, so a
	// caller can drill into any region on the map. Empty disables drilling.
	DrillAction string `json:"drillAction,omitempty"`
}

// GeoBasemap controls the non-data layers, each independently. The zero
// value disables everything; defaultGeoBasemap gives the per-level house
// defaults.
type GeoBasemap struct {
	// Outline draws the projection boundary (the Equal Earth "globe" edge)
	// with its ocean wash.
	Outline bool `json:"outline,omitempty"`
	// MeridianStep spaces the minor longitude lines, degrees; 0 = none.
	MeridianStep float64 `json:"meridianStep,omitempty"`
	// MajorMeridianStep spaces the emphasized longitude lines, degrees;
	// 0 = none. Minor lines coinciding with a major are skipped.
	MajorMeridianStep float64 `json:"majorMeridianStep,omitempty"`
	// ParallelStep spaces the minor latitude lines, degrees; 0 = none.
	ParallelStep float64 `json:"parallelStep,omitempty"`
	// Equator / Tropics / PolarCircles draw the named parallels in their
	// traditional styles (equator solid, tropics + polar circles dashed).
	Equator      bool `json:"equator,omitempty"`
	Tropics      bool `json:"tropics,omitempty"`
	PolarCircles bool `json:"polarCircles,omitempty"`
}

// defaultGeoBasemap is the house basemap per level: a 10°/30° minor/major
// meridian grid with 15° parallels and the named lines on the world map; a
// tighter 5°/15° grid and no named lines on the cropped US map.
func defaultGeoBasemap(level GeoLevel) GeoBasemap {
	if level == GeoLevelUSStates {
		return GeoBasemap{
			MeridianStep:      5,
			MajorMeridianStep: 15,
			ParallelStep:      5,
		}
	}
	return GeoBasemap{
		Outline:           true,
		MeridianStep:      10,
		MajorMeridianStep: 30,
		ParallelStep:      15,
		Equator:           true,
		Tropics:           true,
		PolarCircles:      true,
	}
}

// GeoRegion assigns a value to one country or state.
type GeoRegion struct {
	Code  string  `json:"code"`
	Value float64 `json:"value"`
}

// GeoPoint is a caller-supplied lon/lat marker (a bubble sized by value).
type GeoPoint struct {
	Name  string  `json:"name"`
	Lon   float64 `json:"lon"`
	Lat   float64 `json:"lat"`
	Value float64 `json:"value"`
}

// --- Output geometry ----------------------------------------------------------

type GeoMap struct {
	ViewBox string
	// Interactive mirrors GeoMapData.Interactive; when true the templ layer
	// marks the svg with data-geomap-interact so geomap-interact.js wires up
	// zoom/pan/reset and the overlay controls.
	Interactive bool
	// Outline is the projection boundary path (pole lines included).
	Outline string
	// Graticule layers, each drawn beneath the landmass shapes so filled
	// geography occludes them. Meridians/Parallels are the minor grid;
	// MajorMeridians are emphasized; the named parallels carry their
	// traditional dashed styling via CSS class.
	Meridians      []string
	MajorMeridians []string
	Parallels      []string
	Equator        string
	Tropics        []string
	PolarCircles   []string
	// Insets are the Alaska/Hawaii window frames on the US map, drawn over
	// the graticule and under the shapes.
	Insets []GeoMapInset
	Shapes []GeoMapShape
	Cities []GeoMapCity
	Points []GeoMapPoint
	Legend GeoMapLegend
}

type GeoMapShape struct {
	Path    string
	Fill    string
	Tip     string
	HasData bool
	// Code is the shape's region code (ISO alpha-2 country or ISO 3166-2
	// state), copied verbatim from the source shape. It may be "" for a
	// shape the generated data left uncoded; the templ layer emits it as
	// data-region for the client (click drill-down, region events).
	Code string
	// Drill is the fully formatted Datastar action for this shape —
	// GeoMapData.DrillAction with %s replaced by Code. It is set only when
	// DrillAction is non-empty AND Code is non-empty (no-data shapes are
	// drillable too), and emitted as data-on:click by the templ layer.
	Drill string
}

type GeoMapCity struct {
	CX, CY string
	Label  string
	Tip    string
}

type GeoMapPoint struct {
	CX, CY, R string
	Fill      string
	Tip       string
}

// GeoMapInset is one inset window frame (viewBox coordinates).
type GeoMapInset struct {
	X, Y, W, H string
}

// GeoMapLegend is a min→max gradient strip. Show is false when the map has
// no valued regions.
type GeoMapLegend struct {
	Show               bool
	MinLabel, MaxLabel string
	Swatches           []string
}

// --- Generated-data plumbing ---------------------------------------------------
// geodata_gen.go (emitted by cmd/beach-geogen from Natural Earth 110m data)
// fills the geo*Plane vars, geoCountries, geoUSStates, geoUSInsetFrames,
// geoUSViewW/H, and geoCities.

// geoPlane scales raw Equal Earth coordinates into a window of the SVG
// viewBox (y growing downward). Off positions the window inside a composite
// viewBox — the US map's Alaska/Hawaii insets are separate planes offset
// into their frames; the world map is a single full-viewBox plane.
type geoPlane struct {
	Lon0, MinX, MaxY, Scale float64
	OffX, OffY              float64
	W, H                    float64
}

type geoShapeSrc struct {
	Code, Name, Path string
}

type geoCitySrc struct {
	Name, Country string
	Lon, Lat      float64
	Rank, Pop     int
	Capital       bool
}

// geoRect is a generated inset frame rectangle.
type geoRect struct {
	X, Y, W, H float64
}

// Equal Earth projection coefficients (Šavrič, Patterson & Jenny 2018).
// Keep in sync with the generator copy in cmd/beach-geogen/main.go.
const (
	eeA1 = 1.340264
	eeA2 = -0.081106
	eeA3 = 0.000893
	eeA4 = 0.003796
)

func (p geoPlane) xy(lonDeg, latDeg float64) (x, y float64) {
	lon := lonDeg - p.Lon0
	// The wrap tolerates float dust in the source data (Natural Earth has
	// points like 180.00000000000006 that must NOT flip to the west edge).
	for lon > 180+1e-9 {
		lon -= 360
	}
	for lon < -180-1e-9 {
		lon += 360
	}
	lam := lon * math.Pi / 180
	phi := latDeg * math.Pi / 180
	m := math.Sqrt(3) / 2
	theta := math.Asin(m * math.Sin(phi))
	t2 := theta * theta
	t6 := t2 * t2 * t2
	ex := lam * math.Cos(theta) / (m * (eeA1 + 3*eeA2*t2 + t6*(7*eeA3+9*eeA4*t2)))
	ey := theta * (eeA1 + eeA2*t2 + t6*(eeA3+eeA4*t2))
	return p.OffX + (ex-p.MinX)*p.Scale, p.OffY + (p.MaxY-ey)*p.Scale
}

// contains reports whether a composite-space point falls inside this
// plane's window.
func (p geoPlane) contains(x, y float64) bool {
	return x >= p.OffX && x <= p.OffX+p.W && y >= p.OffY && y <= p.OffY+p.H
}

// usPlaneFor picks the composite-US plane a lon/lat belongs to: the Alaska
// window (including the far Aleutians across the antimeridian), the Hawaii
// window, or the conterminous plane.
func usPlaneFor(lon, lat float64) geoPlane {
	switch {
	case lat > 48 && (lon < -128 || lon > 165):
		return geoUSAlaskaPlane
	case lat < 25 && lat > 15 && lon < -150 && lon > -165:
		return geoUSHawaiiPlane
	default:
		return geoUSConusPlane
	}
}

// --- Graticule + outline ---------------------------------------------------------

// geoPathFrom renders a sampled lon/lat polyline as an SVG path on the
// plane, rounded to the generator's 0.1-unit resolution. With clip set,
// points outside the plane's window are dropped and the path restarts when
// it re-enters, trimming graticule lines to the window.
func geoPathFrom(p geoPlane, pts [][2]float64, closed, clip bool) string {
	var b strings.Builder
	prevX, prevY := math.NaN(), math.NaN()
	inRun := false
	for _, pt := range pts {
		x, y := p.xy(pt[0], pt[1])
		if clip && !p.contains(x, y) {
			inRun = false
			continue
		}
		rx, ry := math.Round(x*10)/10, math.Round(y*10)/10
		if rx == prevX && ry == prevY {
			continue
		}
		if !inRun {
			fmt.Fprintf(&b, "M%.1f %.1f", rx, ry)
			inRun = true
		} else {
			fmt.Fprintf(&b, "L%.1f %.1f", rx, ry)
		}
		prevX, prevY = rx, ry
	}
	if b.Len() > 0 && closed {
		b.WriteString("Z")
	}
	return b.String()
}

// geoOutline traces the projection boundary: down the antimeridian on the
// east side, along the south pole line, up the west side, and back across
// the north pole line. (Equal Earth has pole lines, not pole points, so the
// polar edges are real segments.)
func geoOutline(p geoPlane) string {
	east := p.Lon0 + 180 - 1e-6
	west := p.Lon0 - 180 + 1e-6
	var pts [][2]float64
	for lat := 90.0; lat >= -90; lat -= 2 {
		pts = append(pts, [2]float64{east, lat})
	}
	for lon := east; lon >= west; lon -= 2 {
		pts = append(pts, [2]float64{lon, -90})
	}
	for lat := -90.0; lat <= 90; lat += 2 {
		pts = append(pts, [2]float64{west, lat})
	}
	for lon := west; lon <= east; lon += 2 {
		pts = append(pts, [2]float64{lon, 90})
	}
	return geoPathFrom(p, pts, true, false)
}

// geoMeridian samples one longitude line pole to pole.
func geoMeridian(p geoPlane, lon float64, clip bool) string {
	var pts [][2]float64
	for lat := 90.0; lat >= -90; lat -= 2 {
		pts = append(pts, [2]float64{lon, lat})
	}
	return geoPathFrom(p, pts, false, clip)
}

// geoParallel samples one latitude line across the full longitude range.
// (Equal Earth parallels are straight, but clipping needs the interior
// samples, so it is sampled like everything else.)
func geoParallel(p geoPlane, lat float64, clip bool) string {
	east := p.Lon0 + 180 - 1e-6
	west := p.Lon0 - 180 + 1e-6
	var pts [][2]float64
	for lon := west; lon <= east; lon += 2 {
		pts = append(pts, [2]float64{lon, lat})
	}
	return geoPathFrom(p, pts, false, clip)
}

// isMultipleOf reports whether v sits on a step-degree grid line.
func isMultipleOf(v, step float64) bool {
	if step <= 0 {
		return false
	}
	m := math.Abs(math.Mod(v, step))
	return m < 1e-9 || step-m < 1e-9
}

// Named parallels (degrees).
const (
	geoTropicLat = 23.43655
	geoPolarLat  = 66.56345
)

// buildBasemap assembles the requested graticule layers on the base plane.
// clip trims lines to the plane window (the US composite clips to the
// conterminous window so lines stay out of the inset strip).
func buildBasemap(out *GeoMap, bm GeoBasemap, p geoPlane, clip bool) {
	if bm.Outline {
		out.Outline = geoOutline(p)
	}
	if bm.MajorMeridianStep > 0 {
		for lon := -180 + bm.MajorMeridianStep; lon < 180; lon += bm.MajorMeridianStep {
			if m := geoMeridian(p, lon, clip); m != "" {
				out.MajorMeridians = append(out.MajorMeridians, m)
			}
		}
	}
	if bm.MeridianStep > 0 {
		for lon := -180 + bm.MeridianStep; lon < 180; lon += bm.MeridianStep {
			if isMultipleOf(lon, bm.MajorMeridianStep) {
				continue // drawn as a major line
			}
			if m := geoMeridian(p, lon, clip); m != "" {
				out.Meridians = append(out.Meridians, m)
			}
		}
	}
	if bm.ParallelStep > 0 {
		for lat := -90 + bm.ParallelStep; lat < 90; lat += bm.ParallelStep {
			if lat == 0 && bm.Equator {
				continue // drawn as the equator
			}
			if pl := geoParallel(p, lat, clip); pl != "" {
				out.Parallels = append(out.Parallels, pl)
			}
		}
	}
	if bm.Equator {
		out.Equator = geoParallel(p, 0, clip)
	}
	if bm.Tropics {
		for _, lat := range []float64{geoTropicLat, -geoTropicLat} {
			if pl := geoParallel(p, lat, clip); pl != "" {
				out.Tropics = append(out.Tropics, pl)
			}
		}
	}
	if bm.PolarCircles {
		for _, lat := range []float64{geoPolarLat, -geoPolarLat} {
			if pl := geoParallel(p, lat, clip); pl != "" {
				out.PolarCircles = append(out.PolarCircles, pl)
			}
		}
	}
}

// --- Color ramps -----------------------------------------------------------------

type geoRampStop struct {
	L, C, H float64
}

// geoRamps are the named fill themes: three-stop OKLCH gradients from dim
// to bold, hue-drifting the way printed relief tints do.
var geoRamps = map[string][]geoRampStop{
	"ocean":  {{0.24, 0.05, 255}, {0.55, 0.15, 245}, {0.80, 0.13, 200}},
	"forest": {{0.25, 0.05, 155}, {0.60, 0.15, 150}, {0.82, 0.16, 120}},
	"ember":  {{0.24, 0.04, 30}, {0.55, 0.18, 35}, {0.80, 0.15, 75}},
	"royal":  {{0.25, 0.06, 290}, {0.55, 0.20, 315}, {0.78, 0.13, 340}},
	"mono":   {{0.25, 0.005, 250}, {0.85, 0.005, 250}},
}

// geoRampFor resolves the fill ramp: a named theme, or the single-hue
// default (the heatmap's ramp shape with a configurable hue).
func geoRampFor(name string, hue float64) []geoRampStop {
	if stops, ok := geoRamps[name]; ok {
		return stops
	}
	if hue == 0 {
		hue = 250
	}
	return []geoRampStop{{0.25, 0.02, hue}, {0.70, 0.18, hue}}
}

// geoRampColor interpolates the ramp at t in [0,1].
func geoRampColor(stops []geoRampStop, t float64) string {
	t = math.Max(0, math.Min(1, t))
	seg := t * float64(len(stops)-1)
	i := int(seg)
	if i >= len(stops)-1 {
		i = len(stops) - 2
	}
	f := seg - float64(i)
	a, b := stops[i], stops[i+1]
	return fmt.Sprintf("oklch(%.2f %.3f %.0f)",
		a.L+(b.L-a.L)*f, a.C+(b.C-a.C)*f, a.H+(b.H-a.H)*f)
}

const geoNoDataFill = "var(--color-line-soft)"

// --- Layout ---------------------------------------------------------------------

// LayoutGeoMap projects a choropleth onto the requested plane. Region codes
// are matched case-insensitively; on GeoLevelUSStates a bare postal code is
// accepted as shorthand for its US- prefixed ISO 3166-2 form. Unknown codes
// are ignored.
func LayoutGeoMap(data GeoMapData) GeoMap {
	level := data.Level
	if level == "" {
		level = GeoLevelWorld
	}

	shapes := geoCountries
	basePlane := geoWorldPlane
	viewW, viewH := geoWorldPlane.W, geoWorldPlane.H
	clipBasemap := false
	if level == GeoLevelUSStates {
		shapes = geoUSStates
		basePlane = geoUSConusPlane
		viewW, viewH = geoUSViewW, geoUSViewH
		clipBasemap = true // keep graticule out of the inset strip
	}

	known := make(map[string]bool, len(shapes))
	for _, s := range shapes {
		if s.Code != "" {
			known[s.Code] = true
		}
	}

	values := make(map[string]float64, len(data.Regions))
	minV, maxV := math.Inf(1), math.Inf(-1)
	for _, r := range data.Regions {
		code := strings.ToUpper(strings.TrimSpace(r.Code))
		if level == GeoLevelUSStates && len(code) == 2 {
			code = "US-" + code
		}
		if !known[code] {
			continue // unknown codes must not stretch the color scale
		}
		values[code] = r.Value
		minV = math.Min(minV, r.Value)
		maxV = math.Max(maxV, r.Value)
	}
	span := maxV - minV
	if span == 0 {
		span = 1
	}

	out := GeoMap{
		ViewBox:     fmt.Sprintf("0 0 %.0f %.0f", viewW, viewH),
		Interactive: data.Interactive,
	}

	bm := defaultGeoBasemap(level)
	if data.Basemap != nil {
		bm = *data.Basemap
	}
	buildBasemap(&out, bm, basePlane, clipBasemap)

	if level == GeoLevelUSStates {
		for _, f := range geoUSInsetFrames {
			out.Insets = append(out.Insets, GeoMapInset{
				X: F(f.X), Y: F(f.Y), W: F(f.W), H: F(f.H),
			})
		}
	}

	ramp := geoRampFor(data.Ramp, data.Hue)

	for _, s := range shapes {
		v, ok := values[s.Code]
		shape := GeoMapShape{Path: s.Path, Fill: geoNoDataFill, HasData: ok, Code: s.Code}
		if data.DrillAction != "" && s.Code != "" {
			shape.Drill = fmt.Sprintf(data.DrillAction, s.Code)
		}
		if ok {
			t := (v - minV) / span
			shape.Fill = geoRampColor(ramp, t)
			shape.Tip = BuildTipHTML(shape.Fill, s.Name,
				[]TipRow{{Label: "value", Value: geoFormatValue(v, data.Unit)}})
		} else {
			shape.Tip = BuildTipHTML("", s.Name, nil)
		}
		out.Shapes = append(out.Shapes, shape)
	}

	if len(values) > 0 {
		const swatches = 6
		leg := GeoMapLegend{
			Show:     true,
			MinLabel: geoFormatValue(minV, data.Unit),
			MaxLabel: geoFormatValue(maxV, data.Unit),
		}
		for i := 0; i < swatches; i++ {
			leg.Swatches = append(leg.Swatches, geoRampColor(ramp, float64(i)/(swatches-1)))
		}
		out.Legend = leg
	}

	project := func(lon, lat float64) (float64, float64, bool) {
		p := basePlane
		if level == GeoLevelUSStates {
			p = usPlaneFor(lon, lat)
		}
		x, y := p.xy(lon, lat)
		return x, y, p.contains(x, y)
	}

	if data.ShowCities {
		maxRank := data.MaxCityRank
		if maxRank == 0 && level == GeoLevelUSStates {
			maxRank = 2
		}
		for _, c := range geoCities {
			if c.Rank > maxRank {
				continue
			}
			if level == GeoLevelUSStates && c.Country != "United States of America" {
				continue
			}
			x, y, ok := project(c.Lon, c.Lat)
			if !ok {
				continue
			}
			out.Cities = append(out.Cities, GeoMapCity{
				CX:    F(x),
				CY:    F(y),
				Label: c.Name,
				Tip: BuildTipHTML("", c.Name, []TipRow{
					{Label: "country", Value: c.Country},
					{Label: "population", Value: CommaInt(c.Pop)},
				}),
			})
		}
	}

	if len(data.Points) > 0 {
		maxP := math.Inf(-1)
		for _, p := range data.Points {
			maxP = math.Max(maxP, p.Value)
		}
		if maxP <= 0 {
			maxP = 1
		}
		for _, p := range data.Points {
			x, y, ok := project(p.Lon, p.Lat)
			if !ok {
				continue
			}
			r := 4 + 14*math.Sqrt(math.Max(p.Value, 0)/maxP)
			out.Points = append(out.Points, GeoMapPoint{
				CX:   F(x),
				CY:   F(y),
				R:    F(r),
				Fill: ColorVar(0),
				Tip: BuildTipHTML(ColorVar(0), p.Name,
					[]TipRow{{Label: "value", Value: geoFormatValue(p.Value, data.Unit)}}),
			})
		}
	}

	return out
}

// geoFormatValue trims trailing zeros so whole numbers stay whole.
func geoFormatValue(v float64, unit string) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
	if unit != "" {
		s += " " + unit
	}
	return s
}

// geoRingsSrc is a generated raw lon/lat polygon set (one country), used by
// the orthographic/3D renderers that re-project at draw time.
type geoRingsSrc struct {
	Code, Name string
	Rings      [][][2]float64
}
