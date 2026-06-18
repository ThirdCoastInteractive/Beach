package quantity

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

// closeEnough compares floats with a tolerance suited to display-grade
// conversions; the package never claims bit-exactness (see the package doc).
func closeEnough(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// --- TCI-236: native-unit storage round-trips ---

func TestNativeUnitStorageVerbatim(t *testing.T) {
	// A value arriving in kW is stored as kW; one in W is stored as W. Neither
	// is normalised to a base unit.
	kw := New(2.5, Kilowatt)
	if kw.Value != 2.5 || kw.Unit != Kilowatt {
		t.Fatalf("kW stored as %v %s, want 2.5 kW", kw.Value, kw.Unit.Symbol)
	}
	w := New(2500, Watt)
	if w.Value != 2500 || w.Unit != Watt {
		t.Fatalf("W stored as %v %s, want 2500 W", w.Value, w.Unit.Symbol)
	}
	// Same physical power, different native units, both preserved.
	if !kw.Equal(w, 1e-9) {
		t.Fatalf("2.5 kW should equal 2500 W")
	}
	if kw.Value == w.Value {
		t.Fatalf("native values must not be coerced to a common scale")
	}
}

func TestInDoesNotMutate(t *testing.T) {
	q := New(2500, Watt)
	conv, err := q.In(Kilowatt)
	if err != nil {
		t.Fatal(err)
	}
	if !closeEnough(conv.Value, 2.5, 1e-9) || conv.Unit != Kilowatt {
		t.Fatalf("In(kW) = %v %s, want 2.5 kW", conv.Value, conv.Unit.Symbol)
	}
	if q.Value != 2500 || q.Unit != Watt {
		t.Fatalf("source mutated to %v %s; quantities must never be rewritten", q.Value, q.Unit.Symbol)
	}
}

func TestCrossDimensionConversionFails(t *testing.T) {
	q := New(100, Watt)
	if _, err := q.In(Foot); err == nil {
		t.Fatal("converting watts to feet should error")
	}
	// Real vs apparent power are distinct dimensions and must not mix.
	if _, err := New(100, Watt).In(VoltAmpere); err == nil {
		t.Fatal("converting W to VA should error (distinct dimensions)")
	}
}

// --- TCI-236: metric<->imperial display conversion for every dimension ---

func TestDisplayConversionEveryDimension(t *testing.T) {
	tests := []struct {
		name string
		from Quantity
		to   Unit
		want float64
		tol  float64
	}{
		// Power (no imperial distinction; just scale).
		{"W->kW", New(2500, Watt), Kilowatt, 2.5, 1e-9},
		{"MW->kW", New(1, Megawatt), Kilowatt, 1000, 1e-6},
		// Energy.
		{"kWh->Wh", New(1.5, KilowattHour), WattHour, 1500, 1e-6},
		// Length metric<->imperial.
		{"m->ft", New(1, Metre), Foot, 3.280839895, 1e-6},
		{"ft->m", New(3.280839895, Foot), Metre, 1, 1e-6},
		{"in->cm", New(1, Inch), Centimetre, 2.54, 1e-9},
		{"mi->km", New(1, Mile), Kilometre, 1.609344, 1e-9},
		// Mass metric<->imperial.
		{"kg->lb", New(1, Kilogram), Pound, 2.2046226218, 1e-6},
		{"lb->kg", New(2.2046226218, Pound), Kilogram, 1, 1e-6},
		{"oz->g", New(1, Ounce), Gram, 28.349523125, 1e-9},
		// Temperature metric<->imperial (offset units).
		{"C->F freezing", New(0, Celsius), Fahrenheit, 32, 1e-6},
		{"C->F boiling", New(100, Celsius), Fahrenheit, 212, 1e-6},
		{"F->C", New(98.6, Fahrenheit), Celsius, 37, 1e-6},
		{"C->K", New(0, Celsius), Kelvin, 273.15, 1e-6},
		// Current.
		{"A->mA", New(2, Ampere), Milliampere, 2000, 1e-6},
		// Voltage.
		{"kV->V", New(0.4, Kilovolt), Volt, 400, 1e-6},
		// Apparent power.
		{"kVA->VA", New(1.5, KilovoltAmpere), VoltAmpere, 1500, 1e-6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.from.In(tt.to)
			if err != nil {
				t.Fatal(err)
			}
			if !closeEnough(got.Value, tt.want, tt.tol) {
				t.Fatalf("%s = %v, want %v", tt.name, got.Value, tt.want)
			}
			if got.Unit != tt.to {
				t.Fatalf("unit = %s, want %s", got.Unit.Symbol, tt.to.Symbol)
			}
		})
	}
}

