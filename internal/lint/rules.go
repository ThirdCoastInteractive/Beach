package lint

import (
	"go/ast"
	"regexp"
	"strings"
)

// lintSelf is this package's own directory, as fileCtx.pkg spells it. Rules
// whose patterns are themselves markup-shaped exempt it: the analyzer's source
// is full of tag fragments that are regexes, not HTML.
const lintSelf = "internal/lint"

// --- Rule 1: raw Datastar attribute strings ----------------------------------

// dataAttrRe matches a Datastar data-* attribute name as it appears in markup:
// data-on, data-bind, data-signals, data-show, data-text, data-class,
// data-attr, data-indicator, data-ref, data-computed, data-effect. Both colon
// (data-on:click) and dash (data-on-load) spellings are caught. The boundary
// requires the token to start an attribute (preceded by a quote, whitespace,
// '<', or start-of-string) so we don't trip on unrelated substrings. The set is
// the free Datastar core plugins only — Pro-only attrs (data-persist, …) are
// not shipped, so app code can't lean on them.
var dataAttrRe = regexp.MustCompile(
	`(^|[\s"'<` + "`" + `])data-(on|bind|signals|show|text|class|attr|indicator|ref|computed|effect)\b`)

// ruleRawDatastar flags string literals containing raw Datastar data-* attribute
// markup outside the sanctioned framework path. The datastar package (the typed
// builders themselves) and the ui kit (which renders the builders' output and a
// couple of self-remove handlers by hand) are the sanctioned emitters; app code
// must go through the datastar builders instead of hand-writing the strings.
func ruleRawDatastar(fc *fileCtx, out *[]Finding) {
	if fc.onPath("datastar", "ui", "lint") {
		return
	}
	fc.stringLits2(func(val string, line int) {
		if dataAttrRe.MatchString(val) {
			*out = append(*out, Finding{
				File:    fc.path,
				Line:    line,
				Rule:    "raw-datastar",
				Message: "raw Datastar data-* attribute in a string literal; use the datastar typed builders (datastar.On/Bind/Signals/Show/...) instead",
			})
		}
	})
}

// --- Rule 2: pgtype imports outside internal/db ------------------------------

// rulePgtype flags any import of a pgtype package outside the generated/db layer.
// pgtype values are a leak of the storage driver into the domain; sqlc is
// configured to purge them, so an import elsewhere means hand-rolled DB glue in
// the wrong place.
func rulePgtype(fc *fileCtx, out *[]Finding) {
	if fc.onPath("internal/db") {
		return
	}
	forEachImport(fc, func(path string, line int) {
		if path == "github.com/jackc/pgx/v5/pgtype" || strings.HasSuffix(path, "/pgtype") {
			*out = append(*out, Finding{
				File:    fc.path,
				Line:    line,
				Rule:    "pgtype-import",
				Message: "pgtype imported outside internal/db; keep driver types in the generated db layer and map to domain types",
			})
		}
	})
}

// --- Rule 3: google/uuid outside a sanctioned package ------------------------

// uuidSanctioned is the set of package directories allowed to import
// google/uuid. Keep this tight: IDs flow as domain types, so the dependency
// should be contained to where UUIDs are actually generated or decoded.
var uuidSanctioned = []string{"internal/db", "auth", "session"}

// ruleUUID flags google/uuid imports outside the sanctioned packages.
func ruleUUID(fc *fileCtx, out *[]Finding) {
	if fc.onPath(uuidSanctioned...) {
		return
	}
	forEachImport(fc, func(path string, line int) {
		if path == "github.com/google/uuid" {
			*out = append(*out, Finding{
				File:    fc.path,
				Line:    line,
				Rule:    "uuid-import",
				Message: "github.com/google/uuid imported outside a sanctioned package (internal/db, auth, session); pass IDs as domain types",
			})
		}
	})
}

// --- Rule 4: hardcoded hex / oklch colors ------------------------------------

// hexColorRe matches a #rgb, #rgba, #rrggbb, or #rrggbbaa literal. It requires a
// non-hex (or string) boundary after so "#define"-style noise and longer hex
// runs (ids) are not mistaken for colors.
var hexColorRe = regexp.MustCompile(`#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})\b`)

// oklchRe matches an oklch( color function opening.
var oklchRe = regexp.MustCompile(`\boklch\(`)

// ruleHardcodedColor flags hex or oklch() colors baked into string literals.
// Colors must come from design tokens via var(--*) so theming stays centralized.
// The chart package legitimately composes var(--*) fallbacks and is allowed to
// reference raw colors only as the currentColor fallback, so we still scan it —
// genuine hex there is a real finding.
// colorSanctioned is where a colour literal is the subject rather than a smell.
// pkg/color converts and formats colours for a living, and pkg/theme derives the
// palette the tokens are generated from — flagging an oklch() there is the same
// category of mistake as flagging the analyzer's own regexes.
var colorSanctioned = []string{lintSelf, "color", "theme"}

