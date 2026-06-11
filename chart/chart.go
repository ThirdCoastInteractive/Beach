// Package chart renders server-side SVG charts in pure Go.
//
// Every chart is final HTML, drawn on the server before it reaches the browser.
// The API is two steps: a Layout* function computes geometry from data, and a
// render function (Render, or the per-type SVG funcs) emits the SVG to an
// io.Writer.
//
//	layout := chart.LayoutHBar(chart.HBarInput{Series: rows, Unit: " kW"})
//	chart.HBarSVG(w, layout)
//
// The library obeys the framework's laws by construction:
//
//   - Token-themed: series colors cycle the design system's
//     --chart-qualitative-1..12; diverging scales use --chart-diverging-*. No
//     hardcoded colors appear in chart code.
//   - Accessible: role="img" on roots with a <title>; color is never the only
//     encoding the caller is asked to rely on.
//   - Responsive without scripts: a viewBox plus preserveAspectRatio lets a
//     chart fill any container while stroke widths stay constant.
//
// Rendering is plain text/template + direct io.Writer; there is no templ or
// client JavaScript involved.
package chart

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Box is a rectangular region in SVG user units.
type Box struct {
	X, Y, W, H float64
}

// Inset returns the box shrunk by the given padding on every side.
func (b Box) Inset(pad float64) Box {
	return Box{X: b.X + pad, Y: b.Y + pad, W: b.W - 2*pad, H: b.H - 2*pad}
}

// Right is the box's right edge (X + W).
func (b Box) Right() float64 { return b.X + b.W }

// Bottom is the box's bottom edge (Y + H).
func (b Box) Bottom() float64 { return b.Y + b.H }

// QualitativeVar returns the CSS variable reference for the i-th qualitative
// series color, cycling through --chart-qualitative-1..12 (1-indexed in the
// token names). The index may be any non-negative integer; it wraps.
func QualitativeVar(i int) string {
	n := (i % 12)
	if n < 0 {
		n += 12
	}
	return fmt.Sprintf("var(--chart-qualitative-%d)", n+1)
}

// DivergingVar returns the CSS variable reference for the i-th diverging color,
// cycling through --chart-diverging-1..N. count is the number of stops in the
// scale (at least 1).
func DivergingVar(i, count int) string {
	if count < 1 {
		count = 1
	}
	n := i % count
	if n < 0 {
		n += count
	}
	return fmt.Sprintf("var(--chart-diverging-%d)", n+1)
}

// niceTicks returns up to roughly count "nice" tick values spanning [min,max].
// It is used by axis-bearing charts to place gridlines on round numbers.
func niceTicks(min, max float64, count int) []float64 {
	if count < 2 {
		count = 2
	}
	if max <= min {
		return []float64{min}
	}
	span := niceNum(max-min, false)
	step := niceNum(span/float64(count-1), true)
	lo := math.Floor(min/step) * step
	hi := math.Ceil(max/step) * step
	var out []float64
	for v := lo; v <= hi+step*0.5; v += step {
		out = append(out, v)
	}
	return out
}

func niceNum(x float64, round bool) float64 {
	if x == 0 {
		return 0
	}
	exp := math.Floor(math.Log10(x))
	frac := x / math.Pow(10, exp)
	var nf float64
	if round {
		switch {
		case frac < 1.5:
			nf = 1
		case frac < 3:
			nf = 2
		case frac < 7:
			nf = 5
		default:
			nf = 10
		}
	} else {
		switch {
		case frac <= 1:
			nf = 1
		case frac <= 2:
			nf = 2
		case frac <= 5:
			nf = 5
		default:
			nf = 10
		}
	}
	return nf * math.Pow(10, exp)
}

// fnum formats a float for SVG output: trims trailing zeros, caps precision at
// 3 decimals, and renders -0 as 0. Keeps golden files stable across platforms.
func fnum(v float64) string {
	if v == 0 || (v > -0.0005 && v < 0.0005) {
		return "0"
	}
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// esc escapes text for inclusion in SVG/XML body or attribute values.
func esc(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

// svg is a small fluent builder over an io.Writer for emitting SVG. It records
// the first write error and is a no-op afterward, so callers can write a whole
// chart then check Err once.
type svg struct {
	w   io.Writer
	err error
}

func newSVG(w io.Writer) *svg { return &svg{w: w} }

func (s *svg) Err() error { return s.err }

func (s *svg) p(parts ...string) {
	if s.err != nil {
		return
	}
	for _, p := range parts {
		if _, err := io.WriteString(s.w, p); err != nil {
			s.err = err
			return
		}
	}
}

// open writes the root <svg> element with a viewBox and accessibility metadata.
func (s *svg) open(w, h float64, title string) {
	s.p(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 `,
		fnum(w), " ", fnum(h),
		`" preserveAspectRatio="xMidYMid meet" role="img" class="chart"`)
	if title != "" {
		s.p(` aria-label="`, esc(title), `">`)
		s.p(`<title>`, esc(title), `</title>`)
	} else {
		s.p(`>`)
	}
}

func (s *svg) close() { s.p(`</svg>`) }

// rect writes a <rect>. fill is a CSS color/variable string; extra is appended
// raw (e.g. `rx="2"`).
func (s *svg) rect(b Box, fill, extra string) {
	s.p(`<rect x="`, fnum(b.X), `" y="`, fnum(b.Y),
		`" width="`, fnum(b.W), `" height="`, fnum(b.H),
		`" fill="`, fill, `"`)
	if extra != "" {
		s.p(" ", extra)
	}
	s.p(`/>`)
}

func (s *svg) line(x1, y1, x2, y2 float64, stroke, extra string) {
	s.p(`<line x1="`, fnum(x1), `" y1="`, fnum(y1),
		`" x2="`, fnum(x2), `" y2="`, fnum(y2),
		`" stroke="`, stroke, `"`)
	if extra != "" {
		s.p(" ", extra)
	}
	s.p(`/>`)
}

func (s *svg) circle(cx, cy, r float64, fill, extra string) {
	s.p(`<circle cx="`, fnum(cx), `" cy="`, fnum(cy),
		`" r="`, fnum(r), `" fill="`, fill, `"`)
	if extra != "" {
		s.p(" ", extra)
	}
	s.p(`/>`)
}

func (s *svg) path(d, fill, stroke, extra string) {
	s.p(`<path d="`, d, `" fill="`, fill, `"`)
	if stroke != "" {
		s.p(` stroke="`, stroke, `"`)
	}
	if extra != "" {
		s.p(" ", extra)
	}
	s.p(`/>`)
}

// text writes a <text> element. anchor is start|middle|end (empty = default).
func (s *svg) text(x, y float64, anchor, cls, body string) {
	s.p(`<text x="`, fnum(x), `" y="`, fnum(y), `"`)
	if anchor != "" {
		s.p(` text-anchor="`, anchor, `"`)
	}
	if cls != "" {
		s.p(` class="`, cls, `"`)
	}
	s.p(`>`, esc(body), `</text>`)
}

// title writes a child <title> for an element group (tooltip/accessible name).
func (s *svg) title(body string) {
	s.p(`<title>`, esc(body), `</title>`)
}

// polyPath builds an SVG path "d" from points; if close, the path is closed.
func polyPath(pts [][2]float64, closed bool) string {
	if len(pts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("M")
	for i, p := range pts {
		if i == 1 {
			b.WriteString("L")
		}
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(fnum(p[0]))
		b.WriteString(",")
		b.WriteString(fnum(p[1]))
	}
	if closed {
		b.WriteString("Z")
	}
	return b.String()
}
