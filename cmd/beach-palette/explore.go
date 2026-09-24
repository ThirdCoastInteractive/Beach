package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/ThirdCoastInteractive/Beach/pkg/beach/view"
	"github.com/ThirdCoastInteractive/Beach/pkg/theme"
)

// The theme explorer: `beach-palette -serve`.
//
// Judging a palette from hex values does not work, and judging it from a strip
// of swatches barely works either — a colour is only judgeable in the shapes it
// will be painted on. So this serves the derivation live against a real slice of
// UI, in both schemes at once.
//
// The two ways in are deliberate. Sliders expose *parameters* when what a person
// has is a preference about *outcomes*, and a dozen parameters is a search with
// no gradient to follow. So the front door is a gallery of finished themes — you
// pick, you do not configure — and the second door is a hue wheel, where the
// three chromatic choices are positions on a circle rather than numbers in
// boxes. The raw parameters are still there, behind a disclosure, for when you
// know exactly which one you want to move.

func serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", pageHandler)
	mux.HandleFunc("/static/js/explore.js", scriptHandler)
	mux.HandleFunc("/api/theme", themeHandler)
	mux.HandleFunc("/api/gallery", galleryHandler)

	fmt.Printf("beach-palette: exploring on http://localhost%s\n", addr)
	srv := &http.Server{Addr: addr, Handler: mux, ErrorLog: log.Default()}
	return srv.ListenAndServe()
}

// paramsFromQuery reads a theme.Params out of the request: a preset as the base,
// then any field the query overrides. A partial query is a valid theme, which is
// what lets the gallery hand off to the wheel without serialising everything.
func paramsFromQuery(r *http.Request) (theme.Preset, theme.Params) {
	q := r.URL.Query()
	key := q.Get("preset")
	base, ok := theme.ByKey(key)
	if !ok {
		base, _ = theme.ByKey(view.ThemePreset)
	}
	p := base.Params

	num := func(name string, dst *float64) {
		if v, err := strconv.ParseFloat(q.Get(name), 64); err == nil {
			*dst = v
		}
	}
	num("nhue", &p.NeutralHue)
	num("accent", &p.AccentHue)
	num("accent2", &p.Accent2Hue)
	num("accent3", &p.Accent3Hue)
	num("good", &p.GoodHue)
	num("warn", &p.WarnHue)
	num("bad", &p.BadHue)
	num("info", &p.InfoHue)
	num("chroma", &p.ChromaPct)
	num("wash", &p.WashAlpha)
	num("nchroma", &p.Dark.NeutralChroma)
	num("paperL", &p.Dark.PaperL)
	num("panelL", &p.Dark.PanelL)
	num("hoverL", &p.Dark.PanelHoverL)
	num("lightPaperL", &p.Light.PaperL)
	num("lightPanelL", &p.Light.PanelL)

	// The light ladder's tint tracks the dark one, as it does in the presets:
	// a light surface shows a tint far more readily, so the same whisper reads
	// as dirty rather than warm.
	if q.Get("nchroma") != "" {
		p.Light.NeutralChroma = p.Dark.NeutralChroma * 0.65
	}
	return base, p
}

// reply is one derived theme as the page consumes it. A derivation that cannot
// satisfy an obligation reports Error and no tokens — the explorer shows the
// reason rather than a blank panel, because "this hue has no legal accent here"
// is the single most useful thing to learn while choosing one.
type reply struct {
	Key   string             `json:"key"`
	Title string             `json:"title"`
	Note  string             `json:"note"`
	Dark  map[string]string  `json:"dark,omitempty"`
	Light map[string]string  `json:"light,omitempty"`
	Hues  map[string]float64 `json:"hues,omitempty"`
	Go    string             `json:"go,omitempty"`
	Error string             `json:"error,omitempty"`
}

func derive(base theme.Preset, p theme.Params) reply {
	rep := reply{Key: base.Key, Title: base.Title, Note: base.Note}
	t, err := theme.Build(p)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	rep.Dark = t.Dark.Tokens()
	rep.Light = t.Light.Tokens()
	rep.Hues = map[string]float64{
		"accent": p.AccentHue, "accent2": p.Accent2Hue, "accent3": p.Accent3Hue,
		"neutral": p.NeutralHue,
	}
	rep.Go = goLiteral(base.Key, p)
	return rep
}

// goLiteral renders the current settings as the edit that would ship them. The
// explorer's whole job is to close the gap between a colour chosen by eye and
// the code that regenerates it, so the two cannot disagree.
func goLiteral(key string, p theme.Params) string {
	return fmt.Sprintf(`// pkg/beach/view/view.go
const ThemePreset = %q

// pkg/theme/presets.go — params(neutralHue, neutralChroma, accent, cat2, cat3, chromaPct)
params(%g, %g, %g, %g, %g, %g)

// role hues: good %g, warn %g, bad %g, info %g
// wash alpha: %g`,
		key,
		p.NeutralHue, p.Dark.NeutralChroma, p.AccentHue, p.Accent2Hue, p.Accent3Hue, p.ChromaPct,
		p.GoodHue, p.WarnHue, p.BadHue, p.InfoHue, p.WashAlpha)
}

func themeHandler(w http.ResponseWriter, r *http.Request) {
	base, p := paramsFromQuery(r)
	writeJSON(w, derive(base, p))
}

// galleryHandler derives every shipped preset. It is the front door: a wall of
// finished themes to choose from, rather than a parameter space to search.
func galleryHandler(w http.ResponseWriter, r *http.Request) {
	out := make([]reply, 0, len(theme.Presets))
	for _, p := range theme.Presets {
		out = append(out, derive(p, p.Params))
	}
	writeJSON(w, out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func scriptHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, exploreJS)
}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A plain substitution, not Fprintf: the page is mostly CSS, and CSS is full
	// of percent signs a format string would try to read as verbs.
	fmt.Fprint(w, strings.Replace(explorePage, presetSlot, presetOptions(), 1))
}

func presetOptions() string {
	var b strings.Builder
	for _, p := range theme.Presets {
		sel := ""
		if p.Key == view.ThemePreset {
			sel = " selected"
		}
		fmt.Fprintf(&b, "<option value=%q%s>%s</option>\n", p.Key, sel, p.Title)
	}
	return b.String()
}
