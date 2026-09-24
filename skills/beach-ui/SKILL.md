---
name: beach-ui
description: >
  Build Beach UI with ui/driftwood, ui.Icon, ui.Defer, pkg/datastar, pkg/chart,
  and pkg/rybitten. Use when adding or changing pages, templ views, components,
  forms, buttons, tables, modals, nav, alerts, charts, CSS, design tokens,
  icons, Datastar attributes, SPA-style navigation, or the specimen. Use when
  the user mentions driftwood, 14KB, tokens, Tailwind, SVG charts, Datastar,
  rybitten, or runs /beach-ui. Do not hand-roll a component kit, CSS framework,
  Chart.js/D3, icon set, or raw data-* attributes. Pair with beach-a11y: every
  component in this kit owes an accessible name, an association, a focus ring
  and a 320px layout.
---

# Beach UI

Follow the `beach` skill first ([../beach/SKILL.md](../beach/SKILL.md), [../beach/references/packages.md](../beach/references/packages.md)). This file is the UI pass: find the kit piece, then call it.

Mechanism: [06-ui.md](../../docs/architecture/06-ui.md). Inventory: [16-components.md](../../docs/architecture/16-components.md). Charts: [14-analytics.md](../../docs/architecture/14-analytics.md). Color: [19-rybitten.md](../../docs/architecture/19-rybitten.md). templ: [templ.guide](https://templ.guide).

## Find, then call

1. Search `pkg/ui/driftwood/*.templ` for `^templ `. That is the component list. Props live in `pkg/ui/driftwood/props.go`.
2. If the pattern is visual-unknown, open `pkg/ui/specimen` (every example mounts it at `/specimen`).
3. Call the package-level templ func. There is no Kit, registry, or `c.Kit()`.

```
@driftwood.Card(driftwood.CardProps{Heading: "Holdings"}) {
    <p>body</p>
}
```

4. Server round-trips go on props as `datastar.Attrs` from `pkg/datastar` (`OnClick`, `OnSubmit`, `Bind`, `Signals`, `Navigate`, …). Raw `data-*` in templ fails `beach-vet`.
5. Glyphs: `ui.Icon("gear", …)` — not `<i>`, not an npm icon pack.
6. Late content: `ui.Defer` with a **fixed height/width**. First paint is the final paint; deferred fill must not move layout.
7. After `.templ` or CSS edits: `make gen`.
8. Adding a component? It is not done until it is in the `renderCase` maps in `pkg/ui/driftwood/driftwood_test.go` **and** `TestZeroValuePropsAreStillAccessible` in `a11y_test.go` — the second is the one that catches a name which only exists when an optional prop is set. See `beach-a11y`.
9. Spacing comes from props, never from markup — see [Spacing](#spacing). This is linted.

Do not add a second kit, copy HTML into the app "because it's faster", or restyle with utility classes in markup. The kit emits `dw-*` classes; tokens live in `pkg/beach/view/css/input.css`.

## Charts

`pkg/chart` is geometry + templ in one package.

1. Search `pkg/chart` for `^func Layout` and `Chart*Fragment`.
2. Layout in Go, render with the matching templ/fragment.
3. Live updates: hub SSE morphs the fragment (boardwalk bar race). Do not re-render on the client.
4. ClickHouse → chart: `ch.Rows` then `ch.ToHBarSeries` / `ToLineSeries` / … (see `pkg/ch/chart.go`). Put the widget behind `ui.Defer`.

Do not add Chart.js, D3, or a new SVG helper. The toolbar/tooltip JS already ships in `pkg/beach/view/static/js/`.

## Spacing

**Never write a spacing number.** Every gap and pad prop takes a `driftwood.Space` (nine rungs: `SpaceNone`, `Space2XS`…`Space3XL`); every width takes a `Measure`. `beach-vet`'s `raw-spacing` rule fails the build on a Tailwind spacing utility in a `class` or `padding`/`margin` in a `style`.

**The zero value is usually right.** `SpaceAuto` (the zero value) means *the component's own default*, not zero — pass nothing and you get the correct spacing. `SpaceNone` is how you ask for zero.

Compose pages from the seven primitives instead of inventing layout:

| Need | Use |
| --- | --- |
| Things stacked vertically | `Stack{Gap}` |
| A row of buttons/badges/meta | `Inline{Gap, Align, Justify}` — always wraps |
| "just pad this" | `Box{Pad, Surface, Border}` |
| A band of a page | `Section{Heading, Level, Lead, Pad}` |
| A centred column | `Center{Width}` |
| Markdown / long-form HTML | `Prose{}` |
| Content + a side rail | `Rail{Rail, Side, Width}` |

`Card` is for a panel with a header and a border; `Box` is for a padded div. `Grid{Cols, Gap, Min}` is the only n-up — it auto-fills, so there is no separate switcher.

## Tokens and color

**The palette is generated. Never hand-edit a token value.** `pkg/theme` derives every token from a preset by solving the WCAG ratios each one owes; `cmd/beach-palette` writes the result into `input.css` between sentinels. Changing the look is `view.ThemePreset` + `make palette`, and `TestGeneratedPaletteMatchesInputCSS` fails the build on a hand-edit. `beach-palette -serve` is the explorer (gallery + hue wheel); `-list` prints the presets.

Tokens: `--color-paper/panel/panel-hover`, `--color-fg-strong/default/muted/disabled`, `--color-line-soft/strong`, `--color-accent*`, `--color-on-accent`, `--color-accent-2/3`, `--color-good/warn/bad/info`, `--color-series-a…o`. Values are `oklch()`. Corners are 0px by law. `beach-vet` flags hex/OKLCH in templ and kit CSS.

**Both schemes exist.** Light is derived, not inverted. Anything you add must work in both — `pkg/theme`'s tests hold every pair of every preset in both. Never assume a dark surface: use `--color-paper`/`--color-panel`, never a literal dark value, and if you add a pair the kit paints, add it to `Scheme.Pairs()`.

Three rules the linter cannot see, all learned the hard way:

- **Readable text uses a token, never a raw `zinc-*` utility.** A utility is outside the contrast test's reach, which is how six kit classes sat at 3.67:1 while the test passed. There are now zero raw `zinc-*` in the sheet — keep it that way.
- **`opacity` is not a de-emphasis tool.** It silently divides whatever contrast the text had, and no token check can see it happen. Use `--color-fg-muted`.
- **A new `dw-*` class name built by string concatenation in Go needs a plain CSS rule**, not `@apply` or `@utility`. Tailwind's scanner cannot see a name that does not exist until runtime, so the utility is never emitted and the class silently matches nothing.

Apps do not copy `app.css` or `datastar.js` — `beach.New` serves the framework static tree at `/static`.

## Interactivity split

| Need | Tool |
| --- | --- |
| Disclose, dialog, tabs, popover, toggle | HTML/CSS (`<details>`, `<dialog>`, `popover`, `:checked`) |
| Navigate inside a shell | `datastar.Navigate` + `beach.Swap` |
| Mutate + patch | `ActionFunc` returning `Patches` |
| Live shared view | `StreamFunc` + `hub` topic |
| Binary / high-rate channel | `SocketFunc` (`App.Socket`) |

Datastar is not a replacement for CSS state. Local presentational state stays in markup.

## Page budget

Target ≤14KB compressed for the first response. Put charts, long lists, and below-fold galleries in `ui.Defer`. The specimen is deliberately over budget; app pages are not.
