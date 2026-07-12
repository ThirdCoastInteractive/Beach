# Architecture 19 — rybitten: RYB color & gamut theming

[← docs index](../README.md) · prev: [hush](18-hush.md) · next: [sync boundary](20-sync.md) · related: [Analytics](14-analytics.md) · [UI toolkit](06-ui.md)

`rybitten` is a dependency-free Go port of meodai's [RYBitten](https://rybitten.space)
— a pseudo-RYB color model derived from Johannes Itten's chromatic circle. It is
the source of the framework's default palette and the one place a whole app's
color character is decided.

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
import "github.com/ThirdCoastInteractive/the-beach/pkg/rybitten"

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

**The current default is the `munsell` gamut.** Its `SeriesVars` output lives in
[`pkg/beach/view/css/input.css`](../../pkg/beach/view/css/input.css) `:root`, built
into the served `app.css`; the `--color-accent*` tokens are a munsell teal picked
by hand in the same sheet. The eight alternate `[data-chart-theme]` palettes (plus
a munsell one) redefine the series tokens per scope — the
[chart toolbar's](14-analytics.md#the-client-interaction-layer) Theme control swaps
that attribute. Role tokens (`--color-good`/`--color-warn`/`--color-bad`/
`--color-info`) are deliberately left alone — green still means good.

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
