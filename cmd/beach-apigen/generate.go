package main

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"
	"text/template"
)

// generate.go turns parsed Query models into beach handler source. The output
// matches the REAL beach API read from the root package: PageFunc returns
// (beach.View, error); ActionFunc returns (beach.Patches, error); binding goes
// through beach.Bind[T]; the principal check is c.MustPrincipal().Can(perm); hub
// publish is hub.Event{Bytes: ...}. Generated files are regenerate-never-edit,
// exactly like sqlc output.

// GenConfig parameterizes a generation run.
type GenConfig struct {
	// Package is the package name for the emitted Go files (the sqlc "out" dir).
	Package string
	// QuerierType is the Go type implementing the sqlc methods, e.g. "*Queries".
	// Handlers close over a value of this type named `q`.
	QuerierType string
	// ComponentImports maps the package selector used in @page/@fragment
	// annotations (e.g. "page") to its import path (e.g.
	// "example.com/app/internal/view/page"). The generated file imports every
	// referenced selector. A selector with no mapping is assumed to live in the
	// generated package itself (no import emitted) — useful when components are
	// generated alongside the handlers.
	ComponentImports map[string]string
}

// File is one generated output: a name and its contents. The plugin maps these to
// sqlc's response files; tests assert on Contents.
type File struct {
	Name     string
	Contents []byte
}

// Generate produces all output files for the queries that carry an @api
// annotation. Queries without one are skipped (ordinary sqlc). The result is a
// handlers file (always) and a NOTIFY migration snippet when any query declares
// @notify.
func Generate(cfg GenConfig, queries []Query) ([]File, error) {
	var wired []Query
	for _, q := range queries {
		if !q.Ann.HasAPI() {
			continue
		}
		if err := q.Ann.validate(q.Name); err != nil {
			return nil, err
		}
		if err := q.validateScope(); err != nil {
			return nil, err
		}
		wired = append(wired, q)
	}
	if len(wired) == 0 {
		return nil, nil
	}
	// Stable output order: sort by route then name so regeneration is deterministic.
	sort.Slice(wired, func(i, j int) bool {
		if wired[i].Ann.Path != wired[j].Ann.Path {
			return wired[i].Ann.Path < wired[j].Ann.Path
		}
		return wired[i].Name < wired[j].Name
	})

	var files []File

	handlers, err := genHandlers(cfg, wired)
	if err != nil {
		return nil, err
	}
	files = append(files, File{Name: "api.gen.go", Contents: handlers})

	if mig := genNotifyMigration(wired); mig != nil {
		files = append(files, File{Name: "notify.gen.sql", Contents: mig})
	}
	return files, nil
}

// validateScope enforces the scope-coverage rule: a query marked @scoped <p>
// must actually take p as a parameter. A scoped entity's tenant predicate lives
// in the SQL as that parameter, so if it is missing the query selects/mutates
// across every customer — exactly the cross-tenant leak the annotation guards
// against. The check needs the query's lifted params (not just its annotations),
// so it lives here on Query rather than on Annotations.validate.
func (q Query) validateScope() error {
	if q.Ann.Scoped == "" {
		return nil
	}
	for _, p := range q.Params {
		if p.JSONName == q.Ann.Scoped {
			return nil
		}
	}
	return fmt.Errorf("%s: @scoped parameter %q is not a parameter of the query — a customer-scoped query must constrain by it", q.Name, q.Ann.Scoped)
}

