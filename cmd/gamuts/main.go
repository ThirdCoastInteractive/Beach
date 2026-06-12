// Command gamuts previews every RYBitten gamut so you can pick a palette by eye.
//
// Run with no arguments to serve an interactive browser on :7777 — each gamut
// renders its smooth hue wheel, a 12-color qualitative palette (what the chart
// tokens become), and its eight named cube corners, all on the framework's dark
// surface so you judge them in context.
//
//	go run ./cmd/gamuts            # serve the browser at http://localhost:7777
//	go run ./cmd/gamuts -css itten # print the :root token block for one gamut
//
// The -css output is exactly what wires a chosen gamut into the theme: drop it
// over the --chart-qualitative tokens in internal/view/css/input.css.
package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/ThirdCoastInteractive/the-beach/rybitten"
)

func main() {
	addr := flag.String("addr", ":7777", "address to serve the gamut browser on")
	css := flag.String("css", "", "print the :root chart-token block for this gamut key and exit")
	accent := flag.String("accent", "", "print the :root --color-sky-* accent overrides for this gamut key and exit")
	count := flag.Int("count", 12, "number of qualitative tokens to generate for -css")
	flag.Parse()

	if key := *css; key != "" {
		g, ok := rybitten.Cubes[key]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown gamut %q; known: %s\n", key, strings.Join(rybitten.Keys, ", "))
			os.Exit(2)
		}
		fmt.Print(rybitten.CSSVars(g, *count))
		return
	}

	if key := *accent; key != "" {
		g, ok := rybitten.Cubes[key]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown gamut %q; known: %s\n", key, strings.Join(rybitten.Keys, ", "))
			os.Exit(2)
		}
		fmt.Print(rybitten.AccentVars(g))
		return
	}

	http.HandleFunc("/", browse)
	log.Printf("gamut browser → http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// cornerNames label the eight cube corners in canonical order.
var cornerNames = [8]string{"white", "red", "yellow", "orange", "blue", "violet", "green", "black"}

type swatch struct {
	Hex   string
	Label string
}

type card struct {
	Key, Title, Author string
	Year               int
	Wheel              template.CSS // a linear-gradient across the hue wheel
	Qualitative        []swatch     // the 12 series colors
	Corners            []swatch     // the eight cube corners
}

func buildCards() []card {
	cards := make([]card, 0, len(rybitten.Keys))
	for _, k := range rybitten.Keys {
		g := rybitten.Cubes[k]

		// Smooth hue wheel as a CSS gradient sampled finely across the cube.
		wheel := rybitten.Hexes(rybitten.Palette(g.Cube, 64, 1.0, 0.5))
		grad := "linear-gradient(90deg," + strings.Join(wheel, ",") + ")"

		qual := make([]swatch, 0, 12)
		for i, h := range rybitten.Hexes(rybitten.QualitativePalette(g.Cube, 12)) {
			qual = append(qual, swatch{Hex: h, Label: fmt.Sprintf("q%d", i+1)})
		}

		corners := make([]swatch, 8)
		for i, c := range g.Cube {
			corners[i] = swatch{Hex: c.Hex(), Label: cornerNames[i]}
		}

		cards = append(cards, card{
			Key: g.Key, Title: g.Title, Author: g.Author, Year: g.Year,
			Wheel: template.CSS(grad), Qualitative: qual, Corners: corners,
		})
	}
	return cards
}

func browse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(w, buildCards()); err != nil {
		log.Println(err)
	}
}

var page = template.Must(template.New("gamuts").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>RYBitten gamuts</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body { margin: 0; background: #09090b; color: #e4e4e7;
    font: 15px/1.5 ui-sans-serif, system-ui, sans-serif; padding: 2rem; }
  h1 { font-size: 1.5rem; margin: 0 0 .25rem; }
  .lede { color: #a1a1aa; margin: 0 0 2rem; max-width: 60ch; }
  .grid { display: grid; gap: 1.25rem;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 22rem), 1fr)); }
  .card { border: 1px solid #27272a; border-radius: .75rem; background: #18181b;
    overflow: clip; }
  .card header { padding: .75rem 1rem; border-bottom: 1px solid #27272a;
    display: flex; align-items: baseline; gap: .5rem; flex-wrap: wrap; }
  .card h2 { font-size: .95rem; margin: 0; }
  .card .meta { color: #a1a1aa; font-size: .8rem; }
  .card .key { margin-left: auto; color: #71717a; font-size: .75rem;
    font-family: ui-monospace, monospace; }
  .wheel { height: 2.5rem; }
  .body { padding: .75rem 1rem 1rem; }
  .label { color: #71717a; font-size: .7rem; text-transform: uppercase;
    letter-spacing: .05em; margin: 0 0 .35rem; }
  .qual { display: grid; grid-template-columns: repeat(12, 1fr); height: 2rem;
    border-radius: .375rem; overflow: clip; }
  .qual div { height: 100%; }
  .corners { display: grid; grid-template-columns: repeat(8, 1fr); gap: .35rem;
    margin-top: .9rem; }
  .corner { display: grid; gap: .3rem; justify-items: center; }
  .corner .chip { width: 100%; aspect-ratio: 1; border-radius: .375rem;
    border: 1px solid #27272a; }
  .corner small { color: #a1a1aa; font-size: .62rem; }
</style></head>
<body>
  <h1>RYBitten gamuts</h1>
  <p class="lede">Every gamut, on the framework's dark surface. The wide strip is
  the smooth RYB hue wheel; the 12 blocks below it are the qualitative series
  colors that become <code>--chart-qualitative-*</code>; the eight chips are the
  raw cube corners. Pick one, then wire it with
  <code>go run ./cmd/gamuts -css &lt;key&gt;</code>.</p>
  <div class="grid">
  {{range .}}
    <section class="card">
      <header>
        <h2>{{.Title}}</h2>
        <span class="meta">{{.Author}} · {{.Year}}</span>
        <span class="key">{{.Key}}</span>
      </header>
      <div class="wheel" style="background:{{.Wheel}}"></div>
      <div class="body">
        <p class="label">qualitative palette</p>
        <div class="qual">
          {{range .Qualitative}}<div style="background:{{.Hex}}" title="{{.Label}} {{.Hex}}"></div>{{end}}
        </div>
        <div class="corners">
          {{range .Corners}}
            <div class="corner">
              <div class="chip" style="background:{{.Hex}}" title="{{.Hex}}"></div>
              <small>{{.Label}}</small>
            </div>
          {{end}}
        </div>
      </div>
    </section>
  {{end}}
  </div>
</body></html>`))
