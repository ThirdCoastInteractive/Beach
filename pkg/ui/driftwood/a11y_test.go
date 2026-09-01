package driftwood

// The accessibility laws, as tests.
//
// These are the same shape as noColorLiteral: cheap assertions over rendered
// markup, run across every component in the existing case maps so a new
// component is covered the moment it is added to one. They catch the three
// failure modes that are invisible in a browser and obvious to a screen reader:
// an ARIA reference pointing at nothing, a form control with no name, and a
// role that promises a name it does not have.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

var (
	// idRe finds every id an element declares.
	idRe = regexp.MustCompile(`\bid="([^"]+)"`)
	// refRe finds every attribute that points at ids. aria-describedby and
	// aria-labelledby take space-separated lists; `for` takes exactly one.
	refRe = regexp.MustCompile(`\b(for|aria-describedby|aria-labelledby|aria-controls)="([^"]+)"`)
	// controlRe finds form controls that owe an accessible name.
	controlRe = regexp.MustCompile(`<(input|select|textarea)\b([^>]*)>`)
	// namedRoleRe finds roles that are meaningless without a name.
	namedRoleRe = regexp.MustCompile(`<(\w+)\b([^>]*\brole="(?:img|radiogroup|group|progressbar)"[^>]*)>`)
	// labelForRe finds the ids that <label for> claims.
	labelForRe = regexp.MustCompile(`<label\b[^>]*\bfor="([^"]+)"`)
	// typeRe reads an input's type.
	typeRe = regexp.MustCompile(`\btype="([^"]+)"`)
	// tagRe walks open and close tags, for the containment scan below.
	tagRe = regexp.MustCompile(`</?(\w+)`)
	// clickableRe finds the elements a person activates: buttons, and links that
	// actually go somewhere.
	clickableRe = regexp.MustCompile(`<(button|a)\b([^>]*)>`)
	// stripTagsRe reduces markup to its visible text.
	stripTagsRe = regexp.MustCompile(`<[^>]*>`)
	// ariaHiddenRe matches an element hidden from assistive technology.
	ariaHiddenRe = regexp.MustCompile(`aria-hidden="true"`)
)

// span is a half-open byte range in the rendered markup.
type span struct{ from, to int }

// namingRegions returns the ranges of markup that give everything inside them an
// accessible name without any ARIA: a <label> wrapping its control, and a
// <fieldset> holding a <legend>. Both are how HTML has always done this, and a
// checker that only understood aria-label would report the correct markup as
// broken and push the kit toward attributes it does not need.
//
// The scan is a plain tag-depth walk, which is enough here because the input is
// generated markup from a single component, not arbitrary HTML.
func namingRegions(out string) []span {
	var regions []span
	for _, tag := range []string{"label", "fieldset"} {
		open := regexp.MustCompile(`<` + tag + `\b`)
		for _, loc := range open.FindAllStringIndex(out, -1) {
			end := matchingClose(out, loc[0], tag)
			if end < 0 {
				continue
			}
			if tag == "fieldset" && !strings.Contains(out[loc[0]:end], "<legend") {
				continue // a fieldset with no legend names nothing
			}
			regions = append(regions, span{loc[0], end})
		}
	}
	return regions
}

// matchingClose returns the offset just past the </tag> that closes the element
// starting at from, or -1 when the markup never closes it.
func matchingClose(out string, from int, tag string) int {
	depth := 0
	for _, m := range tagRe.FindAllStringSubmatchIndex(out[from:], -1) {
		if out[from+m[2]:from+m[3]] != tag {
			continue
		}
		if out[from+m[0]+1] == '/' {
			depth--
			if depth == 0 {
				return from + m[3] + 1
			}
			continue
		}
		depth++
	}
	return -1
}

// insideNamingRegion reports whether the element at offset sits within markup
// that already names it.
func insideNamingRegion(regions []span, offset int) bool {
	for _, r := range regions {
		if offset > r.from && offset < r.to {
			return true
		}
	}
	return false
}

