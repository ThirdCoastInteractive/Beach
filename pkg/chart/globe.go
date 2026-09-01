// Orthographic globe chart. LayoutGlobe projects the generated country
// rings (geodata_rings_gen.go) onto a hemisphere disc as one SSR frame;
// the templ layer (globe.templ) emits the SVG, and when animation is
// requested the client module (globe.js) takes over with a canvas that
// re-projects the same data every frame. The SSR frame is always emitted
// so a page without JS still shows a correct static globe.
package chart

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"
)

// --- Input -------------------------------------------------------------------

// GlobeData describes an orthographic globe. The zero value renders a
// solid shaded globe centered on (0°, 0°) filling its frame.
type GlobeData struct {
	// Style selects the rendering: "solid" (default) draws a single
	// shaded landmass with a limb-darkening overlay; "themed" draws a
	// choropleth with ramp fills and tooltips from Regions; "wire" draws
	// fill-less translucent outlines with the graticule always on.
	Style string
	// Regions assigns choropleth values by ISO 3166-1 alpha-2 code
	// ("US", "GB"); used by the "themed" style. Unknown codes are
	// ignored so they cannot stretch the color scale.
	Regions []GeoRegion
	Unit    string
	// Ramp names a fill theme ("ocean", "forest", "ember", "royal",
	// "mono"); empty uses the single-hue default driven by Hue.
	Ramp string
	Hue  float64
	// Lon0/Lat0 place the camera subpoint (degrees). Geography more
	// than 90° of arc from the subpoint is on the far side and clipped.
	Lon0, Lat0 float64
	// Zoom scales the globe disc: 1 (default) fits the disc in the
	// frame; >1 zooms in, pushing the limb out of frame for the
	// partial-limb "orbit glance" look.
	Zoom float64
	// CenterX/CenterY place the globe center in the frame as 0..1
	// fractions (default 0.5, 0.5). With Zoom > 1 an off-center value
	// leaves only a sweep of the limb visible in the corner.
	CenterX, CenterY float64
	// RotateSpeed spins the globe eastward, degrees per second. Any
	// value > 0 makes the client module animate; 0 stays static.
	RotateSpeed float64
	// Orbit adds a slow gentle latitude oscillation (±8°, ~40 s period)
	// on top of the rotation, like drifting over the planet.
	Orbit bool
	// Graticule draws the 15° meridian/parallel grid clipped to the
	// visible hemisphere. Always on for the "wire" style.
	Graticule bool
}

// --- Output geometry ------------------------------------------------------------

// Globe is one server-rendered orthographic frame plus the client
// payload for the animated takeover.
type Globe struct {
	ViewBox string
	// CX/CY/R are the limb circle geometry in viewBox units: disc
	// center (CenterX·1000, CenterY·1000) and radius 490·Zoom.
	CX, CY, R string
	// Style is the normalized style name ("solid", "themed", "wire").
	Style string
	// ShadeID is the per-instance radialGradient id for the solid
	// style's limb-darkening overlay (unique so several globes can
	// share a page).
	ShadeID string
	// Animate is true when RotateSpeed > 0 or Orbit is set; the templ
	// layer then tags the figure data-globe and embeds Payload.
	Animate bool
	// Graticule paths (meridians + parallels), clipped to the visible
	// hemisphere.
	Graticule []string
	Shapes    []GlobeShape
	// Payload feeds globe.js via templ.JSONScript when Animate is set.
	Payload GlobePayload
}

// GlobeShape is one projected country. Countries entirely on the far
// side are omitted (empty path).
type GlobeShape struct {
	Code, Name string
	Path       string
	// Fill is the landmass color: the neutral no-data fill for "solid",
	// a ramp color or the neutral fill for "themed", "none" for "wire".
	Fill    string
	Tip     string // data-tip tooltip HTML; themed style only
	HasData bool
}

