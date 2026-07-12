// Package speech turns plain-English text into ARPABET phonemes for klattsch,
// the browser-side formant synth that gives the Muskie its voice. klattsch
// speaks phonemes, not words, and ships no text front end — so this is that
// front end: a CMU Pronouncing Dictionary lookup (the bulk of the work) with a
// compact letter-to-sound fallback for words the dictionary doesn't know.
//
// The output token vocabulary is exactly klattsch's English bank
// (klatt1980-en): the 39 standard ARPABET symbols with CMU stress digits
// stripped (klattsch carries stress separately), plus "." and "," which its
// sequencer reads as sentence and clause pauses.
package speech

import (
	"bufio"
	"io"
	"strings"
	"unicode"
)

// G2P holds the parsed pronunciation dictionary. Build one with New (or Default
// for the embedded dictionary); it is read-only after construction and safe for
// concurrent use.
type G2P struct {
	dict map[string][]string // UPPERCASE word -> stress-stripped phonemes
}

// New parses a CMU-format dictionary (one entry per line: "word P H ON EM ES",
// '#' starting a comment, "word(2)" marking alternate pronunciations). Only the
// first pronunciation of each word is kept and stress digits are stripped.
func New(dict io.Reader) (*G2P, error) {
	g := &G2P{dict: make(map[string][]string, 140000)}
	sc := bufio.NewScanner(dict)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i] // strip trailing comment
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		word := fields[0]
		if strings.IndexByte(word, '(') >= 0 {
			continue // alternate pronunciation, e.g. "the(2)"; keep the primary
		}
		word = strings.ToUpper(word)
		if _, seen := g.dict[word]; seen {
			continue
		}
		phs := make([]string, 0, len(fields)-1)
		for _, p := range fields[1:] {
			phs = append(phs, stripStress(p))
		}
		g.dict[word] = phs
	}
	return g, sc.Err()
}

// Phonemes converts free text into a klattsch phoneme string: words become
// space-separated ARPABET, sentence punctuation (. ! ?) becomes ".", and clause
// punctuation (, ; :) becomes "," — both of which klattsch renders as pauses.
func (g *G2P) Phonemes(text string) string {
	var (
		out  []string
		word []rune
	)
	pause := func(tok string) {
		if len(out) == 0 {
			return // no leading pause
		}
		if last := out[len(out)-1]; last == "." || last == "," {
			return // don't stack pauses
		}
		out = append(out, tok)
	}
	flush := func() {
		if len(word) == 0 {
			return
		}
		out = append(out, g.word(string(word))...)
		word = word[:0]
	}
	for _, r := range text {
		switch {
		case unicode.IsLetter(r):
			word = append(word, r)
		case r == '\'' || r == '’':
			word = append(word, '\'') // keep apostrophes (e.g. river's, don't)
		case r == '.' || r == '!' || r == '?':
			flush()
			pause(".")
		case r == ',' || r == ';' || r == ':':
			flush()
			pause(",")
		default:
			flush()
		}
	}
	flush()
	for len(out) > 0 { // trim any trailing pause
		if last := out[len(out)-1]; last == "." || last == "," {
			out = out[:len(out)-1]
			continue
		}
		break
	}
	return strings.Join(out, " ")
}

// word resolves a single word to phonemes: exact dictionary hit, then a hit
// with surrounding apostrophes trimmed (quoted words), then the rule fallback.
func (g *G2P) word(w string) []string {
	up := strings.ToUpper(w)
	if ph, ok := g.dict[up]; ok {
		return ph
	}
	if t := strings.Trim(up, "'"); t != up {
		if ph, ok := g.dict[t]; ok {
			return ph
		}
	}
	return fallback(strings.ToLower(w))
}

// stripStress removes a trailing CMU stress digit (AH0 -> AH); consonants,
// which carry no digit, pass through unchanged.
func stripStress(p string) string {
	if n := len(p); n > 0 && p[n-1] >= '0' && p[n-1] <= '9' {
		return p[:n-1]
	}
	return p
}

// --- letter-to-sound fallback ------------------------------------------------
//
// A deliberately small English spelling-to-sound approximation for words the
// dictionary lacks. It is not linguistically exact — most real-word vocabulary
// hits the 135k-entry dictionary, so this only catches the rare miss, and a
// roughly-right robotic pronunciation suits the Muskie fine.

