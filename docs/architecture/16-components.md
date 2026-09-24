# Architecture 16 — Component catalog & the specimen

[← docs index](../README.md) · prev: [Example apps](15-examples.md)

[06-ui.md](06-ui.md) defines the *mechanism* — views as templ components, the
semantic token contract, the house laws, `ui.Defer`. This doc is the
*inventory*: the concrete components `ui/driftwood` exports as package-level templ
functions (`@driftwood.Card(p) { ... }`), and the **specimen** page (`ui/specimen`)
that renders all of them at once.

## A comprehensive hypermedia component library

This is a broad library, not a minimal set. It covers application UIs (dashboards, CRUD,
realtime) **and** marketing surfaces (heroes, pricing, FAQs) **and** commerce (product
grids, carts, checkout). Breadth is the point — a beach app should reach for a kit
component, not hand-roll one.

These are concrete UI patterns, not speculative abstraction. Shipping a pricing
table is not inventing a config system nobody asked for.

The catalog is sequenced, not gated. Two markers:

- **Core** — exercised by [boardwalk / driftbottle / pantry](15-examples.md), so it's
  built and tested first and ships as `driftwood` package-level components with the
  trio. Tagged with the proving app: **B**oardwalk, **D**riftbottle, **P**antry.
- **Catalog** — the rest of the library, marketing and commerce included. Provided and
  proven in the [specimen](#the-specimen) (the trio doesn't exercise it, so the specimen
  is its proving ground), built out as the kit fills in. No "an app must ask first" gate.

