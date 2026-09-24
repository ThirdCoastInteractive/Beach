package chart

import "fmt"

// --- Input data types -------------------------------------------------------

type HBarSeries struct {
	Series []HBarEntry `json:"series"`
	Unit   string      `json:"unit,omitempty"`
	Max    int         `json:"max,omitempty"`
}

type HBarEntry struct {
	Label string `json:"label"`
	Value int    `json:"value"`
	Max   int    `json:"max,omitempty"`
}

type StackedBarSeries struct {
	Series []StackedBarRow `json:"series"`
	Unit   string          `json:"unit,omitempty"`
	Max    int             `json:"max,omitempty"`
}

type StackedBarRow struct {
	Label    string              `json:"label"`
	Segments []StackedBarSegment `json:"segments"`
}

type StackedBarSegment struct {
	Label string `json:"label"`
	Value int    `json:"value"`
	Color string `json:"color,omitempty"`
}

type BulletSeries struct {
	Series []BulletRow `json:"series"`
	Unit   string      `json:"unit,omitempty"`
	Max    int         `json:"max,omitempty"`
}

type BulletRow struct {
	Label    string          `json:"label"`
	Measures []BulletMeasure `json:"measures"`
}

type BulletMeasure struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

type GroupedBarData struct {
	Rows     []GroupedBarRow
	Measures []string
	Unit     string
	Max      int
}

type GroupedBarRow struct {
	Label  string
	Values []int
}

// --- Output geometry types ---------------------------------------------------

type Bar struct {
	Label, ValueText, Title, Tip, Color string
	BarY, BarH, FillW, CenterY          string
}

type BarChart struct {
	LabelX, PlotX, TrackW string
	Bars                  []Bar
}

type StackedBar struct {
	LabelX, PlotX, TrackW string
	Rows                  []StackedRow
	Legend                []LegendItem
}

type StackedRow struct {
	Label, Total, Tip   string
	BarY, BarH, CenterY string
	Segs                []StackedSeg
}

type StackedSeg struct {
	X, W, Color string
}

type BulletBar struct {
	LabelX, PlotX, TrackW string
	Rows                  []BulletBarRow
	Legend                []LegendItem
}

type BulletBarRow struct {
	Label, Total, Tip   string
	BarY, BarH, CenterY string
	Bars                []BulletSeg
}

type BulletSeg struct {
	X, W, Color string
}

type GroupedBar struct {
	LabelX, PlotX, TrackW string
	Rows                  []GroupedRow
	Legend                []LegendItem
}

type GroupedRow struct {
	Label, Tip          string
	BarY, BarH, CenterY string
	Bars                []GroupedSeg
}

type GroupedSeg struct {
	X, Y, W, H, Color string
}

// --- Layout functions -------------------------------------------------------

func LayoutHBar(data HBarSeries) BarChart {
	out := BarChart{
		LabelX: Pct(HBarLabelGutter - 2),
		PlotX:  Pct(HBarPlotX),
		TrackW: Pct(HBarTrackW),
	}
	n := len(data.Series)
	if n == 0 {
		return out
	}
	gmax := float64(data.Max)
	if gmax == 0 {
		for _, e := range data.Series {
			m := float64(e.Max)
			if m == 0 {
				m = float64(e.Value)
			}
			gmax = MaxF(gmax, m)
		}
	}
	if gmax == 0 {
		gmax = 1
	}
	band := 100.0 / float64(n)
	barTopOff := band * HBarBarInset
	barH := band * (1 - 2*HBarBarInset)

	for i, e := range data.Series {
		bandTop := float64(i) * band
		v := float64(e.Value)
		bmax := float64(e.Max)
		if bmax == 0 {
			bmax = gmax
		}
		p := 0.0
		if bmax > 0 {
			p = v / bmax
		}
		frac := v / gmax
		if frac > 1 {
			frac = 1
		}
		color := ColorVar(i)
		if p > 0.85 {
			color = "var(--color-warn)"
		}
		valueText := CommaInt(e.Value) + data.Unit
		title := e.Label + ": " + valueText
		if e.Max > 0 {
			title = fmt.Sprintf("%s: %s of %s (%d%%)", e.Label, valueText, CommaInt(e.Max)+data.Unit, int(p*100+0.5))
		}
		tipRows := []TipRow{{Label: "count", Value: valueText}}
		if e.Max > 0 && e.Max != e.Value {
			tipRows = append(tipRows,
				TipRow{Label: "capacity", Value: CommaInt(e.Max) + data.Unit},
				TipRow{Label: "utilization", Value: fmt.Sprintf("%d%%", int(p*100+0.5))},
			)
		}
		displayLabel := e.Label
		if len(displayLabel) > MaxLabelLen {
			displayLabel = displayLabel[:MaxLabelLen-1] + "…"
		}
		out.Bars = append(out.Bars, Bar{
			Label: displayLabel, ValueText: valueText, Title: title,
			Tip: BuildTipHTML(color, e.Label, tipRows), Color: color,
			BarY: Pct(bandTop + barTopOff), BarH: Pct(barH),
			FillW: Pct(HBarTrackW * frac), CenterY: Pct(bandTop + band/2),
		})
	}
	return out
}

