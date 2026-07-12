// Package quotes is askmuskie's flat-file wisdom loader: the Muskie's entire
// repertoire is a plain-text file (data/quotes.txt) parsed into a Set the HTTP
// layer draws from at random. No database, no LLM — the oracle only knows what
// the file says, and the file is meant to be edited by hand.
//
// File format (deliberately trivial to fill):
//
//	# lines beginning with '#' are comments, ignored
//
//	Still water holds the deepest fish.
//	— Muskie
//
//	The river does not hurry, yet everything arrives.
//
// Quotes are separated by blank lines. Within a quote, a line beginning with an
// em dash (—), en dash (–), or a double hyphen (--) is the attribution; every
// other non-comment line is part of the quote body (joined with spaces). A
// stanza with no body is skipped, so stray blank lines are harmless.
package quotes

import (
	"bufio"
	"io"
	"math/rand"
	"strings"
	"sync"
)

// Quote is one piece of wisdom: the saying and an optional attribution
// (Attribution is "" when the stanza had no dash line).
type Quote struct {
	Text        string
	Attribution string
}

// Parse reads quotes.txt-format content into a slice in file order. It never
// fails on content — malformed stanzas are simply skipped — so the only error
// it can surface is an I/O error from the reader.
func Parse(r io.Reader) ([]Quote, error) {
	var (
		out  []Quote
		body []string
		attr string
	)
	flush := func() {
		text := strings.TrimSpace(strings.Join(body, " "))
		if text != "" {
			out = append(out, Quote{Text: text, Attribution: attr})
		}
		body = body[:0]
		attr = ""
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			flush() // blank line ends the current stanza
		case strings.HasPrefix(trimmed, "#"):
			continue // comment
		case isAttribution(trimmed):
			attr = strings.TrimSpace(stripDash(trimmed))
		default:
			body = append(body, trimmed)
		}
	}
	flush() // final stanza (no trailing blank line)
	return out, sc.Err()
}

// isAttribution reports whether a line is a dash-led attribution line.
func isAttribution(s string) bool {
	return strings.HasPrefix(s, "—") || strings.HasPrefix(s, "–") || strings.HasPrefix(s, "--")
}

// stripDash removes the leading dash marker from an attribution line.
func stripDash(s string) string {
	for _, p := range []string{"—", "–", "--"} {
		if strings.HasPrefix(s, p) {
			return strings.TrimPrefix(s, p)
		}
	}
	return s
}

// Set is a concurrency-safe bag of quotes with a random picker that avoids
// repeating the quote it returned last (so consecutive asks feel varied).
type Set struct {
	quotes []Quote

	mu   sync.Mutex
	last int // index of the most recent pick, or -1 before the first
}

// NewSet wraps a parsed slice. An empty slice is allowed; Random reports it.
func NewSet(quotes []Quote) *Set {
	return &Set{quotes: quotes, last: -1}
}

// Len reports how many quotes are loaded.
func (s *Set) Len() int { return len(s.quotes) }

// Random returns a quote at random, never the same one twice in a row (unless
// only one is loaded). The bool is false only when the set is empty.
func (s *Set) Random() (Quote, bool) {
	if len(s.quotes) == 0 {
		return Quote{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.quotes) == 1 {
		s.last = 0
		return s.quotes[0], true
	}
	i := rand.Intn(len(s.quotes))
	if i == s.last {
		i = (i + 1) % len(s.quotes) // shift off the repeat
	}
	s.last = i
	return s.quotes[i], true
}
