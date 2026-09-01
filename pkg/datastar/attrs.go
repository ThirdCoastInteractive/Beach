// Package datastar is the only sanctioned way to emit Datastar data-*
// attributes and to open a server-sent event stream inside Beach.
//
// Raw data-* strings in templ are a build failure via the beach vet analyzer;
// the typed builders here render the attributes by construction. Colon event
// syntax (data-on:click) is what the builders emit; nobody types it.
//
// Two halves live here:
//
//   - Attribute builders (Attr): typed constructors for data-on:click,
//     data-bind, data-signals, data-show, etc. They render to attribute strings
//     (via Attr.String) or to a templ.Attributes map (via Attrs.Templ) for use
//     in templ components.
//   - SSE helpers: IsDatastar(r) reads the Datastar-Request header, and
//     NewSSE(w, r) wraps the upstream datastar SDK generator with
//     per-event gzip flush negotiated from Accept-Encoding.
package datastar

import (
	"encoding/json"
	"html"
	"strings"

	"github.com/a-h/templ"
)

// Attr is a single rendered Datastar data-* attribute: a name (already
// including the leading "data-" and any ":modifier") and its value. A bare
// boolean-style attribute (no value) has an empty Val.
type Attr struct {
	Name string
	Val  string
}

// String renders the attribute as it appears inside an HTML start tag, e.g.
//
//	data-on:click="@post('/x')"
//
// The value is HTML-attribute escaped. A valueless attribute renders as just
// its name.
func (a Attr) String() string {
	if a.Val == "" {
		return a.Name
	}
	return a.Name + `="` + html.EscapeString(a.Val) + `"`
}

// Attrs is an ordered collection of Datastar attributes. It is the return type
// of the multi-attribute helpers and composes cleanly with append.
type Attrs []Attr

// String renders every attribute space-separated, in order, ready to splice
// into a start tag.
func (as Attrs) String() string {
	parts := make([]string, len(as))
	for i, a := range as {
		parts[i] = a.String()
	}
	return strings.Join(parts, " ")
}

// Templ converts the attributes into a templ.Attributes map so they can be
// spread into a templ element with @attrs... — the sanctioned bridge from these
// builders into a templ component. A valueless attribute maps to bool true,
// which templ renders as a bare attribute.
func (as Attrs) Templ() templ.Attributes {
	out := make(templ.Attributes, len(as))
	for _, a := range as {
		if a.Val == "" {
			out[a.Name] = true
		} else {
			out[a.Name] = a.Val
		}
	}
	return out
}

// On builds a data-on:<event> handler attribute. The event name is the bare
// DOM event ("click", "submit", "keydown", ...); the colon syntax is emitted
// for you. The expression is the Datastar action expression, e.g.
// "@post('/coins/purchase/1')".
//
// NOTE: there is no DOM "load" event on most elements, and Datastar has no
// synthetic one — use Init to run an expression once when an element mounts
// (e.g. to open an SSE stream on page load).
//
//	On("click", "@post('/x')") -> data-on:click="@post('/x')"
func On(event, expr string) Attr {
	return Attr{Name: "data-on:" + event, Val: expr}
}

// Init builds a data-init attribute: Datastar runs the expression exactly once
// when the element is initialized. This is the correct way to kick off work on
// load — most commonly opening a long-lived SSE stream as the page renders.
//
//	Init("@get('/live')") -> data-init="@get('/live')"
func Init(expr string) Attr {
	return Attr{Name: "data-init", Val: expr}
}

// OnInterval builds a data-on-interval attribute that re-runs expr on a fixed
// client-side timer (Datastar's on-interval plugin). The cadence comes from a
// static "duration" modifier — e.g. data-on-interval__duration.5s — read once at
// element mount and NOT reactive to a signal; to change the cadence, re-render
// the element with a different dur. The client parses "ms", "s", and bare-number
// (ms) units ONLY, so pass "60s"/"300s", never "1m"/"5m" (which would parse to
// ~1ms — a runaway poll). Append ".leading" to dur (e.g. "5s.leading") to fire
// once immediately on mount instead of waiting a full interval.
//
//	OnInterval("5s", "@get('/x')")         -> data-on-interval__duration.5s="@get('/x')"
//	OnInterval("5s.leading", "@get('/x')") -> data-on-interval__duration.5s.leading="@get('/x')"
//
// dur "" omits the modifier, leaving the plugin's 1s default.
func OnInterval(dur, expr string) Attr {
	name := "data-on-interval"
	if dur != "" {
		name += "__duration." + dur
	}
	return Attr{Name: name, Val: expr}
}

