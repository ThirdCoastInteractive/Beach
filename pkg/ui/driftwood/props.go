// Package driftwood is Beach's component kit: a package of exported,
// package-level templ components that render clean semantic HTML. Every color,
// space, radius, and motion value comes from a CSS custom property defined in
// the app stylesheet; no literal hex/OKLCH ever appears in the markup. Markup
// carries short dw-* semantic classes styled in internal/view/css/input.css,
// which compiles into the single /static/css/app.css the Page shell links.
//
// Apps and the framework call the components directly:
//
//	@driftwood.Card(driftwood.CardProps{Heading: "Holdings"}) {
//		<p>body</p>
//	}
package driftwood

import (
	"context"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/datastar"
	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
	"github.com/a-h/templ"
)

// Props structs are immutable by convention: build one, pass it, never mutate.
// Each may carry an optional datastar.Attrs for server round-trips only.

// Surface names a design surface a component renders on. The kit maps each to a
// background token (e.g. SurfacePanel -> var(--neutral-2)) and its matching
// -on ink; callers never write a color literal.
type Surface string

const (
	SurfacePanel Surface = "panel"
	SurfaceWell  Surface = "well"
	SurfacePlain Surface = ""
)

// Role is a semantic accent shared by buttons, badges, and alerts. It selects a
// design palette (--primary-*, --error-*, ...), never a literal color.
type Role string

const (
	RoleAccent  Role = "accent"
	RoleDanger  Role = "danger"
	RoleGhost   Role = "ghost"
	RoleQuiet   Role = "quiet"
	RoleGood    Role = "good"
	RoleWarn    Role = "warn"
	RoleInfo    Role = "info"
	RoleNeutral Role = ""
)

// Size is a fluid-scale step (--size-*) shared across controls.
type Size string

const (
	SizeSm Size = "sm"
	SizeMd Size = ""
	SizeLg Size = "lg"
)

// Space is a step on the kit's spacing ladder, shared by every gap and pad prop.
//
// It is a closed set on purpose. Spacing is the thing that goes wrong most often
// in generated markup — too much, too little, or none at all — and every one of
// those failures starts with someone writing a number. A caller here cannot
// write a number: they pick a rung, or they leave the field alone and take the
// component's own default, which is the right answer nearly every time.
//
// The rungs are a ~1.5x ladder rather than a linear one, so adjacent steps are
// visibly different and a layout cannot drift into "8px here, 9px there".
// [SpaceAuto] is the zero value: it means "whatever this component already
// knows", not "zero" — that is [SpaceNone].
type Space string

const (
	SpaceAuto Space = ""     // the component's own default; the zero value
	SpaceNone Space = "none" // 0
	Space2XS  Space = "2xs"  // 0.25rem
	SpaceXS   Space = "xs"   // 0.5rem
	SpaceSm   Space = "sm"   // 0.75rem
	SpaceMd   Space = "md"   // 1rem
	SpaceLg   Space = "lg"   // 1.5rem
	SpaceXL   Space = "xl"   // 2rem
	Space2XL  Space = "2xl"  // 3rem
	Space3XL  Space = "3xl"  // 4rem
)

// Spaces is every rung, in order. TestSpaceScaleIsClosed walks it to prove each
// one has CSS behind it — a constant with no rule is a silent no-op, which is
// worse than not offering the rung at all.
var Spaces = []Space{SpaceNone, Space2XS, SpaceXS, SpaceSm, SpaceMd, SpaceLg, SpaceXL, Space2XL, Space3XL}

// Measure is how wide a column of content is allowed to get.
//
// The names are about reading, not about pixels: [MeasureText] is the ~65
// character line that long-form prose wants (typography's measure, and the
// reason the type is called this), while the rest step out from there. Naming
// them by intent is what stops a caller reaching for a literal width, which is
// how a layout ends up with a 1400px paragraph nobody can read.
type Measure string

const (
	MeasureDefault Measure = ""       // the standard page column; the zero value
	MeasureText    Measure = "text"   // ~65ch — the readable measure for prose
	MeasureNarrow  Measure = "narrow" // a form or a single-column dialog page
	MeasureWide    Measure = "wide"   // dashboards and tables
	MeasureFull    Measure = "full"   // no constraint but the gutter
)

// Measures is every step, for the same closed-set check as [Spaces].
var Measures = []Measure{MeasureDefault, MeasureText, MeasureNarrow, MeasureWide, MeasureFull}

// Align is cross-axis alignment for the layout primitives.
type Align string

const (
	AlignStretch  Align = "" // fill the cross axis; the zero value
	AlignStart    Align = "start"
	AlignCenter   Align = "center"
	AlignEnd      Align = "end"
	AlignBaseline Align = "baseline"
)

// Justify is main-axis distribution for [Inline].
type Justify string

const (
	JustifyStart   Justify = "" // the zero value
	JustifyCenter  Justify = "center"
	JustifyEnd     Justify = "end"
	JustifyBetween Justify = "between"
)

// Ratio names an aspect-ratio step for the media box, mapped to a dw-aspect-*
// class in app.css.
type Ratio string

const (
	RatioSquare Ratio = "square" // 1:1
	RatioWide   Ratio = "wide"   // 16:9
	RatioPhoto  Ratio = "photo"  // 4:3
	RatioPortr  Ratio = "portrait"
)

// Fit is the object-fit of media inside its aspect box.
type Fit string

const (
	FitCover   Fit = "cover"
	FitContain Fit = "contain"
)

// --- Layout ---

// ContainerProps constrains a page's content column.
//
// Width was a free-form token name until it turned out to name nothing: the
// style it emitted referenced a --size-* custom property no sheet defined, so
// setting it made the declaration invalid at computed-value time and *removed*
// the max-width rather than setting one. A closed [Measure] cannot miss.
type ContainerProps struct {
	Width Measure // the content column's width; zero value is the standard page
	Pad   Space   // gutter around the column; zero value is the standard gutter
	Attrs datastar.Attrs
}

type CardProps struct {
	Surface Surface // panel (default) or well
	// Pad overrides the body padding. Left alone, a card pads itself correctly;
	// this exists for the two cases where it genuinely should not — a card whose
	// body is a full-bleed table or image (SpaceNone), and a card carrying a
	// page's primary content rather than a widget (SpaceLg).
	Pad     Space
	Heading string // optional title rendered in the card header
	// Level is the heading level for Heading, 2–6. It defaults to 3, which is
	// right under a SectionHeading (h2) and wrong directly under a PageHeading
	// (h1) — a skipped level is how a screen reader's heading list loses the
	// shape of the page (WCAG 1.3.1). The kit cannot know where a card sits, so
	// the caller owns the outline.
	Level int
	Attrs datastar.Attrs
}

type PageHeadingProps struct {
	Title    string
	Subtitle string
	Attrs    datastar.Attrs
}