func LayoutStackedBar(data StackedBarSeries) StackedBar {
	out := StackedBar{
		LabelX: Pct(HBarLabelGutter - 2),
		PlotX:  Pct(HBarPlotX),
		TrackW: Pct(HBarTrackW),
	}
	n := len(data.Series)
	if n == 0 {
		return out
	}
	gmax := float64(data.Max)
	if gmax == 0 {
		for _, row := range data.Series {
			var total float64
			for _, seg := range row.Segments {
				total += float64(seg.Value)
			}
			gmax = MaxF(gmax, total)
		}
	}
	if gmax == 0 {
		gmax = 1
	}
	band := 100.0 / float64(n)
	barTopOff := band * HBarBarInset
	barH := band * (1 - 2*HBarBarInset)
	legendSeen := make(map[string]bool)

	for i, row := range data.Series {
		bandTop := float64(i) * band
		var total int
		for _, seg := range row.Segments {
			total += seg.Value
		}
		displayLabel := row.Label
		if len(displayLabel) > MaxLabelLen {
			displayLabel = displayLabel[:MaxLabelLen-1] + "…"
		}
		tipRows := make([]TipRow, 0, len(row.Segments))
		for _, seg := range row.Segments {
			tipRows = append(tipRows, TipRow{Label: seg.Label, Value: CommaInt(seg.Value) + data.Unit})
		}
		tipRows = append(tipRows, TipRow{Label: "total", Value: CommaInt(total) + data.Unit})

		sr := StackedRow{
			Label: displayLabel, Total: CommaInt(total) + data.Unit,
			Tip:  BuildTipHTML("", row.Label, tipRows),
			BarY: Pct(bandTop + barTopOff), BarH: Pct(barH), CenterY: Pct(bandTop + band/2),
		}
		var offset float64
		for si, seg := range row.Segments {
			segW := (float64(seg.Value) / gmax) * HBarTrackW
			color := seg.Color
			if color == "" {
				color = ColorVar(si)
			}
			sr.Segs = append(sr.Segs, StackedSeg{X: Pct(HBarPlotX + offset), W: Pct(segW), Color: color})
			offset += segW
			if !legendSeen[seg.Label] {
				legendSeen[seg.Label] = true
				out.Legend = append(out.Legend, LegendItem{Label: seg.Label, Color: color})
			}
		}
		out.Rows = append(out.Rows, sr)
	}
	return out
}

