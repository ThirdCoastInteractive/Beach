---
name: beach-a11y
description: >
  Keep Beach UI accessible: WCAG 2.1 AA is a framework law, not a review note.
  Use when adding or changing any driftwood component, templ view, form, chart,
  icon, focus style, colour token, or anything a person sees, hears or tabs to.
  Use when the user mentions accessibility, a11y, WCAG, ARIA, screen reader,
  keyboard, focus, contrast, reflow, alt text, aria-label, live region, or runs
  /beach-a11y. Do not add an ARIA role you cannot honour, a hardcoded
  aria-label, a colour pair you have not measured, or a component that is only
  accessible when every optional prop is filled in.
---

# Beach accessibility

Follow the `beach` skill first ([../beach/SKILL.md](../beach/SKILL.md)), and `beach-ui` for the component itself. This file is the accessibility pass.

Target: **WCAG 2.1 Level AA**. Law: [06-ui.md](../../docs/architecture/06-ui.md#accessible-by-construction). Audit, reasoning and the known exceptions: [RFC 06](../../docs/rfc/06-accessibility.md). Per-component contract: [16-components.md](../../docs/architecture/16-components.md).

Agents fail here by writing ARIA that looks right and says nothing. Every rule below exists because that already happened once.

## The six clauses

1. **A name, from the catalog.** Anything announced — an icon-only button, a landmark, a close control — takes its name from `i18n.T(ctx, "ui.a11y.…")`. A literal `aria-label` inside `pkg/ui` or `pkg/chart` fails `beach-vet` (`a11y-literal-name`), because a name a screen reader reads out is content: "Close" on a Spanish page is as wrong as an untranslated heading.
2. **Association, not adjacency.** A control and its label, help and error are wired with `for` / `aria-describedby`, or `<fieldset>`/`<legend>` when one label cannot name the set. `Field` does this for you off the context — put the control inside a `Field` and it is done.
3. **A visible focus ring** on anything focusable. Add the class to the focus-law selector list in `input.css`; CSS is the one place `beach-vet` cannot look.
4. **Colour is never the only encoding**, and every pair is measured. See `beach-ui` on tokens.
5. **No role you cannot honour.** An unnamed `role="img"` announces "image" and stops. A `role="tablist"` over things that are not tabs is worse than no role. **Dropping the role is always the better answer** — that is why `Tabs` is honestly a radio group and an unnamed chart is `aria-hidden`.
6. **Reflow to 320px.** Rows of controls wrap, anything holding an input gets `min-w-0`, anything genuinely two-dimensional scrolls in its own box.

## Adding a component

```
1. Name it        i18n.T(ctx, "ui.a11y.…") for kit names; a prop for app text.
2. Associate it   Field handles label/help/error. Groups take <fieldset>/<legend>.
3. Focus it       add the class to the focus-law list in input.css.
4. Test it        add it to BOTH renderCase maps and the zero-value suite.
5. make gen && go test ./pkg/ui/... && go run ./cmd/beach-vet .
```

Step 4 is the one that gets skipped and the one that matters. `TestZeroValuePropsAreStillAccessible` renders from `ComponentProps{}`: a component that names itself from an optional prop is conformant when the prop is set and silently broken when it is not, which is the likelier case in a real app. It found six defects on its first run.

## Traps

| Looks fine | Actually |
| --- | --- |
| `aria-label={ p.Label }` on an optional prop | `aria-label=""` — a nameless control that reads, in review and to a linter, exactly like a handled one. Write the attribute conditionally. |
| `data-attr:aria-pressed={ expr }` | Datastar writes a boolean-valued `data-attr` as an HTML boolean attribute: empty when true, **removed** when false. A removed `aria-pressed` means "not a toggle button". Wrap with `ariaBoolExpr`. |
| A toast carrying its own `aria-live` | A region that arrives with its content is not announced. Patch **into** the region `Shell` already shipped (`driftwood.LiveRegion`, `beach.ToastTarget`), or use `beach.Patch{Announce: …}`. |
| `aria-label="Previous page"` over visible "Prev" | WCAG 2.5.3: someone driving by voice says what they can see. The accessible name must *contain* the visible text. |
| A second `Shell` on the page | Three duplicated singletons (landmark, bypass link, live region) and their ids. Use `AppShellProps{Embedded: true}` for a demo; `driftwood.Main` for a page with its own frame. |
| `text-zinc-500`, or `opacity` for de-emphasis | Outside what the contrast test can see. Use `--color-fg-muted`. |

## Enforcement

| Check | Catches |
| --- | --- |
| `beach-vet` | `<img>` with no `alt`; `role="img"` with no name; a literal `aria-label` in the kit |
| `driftwood/a11y_test.go` | dangling ARIA refs, unnamed controls, unnamed buttons/links, unnamed roles — over both `renderCase` maps **and** zero-value props |
| `driftwood/contrast_test.go` | every token pair the kit paints, and every animation missing from a `prefers-reduced-motion` block |
| `chart/a11y_test.go` | a chart that is neither named nor hidden |
| `beach/a11y_test.go` | `Patch{Announce}` targeting the live region; `Config.Locales` resolution |
| `beach i18n` | a `ui.a11y.*` key added, renamed or dropped without the catalog |

Then the browser, which finds what none of these can: Lighthouse's accessibility audit on `/specimen` and on a real page, desktop and mobile, at 320px. It found the raw-utility contrast failures and the Label-in-Name break that every test above had passed.

## Known exceptions

Do not "fix" these silently — they are decisions, recorded in [RFC 06](../../docs/rfc/06-accessibility.md):

- `Tooltip` does not meet 1.4.13 (dismissible needs an Escape handler, which needs script).
- No `aria-expanded` on `popovertarget` triggers: without script it would freeze at `"false"` and start lying. A stale ARIA state is worse than an absent one.
- `dir` ships; the physical→logical CSS sweep for RTL does not.
- No `forced-colors` / `prefers-contrast` support; the palette is dark-only by decision.
- Framework client modules (`chart-toolbar.js`) cannot reach the i18n catalog, so their visible strings stay English.
