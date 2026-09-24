package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// appData is the render context handed to every .tmpl file.
type appData struct {
	App         string // the app name as given on the command line
	Module      string // Go module path for the stamped app
	ReplacePath string // relative path from the app dir to this Beach checkout
	Requires    string // the framework's transitive require block, mirrored verbatim
}

// templateRoot is the prefix inside templatesFS that holds the app skeleton.
const templateRoot = "templates/app"

// dotfileNames maps a stamped filename to the on-disk name it should take. The
// embed tooling will not carry a leading dot reliably across the template tree,
// so a couple of dotfiles are stored under plain names and renamed on stamp.
var dotfileNames = map[string]string{
	"gitignore": ".gitignore",
	"env":       ".env",
}

// cmdNew implements `beach new <app> [--dir DIR]`.
func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	dir := fs.String("dir", ".", "parent directory for the new app")
	module := fs.String("module", "", "Go module path (default: the app name)")
	// Reorder so flags may appear before or after the positional app name; the
	// stdlib flag package otherwise stops parsing at the first non-flag token.
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("new: expected exactly one app name (got %d)", fs.NArg())
	}
	app := fs.Arg(0)
	if err := validAppName(app); err != nil {
		return err
	}

	target := filepath.Join(*dir, app)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("new: %s already exists", target)
	}

	mod := *module
	if mod == "" {
		mod = app
	}

	replacePath, err := replaceToCheckout(target)
	if err != nil {
		return err
	}

	requires, err := frameworkRequires()
	if err != nil {
		return err
	}

	data := appData{App: app, Module: mod, ReplacePath: replacePath, Requires: requires}
	if err := stamp(target, data); err != nil {
		return err
	}

	// Copy the checkout's go.sum so the stamped app can verify the framework's
	// transitive dependency checksums offline (its require set is a subset of the
	// framework's, so the framework go.sum is a valid superset). Without this the
	// app needs `go mod download`/network to build.
	if err := copyGoSum(target); err != nil {
		return err
	}

	fmt.Printf("stamped %s into %s\n", app, target)
	fmt.Printf("  cd %s && make up\n", target)
	return nil
}

// reorderArgs moves flag tokens (and their values) ahead of bare positional
// arguments so the stdlib flag package, which stops at the first non-flag token,
// still sees flags that the user wrote after the positional. Flags here are all
// value-taking (--dir, --module), so a flag token consumes the following token
// unless it is in --flag=value form.
func reorderArgs(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
}

// stamp walks the embedded skeleton and writes each file under target, rendering
// .tmpl files and copying the rest verbatim.
func stamp(target string, data appData) error {
	return fs.WalkDir(templatesFS, templateRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(templateRoot, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "." {
				return os.MkdirAll(target, 0o755)
			}
			return os.MkdirAll(filepath.Join(target, rel), 0o755)
		}

		out := filepath.Join(target, rel)
		content, err := templatesFS.ReadFile(p)
		if err != nil {
			return err
		}

		if strings.HasSuffix(out, ".tmpl") {
			out = strings.TrimSuffix(out, ".tmpl")
			rendered, err := render(rel, content, data)
			if err != nil {
				return err
			}
			content = rendered
		}

		out = applyDotfile(out)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, content, 0o644)
	})
}

// render executes content as a text/template against data.
func render(name string, content []byte, data appData) ([]byte, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// applyDotfile rewrites the final path component when it is one of the dotfile
// placeholder names (gitignore -> .gitignore, env -> .env).
func applyDotfile(out string) string {
	base := filepath.Base(out)
	if dot, ok := dotfileNames[base]; ok {
		return filepath.Join(filepath.Dir(out), dot)
	}
	return out
}

// replaceToCheckout returns a relative path usable in a go.mod `replace` that
// points from the stamped app directory back to this Beach checkout (the
// repo whose go.mod declares the module). The CLI runs from the checkout, so
// the checkout root is found by walking up from the working directory until a
// go.mod naming the framework module is found; falls back to cwd.
func replaceToCheckout(appDir string) (string, error) {
	checkout, err := findCheckout()
	if err != nil {
		return "", err
	}
	absApp, err := filepath.Abs(appDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absApp, checkout)
	if err != nil {
		// Different volumes (e.g. app on C:, checkout on Y:) make a relative path
		// impossible; an absolute replace target is valid and keeps the app
		// buildable. go.mod tolerates absolute replace paths.
		return filepath.ToSlash(checkout), nil
	}
	// go.mod replace paths use forward slashes on every platform.
	rel = filepath.ToSlash(rel)
	// A replace target must be explicitly relative.
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel, nil
}

// frameworkRequires reads the checkout's go.mod and returns its require
// directives as a single block, ready to splice into the stamped go.mod. Both
// the single-line `require x v1` form and the `require ( ... )` block form are
// captured verbatim (indirect comments included) so the stamped module graph is
// a faithful mirror of the framework's.
func frameworkRequires() (string, error) {
	checkout, err := findCheckout()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(checkout, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("new: read framework go.mod: %w", err)
	}

	var out []string
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	inBlock := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case inBlock:
			if t == ")" {
				out = append(out, ")")
				inBlock = false
				continue
			}
			out = append(out, "\t"+t)
		case strings.HasPrefix(t, "require ("):
			out = append(out, "require (")
			inBlock = true
		case strings.HasPrefix(t, "require "):
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n"), nil
}

// copyGoSum copies the framework checkout's go.sum into the stamped app dir.
// The app's require set is a subset of the framework's, so the framework go.sum
// covers every module the app resolves through the local replace — letting the
// app build offline with checksum verification intact.
func copyGoSum(target string) error {
	checkout, err := findCheckout()
	if err != nil {
		return err
	}
	src := filepath.Join(checkout, "go.sum")
	b, err := os.ReadFile(src)
	if err != nil {
		// No go.sum in the checkout is unusual but not fatal to stamping; the user
		// can run `go mod download` themselves.
		return nil
	}
	return os.WriteFile(filepath.Join(target, "go.sum"), b, 0o644)
}

// findCheckout walks up from the working directory looking for the go.mod that
// declares the Beach module path.
func findCheckout() (string, error) {
	const moduleLine = "module github.com/ThirdCoastInteractive/Beach"
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		gomod := filepath.Join(dir, "go.mod")
		if b, err := os.ReadFile(gomod); err == nil {
			if strings.Contains(string(b), moduleLine) {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Not inside the checkout; fall back to the working dir so the path is
			// at least well-formed.
			cwd, _ := os.Getwd()
			return cwd, nil
		}
		dir = parent
	}
}

// validAppName rejects names that would not make a valid directory or module
// path segment.
func validAppName(name string) error {
	if name == "" {
		return fmt.Errorf("new: app name must not be empty")
	}
	for _, r := range name {
		ok := r == '-' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')
		if !ok {
			return fmt.Errorf("new: app name %q has invalid character %q (use letters, digits, - or _)", name, r)
		}
	}
	return nil
}