type SectionHeadingProps struct {
	Label string
	// Level is the heading level, 2–6; defaults to 2.
	Level int
	Attrs datastar.Attrs
}

type DividerProps struct {
	Label string // optional labeled divider
	Attrs datastar.Attrs
}

// AppShellProps is the top-level page frame: a topbar across the top, optional
// fixed sidebar on the left, and the main content area. Sidebar is rendered when
// non-nil. The children of Shell are the main region.
type AppShellProps struct {
	Topbar  templ.Component // optional navbar/topbar slot
	Sidebar templ.Component // optional left sidebar nav; nil = stacked shell
	// SidebarLabel names the <aside> landmark so a screen reader's landmark
	// list distinguishes it from the topbar. Defaults to the kit's translated
	// "Sidebar"; set it when an app's rail holds something more specific.
	SidebarLabel string
	// Embedded marks this Shell as a preview inside another page rather than
	// the page root — the shape a component gallery or a docs page needs.
	//
	// A page has exactly one main landmark, one bypass link, and one live
	// region. A second Shell on the same page would duplicate all three, along
	// with their ids, which is worse than having none: a screen reader's
	// landmark list gains a phantom, and a patch aimed at the live region has
	// two targets. So an embedded Shell renders its frame and nothing else.
	Embedded bool
	Attrs    datastar.Attrs
}

// CardHeadingProps is a richer card header: title + optional meta line + an
// action slot rendered on the right (a Card's Heading string is the simple case).
type CardHeadingProps struct {
	Title string
	// Level is the heading level for Title, 2–6; defaults to 3. See CardProps.
	Level  int
	Meta   string          // optional secondary line
	Action templ.Component // optional right-aligned action slot
	Attrs  datastar.Attrs
}

// GridProps is a responsive card grid. Cols is the column count at the widest
// breakpoint; cells are the children.
//
// The grid is intrinsically responsive — it auto-fills at a minimum cell width
// rather than switching at breakpoints — so it is also the kit's "n-up that
// stacks when it has to". There is no separate switcher component, because this
// is one.
type GridProps struct {
	Cols int   // target columns at the widest breakpoint (default 3)
	Gap  Space // gap between cells; zero value is the standard grid gap
	// Min is the narrowest a cell may get before the grid drops a column. The
	// zero value suits cards; MeasureNarrow gives wider cells and so fewer of
	// them, MeasureText is for a grid of prose columns.
	Min   Measure
	Attrs datastar.Attrs
}

// SplitProps is a two-pane master-detail layout: a list pane and a detail pane.
type SplitProps struct {
	List   templ.Component
	Detail templ.Component
	Gap    Space // between the panes; zero value is the standard gap
	Attrs  datastar.Attrs
}

// DashGridProps is the responsive 12-column dashboard grid: a single column on a
// phone, six columns at the md breakpoint and twelve at lg (denser again past an
// ultrawide 1920px). Children are DashCol cells that declare their span, so a
// layout reads as stacked rows on mobile and side-by-side columns on a desktop.
type DashGridProps struct {
	Attrs datastar.Attrs
}

// DashColProps is one cell of a DashGrid. Span is the column span at the widest
// breakpoint (one of 3, 4, 6, 8, 12 of 12); the grid collapses it toward a
// full-width row as the viewport narrows. Rows optionally spans 1–4 grid rows for
// a taller cell.
type DashColProps struct {
	Span  int // 3, 4, 6, 8, or 12 (defaults to 12)
	Rows  int // optional row span 1–4 (0 = single row)
	Attrs datastar.Attrs
}

// --- Layout primitives ---
//
// Seven small components that exist for one reason: composing a page should not
// require inventing spacing. Between them they cover what page markup actually
// does — stack things, sit things in a row, put a padded box around something,
// break a page into sections, centre a column, set long-form text, and hang a
// rail beside content. Each takes a [Space], so the only spacing decision a
// caller can make is which rung, and leaving the field alone picks a good one.

// StackProps is a vertical flow with one gap between children — the single most
// common layout in any page, and the one most often built by hand out of
// margins that then fight each other. Stack owns the gap; children own nothing.
type StackProps struct {
	Gap   Space // between children; zero value is the standard vertical rhythm
	Align Align // cross-axis alignment; zero value stretches
	Attrs datastar.Attrs
}

// InlineProps is a horizontal row that wraps — button rows, badge rows, the meta
// line under a title.
//
// It wraps rather than scrolls or overflows, which is not a style preference: a
// row of controls that will not wrap is one of the two or three things that
// actually pushes a page sideways at 320px (WCAG 1.4.10).
type InlineProps struct {
	Gap     Space   // between items; zero value is the standard inline gap
	Align   Align   // cross-axis; zero value centres, which is what a control row wants
	Justify Justify // main-axis distribution; zero value packs to the start
	Attrs   datastar.Attrs
}

// BoxProps is padding and an optional surface, and nothing else.
//
// It is the answer to "I just need this padded", which otherwise becomes a
// hand-written class or an inline style. Reach for [Card] when the thing is a
// panel with a header and a border; reach for Box when it is a padded div.
type BoxProps struct {
	Pad     Space   // inside the box; zero value is the standard box padding
	Surface Surface // optional background; zero value is transparent
	Border  bool    // draw a hairline edge
	Attrs   datastar.Attrs
}

// SectionProps is one band of a page: block padding, an optional heading, and
// the section's content as children.
//
// It renders a real <section>, and its Heading goes through the same [heading]
// ladder every other kit component uses — so a page built out of these has an
// outline a screen reader can navigate, rather than a stack of divs with big
// text in them (WCAG 1.3.1). A Section with no Heading is an unlabelled region,
// which is fine for a layout band and wrong for a content one.
type SectionProps struct {
	Heading string // optional; rendered as the section's heading
	Level   int    // heading level for Heading, 2–6; defaults to 2
	Lead    string // optional standfirst line under the heading
	Pad     Space  // block padding above and below; zero value is the standard band
	Gap     Space  // between the heading block and the content
	Attrs   datastar.Attrs
}

// CenterProps is a horizontally centred column at a chosen [Measure] — a login
// form, an empty state, a confirmation page, an article.
//
// Distinct from [Container], which is the page's own content column: a Center
// can sit inside one, and often does.
type CenterProps struct {
	Width Measure // zero value is the readable text measure, not the page width
	// Gutter keeps the column off the viewport edge on a narrow screen. Zero
	// value is the standard gutter; SpaceNone is for a Center already inside
	// something padded.
	Gutter Space
	Attrs  datastar.Attrs
}

// ProseProps sets a run of long-form HTML — markdown output, a policy page, an
// article body.
//
// It is the one component in the kit that styles elements it did not render.
// That is deliberate and it is the point: prose arrives as a blob of <p>, <ul>,
// <h2>, <blockquote> and <pre> from a renderer that knows nothing about the kit,
// so something has to give that blob a measure, a rhythm and link styling. The
// alternative is every app writing the same stylesheet.
type ProseProps struct {
	Width Measure // zero value is the readable text measure
	Attrs datastar.Attrs
}

