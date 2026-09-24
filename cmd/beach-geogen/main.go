// beach-geogen generates pkg/chart/geodata_gen.go from the Natural Earth
// 1:110m TopoJSON files embedded under data/. It projects country and US
// state outlines through the Equal Earth projection onto fixed SVG planes
// and emits the resulting path strings plus a city gazetteer kept in raw
// lon/lat (projected at layout time so it can ride any plane).
//
// The world plane is fitted to the full projection extent (not the data
// bbox) so the projection outline and pole lines land inside the viewBox.
// The US map is a traditional composite: the conterminous states on a
// 96°W-centered plane with Alaska and Hawaii re-projected into inset
// windows below.
//
// Run via `make gen-geo` from the repo root. The output is committed, so
// this only needs re-running when the source data or projection changes.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

//go:embed data/world-110m.json data/states-provinces-110m.json data/cities-110m.json
var dataFS embed.FS

// --- TopoJSON (unquantized: arcs are absolute lon/lat, no transform) ---------

type topology struct {
	Objects map[string]topoObject `json:"objects"`
	Arcs    [][][2]float64        `json:"arcs"`
}

type topoObject struct {
	Geometries []topoGeometry `json:"geometries"`
}

type topoGeometry struct {
	Type        string          `json:"type"`
	Arcs        json.RawMessage `json:"arcs"`
	Coordinates [2]float64      `json:"coordinates"`
	Properties  map[string]any  `json:"properties"`
}

// ring assembles one TopoJSON ring (a list of arc indexes, negative =
// reversed complement) into a point list.
func ring(topo *topology, arcIdxs []int) [][2]float64 {
	var pts [][2]float64
	for _, ai := range arcIdxs {
		var arc [][2]float64
		if ai >= 0 {
			arc = topo.Arcs[ai]
		} else {
			src := topo.Arcs[^ai]
			arc = make([][2]float64, len(src))
			for i := range src {
				arc[i] = src[len(src)-1-i]
			}
		}
		if len(pts) > 0 && len(arc) > 0 {
			arc = arc[1:] // consecutive arcs share their join point
		}
		pts = append(pts, arc...)
	}
	return pts
}

// polygons returns the geometry's rings as point lists (Polygon and
// MultiPolygon; anything else is skipped).
func polygons(topo *topology, g topoGeometry) [][][2]float64 {
	var out [][][2]float64
	switch g.Type {
	case "Polygon":
		var arcs [][]int
		if err := json.Unmarshal(g.Arcs, &arcs); err != nil {
			log.Fatalf("polygon arcs: %v", err)
		}
		for _, r := range arcs {
			out = append(out, ring(topo, r))
		}
	case "MultiPolygon":
		var arcs [][][]int
		if err := json.Unmarshal(g.Arcs, &arcs); err != nil {
			log.Fatalf("multipolygon arcs: %v", err)
		}
		for _, poly := range arcs {
			for _, r := range poly {
				out = append(out, ring(topo, r))
			}
		}
	}
	return out
}

// --- Equal Earth projection (Šavrič, Patterson & Jenny 2018) -----------------

const (
	eeA1 = 1.340264
	eeA2 = -0.081106
	eeA3 = 0.000893
	eeA4 = 0.003796
)

// equalEarth maps lon/lat degrees to projection-plane coordinates with the
// central meridian at lon0. Keep in sync with the runtime copy in
// pkg/chart/geomap.go.
func equalEarth(lonDeg, latDeg, lon0 float64) (x, y float64) {
	lon := lonDeg - lon0
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
	x = lam * math.Cos(theta) / (m * (eeA1 + 3*eeA2*t2 + t6*(7*eeA3+9*eeA4*t2)))
	y = theta * (eeA1 + eeA2*t2 + t6*(eeA3+eeA4*t2))
	return x, y
}

// --- Plane fitting ------------------------------------------------------------

// plane scales raw Equal Earth coordinates into a width-wide window with y
// growing downward; Off positions the window in the composite viewBox.
type plane struct {
	Lon0, MinX, MaxY, Scale float64
	OffX, OffY              float64
	W, H                    float64
}

// fitPlane fits the shapes' projected bbox (plus any extra lon/lat points,
// e.g. the projection extremes for a full-globe fit) into a width-wide
// window at offset 0,0.
func fitPlane(lon0 float64, shapes []shapeRings, extra [][2]float64, width, padFrac float64) plane {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	take := func(lon, lat float64) {
		x, y := equalEarth(lon, lat, lon0)
		minX = math.Min(minX, x)
		maxX = math.Max(maxX, x)
		minY = math.Min(minY, y)
		maxY = math.Max(maxY, y)
	}
	for _, s := range shapes {
		for _, r := range s.rings {
			for _, pt := range r {
				take(pt[0], pt[1])
			}
		}
	}
	for _, pt := range extra {
		take(pt[0], pt[1])
	}
	pad := (maxX - minX) * padFrac
	minX -= pad
	maxX += pad
	minY -= pad
	maxY += pad
	scale := width / (maxX - minX)
	return plane{
		Lon0:  lon0,
		MinX:  minX,
		MaxY:  maxY,
		Scale: scale,
		W:     width,
		H:     math.Ceil((maxY - minY) * scale),
	}
}

