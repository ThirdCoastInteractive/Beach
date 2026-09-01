// Sun clock: a world map on the Equal Earth plane showing the day/night
// terminator — the classic "daylight map". A translucent night polygon (and
// optional civil-twilight band) is drawn OVER the neutral landmass so the
// geography stays readable underneath, a sun glyph marks the subsolar point,
// and a header row carries the UTC clock + subsolar position readout.
//
// Solar math is the low-precision visualization model, accurate to well under
// a degree of map ink:
//
//   - Declination δ ≈ −23.44° · cos(2π·(N+10)/365.25), N = day of year.
//     Within ~±0.3° of the true value; the seasonal shape is exact enough for
//     a terminator you read at continental scale.
//   - Subsolar longitude λ_ss ≈ −15°·(h−12), h = UTC decimal hours. This
//     ignores the equation of time, so it can be off by up to ~±4° (≈±16
//     minutes of sun travel) around early November / mid February. Fine for
//     a viz; do not use it for prayer times or solar-panel math.
//   - Sun altitude at (λ, φ): sin(alt) = sinφ·sinδ + cosφ·cosδ·cos(λ−λ_ss).
//     Exact given δ and λ_ss above (no refraction: the horizon is geometric,
//     so "day" flips ~0.83° later than visual sunrise).
//   - Terminator latitude per longitude: φ_t(λ) = atan(−cos(λ−λ_ss)/tan δ),
//     the alt = 0 contour of the formula above. |δ| is clamped to ≥ 0.5°
//     near the equinoxes so tan δ never explodes; the clamped terminator is
//     a near-meridian curve, which is what the true one looks like anyway.
package chart

import (
	"fmt"
	"math"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/datastar"
)

// --- Input types ------------------------------------------------------------

// GeoClockCity is a caller-picked city to mark on the sun clock.
type GeoClockCity struct {
	Name     string
	Lon, Lat float64
	// TZ, when set, adds a local-time label ("14:32") to the marker and the
	// tooltip. Nil renders a day/night dot with no local time.
	TZ *time.Location
}

// GeoClockData describes a sun-clock map: the instant to render, the cities
// to mark, and the usual basemap/refresh knobs.
type GeoClockData struct {
	// Time is the instant the map shows. Required; converted to UTC
	// internally, so any zone is fine.
	Time time.Time
	// Cities are caller-picked markers.
	Cities []GeoClockCity
	// ShowCities adds the gazetteer megacities (Natural Earth rank ≤ 1)
	// automatically, deduped by name against Cities. Gazetteer cities carry
	// no timezone, so they render as day/night dots only.
	ShowCities bool
	// ShowTwilight draws the civil-twilight band (sun altitude 0 to −6°)
	// between daylight and full night.
	ShowTwilight bool
	// Basemap overrides the graticule/outline layers. Nil takes a quiet
	// default: outline + 30° major meridians + equator only.
	Basemap *GeoBasemap
	// RefreshAction is an optional Datastar action expression (built with
	// the pkg/datastar helpers, e.g. datastar.Get("/api/clock")). When set,
	// the fragment self-refreshes every 60s via data-on-interval. Empty
	// disables auto-refresh.
	RefreshAction string
}

// --- Output geometry --------------------------------------------------------

// GeoClockCityOut is one projected city marker.
type GeoClockCityOut struct {
	CX, CY string
	Label  string
	// TimeLabel is the local wall-clock time ("14:32"), or "" when the city
	// has no known timezone.
	TimeLabel string
	// Day is true when the sun is above the geometric horizon there.
	Day bool
	Tip string
}

// GeoClock is the precomputed sun-clock geometry consumed by GeoClockSVG.
type GeoClock struct {
	ViewBox string
	// Map is the neutral basemap + landmass, built through the same code
	// path as LayoutGeoMap with no regions (every country renders as
	// no-data landmass).
	Map GeoMap
	// NightPath is the closed night polygon (terminator + dark-pole edge),
	// drawn translucent OVER the landmass. TwilightPath is the closed civil
	// twilight band (alt 0 to −6°), "" unless ShowTwilight. TerminatorPath
	// is the open terminator curve alone, for the edge stroke.
	NightPath      string
	TwilightPath   string
	TerminatorPath string
	// SunCX/SunCY position the sun glyph at the subsolar point.
	SunCX, SunCY string
	// UTCLabel is the header clock ("14:32 UTC · Jul 13 2026");
	// SubsolarLabel is the sun position ("sun over 12.3°N 141.0°W").
	UTCLabel      string
	SubsolarLabel string
	Cities        []GeoClockCityOut
	// RefreshAction mirrors GeoClockData.RefreshAction; the templ layer
	// turns it into a data-on-interval attribute on the fragment figure.
	RefreshAction string
}