// RailProps is content with a secondary column beside it — a table of contents,
// a filter panel, a metadata sidebar.
//
// It stacks below the md breakpoint, rail last, because a rail is by definition
// the less important column and a narrow screen should not lead with it. Side
// controls which edge the rail takes on a wide screen.
type RailProps struct {
	Rail templ.Component // the secondary column; nil renders content full-width
	Side string          // "end" (default) or "start"
	Gap  Space           // between content and rail
	// Width is how wide the rail gets. Zero value is the standard rail;
	// MeasureNarrow is for a wider one carrying filters or a form.
	Width Measure
	Attrs datastar.Attrs
}

// --- Nav ---

// NavItem is one link in a navbar/sidebar/breadcrumb trail.
type NavItem struct {
	Label  string
	Href   string
	Active bool
	Icon   string // optional ui icon name
}

type NavbarProps struct {
	Brand string
	Items []NavItem
	Attrs datastar.Attrs
}

type BreadcrumbsProps struct {
	Items []NavItem // last item is the current page
	Attrs datastar.Attrs
}

// Tab is one tab; Get is set only when the panel is deferred (CSS otherwise).
type Tab struct {
	ID     string
	Label  string
	Active bool
	Get    string // @get target for a deferred panel; empty = inline CSS tab
	// Panel is the tab's content. A Tab with no Panel renders an empty panel
	// box — which is what a Get-deferred tab wants, since the fragment patches
	// into it — but a tab set with no panels at all is a bug, not a design.
	Panel templ.Component
}

// MaxTabs is how many tabs one Tabs set can switch between. Selecting a panel
// from a checked radio is a per-index CSS rule, so the kit sheet carries a
// ladder of exactly this many ("the tab ladder" in input.css) — keep the two
// in step. A surface wanting a ninth tab wants navigation, not tabs.
const MaxTabs = 8

type TabsProps struct {
	Name string // radio group name for the CSS :checked mechanism
	// Label names the tab set for assistive technology. The tabs are a radio
	// group, and a radio group without a name is announced as an anonymous set
	// of choices (WCAG 4.1.2).
	Label string
	Tabs  []Tab
	Attrs datastar.Attrs
}

type PaginationProps struct {
	Page    int
	Pages   int
	GetBase string // @get base; the kit appends ?page=N
	Attrs   datastar.Attrs
}

// SidebarSection groups sidebar items under an optional label.
type SidebarSection struct {
	Label string
	Items []NavItem
}

// SidebarNavProps is the fixed left-rail navigation: labeled sections of items
// with icons and active state.
type SidebarNavProps struct {
	Sections []SidebarSection
	Attrs    datastar.Attrs
}

// --- Buttons ---

// ButtonProps configures a Button.
//
// Label (or child text) is what names the button. A button carrying only
// LeadIcon or TailIcon has no accessible name and is announced as "button" —
// use IconButton for that shape instead, which takes the name as a field and so
// cannot be built without one.
type ButtonProps struct {
	Label    string
	Role     Role
	Size     Size
	Hazard   bool // diagonal warning stripes — texture for irreversible/destructive actions
	LeadIcon string
	TailIcon string
	Loading  string // signal expression that flips the loading state, e.g. "$busy"
	Disabled bool
	Type     string // "button" (default), "submit", "reset"
	Attrs    datastar.Attrs
}

type IconButtonProps struct {
	Icon  string
	Label string // accessible label / tooltip
	Role  Role
	Size  Size
	Attrs datastar.Attrs
}

// ButtonGroupProps is a segmented control: the children render joined into a
// single grouped control.
type ButtonGroupProps struct {
	Label string // accessible group label
	Attrs datastar.Attrs
}

// Segment is one toggle button in a Segmented control.
type Segment struct {
	Value  string
	Label  string
	Icon   string // optional leading icon
	Active bool
	On     string // optional @-expression run on click (e.g. "@get('/range/day')")
}

// SegmentedProps is a single-select toggle-button control: a joined row of
// buttons where exactly one is the active choice (aria-pressed). Unlike
// ButtonGroup (visual join only), the active segment carries selected styling
// and the group announces its purpose. Name is the accessible group label.
type SegmentedProps struct {
	Name     string // accessible group label
	Segments []Segment
	Size     Size
	Attrs    datastar.Attrs
}

// MenuItem is one entry in a menu-button popover.
type MenuItem struct {
	Label string
	Href  string // a link item; empty + Action makes a hypermedia item
	Icon  string
	On    string // optional @-expression run on click (e.g. "@post('/x')")
}

// MenuButtonProps is a label + caret that opens a native popover menu (no JS to
// toggle: the popover attribute drives it).
type MenuButtonProps struct {
	ID    string // anchor id wiring the trigger to its popover
	Label string
	Icon  string
	Role  Role
	Size  Size
	Items []MenuItem
	Attrs datastar.Attrs
}

// --- Input purpose ---------------------------------------------------------------

// Autocomplete is what a field is *for*, drawn from the token set WCAG 2.1
// SC 1.3.5 Identify Input Purpose names (its "Input Purposes for User Interface
// Components" list, which is HTML's autofill vocabulary).
//
// Naming the purpose is not only about saving typing. It is what lets a browser
// fill a field someone finds hard to type, and — the reason the criterion
// exists — what lets assistive software substitute a person's own familiar
// icons or wording over a field whose label they cannot read. A field collecting
// something on this list owes the token.
//
// The vocabulary is fixed by the specification, so it is a type rather than a
// string: a typo is a compile error instead of a field that silently autofills
// nothing. AutocompleteNone is the zero value and emits no attribute at all,
// which is different from AutocompleteOff — "off" is a positive instruction to
// the browser not to fill this field, appropriate for a one-time code or a
// field whose value is never the user's own.
type Autocomplete string

const (
	// AutocompleteNone is the zero value: no autocomplete attribute is emitted.
	AutocompleteNone Autocomplete = ""
	// AutocompleteOff asks the browser not to fill the field. Use it for
	// one-time codes and values that are not the user's own data — not as a
	// default, and never on a field this list has a token for.
	AutocompleteOff Autocomplete = "off"
)