func (p plane) xy(lon, lat float64) (float64, float64) {
	x, y := equalEarth(lon, lat, p.Lon0)
	return p.OffX + (x-p.MinX)*p.Scale, p.OffY + (p.MaxY-y)*p.Scale
}

// pathFor renders rings into a compact SVG path, dropping consecutive
// points that collapse at 0.1-unit resolution.
func (p plane) pathFor(rings [][][2]float64) string {
	var b strings.Builder
	for _, r := range rings {
		prevX, prevY := math.NaN(), math.NaN()
		started := false
		for _, pt := range r {
			x, y := p.xy(pt[0], pt[1])
			rx, ry := math.Round(x*10)/10, math.Round(y*10)/10
			if rx == prevX && ry == prevY {
				continue
			}
			if !started {
				fmt.Fprintf(&b, "M%.1f %.1f", rx, ry)
				started = true
			} else {
				fmt.Fprintf(&b, "L%.1f %.1f", rx, ry)
			}
			prevX, prevY = rx, ry
		}
		if started {
			b.WriteString("Z")
		}
	}
	return b.String()
}

func fmtPlane(name string, p plane) string {
	return fmt.Sprintf("var %s = geoPlane{Lon0: %v, MinX: %v, MaxY: %v, Scale: %v, OffX: %v, OffY: %v, W: %v, H: %v}\n",
		name, p.Lon0, p.MinX, p.MaxY, p.Scale, p.OffX, p.OffY, p.W, p.H)
}

// --- Property helpers ---------------------------------------------------------

// clean strips the NUL padding shapefile fixed-width strings carry and
// repairs double-encoded UTF-8 ("SÃ£o Paulo" → "São Paulo"), which the
// source TopoJSON inherited from a cp1252 misread of the shapefile.
func clean(v any) string {
	s, _ := v.(string)
	s = fixDoubleUTF8(strings.TrimRight(s, "\x00"))
	return strings.Join(strings.Fields(s), " ") // collapse stray double spaces
}

// cp1252Rev maps the runes Windows-1252 assigns to bytes 0x80–0x9F back to
// those bytes ("œ" → 0x9C). The misread that double-encoded the source data
// went through cp1252, so these runes appear where raw high bytes belong.
var cp1252Rev = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86,
	'‡': 0x87, 'ˆ': 0x88, '‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C,
	'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95,
	'–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B,
	'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// fixDoubleUTF8 collapses one layer of cp1252→UTF-8 double encoding
// ("SÃ£o" → "São", "Ãœ" → "Ü") when doing so is provably reversible: every
// rune must map to a cp1252 byte and the collapsed bytes must themselves be
// valid multi-byte-bearing UTF-8. Correctly encoded text (pure ASCII, or
// genuine accented runes whose byte form is invalid UTF-8) passes through
// untouched.
func fixDoubleUTF8(s string) string {
	hasHigh := false
	b := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r <= 0x7F:
			b = append(b, byte(r))
		case r <= 0xFF:
			hasHigh = true
			b = append(b, byte(r))
		default:
			c, ok := cp1252Rev[r]
			if !ok {
				return s
			}
			hasHigh = true
			b = append(b, c)
		}
	}
	if !hasHigh || !utf8.Valid(b) {
		return s
	}
	return string(b)
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

// --- Datasets -----------------------------------------------------------------

type shapeRings struct {
	code, name string
	rings      [][][2]float64
}

func loadShapes(file, object, codeKey, codeFallbackKey, nameKey string) []shapeRings {
	raw, err := dataFS.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}
	var topo topology
	if err := json.Unmarshal(raw, &topo); err != nil {
		log.Fatalf("%s: %v", file, err)
	}
	var out []shapeRings
	for _, g := range topo.Objects[object].Geometries {
		code := clean(g.Properties[codeKey])
		if (code == "" || code == "-99") && codeFallbackKey != "" {
			code = clean(g.Properties[codeFallbackKey])
		}
		if code == "-99" {
			code = "" // unresolvable (disputed territories); still drawn as landmass
		}
		out = append(out, shapeRings{
			code:  code,
			name:  clean(g.Properties[nameKey]),
			rings: polygons(&topo, g),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].code != out[j].code {
			return out[i].code < out[j].code
		}
		return out[i].name < out[j].name
	})
	return out
}

