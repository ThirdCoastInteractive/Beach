// Package config loads typed application configuration from the environment.
//
// Fields are declared with `env` struct tags:
//
//	type App struct {
//	    config.Core                                   // embed the framework block
//	    StripeKey string `env:"STRIPE_SECRET_KEY"`    // app-specific
//	}
//
// A tag is `NAME[,modifier...]`. NAME may list `A|B` fallbacks, tried in order
// (`env:"POSTGRES_DSN|DATABASE_URL"`). A `default` tag supplies a value when no
// env var is set; a field with neither a value nor a default is required, and
// MustLoad aborts if any required field is unset.
//
// The `required` modifier makes the requirement explicit and stricter: a
// `required` field must be present AND non-empty (so an explicit `X=` fails,
// unlike a bare no-default field). `required` and `default` are mutually
// exclusive — declaring both is a config error.
//
// Modifiers: required, lower, upper, trim, trimslash, url (validates the value
// parses). Supported field kinds: string, bool, the integer kinds, the float
// kinds, and time.Duration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Core is the framework's base config block. Apps embed it in their own struct.
type Core struct {
	AppEnv  string `env:"APP_ENV,lower" default:"development"`
	Port    string `env:"PORT" default:"8080"`
	DSN     string `env:"POSTGRES_DSN|DATABASE_URL"`
	BaseURL string `env:"BASE_URL,trimslash,url" default:"http://localhost:8080"`
}

// Production reports whether the app is running in production.
func (c Core) Production() bool { return c.AppEnv == "production" }

// Load populates a new *T from the environment, returning an error that names
// every missing required field at once.
func Load[T any]() (*T, error) {
	dst := new(T)
	v := reflect.ValueOf(dst).Elem()
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("config: %T is not a struct", *dst)
	}
	var missing []string
	if err := loadStruct(v, &missing); err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("config: missing required env: %s", strings.Join(missing, ", "))
	}
	return dst, nil
}

// MustLoad is Load but prints the error to stderr and exits non-zero on failure.
// It is the call boot code makes: a misconfigured process should not start.
func MustLoad[T any]() *T {
	c, err := Load[T]()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return c
}

func loadStruct(v reflect.Value, missing *[]string) error {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		fv := v.Field(i)

		// Recurse into embedded structs (e.g. config.Core).
		if f.Anonymous && fv.Kind() == reflect.Struct {
			if err := loadStruct(fv, missing); err != nil {
				return err
			}
			continue
		}

		tag := f.Tag.Get("env")
		if tag == "" || !fv.CanSet() {
			continue
		}
		names, mods := parseTag(tag)
		required, mods := splitRequired(mods)

		def, hasDefault := f.Tag.Lookup("default")
		if required && hasDefault {
			return fmt.Errorf("config: %s: `required` and `default` are mutually exclusive", f.Name)
		}

		raw, found := lookup(names)
		if !found {
			if !hasDefault {
				*missing = append(*missing, names[0])
				continue
			}
			raw = def
		}
		// A required field must be present and non-empty: an explicit `X=` is a
		// misconfiguration, not an opt-out.
		if required && raw == "" {
			*missing = append(*missing, names[0])
			continue
		}

		val, err := applyMods(raw, mods)
		if err != nil {
			return fmt.Errorf("config: %s: %w", f.Name, err)
		}
		if err := setField(fv, val); err != nil {
			return fmt.Errorf("config: %s: %w", f.Name, err)
		}
	}
	return nil
}

// splitRequired pulls the `required` directive out of the modifier list: it is
// a field-level requirement, not a value transform, so it must not reach
// applyMods (which would reject it as unknown).
func splitRequired(mods []string) (required bool, rest []string) {
	for _, m := range mods {
		if strings.TrimSpace(m) == "required" {
			required = true
			continue
		}
		rest = append(rest, m)
	}
	return required, rest
}

func parseTag(tag string) (names, mods []string) {
	parts := strings.Split(tag, ",")
	for _, n := range strings.Split(parts[0], "|") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	return names, parts[1:]
}

func lookup(names []string) (string, bool) {
	for _, n := range names {
		if v, ok := os.LookupEnv(n); ok {
			return v, true
		}
	}
	return "", false
}

func applyMods(s string, mods []string) (string, error) {
	for _, m := range mods {
		switch strings.TrimSpace(m) {
		case "", "optional":
			// no-op transforms
		case "lower":
			s = strings.ToLower(s)
		case "upper":
			s = strings.ToUpper(s)
		case "trim":
			s = strings.TrimSpace(s)
		case "trimslash":
			s = strings.TrimRight(s, "/")
		case "url":
			if _, err := url.ParseRequestURI(s); err != nil {
				return "", fmt.Errorf("invalid url %q", s)
			}
		default:
			return "", fmt.Errorf("unknown modifier %q", m)
		}
	}
	return s, nil
}

var durationType = reflect.TypeOf(time.Duration(0))

func setField(fv reflect.Value, s string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(s)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fv.Type() == durationType {
			d, err := time.ParseDuration(s)
			if err != nil {
				return err
			}
			fv.SetInt(int64(d))
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Float32, reflect.Float64:
		fl, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(fl)
	default:
		return fmt.Errorf("unsupported kind %s", fv.Kind())
	}
	return nil
}

// LoadEnvFile reads KEY=VALUE lines from path and sets any variable not already
// present in the environment. A missing file is not an error, so callers can
// load a development .env unconditionally. Real env always wins over the file.
func LoadEnvFile(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, val)
		}
	}
	return nil
}