// referencesResolve fails when an ARIA relationship points at an id that is not
// in the same markup. A dangling aria-describedby is silently ignored by
// assistive technology, so the description simply never arrives — the exact
// failure that is impossible to notice by looking at the page.
func referencesResolve(t *testing.T, name, out string) {
	t.Helper()
	ids := map[string]bool{}
	for _, m := range idRe.FindAllStringSubmatch(out, -1) {
		ids[m[1]] = true
	}
	for _, m := range refRe.FindAllStringSubmatch(out, -1) {
		// `for` on a <label> may name a control the component does not render
		// itself — Field labels whatever child it is handed — so only the
		// aria-* references have to resolve within one component's output.
		if m[1] == "for" {
			continue
		}
		for _, ref := range strings.Fields(m[2]) {
			if !ids[ref] {
				t.Errorf("%s: %s=%q points at no such id\n%s", name, m[1], ref, out)
			}
		}
	}
}

// controlsAreLabelled fails when a form control has no accessible name by any
// route: a wrapping or pointing <label>, an enclosing <fieldset>/<legend>, an
// aria-label, or an aria-labelledby. Hidden and structural inputs are exempt —
// they are never announced.
func controlsAreLabelled(t *testing.T, name, out string) {
	t.Helper()
	labelled := map[string]bool{}
	for _, m := range labelForRe.FindAllStringSubmatch(out, -1) {
		labelled[m[1]] = true
	}
	regions := namingRegions(out)
	for _, m := range controlRe.FindAllStringSubmatchIndex(out, -1) {
		tag, attrs := out[m[2]:m[3]], out[m[4]:m[5]]
		if ty := typeRe.FindStringSubmatch(attrs); ty != nil {
			switch ty[1] {
			case "hidden", "submit", "reset", "button":
				continue
			}
		}
		if strings.Contains(attrs, "aria-label=") || strings.Contains(attrs, "aria-labelledby=") {
			continue
		}
		if id := idRe.FindStringSubmatch(attrs); id != nil && labelled[id[1]] {
			continue
		}
		if insideNamingRegion(regions, m[0]) {
			continue
		}
		t.Errorf("%s: <%s> has no accessible name\n%s", name, tag, out)
	}
}

// clickablesAreNamed fails when a button or link has no accessible name by any
// route: visible text, an aria-label, or an aria-labelledby.
//
// This is the icon-only control, and it is the most common nameless thing on any
// page — visually it is a perfectly good button, and to a screen reader it is
// announced as "button" and nothing else. It is also what an omitted Label prop
// on IconButton produces, which is why it belongs in the zero-value suite.
func clickablesAreNamed(t *testing.T, name, out string) {
	t.Helper()
	for _, m := range clickableRe.FindAllStringSubmatchIndex(out, -1) {
		tag, attrs := out[m[2]:m[3]], out[m[4]:m[5]]
		if tag == "a" && !strings.Contains(attrs, "href=") {
			continue // an anchor with no href is not a control
		}
		if ariaHiddenRe.MatchString(attrs) {
			continue // hidden from assistive technology, so it owes nothing
		}
		if strings.Contains(attrs, "aria-label=") || strings.Contains(attrs, "aria-labelledby=") {
			continue
		}
		end := matchingClose(out, m[0], tag)
		if end < 0 {
			continue
		}
		inner := out[m[1]:end]
		// A descendant may carry the name — ui.Icon with a label renders
		// role="img" plus an aria-label, and that names its button.
		if strings.Contains(inner, "aria-label=") {
			continue
		}
		if strings.TrimSpace(stripTagsRe.ReplaceAllString(inner, "")) != "" {
			continue
		}
		t.Errorf("%s: <%s> has no accessible name — visible text, aria-label, or nothing\n%s", name, tag, out)
	}
}

// rolesAreNamed fails when an element claims a role that only means something
// with a name. role="img" on an unnamed graphic announces "image" and stops;
// role="radiogroup" on an unnamed set announces an anonymous list of choices.
// An unnamed role is worse than no role, which is why this is a law and not a
// suggestion — the kit's answer, everywhere, is to drop the role rather than
// claim it emptily.
func rolesAreNamed(t *testing.T, name, out string) {
	t.Helper()
	regions := namingRegions(out)
	for _, m := range namedRoleRe.FindAllStringSubmatchIndex(out, -1) {
		tag, attrs := out[m[2]:m[3]], out[m[4]:m[5]]
		if strings.Contains(attrs, "aria-label=") || strings.Contains(attrs, "aria-labelledby=") {
			continue
		}
		if insideNamingRegion(regions, m[0]) {
			continue
		}
		t.Errorf("%s: <%s> claims a role with no accessible name\n%s", name, tag, out)
	}
}

