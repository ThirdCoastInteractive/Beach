package typefaces

import (
	"fmt"
	"strings"
)

// Roles assigns families to semantic typography roles. Empty roles inherit.
// Family names are escaped, not interpreted as raw CSS font stacks.
type Roles struct{ Display, Body, Caption, Label, Mono string }

func (r Roles) CSS(scope string) (string, error) {
	if !idPattern.MatchString(scope) {
		return "", fmt.Errorf("typefaces: invalid chrome scope %q", scope)
	}
	var b strings.Builder
	fmt.Fprintf(&b, ".chrome-%s{", scope)
	for _, p := range []struct{ role, family string }{{"display", r.Display}, {"body", r.Body}, {"caption", r.Caption}, {"label", r.Label}, {"mono", r.Mono}} {
		if p.family != "" {
			fallback := "sans-serif"
			if p.role == "mono" {
				fallback = "monospace"
			}
			fmt.Fprintf(&b, "--font-%s:%s,%s;", p.role, cssString(p.family), fallback)
		}
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// WebFace describes a shipped, same-origin font. Use Library.File for export
// fonts and Library.CSS for installed web fonts; this type is for bundled assets.
type WebFace struct {
	Family, Source string
	Weight         int
	Italic         bool
}

func (f WebFace) CSS() (string, error) {
	if f.Family == "" || f.Weight < 100 || f.Weight > 900 || !strings.HasPrefix(f.Source, "/static/") || strings.ContainsAny(f.Source, "\"'<>\\\r\n?#") || strings.Contains(f.Source, "..") || !strings.HasSuffix(f.Source, ".woff2") {
		return "", fmt.Errorf("typefaces: invalid web face %q", f.Family)
	}
	style := "normal"
	if f.Italic {
		style = "italic"
	}
	return fmt.Sprintf("@font-face{font-family:%s;src:url(%s) format(\"woff2\");font-weight:%d;font-style:%s;font-display:swap;}\n", cssString(f.Family), cssString(f.Source), f.Weight, style), nil
}