func LayoutBullet(data BulletSeries, hue float64) BulletBar {
	out := BulletBar{
		LabelX: Pct(HBarLabelGutter - 2),
		PlotX:  Pct(HBarPlotX),
		TrackW: Pct(HBarTrackW),
	}
	n := len(data.Series)
	if n == 0 {
		return out
	}
	gmax := float64(data.Max)
	if gmax == 0 {
		for _, row := range data.Series {
			for _, m := range row.Measures {
				gmax = MaxF(gmax, float64(m.Value))
			}
		}
	}
	if gmax == 0 {
		gmax = 1
	}
	maxMeasures := 0
	for _, row := range data.Series {
		if len(row.Measures) > maxMeasures {
			maxMeasures = len(row.Measures)
		}
	}
	ramp := GaugeRamp(maxMeasures, hue)
	legendSeen := make(map[string]bool)
	band := 100.0 / float64(n)
	barTopOff := band * HBarBarInset
	barH := band * (1 - 2*HBarBarInset)

	for i, row := range data.Series {
		bandTop := float64(i) * band
		displayLabel := row.Label
		if len(displayLabel) > MaxLabelLen {
			displayLabel = displayLabel[:MaxLabelLen-1] + "…"
		}
		tipRows := make([]TipRow, 0, len(row.Measures))
		for _, m := range row.Measures {
			tipRows = append(tipRows, TipRow{Label: m.Label, Value: CommaInt(m.Value) + data.Unit})
		}
		total := ""
		if len(row.Measures) > 0 {
			total = CommaInt(row.Measures[len(row.Measures)-1].Value) + data.Unit
		}
		br := BulletBarRow{
			Label: displayLabel, Total: total, Tip: BuildTipHTML("", row.Label, tipRows),
			BarY: Pct(bandTop + barTopOff), BarH: Pct(barH), CenterY: Pct(bandTop + band/2),
		}
		sorted := make([]BulletMeasure, len(row.Measures))
		copy(sorted, row.Measures)
		for a := 0; a < len(sorted); a++ {
			for b := a + 1; b < len(sorted); b++ {
				if sorted[a].Value < sorted[b].Value {
					sorted[a], sorted[b] = sorted[b], sorted[a]
				}
			}
		}
		for j, m := range sorted {
			frac := float64(m.Value) / gmax
			if frac > 1 {
				frac = 1
			}
			br.Bars = append(br.Bars, BulletSeg{X: Pct(HBarPlotX), W: Pct(HBarTrackW * frac), Color: ramp[j]})
			if !legendSeen[m.Label] {
				legendSeen[m.Label] = true
				rampIdx := len(row.Measures) - 1 - j
				if rampIdx < 0 {
					rampIdx = 0
				}
				out.Legend = append(out.Legend, LegendItem{Label: m.Label, Color: ramp[rampIdx]})
			}
		}
		out.Rows = append(out.Rows, br)
	}
	return out
}

func LayoutGroupedBar(data GroupedBarData) GroupedBar {
	out := GroupedBar{
		LabelX: Pct(HBarLabelGutter - 2),
		PlotX:  Pct(HBarPlotX),
		TrackW: Pct(HBarTrackW),
	}
	n := len(data.Rows)
	nMeasures := len(data.Measures)
	if n == 0 || nMeasures == 0 {
		return out
	}
	gmax := float64(data.Max)
	if gmax == 0 {
		for _, row := range data.Rows {
			for _, v := range row.Values {
				gmax = MaxF(gmax, float64(v))
			}
		}
	}
	if gmax == 0 {
		gmax = 1
	}
	for mi, m := range data.Measures {
		out.Legend = append(out.Legend, LegendItem{Label: m, Color: ColorVar(mi)})
	}
	band := 100.0 / float64(n)
	barTopOff := band * HBarBarInset
	barH := band * (1 - 2*HBarBarInset)
	subH := barH / float64(nMeasures)

	for i, row := range data.Rows {
		bandTop := float64(i) * band
		displayLabel := row.Label
		if len(displayLabel) > MaxLabelLen {
			displayLabel = displayLabel[:MaxLabelLen-1] + "…"
		}
		tipRows := make([]TipRow, 0, nMeasures)
		for mi, v := range row.Values {
			if mi < len(data.Measures) {
				tipRows = append(tipRows, TipRow{Label: data.Measures[mi], Value: CommaInt(v) + data.Unit})
			}
		}
		gr := GroupedRow{
			Label: displayLabel, Tip: BuildTipHTML("", row.Label, tipRows),
			BarY: Pct(bandTop + barTopOff), BarH: Pct(barH), CenterY: Pct(bandTop + band/2),
		}
		for mi, v := range row.Values {
			frac := float64(v) / gmax
			if frac > 1 {
				frac = 1
			}
			y := bandTop + barTopOff + float64(mi)*subH
			gr.Bars = append(gr.Bars, GroupedSeg{
				X: Pct(HBarPlotX), Y: Pct(y), W: Pct(HBarTrackW * frac), H: Pct(subH * 0.85), Color: ColorVar(mi),
			})
		}
		out.Rows = append(out.Rows, gr)
	}
	return out
}
