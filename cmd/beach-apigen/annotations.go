package main

import (
	"fmt"
	"strings"
)

// Annotations are the directives beach-apigen reads from the comment block above
// a named sqlc query. They are the entire grammar — there is no other
// configuration surface. See docs/architecture/13-apigen.md.
//
//	-- name: CreateItem :one
//	-- @api POST /items
//	-- @requires pantry:write
//	-- @scoped customer_id
//	-- @notify items
//	-- @fragment page.ItemCard
//
// A query with no @api annotation is plain sqlc and produces no beach wiring.
type Annotations struct {
	// Method is the HTTP method from "@api METHOD /path" (upper-cased).
	Method string
	// Path is the route pattern from "@api METHOD /path" (ServeMux syntax, so
	// "{id}" path params are kept verbatim).
	Path string
	// Page is the "pkg.Component" rendered on an @api GET navigation.
	Page string
	// Fragment is the "pkg.Component" patched on a Datastar request / mutation.
	Fragment string
	// Notify is the channel (Postgres NOTIFY + hub topic) a mutation publishes to.
	Notify string
	// Requires is the "resource:action" permission the handler enforces.
	Requires string
	// Scoped is the name of the query parameter that constrains a customer-scoped
	// entity, declared via "@scoped <paramName>". When set, generation fails unless
	// the query actually takes that parameter — so a query over a scoped table can
	// never silently omit its tenant predicate and leak across customers.
	Scoped string
	// Skip suppresses the generated handler stub ("@handler skip"); the route is
	// still recorded so the authz analyzer can check the hand-written handler.
	Skip bool
}

// HasAPI reports whether the query is exposed as a beach route at all. Without an
// @api annotation the query is ordinary sqlc and apigen ignores it.
func (a Annotations) HasAPI() bool { return a.Method != "" }

// IsMutation reports whether the @api method is a write (POST/PUT/DELETE/PATCH)
// — i.e. it generates an ActionFunc rather than a PageFunc.
func (a Annotations) IsMutation() bool {
	switch a.Method {
	case "POST", "PUT", "DELETE", "PATCH":
		return true
	default:
		return false
	}
}

// parseAnnotations extracts the annotations from a query's comment text. The
// input is the raw comment block sqlc hands us (each physical line may or may not
// carry a leading "--"); we tolerate both so the parser works on raw .sql text in
// tests and on sqlc-stripped comments in the plugin path.
//
// Unknown "@" directives are an error: a typo like "@notifies" must fail loudly
// rather than silently do nothing. Lines without a leading "@" are SQL or prose
// and are ignored.
func parseAnnotations(comment string) (Annotations, error) {
	var a Annotations
	for _, raw := range strings.Split(comment, "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimPrefix(line, "--")
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "@") {
			continue
		}
		// Split directive from its argument: "@api GET /x" -> ("@api", "GET /x").
		directive, arg, _ := strings.Cut(line, " ")
		arg = strings.TrimSpace(arg)
		switch directive {
		case "@api":
			method, path, ok := strings.Cut(arg, " ")
			if !ok || method == "" || strings.TrimSpace(path) == "" {
				return a, fmt.Errorf("@api needs METHOD and /path, got %q", arg)
			}
			a.Method = strings.ToUpper(strings.TrimSpace(method))
			a.Path = strings.TrimSpace(path)
		case "@page":
			if arg == "" {
				return a, fmt.Errorf("@page needs a pkg.Component")
			}
			a.Page = arg
		case "@fragment":
			if arg == "" {
				return a, fmt.Errorf("@fragment needs a pkg.Component")
			}
			a.Fragment = arg
		case "@notify":
			if arg == "" {
				return a, fmt.Errorf("@notify needs a channel name")
			}
			a.Notify = arg
		case "@requires":
			if arg == "" {
				return a, fmt.Errorf("@requires needs a permission")
			}
			a.Requires = arg
		case "@scoped":
			if arg == "" {
				return a, fmt.Errorf("@scoped needs a parameter name")
			}
			a.Scoped = arg
		case "@handler":
			if arg != "skip" {
				return a, fmt.Errorf("@handler only supports \"skip\", got %q", arg)
			}
			a.Skip = true
		default:
			return a, fmt.Errorf("unknown annotation %q", directive)
		}
	}
	return a, nil
}

// validate checks an annotation set for internal consistency once fully parsed.
// It is separate from parsing so the rules read as a checklist.
func (a Annotations) validate(queryName string) error {
	if !a.HasAPI() {
		// No @api: nothing to validate, nothing to generate.
		return nil
	}
	switch a.Method {
	case "GET", "POST", "PUT", "DELETE", "PATCH":
	default:
		return fmt.Errorf("%s: unsupported @api method %q", queryName, a.Method)
	}
	if a.Method == "GET" && a.Page == "" && a.Fragment == "" {
		return fmt.Errorf("%s: @api GET needs a @page or @fragment to render", queryName)
	}
	if a.IsMutation() && a.Fragment == "" && a.Notify == "" {
		// A mutation with neither a fragment to patch nor a notify to publish has
		// no observable effect on the client — almost always a mistake.
		return fmt.Errorf("%s: a mutating @api needs a @fragment to patch or a @notify to publish", queryName)
	}
	return nil
}