// Identity. The person using the form.
const (
	AutocompleteName              Autocomplete = "name"               // Full name
	AutocompleteHonorificPrefix   Autocomplete = "honorific-prefix"   // Prefix or title (e.g., "Mr.", "Ms.", "Dr.", "M lle ")
	AutocompleteGivenName         Autocomplete = "given-name"         // Given name (in some Western cultures, also known as the first name )
	AutocompleteAdditionalName    Autocomplete = "additional-name"    // Additional names (in some Western cultures, also known as middle names , forenames other than the first name)
	AutocompleteFamilyName        Autocomplete = "family-name"        // Family name (in some Western cultures, also known as the last name or surname )
	AutocompleteHonorificSuffix   Autocomplete = "honorific-suffix"   // Suffix (e.g., "Jr.", "B.Sc.", "MBASW", "II")
	AutocompleteNickname          Autocomplete = "nickname"           // Nickname, screen name, handle: a typically short name used instead of the full name
	AutocompleteOrganizationTitle Autocomplete = "organization-title" // Job title (e.g., "Software Engineer", "Senior Vice President", "Deputy Managing Director")
	AutocompleteOrganization      Autocomplete = "organization"       // Company name corresponding to the person, address, or contact information in the other fields associated with this field
	AutocompleteUsername          Autocomplete = "username"           // A username
	AutocompleteNewPassword       Autocomplete = "new-password"       // A new password (e.g., when creating an account or changing a password)
	AutocompleteCurrentPassword   Autocomplete = "current-password"   // The current password for the account identified by the username field (e.g., when logging in)
	AutocompleteBday              Autocomplete = "bday"               // Birthday
	AutocompleteBdayDay           Autocomplete = "bday-day"           // Day component of birthday
	AutocompleteBdayMonth         Autocomplete = "bday-month"         // Month component of birthday
	AutocompleteBdayYear          Autocomplete = "bday-year"          // Year component of birthday
	AutocompleteSex               Autocomplete = "sex"                // Gender identity (e.g., Female, Fa’afafine)
	AutocompleteLanguage          Autocomplete = "language"           // Preferred language
	AutocompleteUrl               Autocomplete = "url"                // Home page or other web page corresponding to the company, person, address, or contact information in the other fields associated with this field
	AutocompletePhoto             Autocomplete = "photo"              // Photograph, icon, or other image corresponding to the company, person, address, or contact information in the other fields associated with this field
	AutocompleteImpp              Autocomplete = "impp"               // URL representing an instant messaging protocol endpoint (for example, " aim:goim?screenname=example " or " xmpp:fred@example.net ")
)

// Address. Where they are.
const (
	AutocompleteStreetAddress Autocomplete = "street-address" // Street address (multiple lines, newlines preserved)
	AutocompleteAddressLine1  Autocomplete = "address-line1"  // Street address (one line per field, line 1)
	AutocompleteAddressLine2  Autocomplete = "address-line2"  // Street address (one line per field, line 2)
	AutocompleteAddressLine3  Autocomplete = "address-line3"  // Street address (one line per field, line 3)
	AutocompleteAddressLevel4 Autocomplete = "address-level4" // The most fine-grained administrative level, in addresses with four administrative levels
	AutocompleteAddressLevel3 Autocomplete = "address-level3" // The third administrative level, in addresses with three or more administrative levels
	AutocompleteAddressLevel2 Autocomplete = "address-level2" // The second administrative level, in addresses with two or more administrative levels; in the countries with two administrative levels, this would typically be the city, town, village, or other locality within which the relevant street address is found
	AutocompleteAddressLevel1 Autocomplete = "address-level1" // The broadest administrative level in the address, i.e., the province within which the locality is found; for example, in the US, this would be the state; in Switzerland it would be the canton; in the UK, the post town
	AutocompleteCountry       Autocomplete = "country"        // Country code
	AutocompleteCountryName   Autocomplete = "country-name"   // Country name
	AutocompletePostalCode    Autocomplete = "postal-code"    // Postal code, post code, ZIP code, CEDEX code (if CEDEX, append "CEDEX", and the arrondissement , if relevant, to the address-level2 field)
)

// Payment. The instrument, not the person.
const (
	AutocompleteCcName              Autocomplete = "cc-name"              // Full name as given on the payment instrument
	AutocompleteCcGivenName         Autocomplete = "cc-given-name"        // Given name as given on the payment instrument (in some Western cultures, also known as the first name )
	AutocompleteCcAdditionalName    Autocomplete = "cc-additional-name"   // Additional names given on the payment instrument (in some Western cultures, also known as middle names , forenames other than the first name)
	AutocompleteCcFamilyName        Autocomplete = "cc-family-name"       // Family name given on the payment instrument (in some Western cultures, also known as the last name or surname )
	AutocompleteCcNumber            Autocomplete = "cc-number"            // Code identifying the payment instrument (e.g., the credit card number)
	AutocompleteCcExp               Autocomplete = "cc-exp"               // Expiration date of the payment instrument
	AutocompleteCcExpMonth          Autocomplete = "cc-exp-month"         // Month component of the expiration date of the payment instrument
	AutocompleteCcExpYear           Autocomplete = "cc-exp-year"          // Year component of the expiration date of the payment instrument
	AutocompleteCcCsc               Autocomplete = "cc-csc"               // Security code for the payment instrument (also known as the card security code (CSC), card validation code (CVC), card verification value (CVV), signature pancc-csc - Security code for the payment instrument (also known as the card security code (CSC), card validation code (CVC), card verification value (CVV), signature panel code (SPC), credit card ID (CCID), etc)
	AutocompleteCcType              Autocomplete = "cc-type"              // Type of payment instrument
	AutocompleteTransactionCurrency Autocomplete = "transaction-currency" // The currency that the user would prefer the transaction to use
	AutocompleteTransactionAmount   Autocomplete = "transaction-amount"   // The amount that the user would like for the transaction (e.g., when entering a bid or sale price)
)

// Contact. How to reach them.
const (
	AutocompleteTel            Autocomplete = "tel"              // Full telephone number, including country code
	AutocompleteTelCountryCode Autocomplete = "tel-country-code" // Country code component of the telephone number
	AutocompleteTelNational    Autocomplete = "tel-national"     // Telephone number without the country code component, with a country-internal prefix applied if applicable
	AutocompleteTelAreaCode    Autocomplete = "tel-area-code"    // Area code component of the telephone number, with a country-internal prefix applied if applicable
	AutocompleteTelLocal       Autocomplete = "tel-local"        // Telephone number without the country code and area code components
	AutocompleteTelLocalPrefix Autocomplete = "tel-local-prefix" // First part of the component of the telephone number that follows the area code, when that component is split into two components
	AutocompleteTelLocalSuffix Autocomplete = "tel-local-suffix" // Second part of the component of the telephone number that follows the area code, when that component is split into two components
	AutocompleteTelExtension   Autocomplete = "tel-extension"    // Telephone number internal extension code
	AutocompleteEmail          Autocomplete = "email"            // E-mail address
)

// --- Forms ---

