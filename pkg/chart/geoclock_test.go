package chart

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Fixed instants keep every geoclock test deterministic.
var (
	geoClockJuneSolstice = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	geoClockDecSolstice  = time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)
	geoClockMarEquinox   = time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
)

func TestSolarDeclination(t *testing.T) {
	if d := solarDeclinationDeg(geoClockJuneSolstice); d <= 20 {
		t.Errorf("June solstice declination = %.2f, want > 20", d)
	}
	if d := solarDeclinationDeg(geoClockDecSolstice); d >= -20 {
		t.Errorf("December solstice declination = %.2f, want < -20", d)
	}
	if d := solarDeclinationDeg(geoClockMarEquinox); math.Abs(d) >= 3 {
		t.Errorf("March equinox declination = %.2f, want |d| < 3", d)
	}
}

func TestSubsolarLongitude(t *testing.T) {
	if lon := subsolarLonDeg(geoClockJuneSolstice); math.Abs(lon) >= 5 {
		t.Errorf("12:00 UTC subsolar lon = %.2f, want |lon| < 5", lon)
	}
	at18 := time.Date(2026, 6, 21, 18, 0, 0, 0, time.UTC)
	if lon := subsolarLonDeg(at18); math.Abs(lon-(-90)) > 0.01 {
		t.Errorf("18:00 UTC subsolar lon = %.2f, want -90", lon)
	}
	// Non-UTC input is normalized internally: 13:00 UTC+1 == 12:00 UTC.
	cet := time.Date(2026, 6, 21, 13, 0, 0, 0, time.FixedZone("CET", 3600))
	if lon := subsolarLonDeg(cet); math.Abs(lon) >= 5 {
		t.Errorf("13:00 CET subsolar lon = %.2f, want |lon| < 5", lon)
	}
}

func TestSunAltitudePolarFlags(t *testing.T) {
	cases := []struct {
		name    string
		at      time.Time
		lat     float64
		wantDay bool
	}{
		{"june 80N day", geoClockJuneSolstice, 80, true},
		{"june 80S night", geoClockJuneSolstice, -80, false},
		{"december 80N night", geoClockDecSolstice, 80, false},
		{"december 80S day", geoClockDecSolstice, -80, true},
	}
	for _, c := range cases {
		decl := solarDeclinationDeg(c.at)
		ss := subsolarLonDeg(c.at)
		// Any longitude: |lat| = 80 with |decl| > 20 puts the whole
		// parallel on one side of the terminator.
		for _, lon := range []float64{-120, 0, 90} {
			alt := sunAltitudeDeg(lon, c.lat, decl, ss)
			if (alt >= 0) != c.wantDay {
				t.Errorf("%s: altitude at lon %.0f = %.2f, want day=%v",
					c.name, lon, alt, c.wantDay)
			}
		}
	}
}

func TestEquinoxPolesConsistentWithClampedDecl(t *testing.T) {
	decl := solarDeclinationDeg(geoClockMarEquinox)
	declC := clampDeclDeg(decl)
	if math.Abs(declC) < geoClockMinDecl {
		t.Fatalf("clamped decl %.3f under the %.1f floor", declC, geoClockMinDecl)
	}
	ss := subsolarLonDeg(geoClockMarEquinox)
	darkPole := geoClockDarkPoleLat(declC)
	litPole := -darkPole
	if alt := sunAltitudeDeg(0, darkPole*0.999, declC, ss); alt >= 0 {
		t.Errorf("dark pole (lat %.0f) altitude = %.2f, want < 0", darkPole, alt)
	}
	if alt := sunAltitudeDeg(0, litPole*0.999, declC, ss); alt < 0 {
		t.Errorf("lit pole (lat %.0f) altitude = %.2f, want >= 0", litPole, alt)
	}
}

