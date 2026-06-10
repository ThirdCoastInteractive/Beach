# Architecture 16 — Component catalog & the specimen

[← docs index](../README.md) · prev: [Example apps](15-examples.md)

[06-ui.md](06-ui.md) defines the *mechanism* — the `ui.Kit` interface, the semantic
token contract, the performance laws, `ui.Defer`. This doc is the *inventory*: the
concrete components and layouts a kit provides, and the **specimen** page that renders
all of them at once.

## A comprehensive hypermedia component library

This is a broad library, not a minimal set. It covers application UIs (dashboards, CRUD,
realtime) **and** marketing surfaces (heroes, pricing, FAQs) **and** commerce (product
grids, carts, checkout). Breadth is the point — a beach app should reach for a kit
component, not hand-roll one.

This doesn't fight the [framework-itis rule](../rfc/04-program.md#risks): that rule warns
against speculative *abstraction* — building a layer for something that happened once.
These are concrete, universally-understood UI patterns, not abstractions. Shipping a
pricing table carries none of the risk of inventing a config system nobody asked for.

The catalog is sequenced, not gated. Two markers:

- **Core** — exercised by [boardwalk / driftbottle / pantry](15-examples.md), so it's
  built and tested first and ships in the `ui` interface + `driftwood` reference kit
  with the trio. Tagged with the proving app: **B**oardwalk, **D**riftbottle,
  **P**antry.
- **Catalog** — the rest of the library, marketing and commerce included. Provided and
  proven in the [specimen](#the-specimen) (the trio doesn't exercise it, so the specimen
  is its proving ground), built out as the kit fills in. No "an app must ask first" gate.

