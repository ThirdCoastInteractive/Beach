// Package chrome defines the presentation of shared Beach components. Chromes
// are registered at boot and selected by the application owner at request time.
package chrome

import (
	"context"
	"fmt"
	"regexp"

	"github.com/ThirdCoastInteractive/Beach/pkg/theme"
	"github.com/ThirdCoastInteractive/Beach/pkg/typefaces"
)

// Definition is copied at registration. Asset paths are relative to /static/.
// Styles must be scoped to .chrome-<ID>; markup and behavior remain shared.
type Definition struct {
	Typography  typefaces.Roles
	Fonts       []typefaces.WebFace
	ID, Name    string
	Stylesheet  string
	ThemePreset string
	// Navigation is "side" or "top"; ProductLayout is "split" or "stage".
	Navigation, ProductLayout string
	// Density is "comfortable" or "compact".
	Density string
}

func Driftwood() Definition {
	return Definition{ID: "driftwood", Name: "Driftwood", ThemePreset: "driftwood", Navigation: "side", ProductLayout: "split", Density: "comfortable"}
}

func Afterimage() Definition {
	return Definition{ID: "afterimage", Name: "Afterimage", Stylesheet: "css/afterimage.css", ThemePreset: "afterimage", Navigation: "top", ProductLayout: "stage", Density: "compact",
		Typography: typefaces.Roles{Display: "Afterimage"},
		Fonts:      []typefaces.WebFace{{Family: "Afterimage", Source: "/static/fonts/BarlowCondensed-Bold.woff2", Weight: 700}, {Family: "Afterimage", Source: "/static/fonts/BarlowCondensed-ExtraBold.woff2", Weight: 800}},
	}
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var assetPath = regexp.MustCompile(`^[a-zA-Z0-9_/-]+\.css$`)

func (d Definition) Validate() error {
	for _, f := range d.Fonts {
		if _, err := f.CSS(); err != nil {
			return err
		}
	}
	if !identifier.MatchString(d.ID) || d.Name == "" {
		return fmt.Errorf("chrome: invalid identity %q", d.ID)
	}
	if d.Stylesheet != "" && !assetPath.MatchString(d.Stylesheet) {
		return fmt.Errorf("chrome %s: invalid stylesheet", d.ID)
	}
	if d.Navigation != "side" && d.Navigation != "top" {
		return fmt.Errorf("chrome %s: invalid navigation", d.ID)
	}
	if d.ProductLayout != "split" && d.ProductLayout != "stage" {
		return fmt.Errorf("chrome %s: invalid product layout", d.ID)
	}
	if d.Density != "comfortable" && d.Density != "compact" {
		return fmt.Errorf("chrome %s: invalid density", d.ID)
	}
	_, err := theme.BuildPreset(d.ThemePreset)
	return err
}

// Presentation is the request snapshot. Asset URLs identify the resolved chrome,
// so a later owner change cannot alter the styles of an already loaded document.
type Presentation struct {
	Definition
	StylesheetURL, ThemeURL, TypographyURL string
}

type contextKey struct{}

func With(ctx context.Context, p Presentation) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}

func From(ctx context.Context) Presentation {
	if ctx != nil {
		if p, ok := ctx.Value(contextKey{}).(Presentation); ok {
			return p
		}
	}
	return Presentation{Definition: Driftwood(), ThemeURL: "/_beach/theme.css"}
}
