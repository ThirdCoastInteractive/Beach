// Package quantity is Beach's typed value-with-unit system: a domain-
// agnostic way to carry a measured value together with the unit it arrived in.
//
// The central type is [Quantity] — a value plus its native unit, the unit a
// reading arrives in at ingest. There is no canonical base unit and no
// normalization at storage: a value arriving in kW is stored as kW, one
// arriving in W is stored as W. Stored quantities are never rewritten.
//
//	q := quantity.New(2500, quantity.Watt) // 2500 W, stored verbatim
//	q.Value                                // 2500
//	q.Unit                                 // Watt
//
// Conversion happens on display, not at storage. [Quantity.In] converts a
// quantity to another unit of the same dimension for rendering; the stored
// quantity is unchanged. The formatter (see Format) picks the display unit from
// a per-user metric/imperial preference carried on the request context, exactly
// like the active locale in pkg/i18n.
//
// # Scalar representation
//
// The scalar is a float64. Measurements in this system are physical readings —
// power, energy, temperature — where a fixed decimal exponent does not fit every
// dimension and where display conversions (W↔kW, °C↔°F, m↔ft) are inherently
// multiplicative. float64 keeps conversion a single multiply-add with no scale
// bookkeeping, and its ~15 significant digits dwarf the precision of any sensor
// feeding the system. The tradeoff is that float64 cannot represent every
// decimal exactly (0.1 is periodic in binary), so equality should be compared
// with a tolerance and money-like exact-decimal values do not belong here — this
// package is for measured quantities, not currency.
//
// # Stdlib only
//
// The core is a small typed scalar carrying its unit plus per-dimension
// conversion tables. No third-party units library, no reflection, no codegen.
// The package is render-agnostic: plain Go format/parse funcs, no .templ files —
// templ glue is left to each app.
package quantity

import (
	"fmt"
	"math"
)

// Dimension is the physical quantity a unit measures. Two units can be converted
// between only when they share a dimension. Phase is deliberately absent: it is
// an enum label, not a measured quantity.
type Dimension int

// The dimensions supported at launch. ApparentPower (VA/kVA) is distinct from
// Power (real power, W/kW): they share no conversion and must not be mixed.
const (
	Power Dimension = iota
	Energy
	Length
	Mass
	Temperature
	Current
	Voltage
	ApparentPower
)

// String returns the lower-case dimension name, used in error messages.
func (d Dimension) String() string {
	switch d {
	case Power:
		return "power"
	case Energy:
		return "energy"
	case Length:
		return "length"
	case Mass:
		return "mass"
	case Temperature:
		return "temperature"
	case Current:
		return "current"
	case Voltage:
		return "voltage"
	case ApparentPower:
		return "apparent power"
	default:
		return fmt.Sprintf("dimension(%d)", int(d))
	}
}

// Unit identifies a single unit of measure, e.g. watts or feet. It is a small
// value type; compare units with ==. Units are defined as package-level vars
// (Watt, Foot, …) — do not construct Unit values by hand.
type Unit struct {
	// Symbol is the display abbreviation, e.g. "kW", "°F", "ft".
	Symbol string
	// Dim is the dimension this unit measures.
	Dim Dimension
	// scale and offset define the affine conversion to the dimension's internal
	// reference scale: ref = value*scale + offset. They exist only to convert
	// between units of the same dimension for display; no quantity is ever
	// stored on the reference scale. offset is zero for every unit except the
	// temperature units, whose scales do not share a zero.
	scale, offset float64
	// prec is the default number of fractional digits Format uses for this unit.
	prec int
}

// known indexes every unit by symbol so Parse can recognise operator input and
// so the unit table is the single source of truth. It is populated by the unit
// definitions below.
var known = map[string]Unit{}

// def registers and returns a unit. Defining units through def keeps the symbol
// index (known) in lockstep with the package-level unit vars.
func def(symbol string, dim Dimension, scale, offset float64, prec int) Unit {
	u := Unit{Symbol: symbol, Dim: dim, scale: scale, offset: offset, prec: prec}
	known[symbol] = u
	return u
}