// genHandlers renders the Go source for every wired query and gofmt's it.
func genHandlers(cfg GenConfig, queries []Query) ([]byte, error) {
	usesHub := false
	for _, q := range queries {
		if queryNotifies(q) {
			usesHub = true
		}
	}

	// Collect the component package import paths actually referenced. A selector
	// without a configured import path is assumed local to the generated package.
	compImports := componentImports(cfg, queries)

	var b bytes.Buffer
	if err := headerTmpl.Execute(&b, headerData{
		Package:     cfg.Package,
		UsesHub:     usesHub,
		CompImports: compImports,
	}); err != nil {
		return nil, err
	}

	// Register() body collects the route registrations; handler funcs follow.
	var routes bytes.Buffer
	var bodies bytes.Buffer

	for _, q := range queries {
		hname := handlerName(q.Name)
		guard := ""
		if q.Ann.Requires != "" {
			// The route-table guard makes the auth visible at registration, matching
			// the in-handler check the analyzer also verifies.
			guard = fmt.Sprintf(", app.Can(%q)", q.Ann.Requires)
		}
		verb := "Page"
		if q.Ann.IsMutation() {
			verb = "Action"
		}
		factoryArgs := "q"
		if queryNotifies(q) {
			factoryArgs = "q, h"
		}
		// beach exposes one typed mutation hook, app.Action, registered on POST.
		// PUT/DELETE/PATCH share the ActionFunc shape but the framework has no
		// method-parameterized typed registration, so they register on POST too;
		// hypermedia clients tunnel non-POST methods over POST anyway. Flag it so
		// the divergence from the annotation's method is visible at the call site.
		if q.Ann.IsMutation() && q.Ann.Method != "POST" {
			fmt.Fprintf(&routes, "\t// @api %s registered via app.Action (POST); beach has no typed %s hook.\n", q.Ann.Method, q.Ann.Method)
		}
		fmt.Fprintf(&routes, "\tapp.%s(%q, %s(%s)%s)\n", verb, q.Ann.Path, hname, factoryArgs, guard)

		if q.Ann.Skip {
			// @handler skip: record the route but emit no body — the app hand-writes
			// the handler with this name. Leave a stub signature comment so the
			// missing function is an obvious, named compile error rather than a
			// mystery.
			fmt.Fprintf(&bodies, skipNote, hname, q.Name, q.Ann.Method, q.Ann.Path)
			continue
		}

		data := newHandlerData(cfg, q)
		// A scalar-arg query binds through a generated request struct so the
		// Datastar signal name is explicit. Emit it just above its handler.
		if q.ArgIsScalar {
			if err := scalarReqTmpl.Execute(&bodies, data); err != nil {
				return nil, fmt.Errorf("%s: %w", q.Name, err)
			}
		}
		tmpl := pageTmpl
		if q.Ann.IsMutation() {
			tmpl = actionTmpl
		}
		if err := tmpl.Execute(&bodies, data); err != nil {
			return nil, fmt.Errorf("%s: %w", q.Name, err)
		}
	}

	// usesHub (computed above) decides whether we thread the hub + emit the helper.
	regSig := fmt.Sprintf("func Register(app *beach.App, q %s", cfg.QuerierType)
	if usesHub {
		regSig += ", h *hub.Hub"
	}
	regSig += ") {\n"

	fmt.Fprintf(&b, "// Register wires every @api route onto app. Call it from main after\n")
	fmt.Fprintf(&b, "// constructing the sqlc querier%s.\n", hubDoc(usesHub))
	b.WriteString(regSig)
	b.Write(routes.Bytes())
	b.WriteString("}\n\n")
	if usesHub {
		b.WriteString(notifyHelper)
	}
	b.Write(bodies.Bytes())

	src, err := format.Source(b.Bytes())
	if err != nil {
		// Return the unformatted source alongside the error so a template bug is
		// debuggable rather than swallowed.
		return b.Bytes(), fmt.Errorf("gofmt generated source: %w", err)
	}
	return src, nil
}

// handlerData is the template context for one handler.
type handlerData struct {
	Cfg   GenConfig
	Q     Query
	Hname string
	// Call is the Go expression that invokes the sqlc method, e.g.
	// "q.CreateItem(c.Context(), in)".
	Call string
	// BindType is the Go type bound from the request (the Params struct or scalar
	// wrapper), or "" when the query takes no input.
	BindType string
	// Notifies is true when this handler threads the hub and calls notifyPublish —
	// only when @notify is set AND there is a @fragment to render and broadcast.
	Notifies bool

	// ScalarField/ScalarJSON describe the single field of a generated scalar
	// request struct (set only when Q.ArgIsScalar).
	ScalarField string
	ScalarJSON  string

	// HasRow is true when the sqlc call returns a value (a :one/:many row or an
	// exec-variant count). When false (:exec), the call returns only error and the
	// page/fragment components are invoked with no row argument.
	HasRow bool
	// RowArg is "row" when HasRow, "" otherwise — the argument list the component
	// constructors receive.
	RowArg string
}

func newHandlerData(cfg GenConfig, q Query) handlerData {
	d := handlerData{Cfg: cfg, Q: q, Hname: handlerName(q.Name)}
	d.Notifies = q.Ann.Notify != "" && q.Ann.Fragment != ""
	d.HasRow = cmdReturnsRow(q.Cmd)
	if d.HasRow {
		d.RowArg = "row"
	}

	switch {
	case q.ArgType != "":
		d.BindType = q.ArgType
		d.Call = fmt.Sprintf("q.%s(c.Context(), in)", q.Name)
	case q.ArgIsScalar:
		// A scalar arg still binds through a tiny generated request struct so the
		// Datastar signal name is explicit; the field is named after the param.
		d.BindType = q.Name + "Request"
		d.Call = fmt.Sprintf("q.%s(c.Context(), in.%s)", q.Name, exportName(q.ScalarArg))
		d.ScalarField = exportName(q.ScalarArg)
		d.ScalarJSON = q.ScalarArg
	default:
		d.Call = fmt.Sprintf("q.%s(c.Context())", q.Name)
	}
	return d
}

// handlerName is the exported generated handler factory name for a query.
func handlerName(query string) string { return "handle" + query }