Every component, by construction, obeys the performance laws: server-rendered final
state (no pop-in), [HTML-and-CSS-first interactivity](06-ui.md#html-and-css-first) —
script only for server round-trips, and then only typed `datastar.Attrs` (no raw
`data-*`) — semantic tokens only (no hardcoded color), and an inert/empty/skeleton state
so it
composes with [`ui.Defer`](06-ui.md#deferred-sections).

## The specimen

A page that renders **every component in every state**, plus the token sheet, the icon
set, and all 22 [`chart`](14-analytics.md) types. Atlas and manifold13 both have one; it
is the kit's visual contract — open it, see what the kit looks like.

One templ page, `ui.SpecimenPage(kit ui.Kit)`, lives in `ui`. It calls every component
with sample props through the interface, so you render it by passing in whatever kit you
want to look at — no per-kit copy, no command. Mount it at whatever path you like (the
skeleton wires `/_specimen`). That's the whole mechanism.

The specimen renders, in order: the **token sheet** (surfaces, ink, edges, roles,
series a–o), the **icon grid** (every referenced glyph), each **sub-interface** section
below, then the **chart gallery** (all 22 types with representative data). Because it
holds everything at once it's deliberately over budget — which makes it the natural
place to eyeball the [`ui.Defer`](06-ui.md#deferred-sections) skeleton-and-fill.

## The catalog

Organized by `ui.Kit` sub-interface. The existing eight are
Layout, Buttons, Forms, Feedback, Data, Overlays, Media, Messaging; this doc proposes a
ninth, **Nav** (navbars, sidebar, tabs, breadcrumbs, pagination — none fit the other
eight, and every app shell needs them).

### Layout

The page skeleton and the surfaces content sits on.

| Component             | Variants / states                                    | Proven by |
| --------------------- | ---------------------------------------------------- | --------- |
| App shell — stacked   | topbar + content; the default                        | D, P      |
| App shell — sidebar   | fixed left nav + content; collapsible                | P         |
| Container             | max-width + gutter constraints                       | all       |
| Page heading          | title, subtitle, action slot, breadcrumb slot        | B, P      |
| Section heading       | label + optional actions/divider                     | P         |
| Card                  | `panel`/`well` surface; header / body / footer slots | all       |
| Card heading          | title + meta + action slot                           | P         |
| Divider               | plain, labeled                                       | P         |
| Dashboard grid        | responsive widget grid; hosts `ui.Defer` chart cells | P         |
| Split / master-detail | two-pane, list + detail                              | P         |

*Catalog:* three-column (content + right rail), sticky section rails, resizable panes.

### Nav (proposed sub-interface)

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
| Button group | segmented control, single/multi active                                                 | B, P      |
| Menu button  | label + caret → popover menu (native `popover`)                                        | B, P      |

*Catalog:* split button (primary + menu).

### Forms

The CRUD surface — pantry is the showcase. Validation is **server-rendered**: the
handler re-renders the form with field errors; no client-side validation library.

| Component             | Variants / states                                                     | Proven by |
| --------------------- | --------------------------------------------------------------------- | --------- |
| Form layout           | label / control / help / error rows; stacked + 2-column               | P         |
| Text input            | sizes, leading/trailing addon, prefix/suffix, **error** state         | P         |
| Textarea              | auto-grow; char counter                                               | P, D      |
| Select                | native-styled, with placeholder + error                               | P         |
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
| Toast / notification | SSE-pushable, stacked, auto-dismiss + manual close         | B, D      |
| Empty state          | icon + message + optional action                           | B, D, P   |
| Progress bar         | determinate + indeterminate (loads, game/turn timers)      | B, P      |
| Skeleton             | reserved-space placeholder; shared with `ui.Defer`         | all       |
| Spinner              | inline + block                                             | all       |
| Error pages          | 404 / 500 / maintenance; stamped into the skeleton         | all       |

*Catalog:* top-of-page announcement banner, inline callout / note block.

### Overlays

Open/close is **native** — `<dialog>` for modals, the `popover` attribute + CSS anchor
positioning for popovers/flyouts/tooltips, `<details>` for disclosure — so an overlay
works with zero JavaScript. Datastar enters only to **load** a heavy modal or drawer body
via `@get` (keeping first paint under budget), never to toggle visibility. Stacking uses
Stellar's `--zindex-{drawer,dialog,dropdown,toast,tooltip}`.

| Component           | Variants / states                                      | Proven by |
| ------------------- | ------------------------------------------------------ | --------- |
| Modal dialog        | confirm, form-in-modal; focus trap; ESC/backdrop close | B, P      |
| Drawer / slide-over | left/right; filters, detail panels                     | P         |
| Popover / flyout    | menus, info; anchored, dismiss-on-outside              | B, P      |
| Tooltip             | hover/focus; one-liner                                 | all       |
| Confirm preset      | destructive-action dialog (delete)                     | P         |

*Catalog:* command palette (⌘K, `@get` search) — high value; the trio doesn't happen to
use one, so it's proven in the specimen.

### Media

| Component        | Variants / states                                                                                                                                                        | Proven by |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------- |
| Avatar           | image, initials fallback, status dot, stacked group                                                                                                                      | B, D, P   |
| Icon             | the `ui.I("gear", ui.Spin)` API + build-time font subsetting                                                                                                             | all       |
| Aspect-ratio box | reserves a fixed ratio from Stellar's `--aspect-ratio-*` (square, widescreen 16:9, landscape 4:3, portrait, …) or arbitrary; the wrapper images and video sit in        | P         |
| Image            | responsive (`srcset`/`sizes`, `<picture>` for format + art-direction); `cover`/`contain` fit; lazy below-fold, eager+`fetchpriority` above; skeleton/blur-up placeholder | P         |
| Figure           | image + caption (`<figure>`/`<figcaption>`)                                                                                                                              | P         |
| Video            | poster frame; `controls` or muted-loop background; `<track>` captions; `preload="none"` + click/intersection load; responsive                                            | Catalog   |
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
  non-optional); video carries caption `<track>`s and never autoplays with sound.

```go
@ui.Image(ui.ImageProps{
    Ratio: ui.RatioPhoto,                     // reserved 4:3 box — no shift
    Src:   "/media/items/oats.avif",
    Set:   "/media/items/oats-640.avif 640w, /media/items/oats-1280.avif 1280w",
    Sizes: "(max-width: 40rem) 100vw, 20rem",
    Fit:   ui.FitCover,
    Alt:   "Rolled oats, 1kg bag",            // required by the struct
})
```

*Catalog:* gallery / lightbox, zoomable image, `<audio>` player. Video is fully
specified above and ships with the rest of the catalog; the trio just doesn't happen to
embed one.

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
[aspect-ratio image](#media) so nothing shifts. Validated in the [specimen](#the-specimen).

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
Validated in the [specimen](#the-specimen).

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
