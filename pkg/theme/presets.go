package theme

// The shipped presets.
//
// A preset is a dozen numbers, not a list of colours — the whole palette falls
// out of [Build]. That makes them cheap to write, cheap to review in a diff, and
// impossible to get into a state where one member fails contrast while the rest
// pass. It also makes them the natural thing for the studio's gallery to be a
// gallery *of*: choosing from a wall of finished themes is a far better fit for
// how people actually pick a look than steering ten sliders toward one.
//
// The hue vocabulary, in OKLCH degrees: 25 red, 55 orange, 90 yellow, 145 green,
// 195 cyan, 250 blue, 285 violet, 330 magenta.

// Preset is a named parameter set with a note explaining what it is for.
type Preset struct {
	Key   string
	Title string
	Note  string
	Params
}

// darkLadder and lightLadder are the two surface ladders every preset starts
// from. They are shared rather than repeated because the *ladder* is the part
// that is hard to get right, and the part a preset almost never needs to vary —
// what distinguishes one preset from another is its hues.
//
// The dark ladder is deep but not black: a page at L=0.16 leaves room for a card
// to sit above it and a raised panel above that, which is the separation a pure
// #000 page cannot offer. The light ladder inverts the *relationship* rather
// than the numbers — a soft ground with white cards raised off it, which is what
// light interfaces actually do.
func darkLadder(neutralChroma float64) SchemeParams {
	return SchemeParams{
		Dark:          true,
		PaperL:        0.16,
		PanelL:        0.215,
		PanelHoverL:   0.27,
		LineSoftL:     0.30,
		FgStrongL:     0.985,
		FgDefaultL:    0.94,
		NeutralChroma: neutralChroma,
		SeriesL:       0.72,
	}
}

func lightLadder(neutralChroma float64) SchemeParams {
	return SchemeParams{
		Dark:        false,
		PaperL:      0.975, // a soft ground…
		PanelL:      1.0,   // …with white cards raised off it
		PanelHoverL: 0.945,
		LineSoftL:   0.905,
		FgStrongL:   0.17,
		FgDefaultL:  0.28,
		// Light surfaces show a tint far more readily than dark ones, so the
		// same whisper of chroma that reads as warm on a dark page reads as
		// dirty on a light one. Two thirds is about where that turns over.
		NeutralChroma: neutralChroma * 0.65,
		SeriesL:       0.58,
	}
}

// params assembles a preset's parameters from its hues.
func params(neutralHue, neutralChroma, accent, cat2, cat3, chromaPct float64) Params {
	return Params{
		NeutralHue: neutralHue,
		AccentHue:  accent,
		Accent2Hue: cat2,
		Accent3Hue: cat3,

		// Conventional, and deliberately not a per-preset choice: a visitor
		// reads green as good before they read anything else on the page.
		GoodHue: 145,
		WarnHue: 85,
		BadHue:  27,
		InfoHue: 245,

		ChromaPct: chromaPct,
		WashAlpha: 0.18,

		Dark:  darkLadder(neutralChroma),
		Light: lightLadder(neutralChroma),
	}
}

// Presets is every shipped preset, in a stable order. Range over this, never a
// map: gallery order and generated output both have to be deterministic.
var Presets = []Preset{
	{
		Key: "driftwood", Title: "Driftwood",
		Note:   "The default. Sun-bleached warm neutrals under a tidepool accent.",
		Params: params(70, 0.006, 195, 35, 300, 0.85),
	},
	{
		Key: "tidepool", Title: "Tidepool",
		Note:   "Cooler ground, deeper water. The accent goes green rather than blue.",
		Params: params(200, 0.007, 165, 25, 290, 0.9),
	},
	{
		Key: "lifeguard", Title: "Lifeguard",
		Note:   "Warm neutrals, a red-orange accent, and no apology for it.",
		Params: params(55, 0.008, 30, 195, 275, 0.95),
	},
	{
		Key: "nightswim", Title: "Night Swim",
		Note:   "Cold, quiet, low-chroma. The nearest thing here to the old palette.",
		Params: params(250, 0.008, 250, 190, 320, 0.6),
	},
	{
		Key: "seaglass", Title: "Sea Glass",
		Note:   "Barely-there neutrals; all the colour lives in the accent and the roles.",
		Params: params(160, 0.003, 175, 45, 305, 0.8),
	},
	{
		Key: "boardwalk", Title: "Boardwalk",
		Note:   "Weathered timber and a fairground yellow.",
		Params: params(60, 0.010, 90, 250, 20, 0.95),
	},
	{
		Key: "redtide", Title: "Red Tide",
		Note:   "A magenta accent on a neutral that leans plum. Loud on purpose.",
		Params: params(340, 0.007, 330, 165, 60, 1.0),
	},
	{
		Key: "kelp", Title: "Kelp",
		Note:   "Deep green ground, olive accent. The most subdued preset here.",
		Params: params(140, 0.009, 145, 35, 265, 0.7),
	},
	{
		Key: "harbor", Title: "Harbor",
		Note:   "Slate neutrals, signal orange. Reads like instrumentation.",
		Params: params(230, 0.006, 55, 200, 290, 0.9),
	},
	{
		Key: "oyster", Title: "Oyster",
		Note:   "Near-achromatic, faintly violet, with a restrained blue accent.",
		Params: params(290, 0.004, 265, 30, 155, 0.65),
	},
	{
		Key: "duneglass", Title: "Dune Glass",
		Note:   "Sand neutrals and a violet accent — the least literal of the set.",
		Params: params(75, 0.008, 290, 175, 40, 0.85),
	},
	{
		Key: "saltmarsh", Title: "Salt Marsh",
		Note:   "Grey-green ground, muted cyan. Quiet without being cold.",
		Params: params(120, 0.005, 205, 40, 315, 0.72),
	},
}

// ByKey returns a preset by key.
func ByKey(key string) (Preset, bool) {
	for _, p := range Presets {
		if p.Key == key {
			return p, true
		}
	}
	return Preset{}, false
}

// Default is the preset a framework with no configuration ships.
const Default = "driftwood"

// BuildPreset derives a preset by key.
func BuildPreset(key string) (Theme, error) {
	p, ok := ByKey(key)
	if !ok {
		return Theme{}, &UnknownPresetError{Key: key}
	}
	t, err := Build(p.Params)
	if err != nil {
		return Theme{}, err
	}
	t.Preset = p
	return t, nil
}

// UnknownPresetError names a preset that is not in [Presets].
type UnknownPresetError struct{ Key string }

func (e *UnknownPresetError) Error() string {
	return "theme: unknown preset " + e.Key
}