func ruleHardcodedColor(fc *fileCtx, out *[]Finding) {
	if fc.onPath(colorSanctioned...) {
		return
	}
	fc.stringLits2(func(val string, line int) {
		// A literal that already routes through a design token on this line is
		// fine even if it also carries a hex fallback inside the var(): that is
		// the sanctioned var(--token, #fallback) shape. Only flag when there is
		// no var(--) token anchoring the color.
		if hexColorRe.MatchString(val) && !strings.Contains(val, "var(--") {
			*out = append(*out, Finding{
				File:    fc.path,
				Line:    line,
				Rule:    "hardcoded-color",
				Message: "hardcoded hex color in a string literal; use a design token via var(--token)",
			})
			return
		}
		if oklchRe.MatchString(val) && !strings.Contains(val, "var(--") {
			*out = append(*out, Finding{
				File:    fc.path,
				Line:    line,
				Rule:    "hardcoded-color",
				Message: "hardcoded oklch() color in a string literal; use a design token via var(--token)",
			})
		}
	})
}

// --- Rule 5: naked http.HandlerFunc outside app.Raw --------------------------

// ruleNakedHandlerFunc heuristically flags registering a plain handler on a
// mux/router by a method that is not the framework's Raw escape hatch. We look
// for calls whose selector is HandleFunc or Handle (net/http mux) — these mount
// raw handlers directly, bypassing the typed Page/Action/Stream shapes. The
// framework's own routing internals (module root) and the ui kit are exempt;
// app code must register through app.Page/Action/Stream/Raw.
func ruleNakedHandlerFunc(fc *fileCtx, out *[]Finding) {
	if fc.onPath("cmd/beach-palette") {
		return // a generator's preview server is a tool, not an application
	}
	// The framework root package "beach" owns the mux and its register/adapt
	// internals; framework libs aren't apps. App code registers via the App.
	if fc.file.Name.Name == "beach" ||
		fc.onPath("ui", "datastar", "chart", "hub", "sim", "ecs") {
		return
	}
	ast.Inspect(fc.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := sel.Sel.Name
		if name != "HandleFunc" && name != "Handle" {
			return true
		}
		// http.HandleFunc(...) and mux.HandleFunc(...) both mount raw handlers.
		*out = append(*out, Finding{
			File:    fc.path,
			Line:    fc.lineOf(call.Pos()),
			Rule:    "naked-handlerfunc",
			Message: "raw " + name + " registration bypasses the typed handler shapes; register via app.Page/Action/Stream, or app.Raw for the documented escape hatch",
		})
		return true
	})
}

// --- Rule 6: custom <script> blocks in app markup ----------------------------

// scriptOpenRe matches an opening <script ...> tag in markup.
var scriptOpenRe = regexp.MustCompile(`(?i)<script\b`)

// datastarBundleRe recognizes the framework's datastar module bundle served
// from /static/js/datastar*.js.
var datastarBundleRe = regexp.MustCompile(`(?i)/static/js/datastar[\w.-]*\.js`)

// selfStaticScriptRe recognizes a self-hosted, bodyless ES module reference: a
// <script> whose src is the app's own /static/js tree and whose element carries
// no inline body. A socket-consuming page needs page script (RFC 05: a Socket
// carries payloads that are not hypermedia, and beach-ws.js + its consumer are
// served like every other framework asset), so same-origin static modules are
// sanctioned. Inline bodies and external origins remain findings.
var selfStaticScriptRe = regexp.MustCompile(`(?i)<script\b[^>]*\bsrc="/static/js/[\w.-]+\.js"[^>]*>\s*</script>`)

// ruleCustomScript flags <script> blocks in string-literal markup. HTML+CSS is
// the default; allowed scripts are the framework's datastar bundle (which the
// ui kit's Page shell emits) and bodyless same-origin modules under
// /static/js/. A custom inline <script> — reload hacks, third-party snippets —
// is a finding.
func ruleCustomScript(fc *fileCtx, out *[]Finding) {
	if fc.onPath("ui", lintSelf) {
		return // the kit owns the document shell; lint owns the <script> regex
	}
	fc.stringLits2(func(val string, line int) {
		// Strip every sanctioned same-origin module reference; whatever
		// <script> remains is unsanctioned.
		rest := selfStaticScriptRe.ReplaceAllString(val, "")
		if !scriptOpenRe.MatchString(rest) {
			return
		}
		if datastarBundleRe.MatchString(rest) {
			return // sanctioned datastar bundle reference
		}
		*out = append(*out, Finding{
			File:    fc.path,
			Line:    line,
			Rule:    "custom-script",
			Message: "custom <script> block in app markup; only the datastar bundle and bodyless same-origin /static/js/ modules are allowed — move behavior to a Datastar action, SSE, or a served ES module",
		})
	})
}

// --- shared AST helpers ------------------------------------------------------

