// Command beach-vet runs Beach lint analyzers over a source tree and prints
// findings. It exits non-zero when any finding is reported, so it slots into
// `make vet` and CI as a gate.
//
// Usage:
//
//	beach-vet [-json] [dir]
//
// dir defaults to "." (the current module root). The "./..." form is accepted
// and treated as ".", since the analyzers already walk recursively. With -json
// the findings are emitted as a JSON array for tooling; otherwise a compact
// "file:line: [rule] message" line per finding plus a summary tail.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ThirdCoastInteractive/Beach/internal/lint"
)

func main() {
	jsonOut := flag.Bool("json", false, "emit findings as a JSON array")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: beach-vet [-json] [dir]")
		fmt.Fprintln(os.Stderr, "  dir defaults to '.'; './...' is accepted and treated as '.'")
		flag.PrintDefaults()
	}
	flag.Parse()

	root := "."
	if args := flag.Args(); len(args) > 0 {
		root = args[0]
	}
	// Accept the conventional go-tool target spelling.
	root = strings.TrimSuffix(root, "/...")
	root = strings.TrimSuffix(root, `\...`)
	if root == "" {
		root = "."
	}

	findings, err := lint.Check(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "beach-vet: "+err.Error())
		os.Exit(2)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if findings == nil {
			findings = []lint.Finding{}
		}
		if err := enc.Encode(findings); err != nil {
			fmt.Fprintln(os.Stderr, "beach-vet: "+err.Error())
			os.Exit(2)
		}
		if len(findings) > 0 {
			os.Exit(1)
		}
		return
	}

	for _, f := range findings {
		fmt.Printf("%s:%d: [%s] %s\n", f.File, f.Line, f.Rule, f.Message)
	}
	if len(findings) == 0 {
		fmt.Println("beach-vet: clean")
		return
	}
	fmt.Printf("beach-vet: %d finding(s)\n", len(findings))
	os.Exit(1)
}