// FieldProps is one label / control / help / error row.
//
// The association is automatic. Field publishes the ids of whatever Help and
// Error text it renders onto the context it hands its children, and the kit's
// controls (Input, Textarea, Select, Checkbox) read them back to fill in their
// own aria-describedby and aria-invalid. So a plain
//
//	@Field(FieldProps{Label: "Name", For: "name", Error: err}) {
//		@Input(TextInputProps{ID: "name", Name: "name"})
//	}
//
// announces "Name, invalid entry, <the error>" with nothing else to wire up
// (WCAG 1.3.1 / 3.3.1). With For empty the label wraps the control instead of
// pointing at it, so the association holds either way.
type FieldProps struct {
	Label string
	For   string // id of the labelled control; empty makes the label wrap it
	Help  string
	Error string // server-rendered field error
	// Required marks the field's label so the requirement is visible and
	// announced. It does not set the control's required attribute — that is
	// TextInputProps.Required, on the control that owns the constraint.
	Required bool
	Attrs    datastar.Attrs
}

// FieldsetProps groups related fields under a legend (a styled
// <fieldset>/<legend>, so the group never falls back to the UA default on
// dark). Help renders a description line under the legend; the grouped controls
// are the children.
type FieldsetProps struct {
	// ID is what the group's help line hangs off, so the <fieldset> can point
	// aria-describedby at it. Without an ID the help still renders but is only
	// visually associated — pass one whenever there is Help.
	ID       string
	Legend   string
	Help     string
	Disabled bool // disable every control in the group at once
	Attrs    datastar.Attrs
}

type TextInputProps struct {
	ID   string
	Name string
	// Label is the control's accessible name for the case where there is no
	// visible label to point at it — a search box, a filter in a toolbar. Inside
	// a Field, leave it empty: the Field's own label is the name, and setting
	// both means the visible text and the announced text can drift apart.
	Label string

	Type         string // "text" (default), "email", "password", "date", ...
	Value        string
	Placeholder  string
	Prefix       string
	Suffix       string
	Min          string       // numeric/date lower bound (min attribute)
	Max          string       // numeric/date upper bound (max attribute)
	Step         string       // numeric step granularity (step attribute; "any" for free)
	InputMode    string       // virtual-keyboard hint: "numeric", "decimal", "tel", ...
	Autocomplete Autocomplete // what the field is for (WCAG 1.3.5); see the token list above
	Spellcheck   string       // "true"/"false" — disable for codes, units, identifiers
	Required     bool         // native required + aria-required
	Error        bool         // force the invalid state; an enclosing Field's Error implies it
	Disabled     bool
	Attrs        datastar.Attrs
}

type TextareaProps struct {
	ID   string
	Name string
	// Label is the control's accessible name for the case where there is no
	// visible label to point at it — a search box, a filter in a toolbar. Inside
	// a Field, leave it empty: the Field's own label is the name, and setting
	// both means the visible text and the announced text can drift apart.
	Label    string
	Value    string
	Rows     int
	AutoGrow bool
	Error    bool
	Attrs    datastar.Attrs
}

// Option is one <option> in a Select.
type Option struct {
	Value    string
	Label    string
	Selected bool
	Disabled bool
}

// OptionGroup is a labelled set of options rendered as an <optgroup>. A Select
// uses Groups for grouped dropdowns and Options for the flat case; when both are
// set the flat Options render first, then the groups.
type OptionGroup struct {
	Label   string
	Options []Option
}

type SelectProps struct {
	ID   string
	Name string
	// Label is the control's accessible name for the case where there is no
	// visible label to point at it. Inside a Field, leave it empty: the Field's
	// own label is the name.
	Label       string
	Options     []Option      // flat options (the simple case)
	Groups      []OptionGroup // grouped options, rendered as <optgroup>
	Placeholder string
	Disabled    bool
	Error       bool
	Attrs       datastar.Attrs
}

// FormProps is the form element wrapper: it sets the layout (stacked or 2-column)
// and an optional @post/@get target. Fields are the children.
type FormProps struct {
	Post   string // @post target (hypermedia submit)
	Get    string // @get target (alternative to Post)
	TwoCol bool   // 2-column label/control layout vs stacked
	Attrs  datastar.Attrs
}

// CheckboxProps is a single checkbox with an inline label.
type CheckboxProps struct {
	ID      string
	Name    string
	Value   string
	Label   string
	Checked bool
	Error   bool
	Attrs   datastar.Attrs
}

// RadioOption is one choice in a RadioGroup.
type RadioOption struct {
	Value   string
	Label   string
	Checked bool
}

// RadioGroupProps is a set of radios sharing a Name; Inline lays them out in a row.
type RadioGroupProps struct {
	Name string
	// Label names the group for assistive technology. role=radiogroup without
	// an accessible name announces an anonymous set of choices (WCAG 4.1.2), so
	// this is effectively required — unless the group sits inside a Field, whose
	// label the group borrows automatically.
	Label   string
	Options []RadioOption
	Inline  bool
	Attrs   datastar.Attrs
}

// ToggleProps is a switch with a label and optional description.
type ToggleProps struct {
	ID          string
	Name        string
	Label       string
	Description string
	Checked     bool
	Attrs       datastar.Attrs
}

// InputGroupProps is a button-attached input (search box, chat send). The input
// is described by Input; Button is the attached trailing button.
type InputGroupProps struct {
	Input  TextInputProps
	Button ButtonProps
	Attrs  datastar.Attrs
}

// FormErrorProps is a form-level error summary banner (distinct from per-field
// errors). Messages lists the individual problems.
// FormErrorProps is the form-level error summary banner.
//
// Messages is the plain-text form; Problems is the same list with an anchor, so
// each entry links to the control it is about. Prefer Problems: a summary a
// keyboard user cannot act on is only half of WCAG 3.3.1. Both render; Messages
// come first.
type FormErrorProps struct {
	Title    string
	Messages []string
	Problems []Problem
	Attrs    datastar.Attrs
}

// Problem is one entry in a FormError summary: what is wrong, and the id of the
// control it is wrong about.
type Problem struct {
	Message string
	For     string // id of the offending control; empty renders plain text
}

// --- Data ---

// Column is a table header; Get is set when the header is sortable (@get).
type Column struct {
	Header string
	// HeaderHidden keeps the header text out of sight but in the accessible
	// tree — the answer for an actions or icon column, where an empty <th>
	// would leave that column's cells with no header at all (WCAG 1.3.1).
	HeaderHidden bool
	Get          string
	// Sort is the column's current sort state — SortAsc, SortDesc, or the zero
	// value for unsorted. It drives aria-sort and picks a directional chevron,
	// so the state is announced and visible rather than implied by the data
	// order alone (WCAG 1.3.1).
	Sort SortDir
}

// SortDir is a sortable column's current state.
type SortDir string

const (
	SortNone SortDir = ""
	SortAsc  SortDir = "ascending"
	SortDesc SortDir = "descending"
)

type TableProps struct {
	Columns []Column
	Rows    [][]string
	Dense   bool
	Empty   string // empty-state message rendered when Rows is empty
	// Caption names the table. A data table with no caption and no surrounding
	// heading is an unlabelled grid to anyone navigating by table (WCAG 1.3.1).
	// It renders as a visible <caption> unless CaptionHidden is set.
	Caption       string
	CaptionHidden bool
	Attrs         datastar.Attrs
}

