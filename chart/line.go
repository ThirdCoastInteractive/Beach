package chart

import (
	"math"
	"strconv"
	"strings"
)

// --- Input types ------------------------------------------------------------

type LineSeriesData struct {
	Series          []LineSeries `json:"series"`
	XLabel          string       `json:"xLabel,omitempty"`
	YLabel          string       `json:"yLabel,omitempty"`
	ShowArea        bool         `json:"showArea,omitempty"`
	MovingAvgWindow int          `json:"movingAvgWindow,omitempty"`
}

type LineSeries struct {
	Label  string      `json:"label"`
	Color  string      `json:"color,omitempty"`
	Points []LinePoint `json:"points"`
}

type LinePoint struct {
	X string  `json:"x"`
	Y float64 `json:"y"`
	T float64 `json:"t,omitempty"`
}

type SparklineData struct {
	Values     []float64 `json:"values"`
	TrendValue string    `json:"trendValue,omitempty"`
	TrendDir   string    `json:"trendDir,omitempty"`
}

// --- Output geometry --------------------------------------------------------

type LineChart struct {
	HoverID                                     string
	PlotLeftPct, PlotTopPct, PlotWPct, PlotHPct float64
	PlotRightPct, PlotBottomPct                 float64
	GridYs                                      []float64
	YTicks                                      []AxisTick
	XTicks                                      []AxisTick
	Series                                      []LineChartSeries
	Dots                                        []LineDot
	ShowArea                                    bool
	MultiSeries                                 bool
	LegendItems                                 []LegendItem
	Hover                                       LineHoverData
	HasRef                                      bool
	RefY                                        float64
	RefLabel                                    string
	Footnote                                    string
}

type LineChartSeries struct {
	Color   string
	PathD   string
	AreaD   string
	MAPathD string
}

type LineDot struct {
	XPct, YPct float64
	Color      string
	Series     int
}

type lineHoverPoint struct {
	FX float64 `json:"fx"`
	VY float64 `json:"vy"`
	V  float64 `json:"v"`
	T  float64 `json:"t"`
}

type lineHoverSeries struct {
	Label string           `json:"label"`
	Color string           `json:"color"`
	Pts   []lineHoverPoint `json:"pts"`
}

type LineHoverData struct {
	Unit     string            `json:"unit"`
	RateUnit string            `json:"rateUnit"`
	XLabels  []string          `json:"xlabels"`
	TMin     float64           `json:"tmin"`
	TMax     float64           `json:"tmax"`
	Series   []lineHoverSeries `json:"series"`
}

type LineOpts struct {
	RefValue float64
	RefLabel string
	Footnote string
	RateUnit string
}

type Sparkline struct {
	Points    string
	AreaD     string
	Value     string
	Label     string
	Tip       string
	Color     string
	MinY      string
	MaxY      string
	LastDotCX string
	LastDotCY string
}

// --- Helpers ----------------------------------------------------------------

const (
	lineGutterLeft   = 9.0
	lineGutterRight  = 3.0
	lineGutterTop    = 8.0
	lineGutterBottom = 14.0
)

func FmtVal(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}

func NiceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(v))
	base := math.Pow(10, exp)
	f := v / base
	switch {
	case f <= 1:
		return base
	case f <= 2:
		return 2 * base
	case f <= 2.5:
		return 2.5 * base
	case f <= 5:
		return 5 * base
	default:
		return 10 * base
	}
}

// --- Layout functions -------------------------------------------------------

