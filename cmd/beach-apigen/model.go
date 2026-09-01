package main

// model.go holds the codegen-facing view of a sqlc query — deliberately decoupled
// from the sqlc protobuf so the generator and its tests never touch the wire
// format. The plugin path (plugin.go) translates the protobuf into these structs;
// tests construct them directly.

// Query is one named sqlc query with its parsed beach annotations and enough of
// its sqlc shape (command, argument struct, return type) to wire a handler.
type Query struct {
	// Name is the sqlc query name, e.g. "CreateItem". The generated sqlc method is
	// Querier.<Name>.
	Name string

	// Cmd is the sqlc command: ":one", ":many", ":exec", ":execrows", etc. It
	// decides how the generated handler calls and uses the sqlc result.
	Cmd string

	// Ann is the parsed annotation block.
	Ann Annotations

	// ArgType is the Go type sqlc generates for this query's parameters, e.g.
	// "CreateItemParams". Empty when the query takes no params or a single scalar.
	ArgType string

	// ArgIsScalar is true when sqlc passes a single scalar arg (e.g. an id) rather
	// than a generated Params struct. ScalarArg/ScalarType then describe it.
	ArgIsScalar bool
	ScalarArg   string // field/param name, e.g. "id"
	ScalarType  string // Go type, e.g. "int64"

	// Params are the fields of the argument struct (for binding + the NOTIFY id
	// lookup). Empty for a no-arg or scalar-arg query.
	Params []Param

	// ReturnType is the Go type a :one/:many query returns (e.g. "Item"). Empty for
	// :exec.
	ReturnType string
}

// HasInput reports whether the query takes request-bound input (a Params struct
// or a scalar arg) — i.e. whether the handler calls beach.Bind.
func (q Query) HasInput() bool { return q.ArgType != "" || q.ArgIsScalar }

// Param is one field of a sqlc Params struct.
type Param struct {
	// Field is the Go field name, e.g. "LocationID".
	Field string
	// JSONName is the json/datastar signal name, e.g. "location_id". Datastar posts
	// signals as JSON, so the generated bind struct tags fields with this.
	JSONName string
	// Type is the Go type, e.g. "int64", "string", "pgtype.Timestamptz".
	Type string
}
