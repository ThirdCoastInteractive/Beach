package ch

import "testing"

// fakeRow stands in for a scanned ch.Rows[T] result: an ordinary struct, no
// ClickHouse needed. The projectors the adapters take pull fields out of it.
type fakeRow struct {
	Rack string
	Draw float64
	TS   float64
}

var sampleRows = []fakeRow{
	{Rack: "A1", Draw: 6.4, TS: 100},
	{Rack: "A2", Draw: 4.1, TS: 200},
	{Rack: "B1", Draw: 7.8, TS: 300},
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
		func(r fakeRow) float64 { return r.Draw })
	if len(in.Series) != len(sampleRows) {
		t.Fatalf("Series len = %d, want %d", len(in.Series), len(sampleRows))
	}
	if in.Series[2].Label != "B1" || in.Series[2].Value != 7.8 {
		t.Errorf("Series[2] = {%q,%v}, want {B1,7.8}",
			in.Series[2].Label, in.Series[2].Value)
	}
}

func TestToTimeSeries(t *testing.T) {
	s := ToTimeSeries("draw", sampleRows,
		func(r fakeRow) float64 { return r.TS },
		func(r fakeRow) float64 { return r.Draw })
	if s.Name != "draw" {
		t.Errorf("Name = %q, want draw", s.Name)
	}
	if len(s.Points) != len(sampleRows) {
		t.Fatalf("Points len = %d, want %d", len(s.Points), len(sampleRows))
	}
	for i, r := range sampleRows {
		if s.Points[i].X != r.TS || s.Points[i].Y != r.Draw {
			t.Errorf("Points[%d] = (%v,%v), want (%v,%v)",
				i, s.Points[i].X, s.Points[i].Y, r.TS, r.Draw)
		}
	}
}

func TestToLineInput(t *testing.T) {
	in := ToLineInput("draw", sampleRows,
		func(r fakeRow) float64 { return r.TS },
		func(r fakeRow) float64 { return r.Draw })
	if len(in.Series) != 1 {
		t.Fatalf("Series len = %d, want 1", len(in.Series))
	}
	if len(in.Series[0].Points) != len(sampleRows) {
		t.Fatalf("Points len = %d, want %d",
			len(in.Series[0].Points), len(sampleRows))
	}
	if in.Series[0].Points[0].X != 100 || in.Series[0].Points[0].Y != 6.4 {
		t.Errorf("first point = (%v,%v), want (100,6.4)",
			in.Series[0].Points[0].X, in.Series[0].Points[0].Y)
	}
}
