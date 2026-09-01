package i18n

// The framework catalog is the floor under every app catalog. This is the test
// for that, and it exists because the absence of it shipped a real defect: the
// scaffold's own skip link rendered the literal string "ui.a11y.skip_to_content"
// on every page, because configuring Locales replaced the framework's strings
// instead of adding to them.

import (
	"context"
	"testing"
	"testing/fstest"
)

// appCatalog builds a catalog holding only an app's own keys, which is what
// every real app has.
func appCatalog(t *testing.T) *Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"locales/en-US.json": &fstest.MapFile{Data: []byte(`{"home.tagline":"A small app"}`)},
		"locales/es-ES.json": &fstest.MapFile{Data: []byte(`{"home.tagline":"Una app pequeña"}`)},
	}
	c, err := Load(fsys, "en-US")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

func TestAnAppCatalogExtendsTheFrameworkCatalog(t *testing.T) {
	c := appCatalog(t)

	// The app's own key still wins, in every locale it defines.
	if got := c.T(WithLocale(context.Background(), "es-ES"), "home.tagline"); got != "Una app pequeña" {
		t.Errorf("the app's own key did not resolve: %q", got)
	}

	// A framework key the app never heard of still resolves to real words.
	for _, key := range []string{"ui.a11y.skip_to_content", "ui.a11y.close", "ui.a11y.live.paused"} {
		got := c.T(context.Background(), key)
		if got == key {
			t.Errorf("%s rendered as its own key — the framework floor is missing", key)
		}
	}
}

func TestAnAppCanOverrideAFrameworkString(t *testing.T) {
	fsys := fstest.MapFS{
		"locales/en-US.json": &fstest.MapFile{Data: []byte(`{"ui.a11y.close":"Dismiss"}`)},
	}
	c, err := Load(fsys, "en-US")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := c.T(context.Background(), "ui.a11y.close"); got != "Dismiss" {
		t.Errorf("the app's wording lost to the framework's: %q", got)
	}
}

func TestAMissingKeyIsStillTheKey(t *testing.T) {
	c := appCatalog(t)
	if got := c.T(context.Background(), "nothing.defines.this"); got != "nothing.defines.this" {
		t.Errorf("a genuinely missing key stopped reporting itself: %q", got)
	}
}