Every component, by construction, obeys the [house laws](06-ui.md#the-laws):
server-rendered final state (no pop-in),
[HTML-and-CSS-first interactivity](06-ui.md#html-and-css-first) — script only for server
round-trips, and then only typed `datastar.Attrs` (no raw `data-*`) — semantic tokens only
(no hardcoded color), an inert/empty/skeleton state so it composes with
[`ui.Defer`](06-ui.md#deferred-sections), and
[accessible by construction](06-ui.md#accessible-by-construction).

That last one is worth spelling out as a checklist, because it is what a new component has
to satisfy before it belongs here:

- **A name, from the catalog.** Anything a screen reader announces — an icon-only button,
  a landmark, a close control — takes its name from `i18n.T(ctx, "ui.a11y.…")`, never a
  literal. `beach-vet` enforces it.
- **Association, not adjacency.** A control and its label, help and error are wired with
  `for`/`aria-describedby` (or `<fieldset>`/`<legend>` when one label cannot name the set),
  not merely rendered next to each other.
- **A visible focus ring** on anything focusable — added to the focus-law selector list in
  `input.css`, since CSS is the one place `beach-vet` cannot see.
- **Tokens for readable text.** Not raw `zinc-*`, not `opacity` for de-emphasis: both sit
  outside what the contrast test can check.
- **A role only if you can honour it.** No `role="img"` without a name, no `role="tablist"`
  without tabs. Dropping the role is always the better answer.
- **It has to survive its own zero value.** Render it from `ComponentProps{}` and it must
  still be conformant: a name that comes from an optional prop needs a fallback, and an
  `aria-*` attribute written unconditionally becomes `aria-label=""` — a nameless control
  that reads, in review and in a linter, exactly like a handled one.
  `TestZeroValuePropsAreStillAccessible` is where that is held.
- **It has to fit in 320px.** Rows of controls wrap, anything holding an input gets
  `min-w-0`, and anything genuinely two-dimensional scrolls inside its own box.
- **Anything that moves or expires needs a way to stop it.** A component that updates on
  its own, animates past five seconds, or disappears on a timer owes the visitor a control
  (WCAG 2.2.2 / 2.2.1). The framework provides both — `driftwood.LiveToggle` for a stream
  and `prefs.AutoDismiss` for a timer — so the component's job is to *honour* them rather
  than to invent its own.
- **A pointer-only interaction is not an interaction.** Anything reachable by hovering has
  to be reachable by the keyboard too (WCAG 2.1.1), and reaching it has to say the same
  thing: a highlight with no announcement is a keyboard path to nothing. The chart kit's
  `chart-keys.js` is the worked example.

## The specimen

A page that renders **every component in every state**, plus the token sheet, the
icon set, the [rybitten](19-rybitten.md) gamut preview, and the chart gallery. Atlas
and manifold13 both have one; it is the kit's visual contract — open it, see what
the kit looks like.

It lives at `ui/specimen` — one templ page, `specimen.Page()`. Mounting it is one
route, and **all four example apps mount it at `/specimen`**:

```go
app.Page("/specimen", func(c *beach.Ctx) (beach.View, error) {
    return beach.View{Page: specimen.Page()}, nil
})
```

The specimen renders, in order: the **token sheet** (surfaces, ink, edges, accent,
roles, series a–o), the **icon grid** (every referenced glyph), the component
sections below, the **gamut preview** (the rybitten palettes that could repaint the
series tokens), then the **chart gallery** — all 22 chart types plus a static
bar-race frame, each wrapped in the `.dash-widget` card structure so the
[chart toolbar](14-analytics.md#the-client-interaction-layer) (Grid / Legend /
Theme / Expand) is live on every one. Because it holds everything at once it's
deliberately over budget — which makes it the natural place to eyeball the
[`ui.Defer`](06-ui.md#deferred-sections) skeleton-and-fill.

## The catalog

Organized by section, matching `driftwood`'s source files (`layout`, `controls`,
`data`, `feedback`, `overlays`, `media`, `messaging`, plus the nav components every
app shell needs). The sections are doc structure, not Go structure — every component
is a flat package-level templ function.

### Layout

The page skeleton and the surfaces content sits on.

| Component             | Variants / states                                    | Proven by |
| --------------------- | ---------------------------------------------------- | --------- |
| App shell — stacked   | topbar + content; the default                        | D, P      |
| App shell — sidebar   | fixed left nav + content; collapsible                | P         |
| Container             | max-width (`Measure`) + gutter                       | all       |
| Page heading          | title, subtitle, action slot, breadcrumb slot        | B, P      |
| Section heading       | label + optional actions/divider                     | P         |
| Card                  | `panel`/`well` surface; header / body / footer slots | all       |
| Card heading          | title + meta + action slot                           | P         |
| Divider               | plain, labeled                                       | P         |
| Dashboard grid        | responsive widget grid; hosts `ui.Defer` chart cells | P         |
| Split / master-detail | two-pane, list + detail                              | P         |
| **Stack**             | vertical flow, one `Gap`, optional `Align`           | Catalog   |
| **Inline**            | wrapping row; `Gap`, `Align`, `Justify`              | Catalog   |
| **Box**               | `Pad` + optional surface and hairline                | Catalog   |
| **Section**           | block padding, optional heading + lead               | Catalog   |
| **Center**            | centred column at a `Measure`                        | Catalog   |
| **Prose**             | long-form HTML the kit did not render                | Catalog   |
| **Rail**              | content + side column; stacks rail-last below md     | Catalog   |

*Catalog:* sticky section rails, resizable panes.

### Spacing

**Spacing is a closed set.** Every gap and pad prop takes a `driftwood.Space` —
nine rungs on a ~1.5x ladder — and every width takes a `Measure`. There is no
free-form variant, which is the point: spacing is the thing that goes wrong most
often in generated markup, and every one of those failures starts with someone
writing a number. `p-7` and `2.3rem` are unspellable through the props.

The zero value is the important part. `SpaceAuto` means *the component's own
default*, not zero — so a caller who passes nothing gets the right answer, and a
caller who passes something can only pick a rung. `SpaceNone` is how you actually
ask for zero.

```go
@driftwood.Stack(driftwood.StackProps{Gap: driftwood.SpaceLg}) {
    @driftwood.Inline(driftwood.InlineProps{}) {   // takes Inline's own default
        @driftwood.Button(driftwood.ButtonProps{Label: "Save", Role: driftwood.RoleAccent})
        @driftwood.Button(driftwood.ButtonProps{Label: "Discard", Role: driftwood.RoleQuiet})
    }
    @driftwood.Card(driftwood.CardProps{Pad: driftwood.SpaceNone}) {  // full-bleed table
        @driftwood.Table(driftwood.TableProps{ /* … */ })
    }
}
```

The **primitives own the space between their children**, never the children
themselves. Sibling margins collapse, fight, and leave the last child with
trailing space nobody asked for; one flex or grid gap has none of those failure
modes. Reach for `Box` when the answer is "pad this"; reach for `Card` when the
thing is a panel with a header and a border.

Two rules the CSS itself depends on:

- **The ladder is plain CSS, not `@apply`.** The class names are assembled in Go
  by concatenation (`"dw-gap-" + string(s)`), and Tailwind emits a utility only
  when its source scanner has seen the literal name — which it cannot, for a name
  that does not exist until runtime. `TestSpaceScaleIsClosed` walks
  `driftwood.Spaces` and checks every rung has a rule, because a rung without one
  fails *silently*: the class lands in the markup, matches nothing, and the prop
  simply does not work.
- **The ladder is unlayered**, so a caller's explicit choice beats a component's
  built-in padding. `.dw-card-body` pads itself, and `Pad: SpaceNone` has to win.

`beach-vet`'s `raw-spacing` rule closes the other door, flagging Tailwind spacing
utilities in a `class` and `padding`/`margin` in a `style` attribute. Mail is
exempt — HTML email clients strip stylesheets, so an email's padding genuinely has
to live on the element. Inline style remains correct for a per-instance dimension
the kit cannot know, such as a reserved height under `ui.Defer`.

### Nav

| Component       | Variants / states                                          | Proven by |
| --------------- | ---------------------------------------------------------- | --------- |
| Topbar / navbar | brand, primary links, user-menu slot, active state         | all       |
| Sidebar nav     | sections, items, icons, active + nested                    | P         |
| Tabs            | CSS (`:checked`/`:target`) when panels are inline; `@get` only when a panel is deferred | B, P      |
| Breadcrumbs     | truncating, with current page                              | P         |
| Pagination      | prev/next + page numbers; hypermedia `@get`, server-paged  | P         |

*Catalog:* command-palette-launched nav, vertical stepped nav (wizards).

### Buttons

| Component    | Variants / states                                                                      | Proven by |
| ------------ | -------------------------------------------------------------------------------------- | --------- |
| Button       | roles (accent/danger/ghost/quiet), sizes, leading/trailing icon, **loading**, disabled | all       |
| Icon button  | square, with tooltip                                                                   | all       |
| Button group | visual join only — pass already-built Buttons as children                              | B, P      |
| Segmented    | single-select toggle row; `role=group`, `aria-pressed`, active fill; sizes             | Catalog   |
| Menu button  | label + caret → popover menu (native `popover`)                                        | B, P      |

*Catalog:* split button (primary + menu).

### Forms

The CRUD surface — pantry is the showcase. Validation is **server-rendered**: the
handler re-renders the form with field errors; no client-side validation library.

| Component             | Variants / states                                                     | Proven by |
| --------------------- | --------------------------------------------------------------------- | --------- |
| Form layout           | label / control / help / error rows; stacked + 2-column               | P         |
| Text input            | leading/trailing addon, prefix/suffix, **error** state; numeric affordances (`step`/`min`/`max`/`inputmode`/`autocomplete`/`spellcheck`) | P         |
| Textarea              | auto-grow (modern `field-sizing: content`); char counter              | P, D      |
| Select                | native, custom caret + `appearance` reset; placeholder, error, `<optgroup>` groups, per-option + whole-control disabled; dark-themed option popup | P         |
| Fieldset / field group | `<fieldset>`/`<legend>` styled for dark; help line; disable whole group | Catalog   |
| Checkbox              | single + checkbox group                                               | P         |
| Radio group           | vertical / inline                                                     | P         |
| Toggle / switch       | with label + description                                              | P         |
| Input group           | button-attached input (search, chat send)                             | D, P      |
| Date input            | native date; min/max                                                  | P         |
| Field error / summary | per-field message + form-level error banner                           | P         |
| Auth form preset      | styled login (email + password + error); the skeleton's working login | P         |

*Catalog:* combobox / autocomplete (`@get` suggestions), tag / multiselect input,
file upload + dropzone, action panel (card whose body is one form action), OTP /
segmented code input.

### Data

How rows, records, and numbers are shown. Charts proper live in
[`chart`](14-analytics.md); what `ui` owns is the **widget wrapper** that puts a chart in
a card with a `ui.Defer` skeleton.

| Component        | Variants / states                                                                     | Proven by |
| ---------------- | ------------------------------------------------------------------------------------- | --------- |
| Table            | sortable headers (`@get`), row actions, row selection, dense variant, **empty state** | B, P      |
| Description list | key/value grid; 1- and 2-column                                                       | B, P      |
| Stat / billboard | big number + label + delta arrow (color is never the only encoding)                   | B, P      |
| Stacked list     | row = avatar/title/meta/actions; feeds and item lists                                 | D, P      |
| Grid list        | card grid (properties, items)                                                         | B, P      |
| Badge / pill     | role-colored status (online, expiry, turn); dot + label                               | all       |
| Activity feed    | timeline of events (game log, history)                                                | B         |
| Chart widget     | card + title + `ui.Defer` skeleton wrapping a `chart` SVG                             | P         |

*Catalog:* interactive calendar / month grid (distinct from the chart calendar heatmap),
tree / nested list, comparison table, kanban column.

### Feedback

| Component            | Variants / states                                          | Proven by |
| -------------------- | ---------------------------------------------------------- | --------- |
| Alert / flash banner | good/warn/bad/info; dismissible; the "flash banners" strip | all       |
| Toast / notification | SSE-pushable, stacked, manual close, auto-dismiss (paused on hover/focus; danger and warn never expire; `prefs.AutoDismiss` turns it off) | B, D |
| Empty state          | icon + message + optional action                           | B, D, P   |
| Progress bar         | determinate + indeterminate (loads, game/turn timers)      | B, P      |
| Skeleton             | reserved-space placeholder; shared with `ui.Defer`         | all       |
| Spinner              | inline + block                                             | all       |
| Error pages          | 404 / 500 / maintenance; stamped into the skeleton         | all       |
| Live toggle          | pause/resume server-pushed updates; renders the element that opens the stream, so a pause stops updates rather than hiding them (SC 2.2.2) | Catalog |
| Lang / Lang block    | a run or passage in another language; carries `lang` + `dir` (SC 3.1.2) | Catalog |

*Catalog:* top-of-page announcement banner, inline callout / note block.

### Overlays

Open/close is **native** — `<dialog>` for modals, the `popover` attribute + CSS anchor
positioning for popovers/flyouts/tooltips, `<details>` for disclosure — so an overlay
works with zero JavaScript. Datastar enters only to **load** a heavy modal or drawer body
via `@get` (keeping first paint under budget), never to toggle visibility. Stacking
lives in the kit sheet, not in markup.

| Component           | Variants / states                                      | Proven by |
| ------------------- | ------------------------------------------------------ | --------- |
| Modal dialog        | confirm, form-in-modal; focus trap; ESC/backdrop close | B, P      |
| Drawer / slide-over | left/right; filters, detail panels                     | P         |
| Popover / flyout    | menus, info; anchored, dismiss-on-outside              | B, P      |
| Tooltip             | hover/focus; one-liner                                 | all       |
| Confirm preset      | destructive-action dialog; the consequence is the dialog's `aria-describedby`, Cancel is focused (SC 3.3.4) | Catalog |

*Catalog:* command palette (⌘K, `@get` search) — high value; the trio doesn't happen to
use one, so it's proven in the specimen.

### Media

| Component        | Variants / states                                                                                                                                                        | Proven by |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------- |
| Avatar           | image, initials fallback, status dot, stacked group                                                                                                                      | B, D, P   |
| Icon             | the `ui.Icon("gear", ...)` API; Unicode `::before` glyph map (subset icon font planned)                                                                                  | all       |
| Aspect-ratio box | reserves a fixed ratio via the `dw-aspect-*` classes (square, wide 16:9, photo 4:3, portrait); the wrapper images and video sit in                                       | P         |
| Image            | responsive (`srcset`/`sizes`, `<picture>` for format + art-direction); `cover`/`contain` fit; lazy below-fold, eager+`fetchpriority` above; skeleton/blur-up placeholder | P         |
| Figure           | image + caption (`<figure>`/`<figcaption>`)                                                                                                                              | P         |
| Video            | poster frame; `controls` or muted-loop background (which still renders controls — see below); `<track>` captions; `preload="none"`; `<source>` format fallback; responsive | Catalog |
| Media object     | avatar/thumb + body + actions row                                                                                                                                        | Catalog   |

**Images and video obey the perf laws — that's the whole reason they're kit
components and not raw `<img>`/`<video>` tags.**

- **Fixed aspect ratio is the [no-pop-in law](06-ui.md#no-pop-in) applied to media.**
  Every image and video sits in an aspect-ratio box (intrinsic `width`/`height` or a
  ratio wrapper) that reserves its exact space, so late-arriving bytes fill reserved
  pixels and never shift layout — same principle as a [`ui.Defer`](06-ui.md#deferred-sections)
  skeleton. A media component without a known ratio is a vet smell.
- **Lazy and async is the [no-blocking law](06-ui.md#no-blocking).** Below-fold media is
  `loading="lazy"` + `decoding="async"`; video is `preload="none"` behind a poster and
  loads on click or viewport intersection. Nothing media-shaped blocks first paint or
  the main thread. Above-fold hero media opts into eager load + `fetchpriority="high"`.
- **Responsive versions are first-class.** `srcset`/`sizes` for resolution switching and
  `<picture>` for both format fallback (AVIF → WebP → original) and art-direction
  (different crops per breakpoint). The component renders the markup; the bytes come from
  the app's own storage, never a third-party CDN or image proxy
  ([no external CDNs](06-ui.md#no-blocking)).
- **Tokens and a11y by construction.** Placeholder/skeleton backgrounds use surface
  tokens (no hardcoded color); `alt` is required (the props struct makes it
  non-optional); a `<track>` with no `Kind` defaults to `captions`, not `subtitles`,
  because captions carry the non-speech sound that subtitles drop.
- **Video never autoplays without a way to stop it.** `Background` forces muted + loop
  *and native controls*: motion that starts on its own and runs past five seconds beside
  other content needs a pause mechanism (WCAG 2.2.2), and a scripted pause button is
  [local interactivity in script](06-ui.md#html-and-css-first), which this kit does not
  do. Autoplay-without-controls is a shape the component cannot be made to produce.

```templ
@driftwood.Image(driftwood.ImageProps{
    Ratio: driftwood.RatioPhoto,              // reserved 4:3 box — no shift
    Src:   "/media/items/oats.avif",
    Set:   "/media/items/oats-640.avif 640w, /media/items/oats-1280.avif 1280w",
    Sizes: "(max-width: 40rem) 100vw, 20rem",
    Fit:   driftwood.FitCover,
    Alt:   "Rolled oats, 1kg bag",            // required by the struct
})
```

*Catalog:* gallery / lightbox, zoomable image, `<audio>` player. Video, `<picture>`
and the media object all ship and are proven in the specimen; the trio just doesn't
happen to embed one.

### Messaging

The conversation surface — driftbottle is the showcase, and the reason this is its own
sub-interface rather than a corner of Data.

| Component        | Variants / states                                          | Proven by |
| ---------------- | ---------------------------------------------------------- | --------- |
| Message list     | own/other alignment, system messages, timestamps, grouping | D         |
| Composer         | textarea + send; enter-to-send; rate-limit / char state    | D         |
| Typing indicator | animated, presence-driven                                  | D         |
| Presence pill    | connected / reconnecting / paired / waiting                | D         |
| Session banner   | "you're chatting with a stranger" / "they left"            | D         |

*Catalog:* threaded replies, reactions, read receipts. (driftbottle's surface is
deliberately minimal; these round out the messaging set.)

## Marketing

Server-rendered marketing surfaces — same kit, same tokens, same perf laws. Heavy
sections (logo clouds, testimonial grids, long pricing tables) are
[`ui.Defer`](06-ui.md#deferred-sections) below the fold; media uses the
[aspect-ratio image](#media) so nothing shifts. Catalog tier — not yet built; the
[specimen](#the-specimen) is the proving ground when it lands.

| Component            | Includes                                                                   |
| -------------------- | -------------------------------------------------------------------------- |
| Hero                 | centered / split / with-screenshot; headline, subhead, CTA, optional media |
| Feature section      | grid / alternating / with-media; icon + title + body cells                 |
| CTA section          | simple / split / with background                                           |
| Bento grid           | mixed-size feature cells                                                   |
| Pricing              | 2–4 tiers, feature-comparison table, monthly/annual toggle                 |
| Stats                | headline-metric row                                                        |
| Testimonials         | single quote / grid / with-avatar                                          |
| Logo cloud           | customer/partner logo strip                                                |
| FAQ                  | accordion / two-column Q&A                                                 |
| Team                 | member-card grid                                                           |
| Blog / content       | post-card grid, prose article body                                         |
| Newsletter / contact | signup + contact-form blocks                                               |
| Marketing header     | nav + flyout/mega menus + CTA                                              |
| Flyout / mega menu   | multi-column dropdown panel                                                |
| Banner               | dismissible top-of-page announcement                                       |
| Footer               | multi-column links, social, legal                                          |

*Page presets* (compositions of the above): landing, pricing, about.

## Commerce

A storefront in hypermedia: the cart re-renders server-side on `@post`, filters are
`@get` query params, quickviews are `@get` fragments in a modal — there is no
client-side cart or product store ([the one boundary](#the-one-boundary-hypermedia)).
Product imagery is the [aspect-ratio image](#media); grids and galleries are `ui.Defer`.
Catalog tier — not yet built; the [specimen](#the-specimen) is the proving ground
when it lands.

| Component           | Includes                                           |
| ------------------- | -------------------------------------------------- |
| Product overview    | gallery + details + options + add-to-cart          |
| Product list / grid | cards with price, badges, quick-add                |
| Category preview    | category tiles                                     |
| Category filters    | faceted sidebar / filter drawer (`@get` params)    |
| Product quickview   | modal `@get` fragment                              |
| Product features    | spec / feature blocks                              |
| Store navigation    | category mega-menu                                 |
| Promo section       | sale / promo callout                               |
| Shopping cart       | line items, qty, totals; full page + slide-over    |
| Checkout form       | contact, address, shipping, payment, order summary |
| Order summary       | totals breakdown, applied discounts                |
| Order history       | past-orders list + status                          |
| Reviews             | rating, review list, rating histogram              |
| Incentives          | shipping / returns / support strip                 |

*Page presets:* storefront, product, category, cart, checkout, order detail, order history.

## The one boundary: hypermedia

The single exclusion, straight from the [charter](../rfc/01-charter.md#non-goals): no
component that needs client-side state or an SPA runtime. Everything here renders final
HTML on the server and stays live through Datastar — `@get`/`@post` fragments, signals,
SSE patches. A cart updates by posting and re-rendering, not by mutating a JS store; a
filter is a URL, not client state. That is the only thing that doesn't belong in a kit.