// cmdReturnsRow reports whether a sqlc command yields a value to bind to `row`.
// :exec returns only error; everything else (:one, :many, :execrows,
// :execresult, :execlastid, :copyfrom) returns a value plus error.
func cmdReturnsRow(cmd string) bool {
	switch cmd {
	case ":exec", "exec", "":
		return false
	default:
		return true
	}
}

// queryNotifies reports whether a generated factory for q threads the hub and
// calls notifyPublish: it has @notify, a @fragment to render, and is not skipped.
func queryNotifies(q Query) bool {
	return q.Ann.Notify != "" && q.Ann.Fragment != "" && !q.Ann.Skip
}

// exportName upper-cases the first letter of a snake/lower identifier segment so
// "id" -> "ID" stays consistent with sqlc's initialisms where simple.
func exportName(s string) string {
	if s == "" {
		return s
	}
	if s == "id" {
		return "ID"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// genNotifyMigration emits one goose-style migration with a NOTIFY trigger per
// distinct @notify channel. Returns nil when no query declares @notify.
func genNotifyMigration(queries []Query) []byte {
	channels := map[string]bool{}
	for _, q := range queries {
		if q.Ann.Notify != "" {
			channels[q.Ann.Notify] = true
		}
	}
	if len(channels) == 0 {
		return nil
	}
	names := make([]string, 0, len(channels))
	for c := range channels {
		names = append(names, c)
	}
	sort.Strings(names)

	var b bytes.Buffer
	b.WriteString("-- Code generated by beach-apigen. DO NOT EDIT.\n")
	b.WriteString("-- NOTIFY triggers for @notify channels. The external-writer seam: any\n")
	b.WriteString("-- writer (app, psql, another service) fans changes out to subscribers.\n")
	b.WriteString("-- +goose Up\n")
	for _, ch := range names {
		notifyMigrationTmpl.Execute(&b, ch)
	}
	b.WriteString("\n-- +goose Down\n")
	for _, ch := range names {
		fmt.Fprintf(&b, "DROP TRIGGER IF EXISTS %s_notify ON %s;\n", ch, ch)
		fmt.Fprintf(&b, "DROP FUNCTION IF EXISTS %s_notify();\n", ch)
	}
	return b.Bytes()
}

// --- templates ---

type headerData struct {
	Package string
	UsesHub bool
	// CompImports are the component package import paths to add, sorted.
	CompImports []string
}

// componentImports returns the sorted, deduped import paths for the component
// package selectors referenced by @page/@fragment annotations. Selectors with no
// entry in cfg.ComponentImports are treated as package-local and omitted.
func componentImports(cfg GenConfig, queries []Query) []string {
	seen := map[string]bool{}
	for _, q := range queries {
		for _, ref := range []string{q.Ann.Page, q.Ann.Fragment} {
			sel, _, ok := strings.Cut(ref, ".")
			if !ok {
				continue
			}
			if path, mapped := cfg.ComponentImports[sel]; mapped {
				seen[path] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

var headerTmpl = template.Must(template.New("header").Parse(
	`// Code generated by beach-apigen. DO NOT EDIT.
//
// Source of truth is the annotated SQL in internal/db/sql/queries. Regenerate
// with sqlc generate; never hand-edit this file.

package {{.Package}}

import (
{{- if .UsesHub}}
	"bytes"
	"context"

	"github.com/a-h/templ"
{{- end}}
	"github.com/ThirdCoastInteractive/Beach/pkg/beach"
{{- if .UsesHub}}
	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
{{- end}}
{{- range .CompImports}}
	{{printf "%q" .}}
{{- end}}
)

`))

// hubDoc tweaks Register's doc comment when the hub is threaded.
func hubDoc(usesHub bool) string {
	if usesHub {
		return " and the hub (for @notify publishes)"
	}
	return ""
}

// notifyHelper is the per-file runtime that renders a fragment to bytes and
// publishes it to the @notify hub topic. It is emitted only when some query
// publishes, so a hub-less generated file has no dead import or helper.
const notifyHelper = `// notifyPublish renders frag to bytes and publishes it on the hub topic. The
// stream loop patches those bytes by the fragment's own element id. The matching
// Postgres NOTIFY trigger (notify.gen.sql) covers writers outside this process.
func notifyPublish(h *hub.Hub, topic string, frag templ.Component) {
	if h == nil || frag == nil {
		return
	}
	var buf bytes.Buffer
	if err := frag.Render(context.Background(), &buf); err != nil {
		return
	}
	h.Publish(topic, hub.Event{Bytes: buf.Bytes()})
}

`

// scalarReqTmpl renders the tiny request struct a scalar-arg query binds from.
// The json tag names the Datastar signal; the value flows to the sqlc call.
var scalarReqTmpl = template.Must(template.New("scalarReq").Parse(
	`// {{.BindType}} binds the single {{.ScalarJSON}} param for {{.Q.Name}}.
type {{.BindType}} struct {
	{{.ScalarField}} {{.Q.ScalarType}} ` + "`json:\"{{.ScalarJSON}}\" form:\"{{.ScalarJSON}}\"`" + `
}

`))

// pageTmpl renders a beach.PageFunc factory. The dual-purpose branch is the
// framework's: returning a View with Page+Fragment is enough.
var pageTmpl = template.Must(template.New("page").Parse(
	`// {{.Hname}} renders {{.Q.Name}} ({{.Q.Ann.Method}} {{.Q.Ann.Path}}).
func {{.Hname}}(q {{.Cfg.QuerierType}}) beach.PageFunc {
	return func(c *beach.Ctx) (beach.View, error) {
{{- if .BindType}}
		in, err := beach.Bind[{{.BindType}}](c)
		if err != nil {
			return beach.View{}, err
		}
{{- end}}
{{- if .HasRow}}
		row, err := {{.Call}}
		if err != nil {
			return beach.View{}, err
		}
		_ = row
{{- else}}
		if {{if not .BindType}}err := {{else}}err = {{end}}{{.Call}}; err != nil {
			return beach.View{}, err
		}
{{- end}}
		return beach.View{
{{- if .Q.Ann.Page}}
			Page: {{.Q.Ann.Page}}({{.RowArg}}),
{{- end}}
{{- if .Q.Ann.Fragment}}
			Fragment: {{.Q.Ann.Fragment}}({{.RowArg}}),
{{- end}}
		}, nil
	}
}

`))

// actionTmpl renders a beach.ActionFunc factory: bind+validate, principal check,
// sqlc call, @notify publish, @fragment patch.
var actionTmpl = template.Must(template.New("action").Parse(
	`// {{.Hname}} performs {{.Q.Name}} ({{.Q.Ann.Method}} {{.Q.Ann.Path}}).
func {{.Hname}}(q {{.Cfg.QuerierType}}{{if .Notifies}}, h *hub.Hub{{end}}) beach.ActionFunc {
	return func(c *beach.Ctx) (beach.Patches, error) {
{{- if .Q.Ann.Requires}}
		if !c.MustPrincipal().Can({{printf "%q" .Q.Ann.Requires}}) {
			return nil, beach.ErrForbidden
		}
{{- end}}
{{- if .BindType}}
		in, err := beach.Bind[{{.BindType}}](c)
		if err != nil {
			return nil, err
		}
{{- end}}
{{- if .HasRow}}
		row, err := {{.Call}}
		if err != nil {
			return nil, err
		}
		_ = row
{{- else}}
		if {{if not .BindType}}err := {{else}}err = {{end}}{{.Call}}; err != nil {
			return nil, err
		}
{{- end}}
{{- if and .Notifies .Q.Ann.Fragment}}
		// @notify {{.Q.Ann.Notify}}: broadcast the rendered fragment to every
		// subscriber of the topic. The matching Postgres NOTIFY trigger
		// (notify.gen.sql) carries {table,id,op} to writers outside this process.
		notifyPublish(h, {{printf "%q" .Q.Ann.Notify}}, {{.Q.Ann.Fragment}}({{.RowArg}}))
{{- else if .Notifies}}
		// @notify {{.Q.Ann.Notify}}: no @fragment to render, so this query relies on
		// the Postgres NOTIFY trigger (notify.gen.sql) for the {table,id,op} signal.
{{- end}}
		return beach.Patches{
{{- if .Q.Ann.Fragment}}
			{Fragment: {{.Q.Ann.Fragment}}({{.RowArg}})},
{{- end}}
		}, nil
	}
}

`))

// notifyMigrationTmpl is one channel's trigger pair. The trigger fires a NOTIFY
// carrying a JSON {table,id,op} payload on every row mutation.
var notifyMigrationTmpl = template.Must(template.New("mig").Parse(
	`
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION {{.}}_notify() RETURNS trigger AS $$
DECLARE
	rec record;
	payload json;
BEGIN
	IF (TG_OP = 'DELETE') THEN rec := OLD; ELSE rec := NEW; END IF;
	payload := json_build_object('table', TG_TABLE_NAME, 'id', rec.id, 'op', lower(TG_OP));
	PERFORM pg_notify('{{.}}', payload::text);
	RETURN rec;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS {{.}}_notify ON {{.}};
CREATE TRIGGER {{.}}_notify
	AFTER INSERT OR UPDATE OR DELETE ON {{.}}
	FOR EACH ROW EXECUTE FUNCTION {{.}}_notify();
`))

// skipNote documents a @handler skip route so the missing hand-written handler is
// a named, intentional compile gap.
const skipNote = `// %s is suppressed by @handler skip on query %s (%s %s).
// Hand-write it with this signature; Register references it.

`