type city struct {
	name, country string
	lon, lat      float64
	rank, pop     int
	capital       bool
}

func loadCities() []city {
	raw, err := dataFS.ReadFile("data/cities-110m.json")
	if err != nil {
		log.Fatal(err)
	}
	var topo topology
	if err := json.Unmarshal(raw, &topo); err != nil {
		log.Fatalf("cities: %v", err)
	}
	var out []city
	for _, g := range topo.Objects["cities"].Geometries {
		if g.Type != "Point" {
			continue
		}
		out = append(out, city{
			name:    clean(g.Properties["NAME"]),
			country: clean(g.Properties["ADM0NAME"]),
			lon:     g.Coordinates[0],
			lat:     g.Coordinates[1],
			rank:    int(num(g.Properties["SCALERANK"])),
			pop:     int(num(g.Properties["POP_MAX"])),
			capital: strings.Contains(strings.ToLower(clean(g.Properties["FEATURECLA"])), "capital"),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		return out[i].name < out[j].name
	})
	return out
}

// --- Emit ----------------------------------------------------------------------

// US composite layout constants: the conterminous plane spans the full
// 1000-unit width; Alaska and Hawaii sit in inset windows in a strip below,
// each with a framePad frame around its window.
const (
	usWidth   = 1000.0
	akWidth   = 300.0
	hiWidth   = 170.0
	insetGapY = 14.0 // strip padding above/below the windows
	insetGapX = 22.0 // gap between the two windows
	insetOffX = 16.0 // left edge of the first window
	framePad  = 8.0
)

// quantRings rounds ring coordinates to 2 decimals (~1.1 km, at the 110m
// data's own precision) and drops consecutive duplicates, for the raw
// lon/lat artifacts consumed by the 3D globe renderers.
func quantRings(rings [][][2]float64) [][][2]float64 {
	out := make([][][2]float64, 0, len(rings))
	for _, r := range rings {
		q := make([][2]float64, 0, len(r))
		for _, pt := range r {
			p := [2]float64{math.Round(pt[0]*100) / 100, math.Round(pt[1]*100) / 100}
			if n := len(q); n > 0 && q[n-1] == p {
				continue
			}
			q = append(q, p)
		}
		if len(q) >= 3 {
			out = append(out, q)
		}
	}
	return out
}

func main() {
	out := flag.String("out", "pkg/chart/geodata_gen.go", "output file")
	ringsOut := flag.String("rings-out", "pkg/chart/geodata_rings_gen.go", "raw-rings output file")
	jsonOut := flag.String("json-out", "pkg/beach/view/static/geo/world-geo.json", "client geo JSON output file")
	flag.Parse()

	countries := loadShapes("data/world-110m.json", "countries", "ISO_A2_EH", "ISO_A2", "ADMIN")
	states := loadShapes("data/states-provinces-110m.json", "states", "iso_3166_2", "", "name")
	cities := loadCities()

	// World: fit to the projection extremes (x widest at the equatorial
	// antimeridian, y at the poles), not the data bbox, so the outline and
	// pole lines land inside the viewBox.
	worldExtent := [][2]float64{
		{180 - 1e-6, 0}, {-180 + 1e-6, 0}, {0, 90}, {0, -90},
	}
	worldPlane := fitPlane(0, countries, worldExtent, usWidth, 0.005)

	// US composite: conterminous / Alaska / Hawaii on separate planes.
	var conus, alaska, hawaii []shapeRings
	for _, s := range states {
		switch s.code {
		case "US-AK":
			alaska = append(alaska, s)
		case "US-HI":
			hawaii = append(hawaii, s)
		default:
			conus = append(conus, s)
		}
	}
	if len(alaska) == 0 || len(hawaii) == 0 {
		log.Fatal("US-AK / US-HI missing from states data")
	}
	conusPlane := fitPlane(-96, conus, nil, usWidth, 0.015)
	akPlane := fitPlane(-152, alaska, nil, akWidth, 0.03)
	hiPlane := fitPlane(-157, hawaii, nil, hiWidth, 0.05)

	akPlane.OffX, akPlane.OffY = insetOffX, conusPlane.H+insetGapY
	hiPlane.OffX, hiPlane.OffY = insetOffX+akWidth+insetGapX+2*framePad, conusPlane.H+insetGapY
	usViewH := math.Ceil(conusPlane.H + insetGapY + math.Max(akPlane.H, hiPlane.H) + insetGapY)

	frames := []plane{akPlane, hiPlane}

	planeFor := func(code string) plane {
		switch code {
		case "US-AK":
			return akPlane
		case "US-HI":
			return hiPlane
		default:
			return conusPlane
		}
	}

	var b strings.Builder
	b.WriteString("// Code generated by beach-geogen; DO NOT EDIT.\n")
	b.WriteString("// Source: Natural Earth 1:110m cultural vectors (public domain),\n")
	b.WriteString("// cmd/beach-geogen/data/. Regenerate with `make gen-geo`.\n\n")
	b.WriteString("package chart\n\n")

	b.WriteString(fmtPlane("geoWorldPlane", worldPlane))
	b.WriteString(fmtPlane("geoUSConusPlane", conusPlane))
	b.WriteString(fmtPlane("geoUSAlaskaPlane", akPlane))
	b.WriteString(fmtPlane("geoUSHawaiiPlane", hiPlane))
	fmt.Fprintf(&b, "\nconst (\n\tgeoUSViewW float64 = %v\n\tgeoUSViewH float64 = %v\n)\n\n", usWidth, usViewH)

	b.WriteString("var geoUSInsetFrames = []geoRect{\n")
	for _, f := range frames {
		fmt.Fprintf(&b, "\t{X: %v, Y: %v, W: %v, H: %v},\n",
			f.OffX-framePad, f.OffY-framePad, f.W+2*framePad, f.H+2*framePad)
	}
	b.WriteString("}\n\n")

	b.WriteString("var geoCountries = []geoShapeSrc{\n")
	for _, s := range countries {
		fmt.Fprintf(&b, "\t{Code: %q, Name: %q, Path: %q},\n", s.code, s.name, worldPlane.pathFor(s.rings))
	}
	b.WriteString("}\n\n")

	b.WriteString("var geoUSStates = []geoShapeSrc{\n")
	for _, s := range states {
		fmt.Fprintf(&b, "\t{Code: %q, Name: %q, Path: %q},\n", s.code, s.name, planeFor(s.code).pathFor(s.rings))
	}
	b.WriteString("}\n\n")

	b.WriteString("var geoCities = []geoCitySrc{\n")
	for _, c := range cities {
		fmt.Fprintf(&b, "\t{Name: %q, Country: %q, Lon: %v, Lat: %v, Rank: %d, Pop: %d, Capital: %v},\n",
			c.name, c.country, c.lon, c.lat, c.rank, c.pop, c.capital)
	}
	b.WriteString("}\n")

	src, err := format.Source([]byte(b.String()))
	if err != nil {
		log.Fatalf("gofmt: %v", err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		log.Fatal(err)
	}

	// Raw lon/lat rings, quantized, in two forms: a Go literal for
	// server-side 3D rendering (pkg/chart stays dependency-free) and a
	// compact JSON asset for the client-side globe renderer.
	var rb strings.Builder
	rb.WriteString("// Code generated by beach-geogen; DO NOT EDIT.\n")
	rb.WriteString("// Raw lon/lat country rings (Natural Earth 1:110m, public domain),\n")
	rb.WriteString("// quantized to 2 decimals, for 3D/orthographic rendering.\n\n")
	rb.WriteString("package chart\n\n")
	rb.WriteString("var geoCountryRings = []geoRingsSrc{\n")

	type jsonCountry struct {
		Code  string         `json:"c"`
		Name  string         `json:"n"`
		Rings [][][2]float64 `json:"r"`
	}
	jc := make([]jsonCountry, 0, len(countries))

	var ringCount, ptCount int
	for _, s := range countries {
		q := quantRings(s.rings)
		jc = append(jc, jsonCountry{Code: s.code, Name: s.name, Rings: q})
		fmt.Fprintf(&rb, "\t{Code: %q, Name: %q, Rings: [][][2]float64{\n", s.code, s.name)
		for _, r := range q {
			ringCount++
			rb.WriteString("\t\t{")
			for _, pt := range r {
				ptCount++
				fmt.Fprintf(&rb, "{%v, %v},", pt[0], pt[1])
			}
			rb.WriteString("},\n")
		}
		rb.WriteString("\t}},\n")
	}
	rb.WriteString("}\n")

	rsrc, err := format.Source([]byte(rb.String()))
	if err != nil {
		log.Fatalf("gofmt rings: %v", err)
	}
	if err := os.WriteFile(*ringsOut, rsrc, 0o644); err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(*jsonOut), 0o755); err != nil {
		log.Fatal(err)
	}
	jbytes, err := json.Marshal(map[string]any{"countries": jc})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*jsonOut, jbytes, 0o644); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("wrote %s: %d countries, %d US states, %d cities (world %gx%g, us %gx%g)\n",
		*out, len(countries), len(states), len(cities), worldPlane.W, worldPlane.H, usWidth, usViewH)
	fmt.Printf("wrote %s + %s: %d rings, %d points, json %dKB\n",
		*ringsOut, *jsonOut, ringCount, ptCount, len(jbytes)/1024)
}
