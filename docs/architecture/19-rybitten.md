# Architecture 19 — rybitten: RYB color & gamut theming

[← docs index](../README.md) · prev: [hush](18-hush.md) · related: [Analytics](14-analytics.md) · [UI toolkit](06-ui.md)

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
import "github.com/ThirdCoastInteractive/the-beach/rybitten"

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

The framework themes through CSS variables: `chart/*` reads
`var(--chart-qualitative-N)`, and the `driftwood` kit's accent compiles (via
Tailwind `@apply sky-*`) to `var(--color-sky-N)`. Both are repainted from a gamut
by two emitters:

| Function | Emits |
| --- | --- |
| `CSSVars(g Gamut, count int)` | A `:root` block of `--chart-qualitative-1…count` from the gamut's hue wheel. |
| `AccentVars(g Gamut)` | `:root` overrides for `--color-sky-*` (see `SkyStops`), sampled from the gamut's blue axis. |

The accent override works because driftwood's `@apply` rules compile to
`var(--color-sky-N)`, and an **unlayered `:root` beats Tailwind's `@layer theme`**
in the cascade — so the override wins with no markup change.

**The current default is the `munsell` gamut.** Its blocks live in
[`internal/view/css/input.css`](../../internal/view/css/input.css) (the
`--chart-qualitative-*` values and the `--color-sky-*` accent overrides), built
into the served `app.css`. Semantic tokens (`--success`/`--warning`/`--error`/
`--info`) are deliberately left alone — green still means success.

Chart SVG text is themed in the same sheet (`.chart-axis`, `.chart-label`,
`.chart-value`, `.chart-gauge-value`, `.chart-kpi-*`): the SVGs carry no color of
their own, so without these rules their text falls back to black and vanishes on
the dark surface.

### Swapping the gamut

Pick a key, regenerate both blocks, rebuild `app.css`:

```sh
go run ./cmd/gamuts -css <key>      # paste over the --chart-qualitative block
go run ./cmd/gamuts -accent <key>   # paste over the --color-sky-* block
npx @tailwindcss/cli -i internal/view/css/input.css -o internal/view/static/css/app.css
```

## cmd/gamuts — the picker

[`cmd/gamuts`](../../cmd/gamuts/main.go) is a small reusable dev tool:

```sh
go run ./cmd/gamuts                 # serve the gallery at http://localhost:7777
go run ./cmd/gamuts -css munsell    # print the chart-token :root block
go run ./cmd/gamuts -accent munsell # print the --color-sky-* accent block
```

The gallery renders every gamut on the framework's dark surface — its smooth hue
wheel, the 12 qualitative series colors, and the eight named cube corners — so a
palette is chosen by eye, in context. There is a `.claude/launch.json` config
(`gamuts`) for the preview harness.
