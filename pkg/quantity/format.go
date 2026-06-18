package quantity

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Format renders q as a display string, converting from its native unit to the
// preferred display unit for the unit system carried by ctx. The native quantity
// is unchanged: the same stored value renders "2.5 kW" under metric and the same
// way here (power has no imperial distinction) but a stored length renders "1 m"
// or "3.28 ft" depending on ctx. Precision and symbol come from the chosen unit.
//
// With no unit system on ctx the default (metric) is used, so Format is usable
// with no configuration at all — mirroring i18n.T.
func Format(ctx context.Context, q Quantity) string {
	return FormatIn(q, UnitSystem(ctx))
}

// FormatIn is Format with an explicit unit system rather than reading one from a
// context — useful in tests and in non-request code paths.
func FormatIn(q Quantity, sys System) string {
	unit := DisplayUnit(q, sys)
	conv, err := q.In(unit)
	if err != nil {
		// q and unit share a dimension by construction, so this cannot fail;
		// fall back to the native quantity rather than panic.
		conv, unit = q, q.Unit
	}
	return FormatUnit(conv.Value, unit)
}

// FormatUnit renders value in unit at the unit's default precision, e.g.
// FormatUnit(2.5, Kilowatt) == "2.5 kW". Trailing zeros from the fixed precision
// are trimmed so whole values read cleanly ("2 kW", not "2.000 kW").
func FormatUnit(value float64, unit Unit) string {
	s := strconv.FormatFloat(value, 'f', unit.prec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	// Avoid a "-0" that FormatFloat can produce for tiny negatives.
	if s == "-0" {
		s = "0"
	}
	return s + " " + unit.Symbol
}

// Parse turns operator input back into a quantity. The input is a number and a
// unit symbol, optionally space-separated: "2.5 kW", "60ft", "-5 °C". The
// quantity is stored in the unit the operator typed — Parse does no conversion,
// honouring native-unit storage. An unrecognised or missing unit symbol is an
// error.
func Parse(s string) (Quantity, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return Quantity{}, fmt.Errorf("quantity: empty input")
	}
	// Split the leading numeric run from the trailing unit symbol. The number is
	// the maximal prefix of sign/digit/decimal/exponent characters.
	i := 0
	for i < len(trimmed) {
		c := trimmed[i]
		isNum := (c >= '0' && c <= '9') || c == '.' || c == '+' || c == '-' ||
			c == 'e' || c == 'E'
		if !isNum {
			break
		}
		i++
	}
	numPart := strings.TrimSpace(trimmed[:i])
	symPart := strings.TrimSpace(trimmed[i:])
	if numPart == "" {
		return Quantity{}, fmt.Errorf("quantity: no number in %q", s)
	}
	if symPart == "" {
		return Quantity{}, fmt.Errorf("quantity: no unit in %q", s)
	}
	value, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return Quantity{}, fmt.Errorf("quantity: bad number %q: %w", numPart, err)
	}
	unit, ok := UnitForSymbol(symPart)
	if !ok {
		return Quantity{}, fmt.Errorf("quantity: unknown unit %q", symPart)
	}
	return Quantity{Value: value, Unit: unit}, nil
}

// UnitForSymbol looks up a unit by its display symbol, e.g. "kW" -> Kilowatt. ok
// reports whether the symbol is known. It is the inverse of Unit.Symbol and the
// recogniser Parse uses.
func UnitForSymbol(symbol string) (Unit, bool) {
	u, ok := known[strings.TrimSpace(symbol)]
	return u, ok
}
