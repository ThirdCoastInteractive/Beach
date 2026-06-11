// Command beach-apigen is a sqlc process plugin that reads six annotations on
// named SQL queries and emits beach handler wiring — PageFunc/ActionFunc
// factories, route registration, and the NOTIFY trigger migration. SQL stays the
// single source of truth; see docs/architecture/13-apigen.md.
//
// As a sqlc process plugin it reads a protobuf GenerateRequest on stdin and
// writes a GenerateResponse on stdout (plugin.go). It also runs standalone for
// development: `beach-apigen -sql dir -out pkgdir -pkg name` parses .sql files
// directly and writes the generated files, no sqlc required.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// parseComponentFlag parses "sel=path,sel2=path2" into a selector->import map.
func parseComponentFlag(s string) map[string]string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		sel, path, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && sel != "" && path != "" {
			out[sel] = path
		}
	}
	return out
}

func main() {
	var (
		sqlDir = flag.String("sql", "", "standalone mode: directory of annotated .sql query files")
		outDir = flag.String("out", "", "standalone mode: directory to write generated files into")
		pkg    = flag.String("pkg", "api", "standalone mode: package name for generated Go files")
		typ    = flag.String("querier", "*Queries", "standalone mode: Go type of the sqlc querier")
		comps  = flag.String("components", "", "standalone mode: component imports as sel=importpath, comma-separated (e.g. page=example.com/app/page)")
	)
	flag.Parse()

	// Standalone mode is the dev/testing entry point: no sqlc, just files in/out.
	if *sqlDir != "" {
		if err := runStandalone(*sqlDir, *outDir, *pkg, *typ, parseComponentFlag(*comps)); err != nil {
			fmt.Fprintln(os.Stderr, "beach-apigen:", err)
			os.Exit(1)
		}
		return
	}

	// Default: act as a sqlc process plugin over stdin/stdout.
	if err := runPlugin(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "beach-apigen:", err)
		os.Exit(1)
	}
}

// runStandalone parses every .sql file under sqlDir, generates handler wiring,
// and writes the output files into outDir. It is the path the integration test
// drives and the way to use apigen without a full sqlc setup.
func runStandalone(sqlDir, outDir, pkg, querier string, comps map[string]string) error {
	queries, err := parseSQLDir(sqlDir)
	if err != nil {
		return err
	}
	files, err := Generate(GenConfig{Package: pkg, QuerierType: querier, ComponentImports: comps}, queries)
	if err != nil {
		return err
	}
	if outDir == "" {
		// No out dir: write to stdout for inspection.
		for _, f := range files {
			fmt.Printf("// === %s ===\n%s\n", f.Name, f.Contents)
		}
		return nil
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(outDir, f.Name), f.Contents, 0o644); err != nil {
			return err
		}
	}
	return nil
}
