// Package lint holds the beach vet analyzers: static checks that enforce
// Beach's house rules over a Go source tree. Every rule is implemented with
// the standard library only (go/parser, go/ast, go/token) — no external deps —
// so the runner stays trivially buildable inside the frozen module.
//
// The unit of analysis is a single *.go file parsed to an *ast.File. Each rule
// inspects the AST (mostly string literals) and reports Findings. The rules are
// heuristics tuned against the real example apps so the sanctioned framework
// path (the datastar package, the ui kit, internal/db) does not false-positive.
//
// Entry point: Check(root) walks root, parses every non-test .go file outside
// vendor/ and testdata/, and returns the union of all rule findings sorted by
// file then line.
package lint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Finding is one rule violation at a source location. File is a slash-cleaned
// path relative to the process working dir (or absolute if root was absolute);
// Line is 1-based; Rule is the stable rule id; Message explains the fix.
type Finding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// fileCtx is the per-file context handed to each rule: the parsed AST, the
// fileset for position lookup, and the slash-separated import path fragment of
// the directory (e.g. "examples/pantry", "datastar", "ui/driftwood") used by
// rules to decide whether the file sits on a sanctioned path.
type fileCtx struct {
	path string // reported file path
	pkg  string // slash dir path relative to root, for sanctioning
	fset *token.FileSet
	file *ast.File
	src  []byte
}

// rule is a single analyzer. It appends its findings for one file.
type rule func(fc *fileCtx, out *[]Finding)

// rules is the ordered analyzer set. Order only affects emission order within a
// line; the final slice is sorted by file/line regardless.
var rules = []rule{
	ruleRawDatastar,
	rulePgtype,
	ruleUUID,
	ruleHardcodedColor,
	ruleNakedHandlerFunc,
	ruleCustomScript,
	ruleImgAlt,
	ruleUnnamedRoleImg,
	ruleLiteralAccessibleName,
	ruleRawSpacing,
}

// Check walks root, analyzes every eligible Go file, and returns all findings
// sorted by file then line. Parse errors on individual files are reported as a
// finding (rule "parse") rather than aborting the whole run, so one bad file
// cannot blind the rest of the tree.
func Check(root string) ([]Finding, error) {
	var findings []Finding

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !eligible(path) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		af, perr := parser.ParseFile(fset, path, src, parser.ParseComments)
		rel := relDir(root, path)
		reported := reportPath(root, path)
		if perr != nil {
			findings = append(findings, Finding{
				File:    reported,
				Line:    parseLine(perr),
				Rule:    "parse",
				Message: "could not parse file: " + perr.Error(),
			})
			return nil
		}
		fc := &fileCtx{path: reported, pkg: rel, fset: fset, file: af, src: src}
		for _, r := range rules {
			r(fc, &findings)
		}
		return nil
	})
	if walkErr != nil {
		return findings, walkErr
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Rule < findings[j].Rule
	})
	return findings, nil
}

// skipDir is true for directory names we never descend into.
func skipDir(name string) bool {
	switch name {
	case "vendor", "node_modules", ".git", "testdata":
		return true
	}
	return false
}

// eligible is true for non-test Go source files.
func eligible(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	return true
}

// relDir returns the slash-separated directory of path relative to root, used
// as the package-path key for sanctioning. Falls back to the raw dir on error.
func relDir(root, path string) string {
	dir := filepath.Dir(path)
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		rel = dir
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		rel = ""
	}
	return rel
}

// reportPath returns the path we print in findings: slash-cleaned, relative to
// root when root is non-trivial so output is stable across machines.
func reportPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// lineOf returns the 1-based line for a token position.
func (fc *fileCtx) lineOf(pos token.Pos) int {
	return fc.fset.Position(pos).Line
}

// stringLits2 walks the file and calls fn for every string-literal value with
// its 1-based line. The literal value is already Go-unquoted (escapes resolved).
func (fc *fileCtx) stringLits2(fn func(val string, line int)) {
	ast.Inspect(fc.file, func(n ast.Node) bool {
		bl, ok := n.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return true
		}
		fn(litValue(bl.Value), fc.lineOf(bl.Pos()))
		return true
	})
}

// litValue unquotes a Go string-literal token (handles both interpreted and raw
// backtick strings). On any failure it strips the outer quote runes as a best
// effort so scanning still sees the inner text.
func litValue(tok string) string {
	if len(tok) >= 2 {
		q := tok[0]
		if (q == '"' || q == '`') && tok[len(tok)-1] == q {
			inner := tok[1 : len(tok)-1]
			if q == '`' {
				return inner
			}
			return unescape(inner)
		}
	}
	return tok
}

// unescape resolves the common escapes that matter to our scanners. We avoid
// strconv.Unquote because it rejects multi-rune content and partial sequences
// we still want to scan; a forgiving pass is safer for a linter.
func unescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteByte(s[i+1])
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseLine pulls a line number out of a parser error message if present.
func parseLine(err error) int {
	// scanner.ErrorList formats as "file:line:col: msg"; we only need a hint.
	msg := err.Error()
	parts := strings.Split(msg, ":")
	if len(parts) >= 2 {
		if n := atoi(parts[1]); n > 0 {
			return n
		}
	}
	return 1
}

func atoi(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// onPath reports whether the file's package directory equals or is nested under
// any of the given slash prefixes.
//
// The prefixes are written in library terms — "ui", "datastar", "internal/db" —
// because beach-vet runs over two shapes of tree: the framework itself, where
// those live under pkg/, and a consuming app stamped by `beach new`, where
// internal/db sits at the root. So a prefix is matched against the package path
// both as-is and with a leading "pkg/" removed. Without that second form none of
// the sanctioning matched inside the framework, and every rule reported its own
// sanctioned package.
func (fc *fileCtx) onPath(prefixes ...string) bool {
	candidates := [2]string{fc.pkg, strings.TrimPrefix(fc.pkg, "pkg/")}
	for _, p := range prefixes {
		for _, c := range candidates {
			if c == p || strings.HasPrefix(c, p+"/") {
				return true
			}
		}
	}
	return false
}