// --- Solar math ---------------------------------------------------------------

// geoClockMinDecl is the declination clamp (degrees). Near the equinoxes
// tan δ → 0 and the terminator formula degenerates; clamping |δ| here keeps
// the curve numerically sane while still reading as a near-meridian line.
const geoClockMinDecl = 0.5

// solarDeclinationDeg approximates the sun's declination (degrees) at t.
// δ ≈ −23.44° · cos(2π·(N+10)/365.25); accurate to about ±0.3°.
func solarDeclinationDeg(t time.Time) float64 {
	n := float64(t.UTC().YearDay())
	return -23.44 * math.Cos(2*math.Pi*(n+10)/365.25)
}

// subsolarLonDeg approximates the subsolar longitude (degrees) at t:
// −15°·(h−12) with h the UTC decimal hour. Ignores the equation of time
// (up to ~±4°, ≈±16 minutes). Result is normalized to [−180, 180].
func subsolarLonDeg(t time.Time) float64 {
	u := t.UTC()
	h := float64(u.Hour()) + float64(u.Minute())/60 + float64(u.Second())/3600
	lon := -15 * (h - 12)
	for lon > 180 {
		lon -= 360
	}
	for lon < -180 {
		lon += 360
	}
	return lon
}

// clampDeclDeg clamps |δ| to ≥ geoClockMinDecl, preserving sign (an exact
// zero counts as northern spring: south stays the lit pole of a +δ).
func clampDeclDeg(declDeg float64) float64 {
	if declDeg >= 0 {
		return math.Max(declDeg, geoClockMinDecl)
	}
	return math.Min(declDeg, -geoClockMinDecl)
}

// sunAltitudeDeg is the geometric solar altitude (degrees) at lon/lat given
// declination δ and subsolar longitude λ_ss (all degrees):
// sin(alt) = sinφ·sinδ + cosφ·cosδ·cos(λ−λ_ss).
func sunAltitudeDeg(lonDeg, latDeg, declDeg, ssLonDeg float64) float64 {
	phi := latDeg * math.Pi / 180
	dec := declDeg * math.Pi / 180
	dl := (lonDeg - ssLonDeg) * math.Pi / 180
	s := math.Sin(phi)*math.Sin(dec) + math.Cos(phi)*math.Cos(dec)*math.Cos(dl)
	return math.Asin(math.Max(-1, math.Min(1, s))) * 180 / math.Pi
}

// terminatorLatDeg is the latitude (degrees) where the sun altitude is zero
// at longitude lonDeg: φ_t(λ) = atan(−cos(λ−λ_ss)/tan δ). Callers pass a
// clamped δ (clampDeclDeg) so tan δ is never ~0.
func terminatorLatDeg(lonDeg, declDeg, ssLonDeg float64) float64 {
	dl := (lonDeg - ssLonDeg) * math.Pi / 180
	dec := declDeg * math.Pi / 180
	return math.Atan(-math.Cos(dl)/math.Tan(dec)) * 180 / math.Pi
}

// altitudeIsolineLatDeg solves the latitude (degrees) where the sun altitude
// equals altDeg at longitude lonDeg — the isoline branch adjacent to the
// terminator on the night side. Writing the altitude equation as
// A·sinφ + B·cosφ = sin(alt) with A = sinδ, B = cosδ·cos(λ−λ_ss), the
// solutions are φ = asin(sin(alt)/R) − atan2(B, A) and its supplement
// (R = √(A²+B²)); exactly one lands in [−90°, 90°], and that is the one
// returned (checked against known cases in geoclock_test.go). When
// |sin(alt)| > R the meridian never gets that dark — the whole night side is
// brighter than altDeg — and the isoline is clamped to the dark pole.
func altitudeIsolineLatDeg(lonDeg, altDeg, declDeg, ssLonDeg float64) float64 {
	dec := declDeg * math.Pi / 180
	dl := (lonDeg - ssLonDeg) * math.Pi / 180
	a := math.Sin(dec)
	b := math.Cos(dec) * math.Cos(dl)
	r := math.Hypot(a, b)
	s := math.Sin(altDeg * math.Pi / 180)
	if math.Abs(s) > r {
		return geoClockDarkPoleLat(declDeg)
	}
	asin := math.Asin(s / r)
	psi := math.Atan2(b, a)
	for _, phi := range []float64{asin - psi, math.Pi - asin - psi} {
		// Normalize into (−π, π], then keep the in-range branch.
		for phi > math.Pi {
			phi -= 2 * math.Pi
		}
		for phi <= -math.Pi {
			phi += 2 * math.Pi
		}
		if phi >= -math.Pi/2 && phi <= math.Pi/2 {
			return phi * 180 / math.Pi
		}
	}
	// Unreachable for |s| ≤ r; fall back to the dark pole.
	return geoClockDarkPoleLat(declDeg)
}

