package main

import (
	"encoding/binary"
	"fmt"
	"io"
)

// plugin.go is the sqlc process-plugin glue: read a GenerateRequest protobuf on
// stdin, translate its queries into our Query model, generate, and write a
// GenerateResponse protobuf on stdout.
//
// NOTE on the protobuf dependency. sqlc's plugin SDK
// (github.com/sqlc-dev/plugin-sdk-go) ships generated Go structs for these
// messages, but Beach's go.mod is frozen and does not require it. Rather than
// add a dependency, this file hand-decodes only the handful of fields apigen
// needs from sqlc's codegen.proto, and hand-encodes the response. The field
// numbers below come straight from that .proto and are pinned by the
// proto-compatibility test; if sqlc renumbers them this codec must follow.
//
// Wire format reference (sqlc codegen.proto):
//
//	message GenerateRequest { Settings settings = 1; Catalog catalog = 2;
//	                          repeated Query queries = 3; string sqlc_version = 4; ... }
//	message Query   { string text=1; string name=2; string cmd=3;
//	                  repeated Column columns=4; repeated Parameter params=5;
//	                  repeated string comments=6; }
//	message Parameter { int32 number=1; Column column=2; }
//	message Column  { string name=1; ... Identifier type=4; ... }
//	message Settings { ... string codegen.out / plugin opts ... }
//	message GenerateResponse { repeated File files=1; }
//	message File    { string name=1; bytes contents=2; }
//
// Only text/name/cmd/comments (and params for arg inference) are read; the rich
// type catalog is not, so the plugin path infers Go types the same way the
// standalone parser does. Wiring real sqlc column types is a clear extension
// point (see typeForColumn) but unnecessary for correct handler shape.

// protobuf wire types.
const (
	wireVarint = 0
	wireI64    = 1
	wireBytes  = 2
	wireI32    = 5
)

// runPlugin implements the process-plugin protocol end to end.
func runPlugin(in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("read plugin request: %w", err)
	}
	req, err := decodeGenerateRequest(raw)
	if err != nil {
		return fmt.Errorf("decode GenerateRequest: %w", err)
	}

	queries := make([]Query, 0, len(req.queries))
	for _, pq := range req.queries {
		q, err := pluginQueryToModel(pq)
		if err != nil {
			return err
		}
		queries = append(queries, q)
	}

	cfg := GenConfig{Package: req.pkg(), QuerierType: req.querier()}
	files, err := Generate(cfg, queries)
	if err != nil {
		// A generation error must reach the user via the response, not just exit:
		// sqlc surfaces GenerateResponse errors. We have no error field in the
		// minimal response, so fail the process — sqlc reports the non-zero exit.
		return err
	}

	resp := encodeGenerateResponse(files)
	if _, err := out.Write(resp); err != nil {
		return fmt.Errorf("write GenerateResponse: %w", err)
	}
	return nil
}

// pluginQueryToModel builds a Query from a decoded protobuf query. The six
// annotations come from the comments; arg shape is inferred from the named params
// (and, failing that, the SQL text) exactly like the standalone parser.
func pluginQueryToModel(pq pbQuery) (Query, error) {
	// sqlc strips the leading "-- " from comments but keeps the text; join them so
	// parseAnnotations sees one block.
	comment := ""
	for _, c := range pq.comments {
		comment += c + "\n"
	}
	ann, err := parseAnnotations(comment)
	if err != nil {
		return Query{}, fmt.Errorf("%s: %w", pq.name, err)
	}
	q := Query{Name: pq.name, Cmd: ":" + pq.cmd, Ann: ann}

	switch len(pq.params) {
	case 0:
	case 1:
		name := pq.params[0].name
		q.ArgIsScalar = true
		q.ScalarArg = name
		q.ScalarType = typeForColumn(pq.params[0])
		q.Params = []Param{{Field: exportName(name), JSONName: name, Type: q.ScalarType}}
	default:
		q.ArgType = q.Name + "Params"
		for _, p := range pq.params {
			q.Params = append(q.Params, Param{Field: exportName(p.name), JSONName: p.name, Type: typeForColumn(p)})
		}
	}
	// sqlc normalizes "cmd" without the leading colon already (e.g. "one"); guard
	// against a stray colon just in case.
	if len(q.Cmd) > 1 && q.Cmd[1] == ':' {
		q.Cmd = q.Cmd[1:]
	}
	return q, nil
}

// typeForColumn maps a sqlc parameter to a Go type. The minimal decoder does not
// read the catalog's column types, so it falls back to the name-based heuristic
// the standalone parser uses. Reading pbParam.goType (when wired) is the clean
// upgrade path.
func typeForColumn(p pbParam) string {
	if p.goType != "" {
		return p.goType
	}
	return scalarTypeFor(p.name)
}

// --- decoded message shapes ---

type pbRequest struct {
	queries     []pbQuery
	pkgName     string
	querierType string
}

func (r pbRequest) pkg() string {
	if r.pkgName != "" {
		return r.pkgName
	}
	return "api"
}

