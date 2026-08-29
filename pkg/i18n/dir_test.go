package i18n

import (
	"encoding/json"
	"testing"
)

func TestDir(t *testing.T) {
	cases := map[string]Direction{
		// The common right-to-left languages, bare and with regions.
		"ar":         RTL,
		"ar-EG":      RTL,
		"he":         RTL,
		"he-IL":      RTL,
		"fa-IR":      RTL,
		"ur":         RTL,
		"AR-eg":      RTL, // tags are case-insensitive
		"  ar-EG  ":  RTL, // and may arrive padded from a header
		"ar-Arab-EG": RTL,

		// Left-to-right, including a language whose name starts like an RTL one.
		"en":    LTR,
		"en-US": LTR,
		"es-ES": LTR,
		"arn":   LTR, // Mapudungun, not Arabic
		"hey":   LTR,
		"":      LTR,
		"xx-YY": LTR,

		// An explicit script settles it, whichever way the language leans.
		"ku-Arab": RTL,
		"ku-Latn": LTR,
		"az-Arab": RTL,
		"az-Latn": LTR,
		"sr-Cyrl": LTR,
	}
	for tag, want := range cases {
		if got := Dir(tag); got != want {
			t.Errorf("Dir(%q) = %q, want %q", tag, got, want)
		}
	}
}

// TestFrameworkCatalogIsComplete checks the framework's own strings resolve.
//
// It derives the key set from catalog.json rather than listing it, because a
// hardcoded list is a second copy that goes stale the moment a key is added and
// then silently stops covering anything. Key-to-source agreement is `beach i18n`'s
// job (it scans for literal T calls); what a test can add is that every key the
// catalog declares actually has a translation behind it — a missing one is not
// fatal at runtime, which is exactly why it needs catching here: the page would
// ship reading "ui.a11y.close" and nothing would break.
func TestFrameworkCatalogIsComplete(t *testing.T) {
	b, err := builtin.ReadFile("catalog.json")
	if err != nil {
		t.Fatalf("read catalog.json: %v", err)
	}
	var entries map[string]struct {
		Label   string `json:"label"`
		Comment string `json:"comment"`
	}
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatalf("catalog.json: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("catalog.json is empty")
	}
	for key, e := range entries {
		if e.Label == "" {
			t.Errorf("%s has no reference label", key)
		}
		if e.Comment == "" {
			t.Errorf("%s has no translator comment — a translator cannot place a bare string", key)
		}
		msg, ok := builtinCat.Lookup("en-US", key)
		if !ok || msg == key {
			t.Errorf("%s is in catalog.json but has no en-US translation", key)
		}
		if msg != e.Label {
			t.Errorf("%s: en-US says %q, catalog.json says %q", key, msg, e.Label)
		}
	}
}
