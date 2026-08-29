# RFC 06 — Accessibility (WCAG 2.1 AA as a framework law)

[← docs index](../README.md) · prev: [WebSockets](05-websockets.md)

**Status: shipped.** The law is in [06-ui.md](../architecture/06-ui.md#accessible-by-construction);
the kit changes are across `pkg/ui`, `pkg/ui/driftwood` and `pkg/chart`; the palette moved in
`pkg/beach/view/css/input.css`; enforcement is three `beach-vet` rules plus
`contrast_test.go` / `a11y_test.go` in `pkg/ui/driftwood`. Demonstrated in
`ui/specimen`'s Accessibility section, which all four example apps mount at `/specimen`.
Decisions taken at implementation time are marked ⚑ below.

## Motivation

[ROADMAP block 3](../ROADMAP.md) has always gated the component kit on the specimen being
"**a11y clean**". The phrase was never defined anywhere in the repo and nothing checked it.

Meanwhile the kit was most of the way there on instinct — 216 `aria-*` attributes,
`ui.Icon` decorative-by-default with an opt-in name, an `.sr-only` utility, "color is never
the only encoding" as a stated house rule — and wrong in the specific places instinct does
not reach. An audit against WCAG 2.1 AA found:

- **Three design tokens failing contrast.** White on `--color-accent` was **3.74:1** where
  SC 1.4.3 needs 4.5 — the primary button, on every page. The input border was **2.29:1**
  against a card where SC 1.4.11 needs 3. The ghost button's hover label was **3.24:1**.
- **Four of ~205 `dw-*` classes defined a focus indicator.** Everything else fell back to
  the UA ring, and two controls (the switch, the tabs) hid their real input where no ring
  could reach it at all (SC 2.4.7).
- **ARIA that was wrong, not merely missing.** `Tabs` claimed `role="tablist"` over
  `<label>` children with no `role="tab"`, no `aria-selected`, and *no panels rendered at
  all*. 26 chart SVGs claimed `role="img"` with no accessible name. A nameless role is
  worse than no role: it interrupts to announce that something unidentifiable is there.
- **`aria-describedby`, `aria-labelledby`, `aria-controls`, `aria-expanded` and `aria-sort`
  appeared zero times in the entire repo.** So a form field's error was visually adjacent
  to its input and programmatically invisible; a dialog had no name; a sortable header
  announced no sort state.
- **Every server-pushed status message in every beach app was silent.** `Toast` carried
  `aria-live` on itself and `render.go` appended it into a region no component rendered. A
  live region that arrives together with its content is not announced (SC 4.1.3). This one
  is specific to the framework's own architecture, which is why it is the defect this RFC
  exists for.

Separately, `pkg/i18n` resolved a locale into the request context and nothing structural
used it: `<html lang>` was hardcoded `"en"`, there was no `dir`, and every accessible name
the kit emitted — "Close", "Dismiss", "Loading", "Primary", "Breadcrumb" — was an English
string literal. A Spanish page announced English to a screen reader. That is the join
between the two features, and it is the spine of this change.

## The three ideas

### 1. An accessible name is content, so it is a translation

The names the kit puts in front of assistive technology moved into the framework's own
embedded catalog as `ui.a11y.*` keys, read with `i18n.T(ctx, "ui.a11y.close")` — `ctx` is
already in scope inside every templ body. An app that ships no locales gets exactly the
previous English wording and pays nothing; one that ships an `es-ES.json` gets the whole
kit announced in Spanish for free.

`<html lang>` and `dir` now follow the request locale too (SC 3.1.1), and the framework's
error pages resolve through the same catalog — the two `framework.error.*` keys that had
sat unused since the package was written are finally what the error page renders.

⚑ **Decision: one new import edge, `ui/driftwood → i18n`.** The kit had no dependencies
outside `datastar`, `chart` and `rybitten`. This is the cost of the idea and it is worth
paying: the alternative is a parallel string table in the kit, which is a catalog with
worse tooling. `i18n` is inert until configured, so a monolingual app pays nothing.

### 2. The framework owns the live region

`driftwood.Shell` renders `driftwood.LiveRegion` — an empty, named, polite region at
`beach.ToastTarget` — on every page. `Toast` no longer declares `aria-live` of its own;
it is patched *into* the region that was already there. `beach.Patch{Announce: string}` is
the general form: any handler can push a status message through the same region, entirely
server-side, no new JavaScript.

⚑ **Decision: `ToastTarget` moved to `driftwood`,** with `beach.ToastTarget` as an alias.
The kit renders the element, so the kit owns its id.

### 3. Typed a11y props, never a generic ARIA bag

`datastar.Attrs` is the only escape hatch on every props struct and every constructor
prefixes `data-`, so an app could not pass `aria-labelledby` to a `Modal` at all. Rather
than open a `templ.Attributes` bag — which would let apps hand-write anything and defeat
the typed-props law — the kit derives the relationship itself: `Modal.Title` →
`aria-labelledby`, `Column.Sort` → `aria-sort`, `TableProps.Caption` → `<caption>`.

⚑ **Decision: a popover trigger gets `aria-haspopup` and `aria-controls`, but not
`aria-expanded`.** The audit listed `aria-expanded` among the attributes that appeared
nowhere, and it is still absent from `MenuButton`, `Popover` and every `<dialog>` trigger —
deliberately. Open/close here is the platform's `popovertarget`, with no script to keep an
attribute in step, so a written `aria-expanded` would be frozen at `"false"` and would
start lying the moment the panel opened. Browsers already derive expanded state from
`popovertarget`. A stale ARIA state is worse than an absent one, which is the same reasoning
as clause 5. The one place it *is* written is `chart-toolbar.js`, which owns its own
open/close and can therefore keep it true.

⚑ **Decision: charts are named through variadic options, not a field per struct.** The plan
was a `Label string` on each of the 26 chart input structs. `Chart*Fragment(id, data,
opts ...FigureOpt)` reaches the same place without touching any of them: Go lets a variadic
parameter be appended to an existing signature without breaking a single call site, the name
lives with the `<figure>` that carries it rather than with the geometry that does not, and
`Describe` — the text alternative, which is the part that actually matters — has somewhere
to go that a `Label` field would not have provided.

⚑ **Decision: `Field` publishes its description ids on the context.** A templ component
cannot reach into an opaque `{ children... }` to add an attribute, and making callers
repeat matching ids by hand is exactly the manual synchronisation that leaves real forms
unlabelled. So `Field` rebinds `ctx` for its subtree with the ids of the help and error
text it rendered, and `Input`/`Textarea`/`Select`/`Checkbox` read them back. The result is
that **no call site changed**: an existing `@Field(...) { @Input(...) }` became conformant
where it stood. This is templ's own documented context mechanism, used for the one thing
it is genuinely better at than threading a parameter.

## What the law says

Five clauses, stated in [06-ui.md](../architecture/06-ui.md#accessible-by-construction)
next to the performance laws, and this is what "a11y clean" now means:

1. Every interactive element has an accessible name, and in the kit that name is a catalog
   key, never a literal.
2. Every control is programmatically associated with its label, its help, and its error.
3. Every focusable element draws a visible focus indicator at ≥3:1.
4. Color is never the only encoding, and every token pair the kit paints meets 4.5:1 for
   text and 3:1 for a component boundary.
5. No ARIA role without the states and children it implies. When the kit cannot honour a
   role, it drops the role rather than claiming it emptily.
6. Every layout reflows to 320px without horizontal scrolling. A data table may scroll —
   SC 1.4.10 exempts it — but inside its own container, never by dragging the page.

Clause 5 is the one that changed the most code, and it is why `Tabs` is now honestly a
radio group, why an unnamed chart is `aria-hidden` rather than a nameless `role="img"`,
and why `role="radiogroup"`, `role="group"` on `ButtonGroup`/`Segmented`, and
`role="tablist"` all appear only when there is a name to go with them.

## Enforcement

Measurement, not review comments — the same posture as the 14KB rule.

- **`beach-vet`** gains `a11y-img-alt`, `a11y-unnamed-role-img` and `a11y-literal-name`.
  The first two scan the generated `*_templ.go` string literals, which is the same markup
  the browser gets. The third inverts the usual sanctioning: it fires only *inside*
  `pkg/ui` and `pkg/chart`, because the kit's names must come from the catalog while an
  app's own names are its own business.
- **`pkg/ui/driftwood/a11y_test.go`** runs four laws over every component in the existing
  `renderCase` maps, so a new component is covered the moment it is added to one: ARIA
  references must resolve, form controls must have a name by some route, buttons and links
  must have one too, and a role that needs a name must have one. It understands `<label>`
  wrapping and `<fieldset>`/`<legend>` naming, so it does not push the kit toward
  attributes HTML already provides.
- **`TestZeroValuePropsAreStillAccessible`** runs the same laws over every component built
  from the emptiest props it accepts. This is the suite that earns its keep: the others
  render components as they are *meant* to be used, which is the wrong place to look for a
  component that names itself from a prop — conformant when the prop is set, silently
  broken when it is not, and an omitted optional field is the likeliest thing to happen in
  an app. It found six, all of the same shape (see below).
- **`pkg/ui/driftwood/contrast_test.go`** reads the real tokens out of `input.css` (through
  `view.Tokens`) and asserts every pair the kit paints, using `rybitten.Contrast`. Contrast
  is a property of pairs, so a token sheet cannot be checked by looking at it; this is the
  pairing decision made executable.
- **`pkg/chart/a11y_test.go`** runs the named-or-hidden law over all 27 chart fragments.
  `beach-vet` catches a nameless `role="img"` written by hand, but not one that becomes
  nameless because a runtime `FigureOpt` was omitted — which is the only way it can happen
  now.
- **`pkg/beach/a11y_test.go`** covers the two things the HTTP layer owns rather than the
  kit: that `Patch{Announce}` *appends into* the live region rather than replacing it (a
  patch that replaced it would destroy the thing being announced into), and that
  `Config.Locales` resolves cookie → `Accept-Language` → default onto the request context,
  staying inert when unconfigured.
- **`TestLiveRegionShipsEmpty`** guards the one property that looks like an oversight:
  the region ships with no content, because an arrival into a region that already had some
  is not announced. Without this test, a future placeholder or heading in there would mute
  every status message in every beach app, silently.

⚑ **Bug found while wiring this up:** `lint.fileCtx.onPath` compared its prefixes
("`ui`", "`datastar`", "`internal/db`") against a package path relative to the *module*
root, so inside the framework nothing ever matched and every rule was reporting its own
sanctioned package — 36 false positives, and `a11y-literal-name` would have been inert.
`onPath` now matches with and without a leading `pkg/`, which is what lets one analyzer
serve both the framework tree and an app tree stamped by `beach new`.

## The palette move

⚑ **Decision: the accent darkened; white button text stayed.** The failing pair was white
on `--color-accent`. Both halves were on the table; darkening the accent keeps the button's
type white, which is what every other solid role button does.

The new value is derived from the same rybitten munsell blue axis the old one was — hue
240, saturation 1.0 — moved from lightness 0.445 to **0.290**, so the token's provenance
comment stays true. The window is narrow: white text needs relative luminance ≤ 0.183 and
the focus ring needs ≥ 0.164 against `--color-panel-hover`, and 0.290 lands at 0.1636.

| Token | Was | Now | |
| --- | --- | --- | --- |
| `--color-accent` | `#3b8ea8` | `#3d7c82` | white 4.76 · ring 4.18 / 3.72 / 3.13 |
| `--color-accent-hover` | `#4091ac` | `#3c7070` | white 5.61 |
| `--color-accent-soft` | `#354b45` | `#192c2f` | ghost hover label 4.58 |
| `--color-line-strong` | `#52525b` | `#71717a` | input edge 4.06 / 3.62 / 3.04 |

⚑ **Hover now darkens rather than lightens.** SC 1.4.3 applies to text as displayed, so a
lighter hover would fail with white on it. On a dark theme a darkening hover reads as
pressed, which is a fine thing for a button to read as.

⚑ **Raw `zinc-*` utilities were the gap the token test could not see.** Six kit classes
set readable text with `text-zinc-500` (3.67:1 on a card) and one used `opacity-60`, which
silently divides whatever contrast the text had. They now use `--color-fg-muted`, which is
covered by the pair test. `opacity` for de-emphasis is out: it moves a color the tokens no
longer describe. Disabled controls keep their dim text — SC 1.4.3 exempts them, and a
disabled field that reads as enabled is a worse bug.

## Known exceptions

Stated plainly, because a conformance claim with no exceptions list is not a claim.

- **`Tooltip` does not meet SC 1.4.13 (Content on Hover or Focus).** The criterion asks
  that hover content be dismissible without moving pointer or focus, which needs an Escape
  handler and therefore script — and
  [HTML and CSS first](../architecture/06-ui.md#html-and-css-first) rules that out for a
  local interaction. The tip is now announced (`aria-describedby`), which it never was; it
  remains for supplementary hints, never for information a user must have.
- **The framework's client modules cannot reach the catalog.** `chart-toolbar.js` builds
  its own controls, so its visible strings ("Grid", "Legend", "Theme", "Expand") are English
  literals in JavaScript. Its *semantics* are correct — the toggles carry `aria-pressed`,
  the theme trigger carries `aria-expanded`/`aria-haspopup` and closes on Escape with focus
  returned, and the menu is labelled by its trigger rather than by an invented string — but
  its wording does not localize. Reaching the catalog from a client module needs a
  server-rendered handoff that does not exist yet.
- **No RTL layout sweep.** `i18n.Dir` ships and the shell emits `dir`, but `input.css` has
  not been swept from physical to logical properties, so an RTL locale gets the right
  direction on text and the wrong side for some spacing. The kit already uses `border-s`,
  `ms-`, `text-start` and `inset-inline-start` in places, so it is partly there.
- **No `forced-colors` or `prefers-contrast` support**, and no light theme — the palette is
  dark-only by [explicit decision](../architecture/11-decisions.md).
- **`Tabs` caps at `MaxTabs` (8).** Selecting a panel from a checked radio is a per-index
  CSS rule, and the sheet carries a ladder of exactly eight. A ninth tab renders and does
  not switch. A surface wanting nine tabs wants navigation.
- **App code is out of scope.** `cmd/hush`'s collage editors are pointer-only, and it has
  five `alt=""` on real user photos; `cmd/askmuskie`'s stylesheet has no focus styles and
  its WebGL canvas has no text alternative. Those are product work, tracked separately.

## What the zero-value suite found

Six components were conformant as demonstrated and broken as defaulted, which is the
failure mode a demo-shaped test cannot see:

| Component | With props omitted | Now |
| --- | --- | --- |
| `Segmented`, `ButtonGroup` | `role="group"` with no name | the role appears only with a name |
| `Progress` | `role="progressbar"` with no name | falls back to the catalog — this role carries a value, so it is worth naming rather than dropping |
| `Composer` | `aria-label=""` | falls back placeholder → catalog; never empty |
| `MenuButton`, `Popover` | caret-only trigger, no name | falls back to the catalog |
| `IconButton` | `aria-label=""` | the attribute is omitted, so the control is *visibly* nameless to a linter rather than looking handled |
| `InputGroup` | unlabelled input | borrows the button's label — the box beside a "Search" button is the search box |

`aria-label=""` deserves the emphasis: it is worse than nothing, because it reads in review
and in a linter exactly like the case has been dealt with.

## What only the browser found

Worth recording, because every one of these reads as correct in the source.

**Raw utilities and `opacity` are invisible to a token test.** Six kit classes set readable
text with `text-zinc-500` — 3.67:1 on a card — and `.dw-msg-time` used `opacity-60`, which
divides whatever contrast the text had. `contrast_test.go` passed the whole time, because
none of those are tokens. Lighthouse found them in a second. Hence the rule in the law:
readable text uses a token, and `opacity` is not a de-emphasis tool.

**An accessible name must contain the visible one.** The first pass gave the pagination
Prev/Next links `aria-label="Previous page"` while they still read "Prev" — better prose,
and a WCAG 2.5.3 (Label in Name) failure: someone driving the page by voice says what they
can see. Numbered links keep their name ("3" → "Page 3", the digit survives); Prev and Next
took none, and their visible text moved into the catalog instead.

**A reactive ARIA state is not a boolean attribute.** `AuthModal`'s tab buttons bind
`aria-pressed` to a Datastar signal, and Datastar writes a boolean-valued `data-attr` the
way HTML writes a boolean attribute: `aria-pressed=""` when true, *attribute removed* when
false. For `disabled` that is exactly right. For an ARIA state it is wrong twice — an empty
value is not a state, and a removed `aria-pressed` does not mean "not pressed", it means
"not a toggle button", so the control stopped being one the moment it was switched off.
`ariaBoolExpr` wraps the expression to produce the strings, and `Button`'s `aria-busy`
needed the same treatment.

**Two Shells on one page is three duplicated singletons.** The specimen embeds a `Shell` to
show what the frame looks like, which — once `Shell` started rendering the bypass link, the
`main` landmark and the live region — meant a second of each, with the same ids. Hence
`AppShellProps.Embedded`: one flag meaning "this is a picture of the frame, not the frame".
It replaced the speculative `NoLiveRegion` flag added earlier in the same change, which had
no other user. `driftwood.Main` is the landmark on its own, for a page that builds its own
frame — the specimen and the error page both do.

## Verification

`make gen && make vet && make test`, `beach-vet` with zero a11y findings, and
`beach i18n --dir pkg --catalog pkg/i18n/catalog.json` reporting the catalog in sync. Then
the real browser:

- **Lighthouse accessibility: 100, zero failures**, desktop and mobile, on the specimen —
  the page holding every component in every state plus all 22 charts — and on a page
  exercising the shell, a form with errors, a sortable table, the tab set, the sign-in
  dialog and a chart widget.
- **Keyboard only.** The skip link is the first stop and moves focus into `<main>`; the tab
  radios take arrow keys and their panels follow; the focus ring is relayed to the visible
  label of the two controls whose real input is clipped. The chart toolbar's theme menu
  reports `aria-expanded`, closes on Escape and returns focus to its trigger; its Grid and
  Legend toggles flip `aria-pressed` with their state.
- **The live region.** An SSE `Patch{Announce}` appends into `#beach-toast` without
  replacing the node — verified `sameNode: true`, which is the property that makes it
  spoken rather than merely present.
- **Locale.** Under an `es-ES` cookie the document declares `lang="es-ES" dir="ltr"`, and
  the skip link, landmark names, sort states, required markers and the whole sign-in dialog
  come back in Spanish.
- **Reflow at 320px.** No horizontal page scrolling on either page — confirmed by driving
  the viewport to 320 and checking that `window.scrollX` cannot leave 0, rather than by
  reading `scrollWidth`, which counts the scrollbar gutter and lies.
- **Budget.** The skip link and live region cost **203 B raw, 85 B gzipped** — 0.6% of the
  14KB budget, on a shell page that lands at 2,139 B gzipped.

Two checks were substituted rather than performed, and it is worth being exact about which:

- **No screen reader was run.** Verification went as far as Chrome's accessibility tree —
  which is what a screen reader reads, and where the names, descriptions, roles and states
  above were confirmed — but nobody listened to NVDA, JAWS or VoiceOver say them. The tree
  being right is necessary and not sufficient; announcement order, verbosity and how a
  particular reader handles the live region are unverified.
- **`prefers-reduced-motion` was not emulated.** Chrome DevTools' emulation surface here
  does not expose it. Instead the guarantee became a test:
  `TestEveryAnimationRespectsReducedMotion` extracts every animating selector in the
  stylesheet and fails unless each is named inside a reduced-motion block — which is the
  stronger check, since the failure mode is a *forgotten* rule rather than a wrong one. It
  found one on its first run (`.ui-skeleton`).