// Delta is a stat's change indicator; Dir is "up"/"down"/"" and is never the
// only encoding (the kit pairs it with an arrow glyph and a label).
type Delta struct {
	Value string
	Dir   string
}

type StatProps struct {
	Label string
	Value string
	Delta Delta
	Attrs datastar.Attrs
}

type BadgeProps struct {
	Label string
	Role  Role
	Dot   bool
	Attrs datastar.Attrs
}

// DescItem is one key/value pair in a description list.
type DescItem struct {
	Term  string
	Value string
}

// DescListProps is a key/value grid. TwoCol renders terms and values side by
// side; otherwise a single-column stack.
type DescListProps struct {
	Items  []DescItem
	TwoCol bool
	Attrs  datastar.Attrs
}

// ListRow is one row in a StackedList: an optional leading avatar, a title and
// meta line, and an optional trailing action.
type ListRow struct {
	Avatar templ.Component // optional leading media slot
	Title  string
	Meta   string
	Href   string          // makes the row a link
	Action templ.Component // optional trailing action slot
	Badge  *BadgeProps     // optional trailing status badge
}

// StackedListProps is a vertical list of rows (feeds, item lists).
type StackedListProps struct {
	Rows  []ListRow
	Attrs datastar.Attrs
}

// GridListProps is a responsive card grid (property tiles, item cards). Cells are
// the children; Cols is the target column count.
type GridListProps struct {
	Cols  int
	Gap   Space   // between cells; zero value is the standard grid gap
	Min   Measure // narrowest a cell may get before a column drops
	Attrs datastar.Attrs
}

// --- Feedback ---

type AlertProps struct {
	Role        Role // good/warn/bad(info)
	Title       string
	Dismissible bool
	Attrs       datastar.Attrs
}

type EmptyStateProps struct {
	Icon    string
	Message string
	Attrs   datastar.Attrs
}

type SpinnerProps struct {
	Size  Size
	Block bool // block (centered) vs inline
	Attrs datastar.Attrs
}

type SkeletonProps struct {
	Width  string
	Height string
	Attrs  datastar.Attrs
}

// ToastPersist is the Dismiss value meaning "never expire". It is distinct from
// the zero value, which means "use the kit's default", so a caller can pin a
// toast open without knowing what the default happens to be.
const ToastPersist time.Duration = -1

// ToastDefaultLife is how long a toast lives when the caller says nothing.
const ToastDefaultLife = 6 * time.Second

// ToastProps is a single notification card, SSE-pushable and stacked by the
// kit's toast region. It carries a role, a message, and a manual close control.
type ToastProps struct {
	Role    Role
	Title   string
	Message string

	// Dismiss is how long the toast stays before fading. Zero takes the kit
	// default; ToastPersist keeps it until the visitor closes it.
	//
	// A timer on content is a time limit, and WCAG 2.2.1 constrains those, so
	// three things hold regardless of what is set here:
	//
	//   - The countdown pauses on hover and on focus-within, so a toast cannot
	//     expire while it is being read.
	//   - A toast carrying work — RoleDanger, RoleWarn — never auto-dismisses,
	//     because an error that vanishes is an error nobody fixed. Setting
	//     Dismiss explicitly overrides that.
	//   - A visitor can switch auto-dismiss off entirely (prefs.AutoDismiss),
	//     which is the criterion's own first remedy: turn the limit off before
	//     encountering it.
	Dismiss time.Duration
	Attrs   datastar.Attrs
}

// ProgressProps is a progress bar. When Max is 0 the bar is indeterminate
// (loads, unknown-duration work); otherwise it shows Value/Max.
type ProgressProps struct {
	Value float64
	Max   float64 // 0 = indeterminate
	Label string  // accessible label
	Role  Role
	Attrs datastar.Attrs
}

// --- Overlays ---

type ModalProps struct {
	ID    string
	Title string
	Get   string // optional @get that lazy-loads a heavy body under budget

	// Describe is the id of an element describing the dialog, wired as
	// aria-describedby on the <dialog> itself. It has to be on the dialog and
	// not on anything inside it: a description is announced along with the
	// name when focus enters the widget, and a description hung on a plain
	// <div> is announced by nothing at all.
	Describe string

	Attrs datastar.Attrs
}

// AuthModalProps configures a reusable login/register modal: a Modal (native
// <dialog>) holding two tabbed Forms. Each form posts its bound signals
// (username/password, + email/agree on register) to the given endpoints; the
// handler sets the session cookie and redirects on success, or patches an error
// fragment into the ErrorTarget slot on failure. Open it from a trigger with
// datastar.OnClick("document.getElementById('<ID>').showModal()").
// The forms submit as plain POSTs (native navigation), so the handler sets the
// session cookie and 303-redirects on success — no inline script, CSP-clean. On
// failure the handler redirects back with an error and the app re-renders this
// modal with OpenOnLoad + Error set so it reopens showing the message.
type AuthModalProps struct {
	ID           string // dialog id (default "auth-modal")
	Title        string // header text (default "Welcome")
	LoginPost    string // form action for login (e.g. "/auth/login")
	RegisterPost string // form action for register (e.g. "/auth/register")
	ErrorTarget  string // id of the error slot (default <ID>-error)
	ShowEmail    bool   // include an optional email field on the register form
	TOSHref      string // when set, register shows an "I accept the terms" checkbox + link
	OpenOnLoad   bool   // reopen the dialog on page load (after a failed attempt)
	Error        string // a message to show in the error slot
	Tab          string // initial tab: "login" (default) or "register"
	Attrs        datastar.Attrs
}

// authErrorID returns the error-slot id for an AuthModal (ErrorTarget or a
// derived default).
func authErrorID(p AuthModalProps) string {
	if p.ErrorTarget != "" {
		return p.ErrorTarget
	}
	return defaultID(p.ID, "auth-modal") + "-error"
}

// authTitle returns the AuthModal header (Title or a default).
func authTitle(ctx context.Context, p AuthModalProps) string {
	return a11yName(p.Title, i18n.T(ctx, "ui.auth.welcome"))
}

// authTabPressed binds a switcher button's aria-pressed to the tab signal, so
// the announced state follows the drawn state instead of being frozen at render
// time. It is the typed-builder route to a reactive attribute; a literal
// aria-pressed here would be a value that stops being true on the first click.
func authTabPressed(tab string) templ.Attributes {
	return datastar.Attrs{
		datastar.AttrBind("aria-pressed", ariaBoolExpr("$authtab === '"+tab+"'")),
	}.Templ()
}

// The following hold the Datastar expressions for the AuthModal tab signal. They
// live in Go (not inline in the .templ) because templ's attribute-expression
// parser is not string-aware and mis-balances the braces.

// authTab returns the initial tab ("login" default; "register" if requested).
func authTab(p AuthModalProps) string {
	if p.Tab == "register" {
		return "register"
	}
	return "login"
}