func LayoutLine(hoverID string, data LineSeriesData, opts ...LineOpts) LineChart {
	var opt LineOpts
	if len(opts) > 0 {
		opt = opts[0]
	}
	out := LineChart{
		HoverID:     hoverID,
		PlotLeftPct: lineGutterLeft,
		PlotTopPct:  lineGutterTop,
		PlotWPct:    100 - lineGutterLeft - lineGutterRight,
		PlotHPct:    100 - lineGutterTop - lineGutterBottom,
		ShowArea:    data.ShowArea,
		MultiSeries: len(data.Series) > 1,
		Footnote:    opt.Footnote,
	}
	out.PlotRightPct = out.PlotLeftPct + out.PlotWPct
	out.PlotBottomPct = out.PlotTopPct + out.PlotHPct

	if len(data.Series) == 0 {
		return out
	}

	maxV := opt.RefValue
	n := 0
	for _, s := range data.Series {
		if len(s.Points) > n {
			n = len(s.Points)
		}
		for _, p := range s.Points {
			maxV = MaxF(maxV, p.Y)
		}
	}
	if n == 0 {
		return out
	}
	yMax := NiceCeil(maxV * 1.02)

	first := data.Series[0].Points
	out.Hover.Unit = data.YLabel
	out.Hover.RateUnit = opt.RateUnit
	out.Hover.XLabels = make([]string, len(first))
	tof := make([]float64, len(first))
	anyT := false
	for i, p := range first {
		out.Hover.XLabels[i] = p.X
		if p.T != 0 {
			anyT = true
		}
	}
	for i := range first {
		if anyT {
			tof[i] = first[i].T
		} else {
			tof[i] = float64(i)
		}
	}
	if len(tof) > 0 {
		out.Hover.TMin = tof[0]
		out.Hover.TMax = tof[len(tof)-1]
	}

	if opt.RefValue > 0 {
		out.HasRef = true
		out.RefY = out.PlotTopPct + (1-opt.RefValue/yMax)*out.PlotHPct
		out.RefLabel = opt.RefLabel
	}

	fx := func(i int) float64 {
		if n <= 1 {
			return 0.5
		}
		return float64(i) / float64(n-1)
	}

	for k := 0; k <= 4; k++ {
		frac := float64(k) / 4
		val := yMax * frac
		yPct := out.PlotTopPct + (1-frac)*out.PlotHPct
		out.GridYs = append(out.GridYs, yPct)
		out.YTicks = append(out.YTicks, AxisTick{Pos: yPct, Label: FmtVal(val)})
	}

	ticks := 7
	if n < ticks {
		ticks = n
	}
	for k := 0; k < ticks; k++ {
		idx := 0
		if ticks > 1 {
			idx = int(math.Round(float64(k) * float64(n-1) / float64(ticks-1)))
		}
		xPct := out.PlotLeftPct + fx(idx)*out.PlotWPct
		label := ""
		if idx < len(first) {
			label = first[idx].X
		}
		out.XTicks = append(out.XTicks, AxisTick{Pos: xPct, Label: label})
	}

	for si, s := range data.Series {
		color := s.Color
		if color == "" {
			color = ColorVar(si)
		}
		var line strings.Builder
		var hpts []lineHoverPoint
		for i, p := range s.Points {
			x := fx(i) * 1000
			vy := p.Y / yMax
			y := (1 - vy) * 1000
			if i == 0 {
				line.WriteString("M ")
			} else {
				line.WriteString(" L ")
			}
			line.WriteString(strconv.FormatFloat(x, 'f', 1, 64))
			line.WriteByte(' ')
			line.WriteString(strconv.FormatFloat(y, 'f', 1, 64))
			hpts = append(hpts, lineHoverPoint{FX: fx(i), VY: vy, V: p.Y, T: tof[i]})

			if n <= 31 {
				out.Dots = append(out.Dots, LineDot{
					XPct:   out.PlotLeftPct + fx(i)*out.PlotWPct,
					YPct:   out.PlotTopPct + (1-vy)*out.PlotHPct,
					Color:  color,
					Series: si,
				})
			}
		}

		ser := LineChartSeries{Color: color, PathD: line.String()}

		if data.ShowArea {
			var area strings.Builder
			area.WriteString(line.String())
			lastX := fx(len(s.Points)-1) * 1000
			firstX := fx(0) * 1000
			area.WriteString(" L ")
			area.WriteString(strconv.FormatFloat(lastX, 'f', 1, 64))
			area.WriteString(" 1000 L ")
			area.WriteString(strconv.FormatFloat(firstX, 'f', 1, 64))
			area.WriteString(" 1000 Z")
			ser.AreaD = area.String()
		}

		if w := data.MovingAvgWindow; w > 1 && len(s.Points) >= w {
			var ma strings.Builder
			cnt := 0
			for i := w - 1; i < len(s.Points); i++ {
				var sum float64
				for j := i - w + 1; j <= i; j++ {
					sum += s.Points[j].Y
				}
				avg := sum / float64(w)
				x := fx(i) * 1000
				y := (1 - avg/yMax) * 1000
				if cnt == 0 {
					ma.WriteString("M ")
				} else {
					ma.WriteString(" L ")
				}
				ma.WriteString(strconv.FormatFloat(x, 'f', 1, 64))
				ma.WriteByte(' ')
				ma.WriteString(strconv.FormatFloat(y, 'f', 1, 64))
				cnt++
			}
			ser.MAPathD = ma.String()
		}

		out.Series = append(out.Series, ser)
		out.Hover.Series = append(out.Hover.Series, lineHoverSeries{
			Label: s.Label, Color: color, Pts: hpts,
		})
		out.LegendItems = append(out.LegendItems, LegendItem{Label: s.Label, Color: color})
	}

	return out
}

func LayoutSparkline(data SparklineData) Sparkline {
	n := len(data.Values)
	if n == 0 {
		return Sparkline{Color: ColorVar(0)}
	}
	minV, maxV := data.Values[0], data.Values[0]
	for _, v := range data.Values {
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

	const (
		plotTop    = 15.0
		plotBottom = 75.0
		plotLeft   = 5.0
		plotRight  = 95.0
	)
	plotH := plotBottom - plotTop
	plotW := plotRight - plotLeft

	var pts, areaD string
	var lastX, lastY float64
	for i, v := range data.Values {
		t := float64(i) / float64(n-1)
		x := plotLeft + t*plotW
		y := plotBottom - ((v-minV)/span)*plotH
		if i > 0 {
			pts += " "
		}
		pts += strconv.FormatFloat(x, 'f', 1, 64) + "," + strconv.FormatFloat(y, 'f', 1, 64)
		if i == 0 {
			areaD = "M " + F(x) + " " + F(plotBottom) + " L " + F(x) + " " + F(y)
		} else {
			areaD += " L " + F(x) + " " + F(y)
		}
		lastX = x
		lastY = y
	}
	areaD += " L " + F(lastX) + " " + F(plotBottom) + " Z"

	color := ColorVar(0)
	tipVal := data.TrendValue
	if tipVal == "" && n > 0 {
		tipVal = strconv.FormatFloat(data.Values[n-1], 'f', 0, 64)
	}

	return Sparkline{
		Points: pts, AreaD: areaD,
		Value: data.TrendValue, Label: data.TrendDir,
		Tip: BuildTipHTML(color, tipVal, nil), Color: color,
		MinY: Pct(plotBottom), MaxY: Pct(plotTop),
		LastDotCX: F(lastX), LastDotCY: F(lastY),
	}
}