func TestTerminatorLatitude(t *testing.T) {
	// The terminator formula must sit on the altitude-zero contour.
	decl := clampDeclDeg(solarDeclinationDeg(geoClockJuneSolstice))
	ss := subsolarLonDeg(geoClockJuneSolstice)
	for lon := -180.0; lon <= 180; lon += 15 {
		lat := terminatorLatDeg(lon, decl, ss)
		if alt := sunAltitudeDeg(lon, lat, decl, ss); math.Abs(alt) > 0.05 {
			t.Errorf("altitude on terminator at lon %.0f = %.3f, want ~0", lon, alt)
		}
	}
}

func TestEquinoxTerminatorNearlyMeridional(t *testing.T) {
	decl := clampDeclDeg(solarDeclinationDeg(geoClockMarEquinox))
	ss := subsolarLonDeg(geoClockMarEquinox)
	// On the noon and midnight meridians the terminator hugs the poles...
	if lat := terminatorLatDeg(ss, decl, ss); math.Abs(lat) < 85 {
		t.Errorf("terminator at noon meridian = %.2f, want |lat| > 85", lat)
	}
	if lat := terminatorLatDeg(ss+180, decl, ss); math.Abs(lat) < 85 {
		t.Errorf("terminator at midnight meridian = %.2f, want |lat| > 85", lat)
	}
	// ...and flips sign across the dawn meridian (Δλ = 90): the curve is a
	// near-meridian pair, i.e. the day/night split follows longitude.
	before := terminatorLatDeg(ss+88, decl, ss)
	after := terminatorLatDeg(ss+92, decl, ss)
	if before*after >= 0 {
		t.Errorf("terminator did not flip across dawn meridian: %.2f vs %.2f", before, after)
	}
}

func TestTwilightIsolineBranch(t *testing.T) {
	// June solstice, midnight meridian: the -6° isoline must sit ~6° on the
	// night side of the terminator (toward the dark south pole for δ > 0),
	// and the altitude there must actually be -6°.
	decl := clampDeclDeg(solarDeclinationDeg(geoClockJuneSolstice))
	ss := subsolarLonDeg(geoClockJuneSolstice)
	midnight := ss + 180
	lat0 := terminatorLatDeg(midnight, decl, ss)
	lat6 := altitudeIsolineLatDeg(midnight, -6, decl, ss)
	if lat6 >= lat0 {
		t.Errorf("-6 isoline (%.2f) not on the dark-pole side of terminator (%.2f)", lat6, lat0)
	}
	if alt := sunAltitudeDeg(midnight, lat6, decl, ss); math.Abs(alt-(-6)) > 0.05 {
		t.Errorf("altitude on -6 isoline = %.3f, want -6", alt)
	}
	// December: dark pole is north, so the isoline sits above the terminator.
	declDec := clampDeclDeg(solarDeclinationDeg(geoClockDecSolstice))
	ssDec := subsolarLonDeg(geoClockDecSolstice)
	midnightDec := ssDec + 180
	if l0, l6 := terminatorLatDeg(midnightDec, declDec, ssDec),
		altitudeIsolineLatDeg(midnightDec, -6, declDec, ssDec); l6 <= l0 {
		t.Errorf("december -6 isoline (%.2f) not north of terminator (%.2f)", l6, l0)
	}
	// Near-equinox dawn meridian: the meridian never reaches -6°, so the
	// isoline clamps to the dark pole.
	if lat := altitudeIsolineLatDeg(90, -6, -geoClockMinDecl, 0); lat != 90 {
		t.Errorf("no-solution isoline = %.2f, want clamp to dark pole 90", lat)
	}
}

