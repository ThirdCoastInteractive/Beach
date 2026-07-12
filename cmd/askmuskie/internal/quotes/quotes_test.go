package quotes

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	const src = `# a leading comment, ignored

Still water holds the deepest fish.
— Muskie

The river does not hurry,
yet everything arrives.
-- An old angler

# comment between stanzas
A quote with no attribution.


`
	got, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Quote{
		{Text: "Still water holds the deepest fish.", Attribution: "Muskie"},
		{Text: "The river does not hurry, yet everything arrives.", Attribution: "An old angler"},
		{Text: "A quote with no attribution.", Attribution: ""},
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d quotes, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("quote %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseEmpty(t *testing.T) {
	got, err := Parse(strings.NewReader("\n\n#only a comment\n\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 quotes from comment-only input, got %d", len(got))
	}
}

func TestRandomEmpty(t *testing.T) {
	if _, ok := NewSet(nil).Random(); ok {
		t.Fatal("Random on empty set should report ok=false")
	}
}

func TestRandomSingle(t *testing.T) {
	s := NewSet([]Quote{{Text: "only one"}})
	for range 5 {
		q, ok := s.Random()
		if !ok || q.Text != "only one" {
			t.Fatalf("single-quote set returned %+v ok=%v", q, ok)
		}
	}
}

func TestRandomNoImmediateRepeat(t *testing.T) {
	s := NewSet([]Quote{{Text: "a"}, {Text: "b"}, {Text: "c"}})
	prev, _ := s.Random()
	for range 200 {
		q, ok := s.Random()
		if !ok {
			t.Fatal("ok=false on non-empty set")
		}
		if q.Text == prev.Text {
			t.Fatalf("Random repeated %q back to back", q.Text)
		}
		prev = q
	}
}
