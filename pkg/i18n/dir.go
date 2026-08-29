package i18n

import "strings"

// Direction is a document's base writing direction, as the HTML dir attribute
// spells it.
type Direction string

const (
	// LTR is left-to-right, the default for every language not listed in
	// rtlLanguages.
	LTR Direction = "ltr"
	// RTL is right-to-left.
	RTL Direction = "rtl"
)

// rtlLanguages is the set of primary language subtags written right-to-left.
// It is a fixed list rather than a Unicode script lookup on purpose: the set is
// small, stable, and knowing it costs nothing, whereas resolving a tag to its
// script would pull in a CLDR table for a handful of answers.
//
// Tags are matched on the primary subtag only, so "ar", "ar-EG" and "ar-Arab-EG"
// all resolve to RTL. The exception is a tag that names a script explicitly —
// see Dir, which honours "-Latn" (Kurdish and Azerbaijani are written in both
// directions depending on script).
var rtlLanguages = map[string]bool{
	"ar":  true, // Arabic
	"arc": true, // Aramaic
	"ckb": true, // Central Kurdish (Sorani)
	"dv":  true, // Divehi
	"fa":  true, // Persian
	"he":  true, // Hebrew
	"ks":  true, // Kashmiri
	"ku":  true, // Kurdish
	"nqo": true, // N'Ko
	"ps":  true, // Pashto
	"sd":  true, // Sindhi
	"ug":  true, // Uyghur
	"ur":  true, // Urdu
	"yi":  true, // Yiddish
}

// rtlScripts is the set of right-to-left script subtags. A tag that names one
// of these is RTL whatever its language is, which is how a language written in
// more than one script resolves correctly ("az-Arab" is RTL, "az" is not).
var rtlScripts = map[string]bool{
	"adlm": true,
	"arab": true,
	"hebr": true,
	"nkoo": true,
	"rohg": true,
	"syrc": true,
	"thaa": true,
	"yezi": true,
}

// Dir returns the base writing direction for a BCP 47 language tag. An unknown
// or empty tag is LTR, which is the safe default: getting direction wrong on a
// left-to-right page is a visible bug, while a missing dir on a right-to-left
// page is one too — but only RTL locales can be identified, so LTR is what
// everything else gets.
//
// An explicit script subtag wins over the language: "ku-Arab" is RTL and
// "ku-Latn" is LTR, even though "ku" alone is RTL.
func Dir(tag string) Direction {
	if tag == "" {
		return LTR
	}
	parts := strings.Split(strings.ToLower(strings.TrimSpace(tag)), "-")
	// A script subtag is exactly four letters and, when present, is the second
	// subtag. It settles the question on its own.
	if len(parts) > 1 && len(parts[1]) == 4 {
		if rtlScripts[parts[1]] {
			return RTL
		}
		return LTR
	}
	if rtlLanguages[parts[0]] {
		return RTL
	}
	return LTR
}
