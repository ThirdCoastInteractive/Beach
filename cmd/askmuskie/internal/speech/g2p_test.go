package speech

import (
	"strings"
	"testing"
)

// miniDict mirrors the CMU format: comments, stress digits, and an alternate.
const miniDict = `;;; a header-ish comment line
hello HH AH0 L OW1
the DH AH0
the(2) DH IY0  # alternate pronunciation, must be ignored
river R IH1 V ER0
still S T IH1 L
`

func newG2P(t *testing.T) *G2P {
	t.Helper()
	g, err := New(strings.NewReader(miniDict))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

func TestDictStripsStressAndAlternates(t *testing.T) {
	g := newG2P(t)
	if got := strings.Join(g.dict["HELLO"], " "); got != "HH AH L OW" {
		t.Errorf("HELLO = %q, want %q", got, "HH AH L OW")
	}
	// The primary "the" is kept; the(2) alternate is dropped.
	if got := strings.Join(g.dict["THE"], " "); got != "DH AH" {
		t.Errorf("THE = %q, want %q (alternate should be ignored)", got, "DH AH")
	}
}

func TestPhonemesSentence(t *testing.T) {
	g := newG2P(t)
	got := g.Phonemes("The river. Still!")
	want := "DH AH R IH V ER . S T IH L"
	if got != want {
		t.Errorf("Phonemes = %q, want %q", got, want)
	}
}

func TestPhonemesClausePause(t *testing.T) {
	g := newG2P(t)
	got := g.Phonemes("the, river")
	if got != "DH AH , R IH V ER" {
		t.Errorf("clause pause = %q", got)
	}
}

func TestPhonemesCaseAndApostrophe(t *testing.T) {
	g := newG2P(t)
	// Uppercase + a curly-quoted word should still resolve via the dictionary.
	if got := g.Phonemes("“THE”"); got != "DH AH" {
		t.Errorf("quoted/upper = %q, want %q", got, "DH AH")
	}
}

func TestOOVFallbackIsValid(t *testing.T) {
	g := newG2P(t)
	// "muskie" isn't in the mini dict, so it exercises the fallback.
	got := g.Phonemes("muskie")
	if strings.TrimSpace(got) == "" {
		t.Fatal("fallback produced no phonemes for an OOV word")
	}
	for _, tok := range strings.Fields(got) {
		if !validToken[tok] {
			t.Errorf("fallback emitted invalid token %q (full: %q)", tok, got)
		}
	}
}

func TestFallbackNeverEmpty(t *testing.T) {
	if got := fallback(""); len(got) == 0 {
		t.Error("fallback(\"\") should still grunt something")
	}
	if got := fallback("123"); len(got) == 0 {
		t.Error("fallback of non-letters should still grunt something")
	}
}

// validToken is klattsch's English bank vocabulary (klatt1980-en): the 39
// ARPABET symbols plus the pause tokens. Any token the front end emits must be
// one of these or klattsch drops it.
var validToken = func() map[string]bool {
	m := map[string]bool{".": true, ",": true}
	for _, p := range strings.Fields(
		"AA AE AH AO AW AY B CH D DH EH ER EY F G HH IH IY JH K L M N " +
			"NG OW OY P R S SH T TH UH UW V W Y Z ZH") {
		m[p] = true
	}
	return m
}()