func TestTemperatureRoundTrip(t *testing.T) {
	for _, c := range []float64{-40, 0, 37, 100, 1000} {
		q := New(c, Celsius)
		f, err := q.In(Fahrenheit)
		if err != nil {
			t.Fatal(err)
		}
		back, err := f.In(Celsius)
		if err != nil {
			t.Fatal(err)
		}
		if !closeEnough(back.Value, c, 1e-6) {
			t.Fatalf("C->F->C lost precision: %v -> %v", c, back.Value)
		}
	}
	// -40 is the famous fixed point.
	f, _ := New(-40, Celsius).In(Fahrenheit)
	if !closeEnough(f.Value, -40, 1e-9) {
		t.Fatalf("-40 C should be -40 F, got %v", f.Value)
	}
}

// --- TCI-237: format precision + symbol ---

func TestFormatUnitPrecisionAndSymbol(t *testing.T) {
	tests := []struct {
		val  float64
		unit Unit
		want string
	}{
		{2.5, Kilowatt, "2.5 kW"},
		{2, Kilowatt, "2 kW"},  // trailing zeros trimmed
		{2500, Watt, "2500 W"}, // zero-precision unit
		{3.280839895, Foot, "3.28 ft"},
		{37, Celsius, "37 °C"},
		{98.6, Fahrenheit, "98.6 °F"},
		{1.5, KilovoltAmpere, "1.5 kVA"},
		{0, Celsius, "0 °C"},
	}
	for _, tt := range tests {
		if got := FormatUnit(tt.val, tt.unit); got != tt.want {
			t.Errorf("FormatUnit(%v, %s) = %q, want %q", tt.val, tt.unit.Symbol, got, tt.want)
		}
	}
}

func TestFormatInChoosesScale(t *testing.T) {
	// DisplayUnit picks the largest unit keeping the value >= 1.
	if got := FormatIn(New(2500, Watt), Metric); got != "2.5 kW" {
		t.Errorf("2500 W -> %q, want 2.5 kW", got)
	}
	if got := FormatIn(New(0.5, Metre), Metric); got != "50 cm" {
		t.Errorf("0.5 m -> %q, want 50 cm", got)
	}
	if got := FormatIn(New(1500, Metre), Metric); got != "1.5 km" {
		t.Errorf("1500 m -> %q, want 1.5 km", got)
	}
}

// --- TCI-237: parse ---

func TestParse(t *testing.T) {
	tests := []struct {
		in    string
		value float64
		unit  Unit
	}{
		{"2.5 kW", 2.5, Kilowatt},
		{"2500W", 2500, Watt},
		{"60ft", 60, Foot},
		{"-5 °C", -5, Celsius},
		{"  1.609344 km ", 1.609344, Kilometre},
		{"1.5e3 Wh", 1500, WattHour},
		{"400 V", 400, Volt},
	}
	for _, tt := range tests {
		q, err := Parse(tt.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.in, err)
		}
		if q.Value != tt.value || q.Unit != tt.unit {
			t.Errorf("Parse(%q) = %v %s, want %v %s", tt.in, q.Value, q.Unit.Symbol, tt.value, tt.unit.Symbol)
		}
	}
}

func TestParseStoresNativeUnit(t *testing.T) {
	// Parse never converts: "2500 W" stays W, not kW.
	q, err := Parse("2500 W")
	if err != nil {
		t.Fatal(err)
	}
	if q.Unit != Watt || q.Value != 2500 {
		t.Fatalf("Parse stored %v %s, want native 2500 W", q.Value, q.Unit.Symbol)
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"", "  ", "2.5", "kW", "2.5 furlongs", "abc kW", "5 xyz"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should error", in)
		}
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	// A stored value formatted in its own unit re-parses to the same quantity.
	q := New(2500, Watt)
	s := FormatUnit(q.Value, q.Unit)
	back, err := Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	if !back.Equal(q, 1e-9) {
		t.Fatalf("round trip %q -> %v %s", s, back.Value, back.Unit.Symbol)
	}
}

// --- TCI-237: context preference; same stored value renders both ways ---

func TestContextPreferenceBothRenderings(t *testing.T) {
	// One stored length, two renderings; stored value never changes.
	stored := New(1, Metre)

	metricCtx := WithUnitSystem(context.Background(), Metric)
	imperialCtx := WithUnitSystem(context.Background(), Imperial)

	gotMetric := Format(metricCtx, stored)
	gotImperial := Format(imperialCtx, stored)

	if gotMetric != "1 m" {
		t.Errorf("metric render = %q, want 1 m", gotMetric)
	}
	if gotImperial != "3.28 ft" {
		t.Errorf("imperial render = %q, want 3.28 ft", gotImperial)
	}
	// The stored quantity is unchanged after both renders.
	if stored.Value != 1 || stored.Unit != Metre {
		t.Fatalf("stored quantity mutated to %v %s", stored.Value, stored.Unit.Symbol)
	}
}