// authSignalsTab is the data-signals init for the tab signal.
func authSignalsTab(tab string) string { return "{authtab: '" + tab + "'}" }

// authOpenScript is the data-init expression that reopens the dialog on load.
func authOpenScript(id string) string {
	return "document.getElementById('" + id + "').showModal()"
}

// authTabClass is the data-class expression that accents the active tab button.
func authTabClass(tab string) string {
	return "{'dw-btn-accent': $authtab === '" + tab + "'}"
}

// TooltipProps is a hover/focus tip attached to its children.
//
// The tip describes the trigger (aria-describedby), so it is announced rather
// than merely displayed. Note the one place the kit knowingly falls short of
// WCAG 2.1: SC 1.4.13 asks that hover content be dismissible without moving
// focus, which a CSS-only tooltip cannot do — see the known exceptions in
// docs/rfc/06-accessibility.md. Use it for supplementary hints, never for
// information a user must have.
type TooltipProps struct {
	ID   string // id linking trigger to tip; defaults to a stable kit id
	Text string
	// Interactive marks the children as already focusable (a button, a link),
	// which stops the tooltip adding a tabindex of its own and creating a
	// second, redundant tab stop.
	Interactive bool
	Attrs       datastar.Attrs
}

// ConfirmProps configures the destructive-action dialog.
//
// WCAG 3.3.4 asks that an action which deletes or modifies a person's data be
// reversible, checked, or confirmed. Confirmed is the one a UI kit can provide,
// and the part that matters is not the extra click — it is that the dialog says
// what is about to happen while there is still time to stop it.
//
// So Message is not decoration: it is the consequence, and it is wired as the
// dialog's accessible description, which means a screen reader reads it with the
// dialog's name rather than only after the user has gone looking. A Confirm with
// a title and no message is a dialog that asks "are you sure?" about nothing.
type ConfirmProps struct {
	ID string
	// Title names the action — "Delete this booking?".
	Title string
	// Message is what will happen and whether it can be undone. Required in
	// spirit; it is the whole reason the dialog exists.
	Message string
	// ConfirmLabel names the destructive button. Say the verb — "Delete
	// booking", not "OK" — so someone reading only the buttons still knows
	// which one does the thing. Empty falls back to the catalog.
	ConfirmLabel string
	CancelLabel  string
	// Post is the action the confirm button submits to.
	Post string
	// Hazard adds the striped treatment the kit reserves for the irreversible.
	Hazard bool
	Attrs  datastar.Attrs
}

// DrawerProps is a slide-over panel. Side is "left" or "right" (default right).
// Open/close is native: it is a <dialog> opened with showModal, same as Modal,
// styled to slide in from the edge. Get lazy-loads a heavy body under budget.
type DrawerProps struct {
	ID    string
	Title string
	Side  string // "left" or "right" (default "right")
	Get   string // optional @get that lazy-loads a heavy body under budget
	Attrs datastar.Attrs
}

// PopoverProps is a native popover flyout anchored to a trigger. The children
// are the panel body. Toggling is the native popover attribute (no script).
type PopoverProps struct {
	ID    string // popover id wiring trigger -> panel
	Label string // trigger label
	Icon  string // optional trigger icon
	Attrs datastar.Attrs
}

// --- Media ---

type AvatarProps struct {
	Src      string
	Initials string // fallback when Src is empty
	// Alt names the person or thing the avatar stands for. Leave it empty when
	// the avatar sits next to that name already — the image is then decorative
	// and repeating the name only doubles the announcement.
	Alt  string
	Size Size
	// Status is the status-dot slug ("online"/"away"/...). It selects the dot's
	// color, which is why StatusLabel exists: color alone is never the encoding.
	Status string
	// StatusLabel is what the dot is announced as. Empty falls back to Status,
	// which is a slug and reads poorly — pass the app's own translated wording.
	StatusLabel string
	Attrs       datastar.Attrs
}

type ImageProps struct {
	Src string
	Set string // srcset — resolution switching for one crop
	// Sources render a <picture>, for what srcset cannot express: format
	// fallback (AVIF → WebP → the original) and art direction (a different
	// crop at a different width). Src stays the last-resort <img>, so a browser
	// that understands none of them still gets a picture.
	Sources []Source
	Sizes   string
	// Alt is the image's text alternative and is required (WCAG 1.1.1). A
	// genuinely decorative image — a texture, a rule, a duplicate of adjacent
	// text — sets Decorative instead; leaving both empty is a beach-vet finding
	// (rule a11y-img-alt), because that is what an accident looks like.
	Alt string
	// Decorative marks the image as conveying nothing, hiding it from assistive
	// technology (alt="" plus aria-hidden). It is mutually exclusive with Alt.
	Decorative bool
	Ratio      Ratio
	Fit        Fit
	Eager      bool // above-fold hero opts into eager + fetchpriority=high
	Attrs      datastar.Attrs
}

type AspectBoxProps struct {
	Ratio Ratio
	Attrs datastar.Attrs
}

// Source is one alternative encoding or crop of a piece of media, for a
// <picture> or a <video>. The browser takes the first it can play or that
// matches, so order is the preference order — modern format first, fallback
// last.
type Source struct {
	Src string
	// Type is the MIME type ("image/avif", "video/webm"). Without it the
	// browser has to fetch to find out, which defeats the point.
	Type string
	// Media is an optional media query, for art direction — a different crop
	// at a different width, not merely a different size of the same crop.
	Media string
}

// Track is a timed text track on a video: captions, subtitles, or a described
// audio cue list.
//
// Captions are not optional decoration. Without them a video is unusable to
// anyone deaf or hard of hearing, and unusable to anyone in a room where sound
// is not possible — which is why WCAG 1.2.2 puts them at Level A.
type Track struct {
	Src string
	// Kind is the track kind: "captions" (default), "subtitles",
	// "descriptions", "chapters", or "metadata". Captions carry speech *and*
	// the non-speech sound that carries meaning; subtitles carry speech only,
	// for someone who can hear it but not understand it. They are not the same
	// thing and one does not substitute for the other.
	Kind string
	// Lang is the BCP 47 tag of the track's language. Required for subtitles.
	Lang string
	// Label is what the viewer picks in the track menu.
	Label   string
	Default bool
}

