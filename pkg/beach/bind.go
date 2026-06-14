package beach

import (
	"reflect"
	"strconv"
)

// setFormFields fills the fields of *dst from the request's parsed form, mapping
// each settable field by its `form:"name"` tag (falling back to the lowercased
// field name). Supported kinds mirror config: string, bool, and the integer
// kinds. Unsupported kinds and absent values are left at their zero value. This
// is deliberately small — the JSON path (Datastar signals) is the primary one;
// form binding covers plain progressive-enhancement submissions.
func setFormFields[T any](c *Ctx, dst *T) {
	v := reflect.ValueOf(dst).Elem()
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}
		name := f.Tag.Get("form")
		if name == "" || name == "-" {
			// No explicit tag: fall back to the field name lowercased, which is a
			// common convention for simple forms.
			name = lowerFirst(f.Name)
		}
		raw := c.R.Form.Get(name)
		if raw == "" {
			continue
		}
		setScalar(fv, raw)
	}
}

// setScalar assigns a string form value into a scalar reflect.Value, parsing as
// needed. Parse failures are ignored (the field keeps its zero value); a handler
// that needs strict numeric validation should declare a Validate() method.
func setScalar(fv reflect.Value, raw string) {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		if b, err := strconv.ParseBool(raw); err == nil {
			fv.SetBool(b)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			fv.SetInt(n)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, err := strconv.ParseUint(raw, 10, 64); err == nil {
			fv.SetUint(n)
		}
	case reflect.Float32, reflect.Float64:
		if n, err := strconv.ParseFloat(raw, 64); err == nil {
			fv.SetFloat(n)
		}
	}
}

// lowerFirst lowercases the first rune of s (ASCII-only, which is all Go field
// names need).
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'A' && b[0] <= 'Z' {
		b[0] += 'a' - 'A'
	}
	return string(b)
}
