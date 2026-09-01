package quantity

// System is a unit-system preference: metric or imperial. It is a display-time
// concern, carried on the request context (see WithUnitSystem) and read by the
// formatter to choose a display unit. Stored quantities never depend on it.
type System int

const (
	// Metric is the default unit system: SI units throughout.
	Metric System = iota
	// Imperial selects US/imperial display units where a dimension distinguishes
	// them (length in inches/feet/miles, mass in ounces/pounds, temperature in
	// Fahrenheit). Dimensions with no imperial distinction (power, energy,
	// current, voltage, apparent power) fall back to their metric units.
	Imperial
)

// String returns the canonical lower-case name, used by the cookie middleware
// and in error messages.
func (s System) String() string {
	switch s {
	case Imperial:
		return "imperial"
	default:
		return "metric"
	}
}

// displayUnits lists, per dimension and system, the units a formatter may pick
// from, ordered small-to-large. DisplayUnit walks this list to choose the unit
// whose magnitude best fits a value (the "pick a sensible scale" step). Metric
// and imperial share a list for dimensions that do not distinguish the two.
var displayUnits = map[Dimension]map[System][]Unit{
	Power: {
		Metric:   {Watt, Kilowatt, Megawatt},
		Imperial: {Watt, Kilowatt, Megawatt},
	},
	Energy: {
		Metric:   {WattHour, KilowattHour, MegawattHour},
		Imperial: {WattHour, KilowattHour, MegawattHour},
	},
	Length: {
		Metric:   {Millimetre, Centimetre, Metre, Kilometre},
		Imperial: {Inch, Foot, Mile},
	},
	Mass: {
		Metric:   {Gram, Kilogram, Tonne},
		Imperial: {Ounce, Pound},
	},
	Temperature: {
		Metric:   {Celsius},
		Imperial: {Fahrenheit},
	},
	Current: {
		Metric:   {Milliampere, Ampere},
		Imperial: {Milliampere, Ampere},
	},
	Voltage: {
		Metric:   {Millivolt, Volt, Kilovolt},
		Imperial: {Millivolt, Volt, Kilovolt},
	},
	ApparentPower: {
		Metric:   {VoltAmpere, KilovoltAmpere, MegavoltAmpere},
		Imperial: {VoltAmpere, KilovoltAmpere, MegavoltAmpere},
	},
}

// DisplayUnits returns the ordered (small-to-large) display units a formatter
// may choose from for dim under sys. The returned slice is a copy; mutating it
// does not affect the package tables. An unknown dimension yields nil.
func DisplayUnits(dim Dimension, sys System) []Unit {
	bySystem, ok := displayUnits[dim]
	if !ok {
		return nil
	}
	src := bySystem[sys]
	out := make([]Unit, len(src))
	copy(out, src)
	return out
}

// DisplayUnit picks the display unit for q under sys: the largest unit whose
// magnitude leaves the converted value at or above 1, so 2500 W renders as
// 2.5 kW and 0.5 m renders as 50 cm. If every unit would leave the value below
// 1 (or the value is zero), the smallest unit is used. The returned unit is
// always of q's dimension; for a dimension with no display table the native
// unit is returned unchanged.
func DisplayUnit(q Quantity, sys System) Unit {
	units := displayUnits[q.Unit.Dim][sys]
	if len(units) == 0 {
		return q.Unit
	}
	chosen := units[0]
	for _, u := range units {
		conv, err := q.In(u)
		if err != nil {
			continue
		}
		if abs(conv.Value) >= 1 {
			chosen = u
		} else {
			break
		}
	}
	return chosen
}

// abs is a tiny float helper kept local so the package needs only math where the
// sign-aware comparisons live.
func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
