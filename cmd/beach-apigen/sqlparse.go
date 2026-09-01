package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// sqlparse.go is the standalone (no-sqlc) front end: it reads annotated .sql
// files directly and builds Query models. It is intentionally a light parser —
// enough to drive generation and tests without the full sqlc engine. The sqlc
// plugin path (plugin.go) builds the same Query models from the richer protobuf.
//
// What it understands:
//   - "-- name: <Name> :<cmd>" query headers (sqlc's own syntax)
//   - the comment block above the SQL body (the six @-annotations)
//   - sqlc named params: "@id", "sqlc.arg(name)", and ":name" — to infer the
//     argument struct. One param => a scalar arg; many => a Params struct.

var (
	nameLineRe = regexp.MustCompile(`(?i)^--\s*name:\s*(\w+)\s+:(\w+)\s*$`)
	// param forms sqlc accepts. We capture the param name to build the arg shape.
	atParamRe = regexp.MustCompile(`@(\w+)`)
	sqlcArgRe = regexp.MustCompile(`sqlc\.(?:arg|narg)\(\s*'?(\w+)'?\s*\)`)
)

// parseSQLDir parses every *.sql file under dir into Query models, sorted by
// name for determinism. Files with no named queries are skipped silently.
func parseSQLDir(dir string) ([]Query, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read sql dir: %w", err)
	}
	var out []Query
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		qs, err := parseSQLFile(string(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, qs...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// queryBlock is one query's raw text: the contiguous run of comment lines
// immediately above and including the "-- name:" header, plus the SQL body up to
// the next header.
type rawBlock struct {
	comment string // every comment line in the block, including annotations
	header  string // the "-- name: X :cmd" line
	body    string // SQL statement text (no comments)
}

// parseSQLFile splits a file into query blocks and parses each into a Query.
func parseSQLFile(src string) ([]Query, error) {
	blocks := splitBlocks(src)
	var out []Query
	for _, b := range blocks {
		m := nameLineRe.FindStringSubmatch(strings.TrimSpace(b.header))
		if m == nil {
			continue
		}
		ann, err := parseAnnotations(b.comment)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m[1], err)
		}
		q := Query{
			Name: m[1],
			Cmd:  ":" + strings.ToLower(m[2]),
			Ann:  ann,
		}
		inferArgs(&q, b.body)
		out = append(out, q)
	}
	return out, nil
}

// splitBlocks breaks SQL text into per-query blocks keyed on "-- name:" headers.
// Comment lines preceding a header attach to that block; the body runs to the
// next header.
func splitBlocks(src string) []rawBlock {
	lines := strings.Split(src, "\n")
	var blocks []rawBlock
	var cur *rawBlock
	// comments accumulates a query's annotation lines. In sqlc syntax they sit
	// immediately below the "-- name:" header and above the SQL body, so they
	// attach to the current block as we read them.
	var comments []string

	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		switch {
		case nameLineRe.MatchString(trimmed):
			// New query. Flush any comments collected for the previous block, then
			// start fresh. (Annotations follow the header, so comments are normally
			// empty here — but a comment above the first header is harmless.)
			finishBlock(cur, comments)
			comments = nil
			blocks = append(blocks, rawBlock{header: trimmed})
			cur = &blocks[len(blocks)-1]
		case strings.HasPrefix(trimmed, "--"):
			comments = append(comments, trimmed)
		case trimmed == "":
			// Blank line: ignore (annotations may be separated from SQL by a blank).
		default:
			// SQL body line.
			finishBlock(cur, comments)
			comments = nil
			if cur != nil {
				cur.body += ln + "\n"
			}
		}
	}
	finishBlock(cur, comments)
	return blocks
}

// finishBlock attaches the collected comment lines to a block's annotation text
// once, on the first SQL line or the next header. Repeated calls with no new
// comments are no-ops, so trailing flushes are safe.
func finishBlock(b *rawBlock, comments []string) {
	if b == nil || len(comments) == 0 || b.comment != "" {
		return
	}
	b.comment = strings.Join(comments, "\n")
}

// inferArgs derives the argument shape from the SQL body's named params. Zero
// params => no input; one => a scalar arg; many => a Params struct (sqlc's
// "<Name>Params"). Types are not known without sqlc's catalog, so scalar args
// default to int64 for "id"-like names and string otherwise — a reasonable
// standalone default that the sqlc plugin path overrides with real types.
func inferArgs(q *Query, body string) {
	names := namedParams(body)
	switch len(names) {
	case 0:
		// no input
	case 1:
		q.ArgIsScalar = true
		q.ScalarArg = names[0]
		q.ScalarType = scalarTypeFor(names[0])
		q.Params = []Param{{Field: exportName(names[0]), JSONName: names[0], Type: q.ScalarType}}
	default:
		q.ArgType = q.Name + "Params"
		for _, n := range names {
			q.Params = append(q.Params, Param{Field: exportName(n), JSONName: n, Type: scalarTypeFor(n)})
		}
	}
}

// namedParams extracts the distinct named parameters from a SQL body, preserving
// first-seen order so a single param is unambiguous.
func namedParams(body string) []string {
	seen := map[string]bool{}
	var order []string
	add := func(n string) {
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		order = append(order, n)
	}
	for _, m := range atParamRe.FindAllStringSubmatch(body, -1) {
		add(m[1])
	}
	for _, m := range sqlcArgRe.FindAllStringSubmatch(body, -1) {
		add(m[1])
	}
	return order
}

// scalarTypeFor is the standalone type guess: id-shaped names are int64, the rest
// string. The sqlc plugin path replaces these with the catalog's real Go types.
func scalarTypeFor(name string) string {
	if name == "id" || strings.HasSuffix(name, "_id") || strings.HasSuffix(name, "ID") {
		return "int64"
	}
	return "string"
}