func (r pbRequest) querier() string {
	if r.querierType != "" {
		return r.querierType
	}
	return "*Queries"
}

type pbQuery struct {
	text     string
	name     string
	cmd      string
	comments []string
	params   []pbParam
}

type pbParam struct {
	name   string
	goType string // not populated by the minimal decoder; reserved
}

// --- decode ---

// decodeGenerateRequest reads the fields apigen needs from a GenerateRequest.
//
// Field numbers are from sqlc's codegen.proto GenerateRequest:
//
//	settings=1, catalog=2, queries=3, sqlc_version=4, plugin_options=5, ...
//
// The repeated Query list is field 3 (an earlier revision of this decoder read
// field 6 — sqlc's global_options — and so saw zero queries against a real sqlc
// run, emitting nothing). The proto-compatibility tests pin this number.
func decodeGenerateRequest(b []byte) (pbRequest, error) {
	var req pbRequest
	err := walkFields(b, func(field int, wt int, val []byte) error {
		switch field {
		case 3: // repeated Query queries
			q, err := decodeQuery(val)
			if err != nil {
				return err
			}
			req.queries = append(req.queries, q)
		}
		return nil
	})
	return req, err
}

func decodeQuery(b []byte) (pbQuery, error) {
	var q pbQuery
	err := walkFields(b, func(field int, wt int, val []byte) error {
		switch field {
		case 1:
			q.text = string(val)
		case 2:
			q.name = string(val)
		case 3:
			q.cmd = string(val)
		case 5: // repeated Parameter
			p, err := decodeParam(val)
			if err != nil {
				return err
			}
			q.params = append(q.params, p)
		case 6: // repeated string comments
			q.comments = append(q.comments, string(val))
		}
		return nil
	})
	return q, err
}

func decodeParam(b []byte) (pbParam, error) {
	var p pbParam
	err := walkFields(b, func(field int, wt int, val []byte) error {
		// Parameter{ number=1, Column column=2 }; the name lives in the Column.
		if field == 2 {
			name, err := decodeColumnName(val)
			if err != nil {
				return err
			}
			p.name = name
		}
		return nil
	})
	return p, err
}

func decodeColumnName(b []byte) (string, error) {
	var name string
	err := walkFields(b, func(field int, wt int, val []byte) error {
		if field == 1 { // Column.name
			name = string(val)
		}
		return nil
	})
	return name, err
}

// walkFields iterates the top-level fields of a protobuf message, calling fn with
// each field number, wire type, and its raw value bytes (for length-delimited
// fields) or the varint/fixed bytes otherwise.
func walkFields(b []byte, fn func(field, wt int, val []byte) error) error {
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			return fmt.Errorf("bad field tag")
		}
		b = b[n:]
		field := int(tag >> 3)
		wt := int(tag & 7)
		switch wt {
		case wireVarint:
			_, m := binary.Uvarint(b)
			if m <= 0 {
				return fmt.Errorf("bad varint")
			}
			if err := fn(field, wt, b[:m]); err != nil {
				return err
			}
			b = b[m:]
		case wireI64:
			if len(b) < 8 {
				return fmt.Errorf("truncated i64")
			}
			if err := fn(field, wt, b[:8]); err != nil {
				return err
			}
			b = b[8:]
		case wireBytes:
			l, m := binary.Uvarint(b)
			if m <= 0 {
				return fmt.Errorf("bad length prefix")
			}
			b = b[m:]
			if uint64(len(b)) < l {
				return fmt.Errorf("truncated length-delimited field")
			}
			if err := fn(field, wt, b[:l]); err != nil {
				return err
			}
			b = b[l:]
		case wireI32:
			if len(b) < 4 {
				return fmt.Errorf("truncated i32")
			}
			if err := fn(field, wt, b[:4]); err != nil {
				return err
			}
			b = b[4:]
		default:
			return fmt.Errorf("unsupported wire type %d", wt)
		}
	}
	return nil
}

// --- encode ---

// encodeGenerateResponse builds a GenerateResponse{ repeated File files = 1 }.
func encodeGenerateResponse(files []File) []byte {
	var out []byte
	for _, f := range files {
		fileMsg := encodeFile(f)
		out = appendTag(out, 1, wireBytes)
		out = appendBytes(out, fileMsg)
	}
	return out
}

// encodeFile builds a File{ string name=1; bytes contents=2 }.
func encodeFile(f File) []byte {
	var out []byte
	out = appendTag(out, 1, wireBytes)
	out = appendBytes(out, []byte(f.Name))
	out = appendTag(out, 2, wireBytes)
	out = appendBytes(out, f.Contents)
	return out
}

func appendTag(b []byte, field, wt int) []byte {
	return binary.AppendUvarint(b, uint64(field)<<3|uint64(wt))
}

func appendBytes(b, val []byte) []byte {
	b = binary.AppendUvarint(b, uint64(len(val)))
	return append(b, val...)
}
