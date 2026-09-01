package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type app struct {
	Core
	Workers int           `env:"WORKERS" default:"4"`
	Debug   bool          `env:"DEBUG" default:"false"`
	Timeout time.Duration `env:"TIMEOUT" default:"5s"`
}

func setDSN(t *testing.T) { t.Helper(); t.Setenv("POSTGRES_DSN", "postgres://localhost/x") }

func TestLoad_defaultsAndModifiers(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://localhost/x") // satisfy the one required field
	t.Setenv("APP_ENV", "PRODUCTION")                  // ,lower
	t.Setenv("BASE_URL", "https://beach.test/")        // ,trimslash,url

	c, err := Load[app]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Production() {
		t.Errorf("AppEnv lower: got %q, want production", c.AppEnv)
	}
	if c.BaseURL != "https://beach.test" {
		t.Errorf("trimslash: got %q", c.BaseURL)
	}
	if c.Port != "8080" {
		t.Errorf("default Port: got %q", c.Port)
	}
	if c.Workers != 4 || c.Debug || c.Timeout != 5*time.Second {
		t.Errorf("typed defaults: %+v", c)
	}
}

func TestLoad_fallbackName(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://fallback/y") // the |B branch of POSTGRES_DSN|DATABASE_URL
	c, err := Load[app]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DSN != "postgres://fallback/y" {
		t.Errorf("fallback DSN: got %q", c.DSN)
	}
}

func TestLoad_missingRequired(t *testing.T) {
	// No DSN set: DSN has no default, so it is required.
	_, err := Load[app]()
	if err == nil {
		t.Fatal("expected missing-required error, got nil")
	}
}

func TestLoad_invalidURL(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://localhost/x")
	t.Setenv("BASE_URL", "not a url")
	if _, err := Load[app](); err == nil {
		t.Fatal("expected url validation error, got nil")
	}
}

func TestLoadEnvFile_doesNotOverrideRealEnv(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	if err := os.WriteFile(f, []byte("FROM_FILE=yes\nPORT=9999\n# comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT", "1234") // real env must win over the file
	if err := LoadEnvFile(f); err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	t.Setenv("POSTGRES_DSN", "postgres://localhost/x")

	c, err := Load[app]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != "1234" {
		t.Errorf("real env should win: got Port %q", c.Port)
	}
}

func TestLoadEnvFile_missingIsOK(t *testing.T) {
	if err := LoadEnvFile(filepath.Join(t.TempDir(), "nope.env")); err != nil {
		t.Errorf("missing file should be nil error, got %v", err)
	}
}

type requiredApp struct {
	Core
	Token string `env:"API_TOKEN,required"`
}

func TestLoad_required_unsetIsMissing(t *testing.T) {
	setDSN(t)
	if _, err := Load[requiredApp](); err == nil {
		t.Fatal("expected missing-required error for unset API_TOKEN, got nil")
	}
}

func TestLoad_required_emptyIsMissing(t *testing.T) {
	setDSN(t)
	t.Setenv("API_TOKEN", "") // present but empty must still fail a required field
	if _, err := Load[requiredApp](); err == nil {
		t.Fatal("expected missing-required error for empty API_TOKEN, got nil")
	}
}

func TestLoad_required_setSucceeds(t *testing.T) {
	setDSN(t)
	t.Setenv("API_TOKEN", "secret")
	c, err := Load[requiredApp]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Token != "secret" {
		t.Errorf("Token: got %q, want secret", c.Token)
	}
}

// requiredWithDefault is a contradiction: required AND a default.
type requiredWithDefault struct {
	Core
	X string `env:"X,required" default:"oops"`
}

func TestLoad_required_withDefaultIsError(t *testing.T) {
	setDSN(t)
	t.Setenv("X", "value")
	if _, err := Load[requiredWithDefault](); err == nil {
		t.Fatal("expected error for required+default, got nil")
	}
}

func TestLoad_float(t *testing.T) {
	setDSN(t)
	t.Setenv("RATE", "3.75")
	c, err := Load[struct {
		Core
		Rate float64 `env:"RATE" default:"2.5"`
	}]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Rate != 3.75 {
		t.Errorf("Rate: got %v, want 3.75", c.Rate)
	}
}

func TestLoad_float_default(t *testing.T) {
	setDSN(t)
	c, err := Load[struct {
		Core
		Rate  float64 `env:"RATE" default:"2.5"`
		Tweak float32 `env:"TWEAK" default:"0.1"`
	}]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Rate != 2.5 || c.Tweak != 0.1 {
		t.Errorf("float defaults: got Rate=%v Tweak=%v", c.Rate, c.Tweak)
	}
}

func TestLoad_float_invalid(t *testing.T) {
	setDSN(t)
	t.Setenv("RATE", "not-a-number")
	if _, err := Load[struct {
		Core
		Rate float64 `env:"RATE" default:"2.5"`
	}](); err == nil {
		t.Fatal("expected parse error for non-numeric float, got nil")
	}
}
