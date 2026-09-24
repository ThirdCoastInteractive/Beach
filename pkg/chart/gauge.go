package chart

import (
	"fmt"
	"math"
)

// --- Input types ------------------------------------------------------------

type GaugeData struct {
	Value      float64 `json:"value"`
	Max        float64 `json:"max"`
	Unit       string  `json:"unit,omitempty"`
	Label      string  `json:"label,omitempty"`
	TrendValue string  `json:"trendValue,omitempty"`
	TrendDir   string  `json:"trendDir,omitempty"`
}

// --- Output geometry --------------------------------------------------------

type Gauge struct {
	Value    string
	Pct      float64
	Subtitle string
	Footer   string
	Tip      string
}

type StackedGauge struct {
	Value  string
	Footer string
	Tiers  []StackedTier
	Tip    string
}

type StackedTier struct {
	Label    string
	ValueStr string
	Pct      float64
	Color    string
}

type Billboard struct {
	Value      string
	Label      string
	Subtitle   string
	Tip        string
	SparklineD string
	TrendValue string
	TrendDir   string
	TrendColor string
}

func LayoutBillboard(b Billboard) Billboard {
	if len(b.SparklineD) == 0 && b.TrendDir != "" {
		switch b.TrendDir {
		case "up":
			b.TrendColor = "var(--color-good)"
		case "down":
			b.TrendColor = "var(--color-bad)"
		default:
			b.TrendColor = "var(--color-fg-muted)"
		}
	}
	return b
}

func BillboardSparklinePath(values []float64) string {
	n := len(values)
	if n < 2 {
		return ""
	}
	minV, maxV := values[0], values[0]
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	span := maxV - minV
	if span == 0 {
		span = 1
	}

	area := ""
	for i, v := range values {
		t := float64(i) / float64(n-1)
		x := t * 100
		y := 100 - ((v-minV)/span)*60
		if i == 0 {
			area = fmt.Sprintf("M 0 100 L %.1f %.1f", x, y)
		} else {
			area += fmt.Sprintf(" L %.1f %.1f", x, y)
		}
	}
	area += " L 100 100 Z"
	return area
}

// --- Gauge arc helpers (used by templ) --------------------------------------

type GaugeArc struct {
	PathD string
	Dash  string
	Color string
}

func GaugeDash(pct float64) string {
	const arcLen = 293.2
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := arcLen * pct
	return fmt.Sprintf("%.1f %.1f", filled, arcLen)
}

func StackedGaugeArcs(tiers []StackedTier) (string, []GaugeArc) {
	const (
		cx = 100.0
		cy = 100.0
		r  = 65.0
	)
	sx := cx + r*math.Cos(210.0*math.Pi/180.0)
	sy := cy - r*math.Sin(210.0*math.Pi/180.0)
	ex := cx + r*math.Cos(-30.0*math.Pi/180.0)
	ey := cy - r*math.Sin(-30.0*math.Pi/180.0)
	pathD := fmt.Sprintf("M %.1f %.1f A %.1f %.1f 0 1 1 %.1f %.1f", sx, sy, r, r, ex, ey)
	arcLen := r * 240.0 * math.Pi / 180.0

	out := make([]GaugeArc, len(tiers))
	for i, t := range tiers {
		p := t.Pct
		if p < 0 {
			p = 0
		}
		if p > 1 {
			p = 1
		}
		out[i] = GaugeArc{
			PathD: pathD,
			Dash:  fmt.Sprintf("%.1f %.1f", arcLen*p, arcLen),
			Color: t.Color,
		}
	}
	return pathD, out
}

func StackedGaugeLegendY(i, n int) string {
	base := 68.0
	step := 8.0
	return Pct(base + float64(i)*step)
}