// geoClockDarkPoleLat is the latitude of the pole in polar night: south when
// the sun is north of the equator, north otherwise.
func geoClockDarkPoleLat(declDeg float64) float64 {
	if declDeg >= 0 {
		return -90
	}
	return 90
}

// --- Polygon assembly ---------------------------------------------------------

// geoClockLonStep is the terminator sampling step (degrees of longitude).
const geoClockLonStep = 2.0

// geoClockClampLon keeps sampled longitudes strictly inside the projection
// seam (matching geoOutline's ±(180−ε) edges) so edge points project to the
// intended side of the map.
func geoClockClampLon(lon float64) float64 {
	const eps = 1e-6
	return math.Max(-180+eps, math.Min(180-eps, lon))
}

// geoClockLatRun appends a latitude run at fixed longitude, from lat "from"
// to lat "to" in ~2° steps, endpoints included. Used to walk the curved
// antimeridian edges of the projection.
func geoClockLatRun(pts [][2]float64, lon, from, to float64) [][2]float64 {
	step := 2.0
	if to < from {
		step = -2.0
	}
	for lat := from; (to-lat)*step > 0; lat += step {
		pts = append(pts, [2]float64{lon, lat})
	}
	return append(pts, [2]float64{lon, to})
}

// geoClockTerminatorPts samples the terminator west → east across the full
// longitude range on the clamped declination.
func geoClockTerminatorPts(declDeg, ssLonDeg float64) [][2]float64 {
	var pts [][2]float64
	for lon := -180.0; lon <= 180.0+1e-9; lon += geoClockLonStep {
		l := geoClockClampLon(lon)
		pts = append(pts, [2]float64{l, terminatorLatDeg(l, declDeg, ssLonDeg)})
	}
	return pts
}

// geoClockNightPath builds the closed night polygon: the terminator west →
// east, down the east antimeridian edge to the dark pole, back along the
// pole line, and up the west edge — all in lon/lat, projected by
// geoPathFrom. Near the equinoxes the clamped terminator hugs two meridians
// and the polygon degenerates gracefully into a near-hemisphere lune.
func geoClockNightPath(p geoPlane, declDeg, ssLonDeg float64) string {
	pole := geoClockDarkPoleLat(declDeg)
	east, west := geoClockClampLon(180), geoClockClampLon(-180)
	pts := geoClockTerminatorPts(declDeg, ssLonDeg)
	edgeLat := terminatorLatDeg(east, declDeg, ssLonDeg)
	pts = geoClockLatRun(pts, east, edgeLat, pole)
	for lon := 180.0; lon >= -180.0-1e-9; lon -= geoClockLonStep {
		pts = append(pts, [2]float64{geoClockClampLon(lon), pole})
	}
	pts = geoClockLatRun(pts, west, pole, edgeLat)
	return geoPathFrom(p, pts, true, false)
}

// geoClockTwilightPath builds the closed civil-twilight band: the region
// between the terminator (alt 0) and the −6° isoline. Where a meridian never
// reaches −6° the isoline clamps to the dark pole, widening the band to the
// map edge — which is what actually happens near the equinox poles.
func geoClockTwilightPath(p geoPlane, declDeg, ssLonDeg float64) string {
	east, west := geoClockClampLon(180), geoClockClampLon(-180)
	pts := geoClockTerminatorPts(declDeg, ssLonDeg)
	t0 := terminatorLatDeg(east, declDeg, ssLonDeg)
	t6 := altitudeIsolineLatDeg(east, -6, declDeg, ssLonDeg)
	pts = geoClockLatRun(pts, east, t0, t6)
	for lon := 180.0; lon >= -180.0-1e-9; lon -= geoClockLonStep {
		l := geoClockClampLon(lon)
		pts = append(pts, [2]float64{l, altitudeIsolineLatDeg(l, -6, declDeg, ssLonDeg)})
	}
	pts = geoClockLatRun(pts, west, altitudeIsolineLatDeg(west, -6, declDeg, ssLonDeg), t0)
	return geoPathFrom(p, pts, true, false)
}

