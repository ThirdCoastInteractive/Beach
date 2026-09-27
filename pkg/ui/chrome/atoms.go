package chrome

import "github.com/ThirdCoastInteractive/Beach/pkg/typefaces"

// Atoms is Nitsche technical print: cream paper, condensed display, coplanar
// panels, side navigation, and compact split workspaces. It changes styling,
// never behavior. Barlow Condensed is bundled; app-only faces stay out of Fonts.
func Atoms() Definition {
	return Definition{
		ID: "atoms", Name: "Atoms",
		Stylesheet:    "css/atoms.css",
		ThemePreset:   "atoms",
		Navigation:    "side",
		ProductLayout: "split",
		Density:       "compact",
		Typography:    typefaces.Roles{Display: "Barlow Condensed"},
		Fonts: []typefaces.WebFace{
			{Family: "Barlow Condensed", Source: "/static/fonts/BarlowCondensed-Bold.woff2", Weight: 700},
			{Family: "Barlow Condensed", Source: "/static/fonts/BarlowCondensed-ExtraBold.woff2", Weight: 800},
		},
	}
}