// Unit definitions. Each dimension picks an arbitrary reference scale (one
// unit's scale is 1, offset 0); the reference exists only so any two units of
// the dimension can be related. Stored quantities never use it.
var (
	// Power — reference: watt.
	Watt     = def("W", Power, 1, 0, 0)
	Kilowatt = def("kW", Power, 1000, 0, 3)
	Megawatt = def("MW", Power, 1_000_000, 0, 3)

	// Energy — reference: watt-hour.
	WattHour     = def("Wh", Energy, 1, 0, 0)
	KilowattHour = def("kWh", Energy, 1000, 0, 3)
	MegawattHour = def("MWh", Energy, 1_000_000, 0, 3)

	// Length — reference: metre.
	Millimetre = def("mm", Length, 0.001, 0, 0)
	Centimetre = def("cm", Length, 0.01, 0, 1)
	Metre      = def("m", Length, 1, 0, 2)
	Kilometre  = def("km", Length, 1000, 0, 2)
	Inch       = def("in", Length, 0.0254, 0, 1)
	Foot       = def("ft", Length, 0.3048, 0, 2)
	Mile       = def("mi", Length, 1609.344, 0, 2)

	// Mass — reference: kilogram.
	Gram     = def("g", Mass, 0.001, 0, 0)
	Kilogram = def("kg", Mass, 1, 0, 2)
	Tonne    = def("t", Mass, 1000, 0, 3)
	Ounce    = def("oz", Mass, 0.028349523125, 0, 1)
	Pound    = def("lb", Mass, 0.45359237, 0, 2)

	// Temperature — reference: kelvin. These are the only units with an offset.
	Kelvin     = def("K", Temperature, 1, 0, 2)
	Celsius    = def("°C", Temperature, 1, 273.15, 1)
	Fahrenheit = def("°F", Temperature, 5.0/9.0, 255.372222222222222, 1)

	// Current — reference: ampere.
	Milliampere = def("mA", Current, 0.001, 0, 0)
	Ampere      = def("A", Current, 1, 0, 2)

	// Voltage — reference: volt.
	Millivolt = def("mV", Voltage, 0.001, 0, 0)
	Volt      = def("V", Voltage, 1, 0, 1)
	Kilovolt  = def("kV", Voltage, 1000, 0, 2)

	// Apparent power — reference: volt-ampere. Distinct dimension from Power.
	VoltAmpere     = def("VA", ApparentPower, 1, 0, 0)
	KilovoltAmpere = def("kVA", ApparentPower, 1000, 0, 3)
	MegavoltAmpere = def("MVA", ApparentPower, 1_000_000, 0, 3)
)

// Quantity is a value paired with its native unit — the unit it arrived in at
// ingest. The zero value is not meaningful; build one with [New]. Quantities are
// stored verbatim and never normalised; convert for display only, via [In].
type Quantity struct {
	Value float64
	Unit  Unit
}

// New returns a quantity of value in unit, stored exactly as given.
func New(value float64, unit Unit) Quantity {
	return Quantity{Value: value, Unit: unit}
}

// Dim reports the dimension of the quantity's native unit.
func (q Quantity) Dim() Dimension { return q.Unit.Dim }

// In converts q to target for display and returns the converted value. The
// receiver is unchanged — quantities are never rewritten. It is an error to
// convert across dimensions (e.g. watts to feet); err is non-nil and the
// returned quantity is the zero value in that case.
func (q Quantity) In(target Unit) (Quantity, error) {
	if q.Unit == target {
		return q, nil
	}
	if q.Unit.Dim != target.Dim {
		return Quantity{}, fmt.Errorf("quantity: cannot convert %s to %s: %s vs %s",
			q.Unit.Symbol, target.Symbol, q.Unit.Dim, target.Dim)
	}
	// value -> reference scale -> target. ref = v*scale + offset.
	ref := q.Value*q.Unit.scale + q.Unit.offset
	out := (ref - target.offset) / target.scale
	return Quantity{Value: out, Unit: target}, nil
}

// Equal reports whether q and other represent the same physical quantity within
// tol on the dimension's reference scale. Quantities of different dimensions are
// never equal. Use Equal rather than == to compare across units, and because
// float64 conversions are not bit-exact.
func (q Quantity) Equal(other Quantity, tol float64) bool {
	if q.Unit.Dim != other.Unit.Dim {
		return false
	}
	a := q.Value*q.Unit.scale + q.Unit.offset
	b := other.Value*other.Unit.scale + other.offset()
	return math.Abs(a-b) <= tol
}

// offset returns the receiving quantity's unit offset; a tiny helper kept so
// Equal reads symmetrically.
func (q Quantity) offset() float64 { return q.Unit.offset }