func TestContextPreferenceTemperatureAndMass(t *testing.T) {
	temp := New(100, Celsius)
	if got := Format(WithUnitSystem(context.Background(), Metric), temp); got != "100 °C" {
		t.Errorf("metric temp = %q, want 100 °C", got)
	}
	if got := Format(WithUnitSystem(context.Background(), Imperial), temp); got != "212 °F" {
		t.Errorf("imperial temp = %q, want 212 °F", got)
	}

	mass := New(1, Kilogram)
	if got := Format(WithUnitSystem(context.Background(), Imperial), mass); got != "2.2 lb" {
		t.Errorf("imperial mass = %q, want 2.2 lb", got)
	}
}

func TestFormatDefaultsToMetric(t *testing.T) {
	// No unit system on context -> metric, like i18n.T with no locale.
	if got := Format(context.Background(), New(1, Metre)); got != "1 m" {
		t.Errorf("default render = %q, want 1 m", got)
	}
	if UnitSystem(context.Background()) != Metric {
		t.Error("empty context should yield Metric")
	}
	if UnitSystem(nil) != Metric { //nolint:staticcheck // nil ctx is a tested edge.
		t.Error("nil context should yield Metric")
	}
}

func TestPowerRendersSameBothSystems(t *testing.T) {
	// Power has no imperial distinction: both systems render identically.
	q := New(2500, Watt)
	m := Format(WithUnitSystem(context.Background(), Metric), q)
	i := Format(WithUnitSystem(context.Background(), Imperial), q)
	if m != i || m != "2.5 kW" {
		t.Errorf("power should render identically both systems: metric=%q imperial=%q", m, i)
	}
}

// --- TCI-237: cookie middleware ---

func TestMiddlewareResolvesCookie(t *testing.T) {
	var seen System
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = UnitSystem(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "imperial"})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != Imperial {
		t.Errorf("cookie imperial resolved to %s", seen)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "metric"})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != Metric {
		t.Errorf("cookie metric resolved to %s", seen)
	}
}

func TestMiddlewareCookieBeatsHeader(t *testing.T) {
	var seen System
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = UnitSystem(r.Context())
	}))
	// Header says en-US (imperial) but cookie says metric: cookie wins.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "metric"})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != Metric {
		t.Errorf("cookie should beat header; got %s", seen)
	}
}

func TestMiddlewareAcceptLanguageFallback(t *testing.T) {
	var seen System
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = UnitSystem(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != Imperial {
		t.Errorf("en-US header should yield imperial, got %s", seen)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-GB,en;q=0.9")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != Metric {
		t.Errorf("en-GB header should yield metric, got %s", seen)
	}
}

func TestMiddlewareDefaultsMetric(t *testing.T) {
	var seen System
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = UnitSystem(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != Metric {
		t.Errorf("no cookie/header should yield metric, got %s", seen)
	}
}

func TestParseSystem(t *testing.T) {
	cases := map[string]struct {
		sys System
		ok  bool
	}{
		"metric":     {Metric, true},
		"Imperial":   {Imperial, true},
		" IMPERIAL ": {Imperial, true},
		"klingon":    {Metric, false},
		"":           {Metric, false},
	}
	for in, want := range cases {
		got, ok := ParseSystem(in)
		if got != want.sys || ok != want.ok {
			t.Errorf("ParseSystem(%q) = %s,%v want %s,%v", in, got, ok, want.sys, want.ok)
		}
	}
}

// --- supporting taxonomy checks ---

func TestDisplayUnitsTableCoversEveryDimension(t *testing.T) {
	dims := []Dimension{Power, Energy, Length, Mass, Temperature, Current, Voltage, ApparentPower}
	for _, d := range dims {
		for _, sys := range []System{Metric, Imperial} {
			if len(DisplayUnits(d, sys)) == 0 {
				t.Errorf("no display units for %s/%s", d, sys)
			}
		}
	}
}

func TestDisplayUnitsCopyIsolated(t *testing.T) {
	got := DisplayUnits(Length, Metric)
	if len(got) == 0 {
		t.Fatal("expected length units")
	}
	got[0] = Watt // mutate the returned slice
	again := DisplayUnits(Length, Metric)
	if again[0] == Watt {
		t.Fatal("DisplayUnits must return a copy, not the backing slice")
	}
}

func TestUnitForSymbol(t *testing.T) {
	if u, ok := UnitForSymbol("kW"); !ok || u != Kilowatt {
		t.Errorf("kW -> %v,%v want Kilowatt,true", u.Symbol, ok)
	}
	if _, ok := UnitForSymbol("nope"); ok {
		t.Error("unknown symbol should not resolve")
	}
}