func TestLayoutGeoClockCityFlags(t *testing.T) {
	sydneyTZ := time.FixedZone("AEST", 10*3600)
	out := LayoutGeoClock(GeoClockData{
		Time: geoClockJuneSolstice,
		Cities: []GeoClockCity{
			{Name: "London", Lon: -0.13, Lat: 51.51, TZ: time.UTC},
			{Name: "Sydney", Lon: 151.21, Lat: -33.87, TZ: sydneyTZ},
			{Name: "Nowhere", Lon: 10, Lat: 10}, // no TZ: dot only
		},
	})
	if len(out.Cities) != 3 {
		t.Fatalf("got %d cities, want 3", len(out.Cities))
	}
	london, sydney, nowhere := out.Cities[0], out.Cities[1], out.Cities[2]
	if !london.Day {
		t.Errorf("London at 12:00 UTC in June should be daylight")
	}
	if london.TimeLabel != "12:00" {
		t.Errorf("London time label = %q, want 12:00", london.TimeLabel)
	}
	if sydney.Day {
		t.Errorf("Sydney at 22:00 local in June should be night")
	}
	if sydney.TimeLabel != "22:00" {
		t.Errorf("Sydney time label = %q, want 22:00", sydney.TimeLabel)
	}
	if nowhere.TimeLabel != "" {
		t.Errorf("city without TZ got time label %q, want empty", nowhere.TimeLabel)
	}
	if !strings.Contains(sydney.Tip, "night") {
		t.Errorf("Sydney tip %q should mention night", sydney.Tip)
	}
}

func TestLayoutGeoClockShowCities(t *testing.T) {
	out := LayoutGeoClock(GeoClockData{Time: geoClockJuneSolstice, ShowCities: true})
	if len(out.Cities) == 0 {
		t.Fatal("ShowCities produced no gazetteer cities")
	}
	for _, c := range out.Cities {
		if c.TimeLabel != "" {
			t.Errorf("gazetteer city %s has a time label %q, want none", c.Label, c.TimeLabel)
		}
	}
}

// geoClockPathCoords extracts every numeric token from an SVG path string.
var geoClockNumRe = regexp.MustCompile(`-?\d+(?:\.\d+)?`)

func geoClockPathCoords(t *testing.T, path string) []float64 {
	t.Helper()
	var out []float64
	for _, m := range geoClockNumRe.FindAllString(path, -1) {
		v, err := strconv.ParseFloat(m, 64)
		if err != nil {
			t.Fatalf("bad number %q in path", m)
		}
		out = append(out, v)
	}
	return out
}

func geoClockCheckPathInViewBox(t *testing.T, name, path, viewBox string) {
	t.Helper()
	vb := geoClockPathCoords(t, viewBox)
	if len(vb) != 4 {
		t.Fatalf("viewBox %q did not parse", viewBox)
	}
	w, h := vb[2], vb[3]
	coords := geoClockPathCoords(t, path)
	if len(coords) == 0 || len(coords)%2 != 0 {
		t.Fatalf("%s: %d coords, want a non-empty even count", name, len(coords))
	}
	const tol = 0.15 // path emission rounds to 0.1 units
	for i := 0; i < len(coords); i += 2 {
		x, y := coords[i], coords[i+1]
		if x < -tol || x > w+tol || y < -tol || y > h+tol {
			t.Fatalf("%s: point (%.1f, %.1f) outside viewBox 0 0 %.0f %.0f", name, x, y, w, h)
		}
	}
}

func TestLayoutGeoClockPaths(t *testing.T) {
	for _, at := range []time.Time{geoClockJuneSolstice, geoClockDecSolstice, geoClockMarEquinox} {
		out := LayoutGeoClock(GeoClockData{Time: at, ShowTwilight: true})
		if out.NightPath == "" {
			t.Fatalf("%s: empty night path", at)
		}
		if !strings.HasSuffix(out.NightPath, "Z") {
			t.Errorf("%s: night path not closed: ...%s", at, out.NightPath[len(out.NightPath)-8:])
		}
		geoClockCheckPathInViewBox(t, "night", out.NightPath, out.ViewBox)
		if out.TwilightPath == "" || !strings.HasSuffix(out.TwilightPath, "Z") {
			t.Errorf("%s: twilight path empty or unclosed", at)
		}
		geoClockCheckPathInViewBox(t, "twilight", out.TwilightPath, out.ViewBox)
		if out.TerminatorPath == "" {
			t.Errorf("%s: empty terminator path", at)
		}
		if strings.HasSuffix(out.TerminatorPath, "Z") {
			t.Errorf("%s: terminator path should be open", at)
		}
		geoClockCheckPathInViewBox(t, "terminator", out.TerminatorPath, out.ViewBox)
		// Sun glyph within the viewBox.
		sx, _ := strconv.ParseFloat(out.SunCX, 64)
		sy, _ := strconv.ParseFloat(out.SunCY, 64)
		if sx < 0 || sx > 1000 || sy < 0 || sy > 492 {
			t.Errorf("%s: sun at (%s, %s) outside plane", at, out.SunCX, out.SunCY)
		}
	}
	// Twilight off by default.
	if out := LayoutGeoClock(GeoClockData{Time: geoClockJuneSolstice}); out.TwilightPath != "" {
		t.Error("TwilightPath set without ShowTwilight")
	}
}