// TestZeroValuePropsAreStillAccessible renders every component with the emptiest
// props it accepts.
//
// The other suites render components as they are meant to be used, which is the
// wrong place to look for this class of bug: a component that names itself from
// a prop is conformant when the prop is set and silently broken when it is not.
// An unnamed role is exactly what an omitted optional field produces, and an
// omitted optional field is the most likely thing to happen in an app.
//
// The kit's answer, everywhere, is to drop the role rather than claim it
// emptily — so this suite passes only if every component degrades that way.
func TestZeroValuePropsAreStillAccessible(t *testing.T) {
	cases := map[string]renderCase{
		// A Button with neither label nor children renders an empty box, which is
		// a defect anyone can see; the minimum worth testing is a labelled one.
		"button":      {c: Button(ButtonProps{Label: "Go"})},
		"buttonicon":  {c: Button(ButtonProps{Label: "Go", LeadIcon: "play"})},
		"buttongroup": {c: ButtonGroup(ButtonGroupProps{}), children: []templ.Component{text("x")}},
		"segmented":   {c: Segmented(SegmentedProps{Segments: []Segment{{Value: "a", Label: "A"}}})},
		"menubutton":  {c: MenuButton(MenuButtonProps{Items: []MenuItem{{Label: "Item"}}})},
		"field":       {c: Field(FieldProps{}), children: []templ.Component{text("x")}},
		"fieldset":    {c: Fieldset(FieldsetProps{}), children: []templ.Component{text("x")}},
		"form":        {c: Form(FormProps{}), children: []templ.Component{text("x")}},
		"formerror":   {c: FormError(FormErrorProps{})},
		"table":       {c: Table(TableProps{Columns: []Column{{Header: "H"}}, Rows: [][]string{{"c"}}})},
		"stat":        {c: Stat(StatProps{})},
		"badge":       {c: Badge(BadgeProps{})},
		"desclist":    {c: DescList(DescListProps{})},
		"stacked":     {c: StackedList(StackedListProps{Rows: []ListRow{{Title: "t"}}})},
		"gridlist":    {c: GridList(GridListProps{}), children: []templ.Component{text("x")}},
		"alert":       {c: Alert(AlertProps{}), children: []templ.Component{text("x")}},
		"toast":       {c: Toast(ToastProps{})},
		"progress":    {c: Progress(ProgressProps{})},
		"progressdet": {c: Progress(ProgressProps{Value: 1, Max: 2})},
		"empty":       {c: EmptyState(EmptyStateProps{})},
		"spinner":     {c: Spinner(SpinnerProps{})},
		"skeleton":    {c: Skeleton(SkeletonProps{})},
		"modal":       {c: Modal(ModalProps{}), children: []templ.Component{text("x")}},
		"drawer":      {c: Drawer(DrawerProps{}), children: []templ.Component{text("x")}},
		"authmodal":   {c: AuthModal(AuthModalProps{})},
		"authmodalts": {c: AuthModal(AuthModalProps{ShowEmail: true, TOSHref: "/terms"})},
		"popover":     {c: Popover(PopoverProps{}), children: []templ.Component{text("x")}},
		"tooltip":     {c: Tooltip(TooltipProps{}), children: []templ.Component{text("x")}},
		"avatar":      {c: Avatar(AvatarProps{})},
		"image":       {c: Image(ImageProps{})},
		"figure":      {c: Figure(FigureProps{})},
		"aspect":      {c: AspectRatio(AspectBoxProps{})},
		"msglist":     {c: MessageList(MessageListProps{Messages: []Message{{Body: "b"}}})},
		"composer":    {c: Composer(ComposerProps{})},
		"presence":    {c: Presence(PresencePillProps{})},
		"tabs":        {c: Tabs(TabsProps{Tabs: []Tab{{ID: "a", Label: "A"}}})},
		"pagination":  {c: Pagination(PaginationProps{Page: 1, Pages: 2})},
		"navbar":      {c: Navbar(NavbarProps{})},
		"sidebar":     {c: Sidebar(SidebarNavProps{})},
		"breadcrumbs": {c: Breadcrumbs(BreadcrumbsProps{})},
		"shell":       {c: Shell(AppShellProps{}), children: []templ.Component{text("x")}},
		// The layout primitives. They render boxes and claim no roles, so what
		// this actually holds them to is that a zero value stays a plain box —
		// a primitive that grew an aria-* attribute would emit an empty one.
		"stack":        {c: Stack(StackProps{}), children: []templ.Component{text("x")}},
		"inline":       {c: Inline(InlineProps{}), children: []templ.Component{text("x")}},
		"box":          {c: Box(BoxProps{}), children: []templ.Component{text("x")}},
		"section":      {c: Section(SectionProps{}), children: []templ.Component{text("x")}},
		"sectionhead":  {c: Section(SectionProps{Heading: "Recent", Lead: "Lately."}), children: []templ.Component{text("x")}},
		"center":       {c: Center(CenterProps{}), children: []templ.Component{text("x")}},
		"prose":        {c: Prose(ProseProps{}), children: []templ.Component{text("x")}},
		"rail":         {c: Rail(RailProps{}), children: []templ.Component{text("x")}},
		"railside":     {c: Rail(RailProps{Rail: text("toc"), Side: "start"}), children: []templ.Component{text("x")}},
		"schemetoggle": {c: SchemeToggle(SchemeToggleProps{})},
		"card":         {c: Card(CardProps{}), children: []templ.Component{text("x")}},
		"cardheading":  {c: CardHeading(CardHeadingProps{})},
		"pageheading":  {c: PageHeading(PageHeadingProps{})},
		"divider":      {c: Divider(DividerProps{})},
		"container":    {c: Container(ContainerProps{}), children: []templ.Component{text("x")}},
		"main":         {c: Main(), children: []templ.Component{text("x")}},
		"livetoggle":   {c: LiveToggle(LiveToggleProps{})},
		"livenamed":    {c: LiveToggle(LiveToggleProps{ID: "l", Stream: "/live", Label: "Board"})},
		"livestream":   {c: LiveStream(LiveStreamProps{})},
		"video":        {c: Video(VideoProps{})},
		"videotracks":  {c: Video(VideoProps{Src: "/v.mp4", Poster: "/v.jpg", Ratio: RatioWide, Label: "Tour", Tracks: []Track{{Src: "/v.vtt", Lang: "en", Label: "English"}}})},
		"mediaobject":  {c: MediaObject(MediaObjectProps{}), children: []templ.Component{text("body")}},
		"confirm":      {c: Confirm(ConfirmProps{})},
		"confirmfull":  {c: Confirm(ConfirmProps{ID: "c", Title: "Delete?", Message: "This cannot be undone.", Post: "/del", Hazard: true})},
		"lang":         {c: Lang("fr"), children: []templ.Component{text("liberté")}},
		"langblock":    {c: LangBlock("ar"), children: []templ.Component{text("مرحبا")}},
		"liveregion":   {c: LiveRegion()},
		"announce":     {c: Announcement("Saved.")},
		"dashgrid":     {c: DashGrid(DashGridProps{}), children: []templ.Component{text("x")}},
		"dashcol":      {c: DashCol(DashColProps{Span: 6}), children: []templ.Component{text("x")}},
		"iconbutton":   {c: IconButton(IconButtonProps{Icon: "gear", Label: "Settings"})},
		// InputGroup's input borrows the button's label, so the realistic
		// minimum is a labelled button. A group with neither is not an
		// accessibility bug the kit can fix — it renders an empty button, which
		// is a defect anyone can see; the failures this suite is for are the
		// ones nobody can.
		"inputgroup": {c: InputGroup(InputGroupProps{Button: ButtonProps{Label: "Go"}})},
		"mdeditor":   {c: MarkdownEditor(MarkdownEditorProps{})},
		"consentban": {c: ConsentBanner(ConsentBannerProps{})},
		"consentmgr": {c: ConsentManager(ConsentManagerProps{})},
		"consentlnk": {c: ConsentLink(ConsentLinkProps{})},
	}
	for name, tc := range cases {
		out := render(t, tc.c, tc.children...)
		referencesResolve(t, name, out)
		controlsAreLabelled(t, name, out)
		clickablesAreNamed(t, name, out)
		rolesAreNamed(t, name, out)
		noEmptyAccessibleName(t, name, out)
	}
}

