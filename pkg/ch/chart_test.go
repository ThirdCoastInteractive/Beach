package ch

import (
	"testing"

	"github.com/ThirdCoastInteractive/the-beach/pkg/chart"
)

// fakeRow stands in for a scanned ch.Rows[T] result: an ordinary struct, no
// ClickHouse needed. The projectors the adapters take pull fields out of it.
type fakeRow struct {
	Rack  string
	Draw  float64
	Units int
	Hour  string
	Live  int
	Spare int
}

var sampleRows = []fakeRow{
	{Rack: "A1", Draw: 6.4, Units: 14, Hour: "00:00", Live: 10, Spare: 4},
	{Rack: "A2", Draw: 4.1, Units: 9, Hour: "01:00", Live: 7, Spare: 2},
	{Rack: "B1", Draw: 7.8, Units: 21, Hour: "02:00", Live: 18, Spare: 3},
}

func TestToSeries(t *testing.T) {
	got := ToSeries(sampleRows,
		func(r fakeRow) string { return r.Rack },
		func(r fakeRow) float64 { return r.Draw })

	if len(got) != len(sampleRows) {
		t.Fatalf("len = %d, want %d", len(got), len(sampleRows))
	}
	// Order must be preserved (SQL ORDER BY is the only ordering knob).
	for i, r := range sampleRows {
		if got[i].Label != r.Rack || got[i].Value != r.Draw {
			t.Errorf("row %d = {%q,%v}, want {%q,%v}",
				i, got[i].Label, got[i].Value, r.Rack, r.Draw)
		}
	}
}

func TestToSeriesEmpty(t *testing.T) {
	got := ToSeries(nil,
		func(r fakeRow) string { return r.Rack },
		func(r fakeRow) float64 { return r.Draw })
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestToHBarSeries(t *testing.T) {
	in := ToHBarSeries(sampleRows,
		func(r fakeRow) string { return r.Rack },
		func(r fakeRow) int { return r.Units })
	if len(in.Series) != len(sampleRows) {
		t.Fatalf("Series len = %d, want %d", len(in.Series), len(sampleRows))
	}
	// Order must be preserved (SQL ORDER BY is the only ordering knob).
	for i, r := range sampleRows {
		if in.Series[i].Label != r.Rack || in.Series[i].Value != r.Units {
			t.Errorf("Series[%d] = {%q,%d}, want {%q,%d}",
				i, in.Series[i].Label, in.Series[i].Value, r.Rack, r.Units)
		}
	}
}

func TestToStackedBarSeries(t *testing.T) {
	in := ToStackedBarSeries(sampleRows,
		func(r fakeRow) string { return r.Rack },
		func(r fakeRow) []chart.StackedBarSegment {
			return []chart.StackedBarSegment{
				{Label: "live", Value: r.Live},
				{Label: "spare", Value: r.Spare},
			}
		})
	if len(in.Series) != len(sampleRows) {
		t.Fatalf("Series len = %d, want %d", len(in.Series), len(sampleRows))
	}
	row := in.Series[2]
	if row.Label != "B1" || len(row.Segments) != 2 {
		t.Fatalf("Series[2] = {%q, %d segments}, want {B1, 2 segments}",
			row.Label, len(row.Segments))
	}
	if row.Segments[0].Label != "live" || row.Segments[0].Value != 18 ||
		row.Segments[1].Label != "spare" || row.Segments[1].Value != 3 {
		t.Errorf("Series[2].Segments = %+v, want live=18 spare=3", row.Segments)
	}
}

func TestToLineSeries(t *testing.T) {
	s := ToLineSeries("draw", sampleRows,
		func(r fakeRow) string { return r.Hour },
		func(r fakeRow) float64 { return r.Draw })
	if s.Label != "draw" {
		t.Errorf("Label = %q, want draw", s.Label)
	}
	if len(s.Points) != len(sampleRows) {
		t.Fatalf("Points len = %d, want %d", len(s.Points), len(sampleRows))
	}
	for i, r := range sampleRows {
		if s.Points[i].X != r.Hour || s.Points[i].Y != r.Draw {
			t.Errorf("Points[%d] = (%q,%v), want (%q,%v)",
				i, s.Points[i].X, s.Points[i].Y, r.Hour, r.Draw)
		}
	}
}

func TestToLineSeriesData(t *testing.T) {
	in := ToLineSeriesData("draw", sampleRows,
		func(r fakeRow) string { return r.Hour },
		func(r fakeRow) float64 { return r.Draw })
	if len(in.Series) != 1 {
		t.Fatalf("Series len = %d, want 1", len(in.Series))
	}
	if len(in.Series[0].Points) != len(sampleRows) {
		t.Fatalf("Points len = %d, want %d",
			len(in.Series[0].Points), len(sampleRows))
	}
	if in.Series[0].Points[0].X != "00:00" || in.Series[0].Points[0].Y != 6.4 {
		t.Errorf("first point = (%q,%v), want (00:00,6.4)",
			in.Series[0].Points[0].X, in.Series[0].Points[0].Y)
	}
}