func TestLayoutGeoClockLabels(t *testing.T) {
	out := LayoutGeoClock(GeoClockData{Time: time.Date(2026, 7, 13, 14, 32, 0, 0, time.UTC)})
	if out.UTCLabel != "14:32 UTC · Jul 13 2026" {
		t.Errorf("UTCLabel = %q", out.UTCLabel)
	}
	if !strings.HasPrefix(out.SubsolarLabel, "sun over ") ||
		!strings.Contains(out.SubsolarLabel, "°N") ||
		!strings.Contains(out.SubsolarLabel, "°W") {
		t.Errorf("SubsolarLabel = %q, want July northern-declination west-afternoon form", out.SubsolarLabel)
	}
}

func TestGeoClockRefreshAction(t *testing.T) {
	action := "@get('/api/clock')"
	out := LayoutGeoClock(GeoClockData{Time: geoClockJuneSolstice, RefreshAction: action})
	if out.RefreshAction != action {
		t.Errorf("RefreshAction = %q, want %q", out.RefreshAction, action)
	}
	attrs := geoClockRefreshAttrs(out)
	if len(attrs) != 1 || attrs[0].Name != "data-on-interval__duration.60s" || attrs[0].Val != action {
		t.Errorf("refresh attrs = %#v, want one data-on-interval__duration.60s", attrs)
	}
	if attrs := geoClockRefreshAttrs(GeoClock{}); len(attrs) != 0 {
		t.Errorf("empty RefreshAction produced attrs %#v", attrs)
	}

	// The fragment markup carries (or omits) the interval attribute.
	var b strings.Builder
	if err := ChartGeoClockFragment("clk", out).Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(b.String(), "data-on-interval__duration.60s") {
		t.Error("fragment missing data-on-interval attribute")
	}
	var b2 strings.Builder
	still := LayoutGeoClock(GeoClockData{Time: geoClockJuneSolstice})
	if err := ChartGeoClockFragment("clk", still).Render(context.Background(), &b2); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(b2.String(), "data-on-interval") {
		t.Error("fragment without RefreshAction still has an interval attribute")
	}
}

func TestLayoutGeoClockBasemapDefault(t *testing.T) {
	out := LayoutGeoClock(GeoClockData{Time: geoClockJuneSolstice})
	if out.Map.Outline == "" {
		t.Error("default basemap missing outline")
	}
	if len(out.Map.MajorMeridians) == 0 {
		t.Error("default basemap missing 30-degree major meridians")
	}
	if out.Map.Equator == "" {
		t.Error("default basemap missing equator")
	}
	if len(out.Map.Meridians) != 0 || len(out.Map.Parallels) != 0 {
		t.Error("quiet default should not carry minor graticule lines")
	}
	if len(out.Map.Shapes) == 0 {
		t.Fatal("no landmass shapes")
	}
	for _, s := range out.Map.Shapes {
		if s.HasData {
			t.Fatal("sun clock landmass should be all no-data neutral fills")
		}
	}
	// Full override plumbs through.
	quiet := LayoutGeoClock(GeoClockData{Time: geoClockJuneSolstice, Basemap: &GeoBasemap{}})
	if quiet.Map.Outline != "" || quiet.Map.Equator != "" {
		t.Error("zero-value basemap override should disable every layer")
	}
}