// VideoProps configures the kit's video. It exists as a component rather than a
// raw <video> for the same reason Image does: the perf and accessibility rules
// are non-negotiable, so they are built in rather than remembered.
//
// What it enforces rather than documents:
//
//   - The frame is reserved by an aspect box before a byte arrives, so late
//     media fills reserved pixels instead of shoving the page down.
//   - preload is "none" by default: a poster paints, and the video downloads
//     when someone asks for it.
//   - Controls are always present. A video that moves on its own for more than
//     five seconds alongside other content needs a way to stop it (WCAG 2.2.2),
//     and native controls *are* that mechanism. The kit will not ship
//     autoplaying motion a visitor cannot pause, and pausing without controls
//     would need script, which local interactivity here never uses.
//   - Autoplay implies muted. Sound that starts on its own is its own criterion
//     (WCAG 1.4.2) and the kit does not offer it at all.
type VideoProps struct {
	Src     string
	Sources []Source // format fallback, first playable wins
	Poster  string
	Ratio   Ratio // reserves the frame; a video without one is a vet smell
	Fit     Fit
	Tracks  []Track

	// Background makes it a muted, looping, autoplaying video. Controls are
	// still rendered — see the type doc.
	Background bool
	// Preload overrides the default "none". Use "metadata" when the duration
	// needs to be known before play.
	Preload string
	// Label is the video's accessible name.
	Label string
	// Describe is the text alternative: what someone who cannot see it would
	// need. It renders screen-reader-only.
	Describe string
	Attrs    datastar.Attrs
}

// MediaObjectProps is the thumbnail-beside-body-beside-actions row: a comment, a
// search result, a notification with an avatar.
type MediaObjectProps struct {
	Media   templ.Component // avatar, thumbnail, icon
	Body    templ.Component
	Actions templ.Component
	Attrs   datastar.Attrs
}

// FigureProps is an image with a caption (<figure>/<figcaption>). The image
// props are embedded so the figure inherits the media perf laws.
type FigureProps struct {
	Image   ImageProps
	Caption string
	Attrs   datastar.Attrs
}

// --- Messaging ---

// Message is one entry in a MessageList.
type Message struct {
	Author string
	Body   string
	Own    bool
	System bool
	At     string // pre-formatted timestamp
}

type MessageListProps struct {
	Messages []Message
	Attrs    datastar.Attrs
}

type ComposerProps struct {
	Name string // bound signal / field name
	// Label names the message box. A placeholder is not a label: it disappears
	// the moment there is text, and some screen readers never announce it
	// (WCAG 3.3.2). Empty falls back to Placeholder so an existing composer
	// still gets a name, but pass a real one.
	Label       string
	Placeholder string
	Post        string // @post target for send
	Attrs       datastar.Attrs
}

type PresencePillProps struct {
	State string // "connected"/"reconnecting"/"paired"/"waiting"
	Label string
	Attrs datastar.Attrs
}

// LiveToggleProps configures the control that pauses and resumes a page's
// server-pushed updates, and the element that opens the stream.
//
// Both halves live in one component on purpose: the mechanism only works if the
// element that opens the stream disappears when the visitor pauses, and keeping
// them together means an app cannot render the button and forget the wiring.
type LiveToggleProps struct {
	// ID is the id of the element that owns the stream. It stays stable across
	// the paused/live swap so a fragment can still target it.
	ID string
	// Stream is the @get target that opens the SSE connection — the same URL an
	// app would otherwise put in a data-init by hand.
	Stream string
	// Label names what is updating ("Board", "Prices"), so the control reads as
	// "Pause Board updates" rather than an anonymous "Pause". Empty falls back
	// to the catalog's generic wording.
	Label string
	Attrs datastar.Attrs
}

// SchemeToggleProps configures the light/dark control.
//
// The choice is tri-state, and the third state is the important one: "match
// system" is not the absence of a choice, it is a choice to defer to the
// operating system, and a two-way toggle cannot express it. A visitor who has
// pinned light and later wants their OS to decide again has nowhere to go.
type SchemeToggleProps struct {
	// Label names the control. Empty takes the catalog's wording.
	Label string
	Attrs datastar.Attrs
}

// LiveStreamProps configures an extra stream holder governed by a page's
// [LiveToggle].
//
// One control can only carry one Stream, and a page with several — boardwalk
// opens three — needs the rest to obey the same pause. This is that holder: the
// same element LiveToggle renders, without a second button beside it.
type LiveStreamProps struct {
	// ID is the id of the element that owns the stream. It stays stable across
	// the paused/live swap so a fragment can still target it.
	ID string
	// Stream is the @get target that opens the SSE connection.
	Stream string
	Attrs  datastar.Attrs
}

// PageProps configures the document shell Page renders.
//
// Lang and Dir are overrides, not the normal path: both default to the request's
// active i18n locale, so a page served under an es-ES locale declares
// lang="es-ES" without the app doing anything (WCAG 3.1.1). Set them only when
// a page's language genuinely differs from the request's.
type PageProps struct {
	Title       string // <title> text
	Lang        string // <html lang>; defaults to the request locale, then "en"
	Dir         string // <html dir>; defaults to the direction of the active lang
	Description string // optional meta description
	BodyClass   string // optional extra class on <body>
	Stylesheet  string // override the kit stylesheet href (default /static/css/app.css)
}

// --- Markdown editor ---

// MarkdownEditorProps configures the kit's Markdown textarea + toolbar.
//
// The island in /static/js/md-editor.js binds to .dw-md, reads data-preview-url /
// data-image-url / data-tus-url / data-csrf, and runs toolbar clicks off
// data-md-cmd. The Page shell does not load that module (CSP script-src is
// 'self', so an inline tag is forbidden). Apps that mount an editor include:
//
//	<script type="module" src="/static/js/md-editor.js" defer></script>
type MarkdownEditorProps struct {
	Name       string // textarea name, default "body"
	Value      string
	Label      string
	PreviewURL string
	ImageURL   string
	TusURL     string
	CSRF       string
	Attrs      datastar.Attrs
}

// --- Consent ---

// ConsentBannerProps is the first-visit cookie prompt: a non-blocking corner
// card, not a modal. It stays in the DOM when closed (hidden) so a footer
// ConsentLink can reopen it. Copy comes from the catalog. Driftwood does not
// import pkg/consent — pass Open from consent.NeedsPrompt and wire the
// allow/necessary/customize actions as Attrs.
type ConsentBannerProps struct {
	Open           bool
	DetailsURL     string
	AllowAttrs     datastar.Attrs // OnClick for allow
	NecessaryAttrs datastar.Attrs
	CustomizeAttrs datastar.Attrs
	Attrs          datastar.Attrs
}

// ConsentCategory is one row in ConsentManager. Necessary rows are listed as
// always-on; the rest render as Toggles named for a form/signal post.
type ConsentCategory struct {
	Name        string // form/signal name, e.g. "analytics"
	Label       string
	Description string
	Necessary   bool
	Allowed     bool
}

// ConsentManagerProps is the always-visible category list for /cookies.
type ConsentManagerProps struct {
	Categories []ConsentCategory
	SaveAttrs  datastar.Attrs
	Attrs      datastar.Attrs
}

// ConsentLinkProps is the footer control that reopens the banner or goes to
// /cookies. Href makes it a link; otherwise it is a button. The label is
// always the catalog's cookie-settings wording.
type ConsentLinkProps struct {
	Href  string
	Attrs datastar.Attrs
}
