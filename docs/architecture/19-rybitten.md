# Architecture 19 — rybitten: RYB color & gamut theming

[← docs index](../README.md) · prev: [Component catalog](16-components.md) · next: [sync boundary](20-sync.md) · related: [Analytics](14-analytics.md) · [UI toolkit](06-ui.md)

`rybitten` is a dependency-free Go port of meodai's [RYBitten](https://rybitten.space)
— a pseudo-RYB color model derived from Johannes Itten's chromatic circle.

**Scope note (2026-08-31).** rybitten is no longer the framework's theming engine.
The design tokens are now derived in OKLCH by [`pkg/theme`](06-ui.md#tokens), and
this package's job is *chart series palettes*, where a historical gamut's
character is the whole point and 3:1 against the page is the only hard constraint.

That is not a knock on the model, it is a statement of what each is for. An RYB
cube is a **lookup**: a hue is a trilinear interpolation between eight fixed
corners, so the palette wears the source gamut's character — which is exactly what
you want from Munsell's wheel or a 1982 Marvel newsprint chart. It is exactly what
you do not want from a UI accent, because you cannot *ask* for a vivid teal, only
for a position on an arc and whatever the cube happens to hold there. On several
gamuts the green-to-blue arc runs through sage and olive, and no amount of tuning
recovers chroma the cube does not contain. OKLCH has no such dead zones, and
`color.MaxChroma` makes "as vivid as this hue gets at this lightness" a question
with an answer. See [06-ui.md](06-ui.md#tokens).

Digital tools mix in additive RGB, which flattens the warm neutrals and muddy
intermediates a painter gets for free. RYBitten instead interpolates through an
**8-corner RYB cube**. Each cube reproduces a historical or artist palette; run
the same hue wheel through a different cube and it repaints in that palette's
character. The whole look swaps by swapping one `Cube` — no per-color edits.

It is a leaf package: zero dependencies, importable by anything (like `ecs` and
`passwords`).

## The model

The math is a faithful port of upstream `main.ts`: smoothstep easing on each
axis, then trilinear interpolation across the cube corners.

```go
import "github.com/ThirdCoastInteractive/Beach/pkg/rybitten"

rgb := rybitten.RYBHSL2RGB([3]float64{30, 1, 0.5}, rybitten.Itten, true)
css := rgb.Hex() // an Itten orange, ~"#f08e1c"
```

| Type | What it is |
| --- | --- |
| `RGB [3]float64` | A color, channels nominally 0–1. `.Hex()`, `.Clamp()`, `.RGB255()`. |
| `Cube [8]RGB` | The eight corner colors of an RYB cube. |
| `Gamut` | A named `Cube` with provenance: `Key`, `Title`, `Author`, `Year`, `Reference`. |

| Function | Does |
| --- | --- |
| `RYB2RGB(ryb [3]float64, cube Cube) RGB` | Trilinear conversion with smoothstep easing. |
| `RYB2RGBEased(ryb, cube, Easing)` | …with a caller-supplied easing function. |
| `RYBHSL2RGB(hsl [3]float64, cube Cube, invertLightness bool) RGB` | Walk an HSL color through RYB space — the workhorse for palettes. |
| `HSLToRGB(hsl [3]float64) [3]float64` | Plain HSL→RGB (the front half of `RYBHSL2RGB`). |
| `Smoothstep`, `Lerp` | The shaping curve and linear interp, exposed. |

**Corner order** is canonical: white, red, yellow, orange, blue, violet, green,
black. Corner `i` sits at RYB coordinate `(i&1, i>>1&1, i>>2&1)` — the bits select
red, yellow, blue. The conversion is **subtractive**: `[0,0,0]` yields the cube's
*white* corner and `[1,1,1]` its *black* corner, the opposite of additive RGB.

## The gamuts

`Cubes` is a `map[string]Gamut` of **36 presets** transcribed verbatim from
upstream — historical color theory (Itten 1961, Munsell 1905, Goethe 1809,
Chevreul 1839, Runge 1810…), reference manuals (Apple HyperCard 1989, Macintosh
1990, Marvel newsprint 1982), contemporary artists (Ippsketch, Roni Kaufman's
"Ten", Tofu's pixel art), and the synthetic `cmy` / `rgb` cubes.

`Keys` lists them in a stable curated order. **Range `Keys`, never the map** —
ranging a Go map scrambles order, and palette output must be deterministic.

```go
g := rybitten.Cubes["munsell"]
fmt.Println(g.Title, g.Author, g.Year) // Munsell Color System Albert Henry Munsell 1905
```

## Generating palettes

```go
pal := rybitten.QualitativePalette(g.Cube, 12) // 12 evenly-spaced hues
hex := rybitten.Hexes(pal)                     // []string of "#rrggbb"
```

| Function | For |
| --- | --- |
| `Palette(cube, n, sat, light)` | `n` evenly-spaced hues — the qualitative series generator. |
| `QualitativePalette(cube, n)` | `Palette` with defaults tuned for a dark surface (full sat, mid lightness). |
| `Ramp(cube, n, hue, sat, lightHi, lightLo)` | One hue, light→dark — a sequential scale. |
| `Hexes([]RGB) []string` | Map colors to CSS hex. |

## Theming the framework

The framework themes through CSS variables: `chart/*` reads the 15 series tokens
`--color-series-a…o` ([06-ui.md](06-ui.md#tokens)). Two emitters turn a gamut into
that vocabulary:

| Function | Emits |
| --- | --- |
| `SeriesVars(g Gamut)` | A `:root` block defining `--color-series-a…o` (`SeriesCount` = 15) from the gamut's hue wheel, with a provenance comment, ready to paste into `input.css`. |
| `Series(g Gamut, n int)` | The same colors as values (`[]RGB`) — for server-side swatches and image rendering. The specimen's gamut preview uses it. |

**The framework's own `--color-series-a…o` no longer come from here.** They are
derived with the rest of the palette, at one shared lightness and one shared
chroma fraction, so the fifteen read as equally prominent — a balance a per-hue
solve destroys and a contrast check cannot see. A gamut's series palette remains
available for a chart that wants a specific historical character, and the eight
alternate `[data-chart-theme]` palettes still redefine the tokens per scope, which
the [chart toolbar's](14-analytics.md#the-client-interaction-layer) Theme control
swaps. Role tokens are deliberately left alone in every palette — green still
means good.

Chart SVG text is themed in the same sheet (the `.cbar-label`/`.cbar-value` and
`.cline-*` rules from the Atlas port): the SVGs carry no color of their own, so
without those rules their text falls back to black and vanishes on the dark
surface.

### Choosing and swapping a gamut

The [specimen page](16-components.md#the-specimen) (mounted at `/specimen` by every
example app) renders the **gamut preview** — each gamut's series palette on the
framework's dark surface — so a palette is chosen by eye, in context. To swap:
call `SeriesVars` with the chosen key (a three-line scratch `main` does it),
paste the block over the `--color-series-*` values in `input.css` `:root`, and
rebuild the sheet:

```go
fmt.Print(rybitten.SeriesVars(rybitten.Cubes["ippsketch"]))
```

```sh
make css   # npx @tailwindcss/cli -i pkg/beach/view/css/input.css -o pkg/beach/view/static/css/app.css
```