// multigraphs are matched front-first, longest patterns earlier, before any
// single-letter rule. An empty phoneme slice means the spelling is silent.
var multigraphs = []struct {
	pat string
	ph  []string
}{
	{"tion", []string{"SH", "AH", "N"}},
	{"sion", []string{"ZH", "AH", "N"}},
	{"ough", []string{"AO"}},
	{"igh", []string{"AY"}},
	{"tch", []string{"CH"}},
	{"dge", []string{"JH"}},
	{"qu", []string{"K", "W"}},
	{"ch", []string{"CH"}},
	{"sh", []string{"SH"}},
	{"th", []string{"TH"}},
	{"ph", []string{"F"}},
	{"wh", []string{"W"}},
	{"ck", []string{"K"}},
	{"ng", []string{"NG"}},
	{"gh", []string{}}, // silent (e.g. "night")
	{"kn", []string{"N"}},
	{"wr", []string{"R"}},
	{"ee", []string{"IY"}},
	{"ea", []string{"IY"}},
	{"oo", []string{"UW"}},
	{"ou", []string{"AW"}},
	{"ow", []string{"AW"}},
	{"oa", []string{"OW"}},
	{"oe", []string{"OW"}},
	{"ai", []string{"EY"}},
	{"ay", []string{"EY"}},
	{"ei", []string{"EY"}},
	{"ey", []string{"EY"}},
	{"oi", []string{"OY"}},
	{"oy", []string{"OY"}},
	{"au", []string{"AO"}},
	{"aw", []string{"AO"}},
	{"ie", []string{"IY"}},
	{"ue", []string{"UW"}},
	{"ui", []string{"UW"}},
}

// singles maps a lone consonant or simple vowel to a phoneme. The
// context-sensitive letters (c, x, y, e) are handled in matchAt.
var singles = map[byte][]string{
	'a': {"AE"}, 'b': {"B"}, 'd': {"D"}, 'f': {"F"}, 'g': {"G"},
	'h': {"HH"}, 'i': {"IH"}, 'j': {"JH"}, 'k': {"K"}, 'l': {"L"},
	'm': {"M"}, 'n': {"N"}, 'o': {"AA"}, 'p': {"P"}, 'q': {"K"},
	'r': {"R"}, 's': {"S"}, 't': {"T"}, 'u': {"AH"}, 'v': {"V"},
	'w': {"W"}, 'z': {"Z"},
}

// fallback approximates phonemes for an unknown word. Input is lowercased; any
// non a-z runes are dropped first. An empty result becomes a single schwa so
// the Muskie always makes a sound.
func fallback(w string) []string {
	var b []byte
	for i := 0; i < len(w); i++ {
		if c := w[i]; c >= 'a' && c <= 'z' {
			b = append(b, c)
		}
	}
	s := string(b)
	if s == "" {
		return []string{"AH"}
	}
	var out []string
	for i := 0; i < len(s); {
		adv, ph := matchAt(s, i)
		out = append(out, ph...)
		i += adv
	}
	if len(out) == 0 {
		return []string{"AH"}
	}
	return out
}

// matchAt returns how many letters to consume at position i and the phonemes
// they produce. s is guaranteed lowercase a-z.
func matchAt(s string, i int) (int, []string) {
	rest := s[i:]
	for _, g := range multigraphs {
		if strings.HasPrefix(rest, g.pat) {
			return len(g.pat), g.ph
		}
	}
	switch c := s[i]; c {
	case 'c':
		if i+1 < len(s) && strings.IndexByte("eiy", s[i+1]) >= 0 {
			return 1, []string{"S"} // soft c: cent, city, cycle
		}
		return 1, []string{"K"}
	case 'x':
		return 1, []string{"K", "S"}
	case 'y':
		if i == 0 {
			return 1, []string{"Y"} // consonantal y: yes
		}
		return 1, []string{"IH"} // vocalic y: happy
	case 'e':
		if i == len(s)-1 && len(s) > 3 {
			return 1, nil // silent final e: hope, large
		}
		return 1, []string{"EH"}
	default:
		if ph, ok := singles[c]; ok {
			return 1, ph
		}
		return 1, nil
	}
}
