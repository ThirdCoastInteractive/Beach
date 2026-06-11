package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// catalogEntry mirrors the i18n catalog.json shape: one label + translator
// comment per key (see i18n/catalog.json).
type catalogEntry struct {
	Label   string `json:"label"`
	Comment string `json:"comment"`
}

// cmdI18n implements `beach i18n [--write] [--dir D]`. It scans Go source for
// literal-key i18n.T(...) calls and reconciles them against catalog.json: in
// verify mode (default) it fails when keys are missing or stale; with --write it
// rewrites the catalog to exactly the discovered key set, preserving existing
// labels/comments.
func cmdI18n(args []string) error {
	fs := flag.NewFlagSet("i18n", flag.ContinueOnError)
	write := fs.Bool("write", false, "rewrite catalog.json to match discovered keys")
	dir := fs.String("dir", ".", "module root to scan and locate catalog.json under")
	catalogPath := fs.String("catalog", "", "catalog.json path (default: <dir>/catalog.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cat := *catalogPath
	if cat == "" {
		cat = filepath.Join(*dir, "catalog.json")
	}

	keys, err := scanKeys(*dir)
	if err != nil {
		return err
	}

	existing, err := loadCatalog(cat)
	if err != nil {
		return err
	}

	missing, stale := diffKeys(keys, existing)

	if *write {
		next := reconcile(keys, existing)
		if err := writeCatalog(cat, next); err != nil {
			return err
		}
		fmt.Printf("wrote %s (%d keys: +%d -%d)\n", cat, len(next), len(missing), len(stale))
		return nil
	}

	if len(missing) == 0 && len(stale) == 0 {
		fmt.Printf("catalog ok: %d keys, %s in sync\n", len(keys), cat)
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "i18n catalog out of date (%s)\n", cat)
	for _, k := range missing {
		fmt.Fprintf(&b, "  missing: %s\n", k)
	}
	for _, k := range stale {
		fmt.Fprintf(&b, "  stale:   %s\n", k)
	}
	fmt.Fprint(&b, "run `beach i18n --write` to fix")
	return fmt.Errorf("%s", b.String())
}

// scanKeys walks dir for *.go files and returns the sorted, de-duplicated set of
// literal first-argument keys passed to any call whose function is named T on an
// "i18n" selector — i18n.T(ctx, "key", ...). Non-literal keys are ignored (the
// literal-only rule); a non-literal key is not an error here, the vet analyzer
// owns that policy.
func scanKeys(dir string) ([]string, error) {
	seen := map[string]bool{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != dir && (name == "vendor" || name == "bin" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			// A file that does not parse is not our problem to flag; skip it so a
			// half-written file does not break catalog maintenance.
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			if !isI18nT(call.Fun) {
				return true
			}
			if k, ok := literalKeyArg(call.Args); ok {
				seen[k] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("i18n: scan %s: %w", dir, err)
	}

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// isI18nT reports whether fun is a selector of the form i18n.T (a T method/func
// on a package or value named "i18n"). This matches both the package-level
// i18n.T and a catalog value conventionally named i18n.
func isI18nT(fun ast.Expr) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "i18n"
}

// literalKeyArg returns the string-literal key argument. i18n.T's signature is
// T(ctx, key, args...); the key is the first string-literal argument among the
// first two positions (covering both T(ctx,"k") and any helper that drops ctx).
func literalKeyArg(args []ast.Expr) (string, bool) {
	limit := len(args)
	if limit > 2 {
		limit = 2
	}
	for i := 0; i < limit; i++ {
		lit, ok := args[i].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			continue
		}
		return s, true
	}
	return "", false
}

// loadCatalog reads catalog.json if present; a missing file yields an empty map.
func loadCatalog(path string) (map[string]catalogEntry, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]catalogEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("i18n: read %s: %w", path, err)
	}
	var m map[string]catalogEntry
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("i18n: parse %s: %w", path, err)
	}
	if m == nil {
		m = map[string]catalogEntry{}
	}
	return m, nil
}

// diffKeys returns keys present in source but absent from the catalog (missing)
// and keys present in the catalog but absent from source (stale).
func diffKeys(keys []string, cat map[string]catalogEntry) (missing, stale []string) {
	inSource := map[string]bool{}
	for _, k := range keys {
		inSource[k] = true
		if _, ok := cat[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range cat {
		if !inSource[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	return missing, stale
}

// reconcile builds the next catalog: exactly the discovered keys, keeping any
// existing label/comment and stubbing new keys with the key as the label.
func reconcile(keys []string, cat map[string]catalogEntry) map[string]catalogEntry {
	next := make(map[string]catalogEntry, len(keys))
	for _, k := range keys {
		if e, ok := cat[k]; ok {
			next[k] = e
			continue
		}
		next[k] = catalogEntry{Label: k, Comment: ""}
	}
	return next
}

// writeCatalog writes the catalog as stable, sorted, indented JSON so diffs stay
// readable in git.
func writeCatalog(path string, cat map[string]catalogEntry) error {
	keys := make([]string, 0, len(cat))
	for k := range cat {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("{\n")
	for i, k := range keys {
		entry := cat[k]
		keyJSON, _ := json.Marshal(k)
		labelJSON, _ := json.Marshal(entry.Label)
		commentJSON, _ := json.Marshal(entry.Comment)
		fmt.Fprintf(&b, "  %s: {\n    \"label\": %s,\n    \"comment\": %s\n  }",
			keyJSON, labelJSON, commentJSON)
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("i18n: write %s: %w", path, err)
	}
	return nil
}