// GlobePayload is the JSON handed to globe.js. It is self-contained:
// the module needs no Go knowledge, just this object plus the world
// geometry fetched from Src.
type GlobePayload struct {
	Style string `json:"style"`
	// Regions maps ISO alpha-2 code to value (themed style).
	Regions map[string]float64 `json:"regions,omitempty"`
	// Ramp is the resolved fill scale as a list of CSS color strings
	// from low to high; the client picks by normalized value.
	Ramp []string `json:"ramp,omitempty"`
	Min  float64  `json:"min"`
	Max  float64  `json:"max"`
	Unit string   `json:"unit,omitempty"`

	Lon0        float64 `json:"lon0"`
	Lat0        float64 `json:"lat0"`
	Zoom        float64 `json:"zoom"`
	CenterX     float64 `json:"centerX"`
	CenterY     float64 `json:"centerY"`
	RotateSpeed float64 `json:"rotateSpeed"`
	Orbit       bool    `json:"orbit"`
	Graticule   bool    `json:"graticule"`
	// Src is the same-origin URL of the raw lon/lat country rings.
	Src string `json:"src"`
}

// globeGeoSrc is the served copy of the generated country rings; the
// client re-projects it every animation frame.
const globeGeoSrc = "/static/geo/world-geo.json"

// globeSeq feeds per-instance gradient ids.
var globeSeq uint64

// --- Projection -----------------------------------------------------------------

const globeDeg = math.Pi / 180

// globeCam is an orthographic camera: subpoint (Lon0, Lat0), disc
// radius R, disc center (CX, CY) in viewBox coordinates (y down).
type globeCam struct {
	lon0             float64
	sinLat0, cosLat0 float64
	r, cx, cy        float64
}

func newGlobeCam(lon0, lat0, r, cx, cy float64) globeCam {
	s, c := math.Sincos(lat0 * globeDeg)
	return globeCam{lon0: lon0, sinLat0: s, cosLat0: c, r: r, cx: cx, cy: cy}
}

// project maps lon/lat (degrees) to viewBox coordinates. cosc is the
// cosine of the angular distance from the subpoint: the point is on the
// visible hemisphere when cosc > 0.
func (g globeCam) project(lon, lat float64) (x, y, cosc float64) {
	sinDl, cosDl := math.Sincos((lon - g.lon0) * globeDeg)
	sinPhi, cosPhi := math.Sincos(lat * globeDeg)
	cosc = g.sinLat0*sinPhi + g.cosLat0*cosPhi*cosDl
	x = g.cx + g.r*cosPhi*sinDl
	y = g.cy - g.r*(g.cosLat0*sinPhi-g.sinLat0*cosPhi*cosDl)
	return
}

