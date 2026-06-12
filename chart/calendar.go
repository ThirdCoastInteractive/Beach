package chart

import "fmt"

// --- Input types ------------------------------------------------------------

type CalendarData struct {
	Days []CalendarDay `json:"days"`
	Unit string        `json:"unit,omitempty"`
}

type CalendarDay struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
	Year  int     `json:"year"`
	Month int     `json:"month"`
	Day   int     `json:"day"`
	DOW   int     `json:"dow"`  // 0=Sun, 6=Sat
	Week  int     `json:"week"` // week of year (0-based)
}

// --- Output geometry --------------------------------------------------------

type Calendar struct {
	Cells       []CalendarCell
	MonthLabels []CalendarMonth
	DayLabels   []CalendarDayLabel
	VH          float64
}

type CalendarCell struct {
	X     float64
	Y     float64
	W     float64
	H     float64
	Color string
	Tip   string
}

type CalendarMonth struct {
	X     float64
	Label string
}

type CalendarDayLabel struct {
	Y     float64
	Label string
}

// --- Helpers ----------------------------------------------------------------

func CalendarViewBox(c Calendar) string {
	vh := c.VH
	if vh < 100 {
		vh = 200
	}
	return "0 0 1000 " + F(vh)
}

// --- Layout -----------------------------------------------------------------

func LayoutCalendar(data CalendarData) Calendar {
	if len(data.Days) == 0 {
		return Calendar{}
	}

	const (
		cellSize = 16.0
		cellPad  = 2.0
		padL     = 40.0
		padT     = 30.0
	)

	var minV, maxV float64
	minV = data.Days[0].Value
	maxV = data.Days[0].Value
	for _, d := range data.Days {
		if d.Value < minV {
			minV = d.Value
		}
		if d.Value > maxV {
			maxV = d.Value
		}
	}
	span := maxV - minV
	if span == 0 {
		span = 1
	}

	maxWeek := 0
	for _, d := range data.Days {
		if d.Week > maxWeek {
			maxWeek = d.Week
		}
	}

	out := Calendar{
		VH: padT + 7*(cellSize+cellPad) + 20,
	}

	for _, d := range data.Days {
		x := padL + float64(d.Week)*(cellSize+cellPad)
		y := padT + float64(d.DOW)*(cellSize+cellPad)

		t := (d.Value - minV) / span
		l := 0.20 + t*0.50
		ch := 0.02 + t*0.16
		color := fmt.Sprintf("oklch(%.2f %.2f 250)", l, ch)

		tip := BuildTipHTML(color, d.Date,
			[]TipRow{{Label: "value", Value: fmt.Sprintf("%.0f", d.Value) + " " + data.Unit}})

		out.Cells = append(out.Cells, CalendarCell{
			X: x, Y: y, W: cellSize, H: cellSize, Color: color, Tip: tip,
		})
	}

	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	seen := make(map[int]bool)
	for _, d := range data.Days {
		if !seen[d.Month] {
			seen[d.Month] = true
			x := padL + float64(d.Week)*(cellSize+cellPad)
			label := ""
			if d.Month >= 1 && d.Month <= 12 {
				label = months[d.Month-1]
			}
			out.MonthLabels = append(out.MonthLabels, CalendarMonth{X: x, Label: label})
		}
	}

	dayNames := []string{"Sun", "", "Tue", "", "Thu", "", "Sat"}
	for i, name := range dayNames {
		if name != "" {
			y := padT + float64(i)*(cellSize+cellPad) + cellSize/2
			out.DayLabels = append(out.DayLabels, CalendarDayLabel{Y: y, Label: name})
		}
	}

	return out
}