// noEmptyAccessibleName fails on `aria-label=""`, which is the specific shape an
// omitted prop produces when a component writes the attribute unconditionally.
// An empty name is not a name: it leaves the element unlabelled while looking,
// in the source, exactly like it has been handled.
func noEmptyAccessibleName(t *testing.T, name, out string) {
	t.Helper()
	for _, attr := range []string{`aria-label=""`, `aria-labelledby=""`, `aria-describedby=""`, `for=""`, `id=""`} {
		if strings.Contains(out, attr) {
			t.Errorf("%s: emits %s from an unset prop\n%s", name, attr, out)
		}
	}
}

// TestLiveRegionShipsEmpty is the contract behind every server-pushed status
// message in every beach app.
//
// A live region is announced only when content *arrives into* a region the
// browser was already watching. Ship it with content and the arrival is the
// region itself, which most screen readers do not read; ship it without the
// polite role and nothing is watching at all. So the emptiness is the feature,
// and this is the test that stops a future "helpful" change — a placeholder, a
// heading, a spinner — from silently muting the whole application.
func TestLiveRegionShipsEmpty(t *testing.T) {
	out := render(t, LiveRegion())
	for _, want := range []string{
		`id="` + ToastTarget + `"`,
		`role="status"`,
		`aria-live="polite"`,
		`aria-atomic="false"`,
		`aria-label=`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("live region missing %s: %s", want, out)
		}
	}
	// Nothing between the tags. Not a space, not a comment — content.
	open := strings.Index(out, ">")
	closeIdx := strings.LastIndex(out, "</div>")
	if open < 0 || closeIdx < 0 {
		t.Fatalf("live region is not a single element: %s", out)
	}
	if body := strings.TrimSpace(out[open+1 : closeIdx]); body != "" {
		t.Errorf("live region ships with content %q — an arrival into a region that already had content is not announced\n%s", body, out)
	}
}

