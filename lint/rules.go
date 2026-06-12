package lint

import (
	"go/ast"
	"regexp"
	"strings"
)

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
func ruleHardcodedColor(fc *fileCtx, out *[]Finding) {
	if fc.onPath("lint") {
		return // the analyzer's own color regexes are not markup
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

// datastarBundleRe recognizes the one sanctioned script: the framework's
// datastar module bundle served from /static/js/datastar*.js.
var datastarBundleRe = regexp.MustCompile(`(?i)/static/js/datastar[\w.-]*\.js`)

// ruleCustomScript flags <script> blocks in string-literal markup. HTML+CSS is
// the default; the only allowed script is the framework's datastar bundle, which
// the ui kit's Page shell emits. App code (and any non-ui package) embedding a
// custom <script> — inline reload hacks, third-party snippets — is a finding.
func ruleCustomScript(fc *fileCtx, out *[]Finding) {
	if fc.onPath("ui", "lint") {
		return // the kit owns the document shell; lint owns the <script> regex
	}
	fc.stringLits2(func(val string, line int) {
		if !scriptOpenRe.MatchString(val) {
			return
		}
		if datastarBundleRe.MatchString(val) {
			return // sanctioned datastar bundle reference
		}
		*out = append(*out, Finding{
			File:    fc.path,
			Line:    line,
			Rule:    "custom-script",
			Message: "custom <script> block in app markup; the-beach is HTML+CSS-first and only the framework's datastar bundle is allowed — move behavior to a Datastar action or SSE",
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