// OnClick is the common case of On("click", expr).
func OnClick(expr string) Attr { return On("click", expr) }

// OnSubmit is the common case of On("submit", expr). Pair it with a form whose
// default submission is prevented by Datastar.
func OnSubmit(expr string) Attr { return On("submit", expr) }

// Bind builds a data-bind:<signal> two-way binding on an input. The signal name
// is the bare path ("email", "form.qty"); the colon syntax is emitted for you.
//
//	Bind("email") -> data-bind:email
func Bind(signal string) Attr {
	return Attr{Name: "data-bind:" + signal}
}

// Signals builds a data-signals attribute from a value that JSON-marshals to
// the initial signal object, e.g. a map or struct. The JSON is rendered as the
// attribute value (HTML-escaped on String/Templ). Marshalling failure yields
// an empty object so a bad value can never silently inject markup.
//
//	Signals(map[string]any{"open": false}) -> data-signals="{\"open\":false}"
func Signals(v any) Attr {
	return Attr{Name: "data-signals", Val: jsonValue(v)}
}

// SignalsExpr builds a data-signals attribute from a raw Datastar expression
// string (already a JS object literal). Use this when you want expressions
// inside the object that JSON cannot express; the caller owns correctness.
func SignalsExpr(expr string) Attr {
	return Attr{Name: "data-signals", Val: expr}
}

// Signal builds a data-signals attribute scoped to a single named signal:
//
//	Signal("open", false) -> data-signals:open="false"
func Signal(name string, v any) Attr {
	return Attr{Name: "data-signals:" + name, Val: jsonValue(v)}
}

// Show builds a data-show attribute whose expression decides visibility:
//
//	Show("$open") -> data-show="$open"
func Show(expr string) Attr {
	return Attr{Name: "data-show", Val: expr}
}

// Text builds a data-text attribute that sets an element's text content to the
// evaluated expression.
func Text(expr string) Attr {
	return Attr{Name: "data-text", Val: expr}
}

// Class builds a data-class attribute from a value mapping class names to
// boolean expressions, JSON-marshalled, e.g.
//
//	Class(map[string]string{"active": "$open"}) -> data-class="{\"active\":\"$open\"}"
func Class(v any) Attr {
	return Attr{Name: "data-class", Val: jsonValue(v)}
}

// ClassToggle builds a data-class-<name> attribute that toggles a single CSS
// class on when expr is truthy. Prefer this over Class for a class driven by a
// signal expression: the object form data-class="{name: expr}" needs the
// expression UNQUOTED, which JSON marshalling (what Class does) cannot produce —
// a quoted "expr" is read as a constant truthy string, so the class sticks on.
//
//	ClassToggle("open", "$menu") -> data-class:open="$menu"
func ClassToggle(name, expr string) Attr {
	return Attr{Name: "data-class:" + name, Val: expr}
}

// Attr builds a data-attr:<name> attribute that binds a DOM attribute to an
// expression:
//
//	AttrBind("disabled", "$busy") -> data-attr:disabled="$busy"
func AttrBind(name, expr string) Attr {
	return Attr{Name: "data-attr:" + name, Val: expr}
}

// Indicator builds a data-indicator:<signal> attribute that flips a boolean
// signal while a request triggered from this element is in flight.
//
//	Indicator("loading") -> data-indicator:loading
func Indicator(signal string) Attr {
	return Attr{Name: "data-indicator:" + signal}
}

// Ref builds a data-ref:<name> attribute exposing the element as a signal.
//
//	Ref("dialog") -> data-ref:dialog
func Ref(name string) Attr {
	return Attr{Name: "data-ref:" + name}
}

// jsonValue marshals v to compact JSON. On error it returns an empty object so
// no malformed or attacker-controlled markup leaks into an attribute. Map keys
// are sorted by encoding/json, giving deterministic output for table tests.
func jsonValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