// globePath renders a lon/lat polyline clipped to the visible
// hemisphere: hidden points are dropped and the path restarts on
// re-entry (the geoPathFrom approach, with the hemisphere test in
// place of the window test). closed appends Z only when nothing was
// clipped, so partial rings never draw a false chord across the limb.
func globePath(cam globeCam, pts [][2]float64, closed bool) string {
	var b strings.Builder
	prevX, prevY := math.NaN(), math.NaN()
	inRun := false
	clipped := false
	for _, pt := range pts {
		x, y, cosc := cam.project(pt[0], pt[1])
		if cosc <= 1e-9 {
			inRun = false
			clipped = true
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
	if b.Len() > 0 && closed && !clipped {
		b.WriteString("Z")
	}
	return b.String()
}

// globeGraticule builds the 15° grid clipped to the visible hemisphere:
// meridians pole to pole, parallels -75°..75° around the full circle.
// (Orthographic needs no longitude wrap handling — projection is pure
// trig on the delta.)
func globeGraticule(cam globeCam) []string {
	var out []string
	for lon := -180.0; lon < 180; lon += 15 {
		var pts [][2]float64
		for lat := -90.0; lat <= 90; lat += 2 {
			pts = append(pts, [2]float64{lon, lat})
		}
		if p := globePath(cam, pts, false); p != "" {
			out = append(out, p)
		}
	}
	for lat := -75.0; lat <= 75; lat += 15 {
		var pts [][2]float64
		for lon := -180.0; lon <= 180; lon += 2 {
			pts = append(pts, [2]float64{lon, lat})
		}
		if p := globePath(cam, pts, false); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// --- Layout ---------------------------------------------------------------------

// LayoutGlobe projects one orthographic frame at (Lon0, Lat0).
// Countries entirely on the far side are dropped; countries straddling
// the limb are split into visible runs (small limb artifacts are
// expected at 110m resolution). Region codes are matched
// case-insensitively; unknown codes are ignored.
func LayoutGlobe(d GlobeData) Globe {
	style := strings.ToLower(strings.TrimSpace(d.Style))
	switch style {
	case "themed", "wire":
	default:
		style = "solid"
	}
	zoom := d.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	cxf, cyf := d.CenterX, d.CenterY
	if cxf == 0 {
		cxf = 0.5
	}
	if cyf == 0 {
		cyf = 0.5
	}
	r := 490 * zoom
	cx, cy := cxf*1000, cyf*1000
	cam := newGlobeCam(d.Lon0, d.Lat0, r, cx, cy)

	out := Globe{
		ViewBox: "0 0 1000 1000",
		CX:      F(cx), CY: F(cy), R: F(r),
		Style:   style,
		ShadeID: fmt.Sprintf("globe-shade-%d", atomic.AddUint64(&globeSeq, 1)),
		Animate: d.RotateSpeed > 0 || d.Orbit,
	}

	grat := d.Graticule || style == "wire"
	if grat {
		out.Graticule = globeGraticule(cam)
	}

	// Choropleth values, scaled only over codes the data actually has.
	known := make(map[string]bool, len(geoCountryRings))
	for _, s := range geoCountryRings {
		if s.Code != "" {
			known[s.Code] = true
		}
	}
	values := make(map[string]float64, len(d.Regions))
	minV, maxV := math.Inf(1), math.Inf(-1)
	if style == "themed" {
		for _, rg := range d.Regions {
			code := strings.ToUpper(strings.TrimSpace(rg.Code))
			if !known[code] {
				continue
			}
			values[code] = rg.Value
			minV = math.Min(minV, rg.Value)
			maxV = math.Max(maxV, rg.Value)
		}
	}
	span := maxV - minV
	if span <= 0 || math.IsInf(span, 0) {
		span = 1
	}
	ramp := geoRampFor(d.Ramp, d.Hue)

	for _, s := range geoCountryRings {
		var sb strings.Builder
		for _, ring := range s.Rings {
			sb.WriteString(globePath(cam, ring, true))
		}
		if sb.Len() == 0 {
			continue // entirely on the far side
		}
		shape := GlobeShape{Code: s.Code, Name: s.Name, Path: sb.String()}
		switch style {
		case "wire":
			shape.Fill = "none"
		case "themed":
			v, ok := values[s.Code]
			shape.HasData = ok
			if ok {
				shape.Fill = geoRampColor(ramp, (v-minV)/span)
				shape.Tip = BuildTipHTML(shape.Fill, s.Name,
					[]TipRow{{Label: "value", Value: geoFormatValue(v, d.Unit)}})
			} else {
				shape.Fill = geoNoDataFill
				shape.Tip = BuildTipHTML("", s.Name, nil)
			}
		default: // solid
			shape.Fill = geoNoDataFill
		}
		out.Shapes = append(out.Shapes, shape)
	}

	// Client payload: resolved colors, no Go knowledge needed.
	pl := GlobePayload{
		Style:       style,
		Unit:        d.Unit,
		Lon0:        d.Lon0,
		Lat0:        d.Lat0,
		Zoom:        zoom,
		CenterX:     cxf,
		CenterY:     cyf,
		RotateSpeed: d.RotateSpeed,
		Orbit:       d.Orbit,
		Graticule:   grat,
		Src:         globeGeoSrc,
	}
	if style == "themed" && len(values) > 0 {
		pl.Regions = values
		pl.Min, pl.Max = minV, maxV
		const swatches = 16
		for i := 0; i < swatches; i++ {
			pl.Ramp = append(pl.Ramp, geoRampColor(ramp, float64(i)/(swatches-1)))
		}
	}
	out.Payload = pl
	return out
}