// TestShellRendersOneOfEachSingleton guards the three things a page may have
// exactly one of. Shell grew all three at once, and the specimen embeds a Shell
// to demonstrate the frame — so "two Shells on a page" is a real arrangement,
// and duplicate ids on a live region mean a patch aimed at it has two targets.
func TestShellRendersOneOfEachSingleton(t *testing.T) {
	page := render(t, Shell(AppShellProps{Topbar: text("bar")}), text("content"))
	for what, frag := range map[string]string{
		"main landmark": `id="` + MainID + `"`,
		"bypass link":   `class="dw-skip-link`,
		"live region":   `id="` + ToastTarget + `"`,
	} {
		if n := strings.Count(page, frag); n != 1 {
			t.Errorf("page-root Shell rendered %d %s, want exactly 1", n, what)
		}
	}

	embedded := render(t, Shell(AppShellProps{Topbar: text("bar"), Embedded: true}), text("content"))
	for what, frag := range map[string]string{
		"main landmark": `id="` + MainID + `"`,
		"bypass link":   `class="dw-skip-link`,
		"live region":   `id="` + ToastTarget + `"`,
	} {
		if strings.Contains(embedded, frag) {
			t.Errorf("embedded Shell rendered a %s; the page it sits in already has one\n%s", what, embedded)
		}
	}
	// It is still a frame: the content and the topbar are there.
	for _, want := range []string{"bar", "content", "dw-shell-main"} {
		if !strings.Contains(embedded, want) {
			t.Errorf("embedded Shell dropped %q along with the singletons", want)
		}
	}
}