// forEachImport calls fn with each import path string and its line.
func forEachImport(fc *fileCtx, fn func(path string, line int)) {
	for _, imp := range fc.file.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		fn(p, fc.lineOf(imp.Pos()))
	}
}

// --- Rule 7: images with no text alternative ---------------------------------

// imgTagRe matches an <img> start tag in markup.
var imgTagRe = regexp.MustCompile(`(?is)<img\b[^>]*>`)

// altAttrRe matches an alt attribute on an element, in either the static form
// (alt="x") or the form templ emits for an interpolated value, where the value
// is written separately and the literal ends at `alt="`.
var altAttrRe = regexp.MustCompile(`(?i)\balt="`)

// ruleImgAlt flags an <img> with no alt attribute at all. Every image owes a
// text alternative (WCAG 1.1.1); a decorative one owes an explicit empty alt,
// which says "nothing to announce here" rather than leaving a screen reader to
// read the file name. The kit's own ImageProps has a Decorative field for
// exactly that, so there is no shape of image this rule cannot be satisfied for.
//
// Generated *_templ.go files carry their markup as string literals, which is
// what makes this checkable at all — the analyzer sees the same tags the browser
// will. A tag split across literals by an interpolated attribute is scanned in
// pieces; the rule only fires on a piece that contains a complete <img ...> with
// no alt, so a split tag is skipped rather than falsely accused.
func ruleImgAlt(fc *fileCtx, out *[]Finding) {
	if fc.onPath(lintSelf) {
		return // the analyzer's own <img> regex is not markup
	}
	fc.stringLits2(func(val string, line int) {
		for _, tag := range imgTagRe.FindAllString(val, -1) {
			if altAttrRe.MatchString(tag) {
				continue
			}
			*out = append(*out, Finding{
				File:    fc.path,
				Line:    line,
				Rule:    "a11y-img-alt",
				Message: "<img> with no alt attribute; give it a text alternative, or an explicit alt=\"\" for a decorative image (driftwood.ImageProps has Alt and Decorative)",
			})
		}
	})
}

// --- Rule 8: roles that promise a name they don't have ------------------------

// roleImgRe matches an element carrying role="img".
var roleImgRe = regexp.MustCompile(`(?is)<[a-z][^>]*\brole="img"[^>]*>`)

// ariaNameRe matches either way of giving an element an accessible name.
var ariaNameRe = regexp.MustCompile(`(?i)\baria-(label|labelledby)="`)

// ruleUnnamedRoleImg flags role="img" on an element with no accessible name.
// This is worse than leaving the role off: an unnamed graphic is announced as
// "image" and nothing else, interrupting to say that something unidentifiable is
// there. The fix is always one of two things — name it, or drop the role and let
// it be decorative (WCAG 1.1.1, 4.1.2).
func ruleUnnamedRoleImg(fc *fileCtx, out *[]Finding) {
	if fc.onPath(lintSelf) {
		return // the analyzer's own role="img" regex is not markup
	}
	fc.stringLits2(func(val string, line int) {
		for _, tag := range roleImgRe.FindAllString(val, -1) {
			if ariaNameRe.MatchString(tag) {
				continue
			}
			*out = append(*out, Finding{
				File:    fc.path,
				Line:    line,
				Rule:    "a11y-unnamed-role-img",
				Message: `role="img" with no aria-label or aria-labelledby; name the graphic, or drop the role and mark it aria-hidden="true"`,
			})
		}
	})
}

// --- Rule 9: hardcoded accessible names in the kit ----------------------------

// ariaLabelLiteralRe matches a *static* aria-label — one whose value is written
// out in the same literal. templ emits an interpolated value as a literal ending
// in `aria-label="`, with the value written separately, so requiring at least one
// character before the closing quote is what distinguishes the two.
var ariaLabelLiteralRe = regexp.MustCompile(`(?i)\baria-label="[^"]+"`)

// ruleLiteralAccessibleName flags a hardcoded aria-label inside the ui kit.
//
// This inverts the usual sanctioning: the kit is the target and app code is
// exempt. An accessible name is content — on a page rendered in Spanish, a
// screen reader saying "Close" is exactly as wrong as an untranslated heading —
// and the kit is shipped to apps in every language, so its names have to come
// from the catalog (i18n.T with a "ui.a11y.*" key) rather than from the source.
// An app's own names are its own business.
func ruleLiteralAccessibleName(fc *fileCtx, out *[]Finding) {
	if !fc.onPath("ui", "chart") {
		return
	}
	fc.stringLits2(func(val string, line int) {
		for _, m := range ariaLabelLiteralRe.FindAllString(val, -1) {
			*out = append(*out, Finding{
				File:    fc.path,
				Line:    line,
				Rule:    "a11y-literal-name",
				Message: "hardcoded accessible name " + m + " in the kit; a name a screen reader reads out is content — route it through i18n.T(ctx, \"ui.a11y.…\")",
			})
		}
	})
}