// --- Labels ---------------------------------------------------------------------

// geoClockUTCLabel formats the header clock, e.g. "14:32 UTC · Jul 13 2026".
func geoClockUTCLabel(t time.Time) string {
	u := t.UTC()
	return u.Format("15:04") + " UTC · " + u.Format("Jan 2 2006")
}

// geoClockSubsolarLabel formats the subsolar readout,
// e.g. "sun over 12.3°N 141.0°W".
func geoClockSubsolarLabel(declDeg, ssLonDeg float64) string {
	ns := "N"
	if declDeg < 0 {
		ns = "S"
	}
	ew := "E"
	if ssLonDeg < 0 {
		ew = "W"
	}
	return fmt.Sprintf("sun over %.1f°%s %.1f°%s",
		math.Abs(declDeg), ns, math.Abs(ssLonDeg), ew)
}

// geoClockRefreshAttrs is the Datastar plumbing for the fragment figure:
// a 60s data-on-interval re-running RefreshAction, or nothing when unset.
// Kept as a Go helper so the templ layer never hand-writes data-* names.
func geoClockRefreshAttrs(c GeoClock) datastar.Attrs {
	if c.RefreshAction == "" {
		return nil
	}
	return datastar.Attrs{datastar.OnInterval("60s", c.RefreshAction)}
}

// --- Layout ---------------------------------------------------------------------

// LayoutGeoClock projects the sun-clock geometry for d.Time onto the Equal
// Earth world plane. The basemap + neutral landmass ride the LayoutGeoMap
// code path with no regions; the night/twilight overlays, sun glyph, and
// city markers are computed here.
func LayoutGeoClock(d GeoClockData) GeoClock {
	t := d.Time.UTC()
	decl := solarDeclinationDeg(t)
	ssLon := subsolarLonDeg(t)
	declC := clampDeclDeg(decl)
	p := geoWorldPlane

	bm := GeoBasemap{Outline: true, MajorMeridianStep: 30, Equator: true}
	if d.Basemap != nil {
		bm = *d.Basemap
	}
	base := LayoutGeoMap(GeoMapData{Level: GeoLevelWorld, Basemap: &bm})

	out := GeoClock{
		ViewBox:       base.ViewBox,
		Map:           base,
		NightPath:     geoClockNightPath(p, declC, ssLon),
		UTCLabel:      geoClockUTCLabel(t),
		SubsolarLabel: geoClockSubsolarLabel(decl, ssLon),
		RefreshAction: d.RefreshAction,
	}
	out.TerminatorPath = geoPathFrom(p, geoClockTerminatorPts(declC, ssLon), false, false)
	if d.ShowTwilight {
		out.TwilightPath = geoClockTwilightPath(p, declC, ssLon)
	}

	sunX, sunY := p.xy(ssLon, decl)
	out.SunCX, out.SunCY = F(sunX), F(sunY)

	addCity := func(name string, lon, lat float64, tz *time.Location) {
		x, y := p.xy(lon, lat)
		alt := sunAltitudeDeg(lon, lat, decl, ssLon)
		day := alt >= 0
		status := "night"
		if day {
			status = "daylight"
		}
		timeLabel := ""
		rows := []TipRow{}
		if tz != nil {
			timeLabel = t.In(tz).Format("15:04")
			rows = append(rows, TipRow{Label: "local time", Value: timeLabel})
		}
		rows = append(rows, TipRow{Label: "sun", Value: status})
		out.Cities = append(out.Cities, GeoClockCityOut{
			CX:        F(x),
			CY:        F(y),
			Label:     name,
			TimeLabel: timeLabel,
			Day:       day,
			Tip:       BuildTipHTML("", name, rows),
		})
	}

	seen := make(map[string]bool, len(d.Cities))
	for _, c := range d.Cities {
		seen[c.Name] = true
		addCity(c.Name, c.Lon, c.Lat, c.TZ)
	}
	if d.ShowCities {
		for _, c := range geoCities {
			if c.Rank > 1 || seen[c.Name] {
				continue
			}
			addCity(c.Name, c.Lon, c.Lat, nil)
		}
	}

	return out
}
