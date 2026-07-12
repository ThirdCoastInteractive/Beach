package speech

import (
	"strings"
	"testing"
)

// TestDefaultDictionary exercises the real embedded CMU dictionary end to end:
// it must parse, resolve common words, and emit only valid klattsch tokens.
func TestDefaultDictionary(t *testing.T) {
	g, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if len(g.dict) < 100000 {
		t.Fatalf("embedded dictionary looks short: %d entries", len(g.dict))
	}

	// "muskie" really is in CMUdict — the oracle should pronounce its own name.
	if got := strings.Join(g.dict["MUSKIE"], " "); got != "M AH S K IY" {
		t.Errorf("MUSKIE = %q, want %q", got, "M AH S K IY")
	}

	got := g.Phonemes("The river does not hurry, yet everything arrives.")
	if strings.TrimSpace(got) == "" {
		t.Fatal("no phonemes for a normal sentence")
	}
	if !strings.Contains(got, ",") || !strings.HasSuffix(got, "V Z") {
		t.Errorf("unexpected rendering: %q", got)
	}
	for _, tok := range strings.Fields(got) {
		if !validToken[tok] {
			t.Errorf("invalid token %q in %q", tok, got)
		}
	}
}
