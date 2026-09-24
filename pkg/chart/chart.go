// Package chart computes SVG geometry for server-rendered charts.
// All layout functions accept data types and return positioned SVG
// geometry structs. The rendering (templ markup) lives in
// internal/view/component; this package is pure geometry with no
// templ or web dependency.
package chart

import (
	"fmt"
	"strconv"
)

// Pct formats a 0-100 value as an SVG percentage string.
func Pct(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64) + "%"
}

// F formats a float for viewBox-based SVG coordinates.
func F(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func MinF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func MaxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// CommaInt formats an integer with thousands separators.
func CommaInt(n int) string {
	s := strconv.Itoa(n)
	neg := false
	if n < 0 {
		neg = true
		s = s[1:]
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b = append(b, ',')
		}
		b = append(b, s[i])
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func HtmlEsc(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b = append(b, []byte("&amp;")...)
		case '<':
			b = append(b, []byte("&lt;")...)
		case '>':
			b = append(b, []byte("&gt;")...)
		case '"':
			b = append(b, []byte("&quot;")...)
		default:
			b = append(b, s[i])
		}
	}
	return string(b)
}

// ColorVar returns the CSS custom property for the i-th series color,
// cycling the 15-color palette.
func ColorVar(i int) string {
	return fmt.Sprintf("var(--color-series-%c)", 'a'+(i%15))
}

// GaugeRamp returns n OKLCH color strings graduated from dim (index 0)
// to bold (index n-1). All share the same hue.
func GaugeRamp(n int, hue float64) []string {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []string{fmt.Sprintf("oklch(0.65 0.16 %.0f)", hue)}
	}
	out := make([]string, n)
	for i := range out {
		t := float64(i) / float64(n-1)
		l := 0.30 + t*0.40
		c := 0.04 + t*0.14
		out[i] = fmt.Sprintf("oklch(%.2f %.2f %.0f)", l, c, hue)
	}
	return out
}

// TipRow is one label-value pair in a chart tooltip.
type TipRow struct {
	Label string
	Value string
}

// BuildTipHTML builds styled tooltip HTML for the data-tip attribute.
func BuildTipHTML(colorVar, title string, rows []TipRow) string {
	dot := ""
	if colorVar != "" {
		dot = `<span class="chart-tooltip-dot" style="background:` + colorVar + `"></span>`
	}
	html := `<div class="chart-tooltip-head">` + dot + `<span>` + HtmlEsc(title) + `</span></div>`
	if len(rows) > 0 {
		html += `<div class="chart-tooltip-rows">`
		for _, r := range rows {
			html += `<div class="chart-tooltip-row">` +
				`<span class="chart-tooltip-label">` + HtmlEsc(r.Label) + `</span>` +
				`<span class="chart-tooltip-value">` + HtmlEsc(r.Value) + `</span></div>`
		}
		html += `</div>`
	}
	return html
}

// Horizontal bar layout constants.
const (
	HBarLabelGutter = 28.0
	HBarValueGutter = 11.0
	HBarPlotX       = HBarLabelGutter + 1.0
	HBarTrackW      = 100.0 - HBarPlotX - HBarValueGutter
	HBarBarInset    = 0.16
	MaxLabelLen     = 22
)

// LegendItem is a labeled color swatch used in chart legends.
type LegendItem struct {
	Label string
	Color string
}

// AxisTick is a positioned label on a chart axis.
type AxisTick struct {
	Pos   float64
	Label string
}

// Itoa converts an int to string (used in templ data attributes).
func Itoa(i int) string {
	return strconv.Itoa(i)
}
