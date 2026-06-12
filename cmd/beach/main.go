// Command beach is the-beach's scaffold CLI. It stamps buildable skeleton apps
// and runs the framework's source-level codegen that does not belong inside the
// sqlc pipeline.
//
// Usage:
//
//	beach new <app> [--dir DIR]   stamp a buildable skeleton app
//	beach ecs gen [--file F]      generate Go components from components.beach.yaml
//	beach i18n [--write]          verify (or write) the i18n catalog from source
//
// `beach new` writes a complete, compiling the-beach app: wiring in main.go
// (one page, one Datastar action, one SSE fragment), driftwood markup in
// views.templ (plus its committed views_templ.go compile output), a Makefile,
// sqlc.yaml (apigen plugin registered),
// docker-compose.yml (Postgres up, ClickHouse commented out), an .env template,
// and a go.mod with a local `replace` so the stamped app builds against this
// checkout without network access.
package main

import (
	"fmt"
	"os"
)

// usage is printed on `beach`, `beach help`, or any malformed invocation.
const usage = `beach — the-beach scaffold CLI

usage:
  beach new <app> [--dir DIR]   stamp a buildable skeleton app into DIR/<app>
  beach ecs gen [--file F]      generate Go component structs from components.beach.yaml
  beach i18n [--write] [--dir D] verify (or --write) catalog.json from i18n.T("key") calls

flags:
  new   --dir DIR    parent directory for the new app (default ".")
  ecs   --file F     schema file (default "components.beach.yaml")
        --out  O      output Go file (default "<dir-of-file>/components_gen.go")
        --pkg  P      package name for generated code (default "components")
  i18n  --write      rewrite catalog.json instead of just verifying
        --dir  D      module root to scan and write catalog under (default ".")
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "new":
		err = cmdNew(os.Args[2:])
	case "ecs":
		err = cmdECS(os.Args[2:])
	case "i18n":
		err = cmdI18n(os.Args[2:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "beach: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "beach: %v\n", err)
		os.Exit(1)
	}
}
