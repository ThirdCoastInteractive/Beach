package components

import (
	"github.com/ThirdCoastInteractive/Beach/pkg/datastar"
	"github.com/a-h/templ"
)

type DisplayHeadingProps struct {
	Eyebrow, Title, Lead string
	Level                int
	Attrs                datastar.Attrs
}

type ProductLayout string

const (
	ProductAuto      ProductLayout = ""
	ProductSplit     ProductLayout = "split"
	ProductExpansive ProductLayout = "stage"
	// Editorial layouts are explicitly selected by a page, never by a chrome.
	ProductCover     ProductLayout = "cover"
	ProductPlate     ProductLayout = "plate"
	ProductTechnical ProductLayout = "technical"
	ProductMonument  ProductLayout = "monument"
	ProductShowcase  ProductLayout = "showcase"
)

type ProductStageProps struct {
	Heading                 DisplayHeadingProps
	Image                   ImageProps
	Layout                  ProductLayout
	Model, Edition, Caption string
	Specs                   []SpecItem
	// Details adds construction photographs and their explanations to an
	// editorial plate. They remain real captions, including on small screens.
	Details []ProductDetail
	Attrs   datastar.Attrs
}

type ProductDetail struct {
	Number, Heading, Body string
	Image                 ImageProps
}

// Worksheet gives a working form a ruled specification-sheet layout. Form
// controls are children; Reference and Summary accept ordinary kit components.
// It is an explicit page choice, independent of the current chrome.
type WorksheetProps struct {
	Heading            DisplayHeadingProps
	Number             string
	Reference, Summary templ.Component
	Attrs              datastar.Attrs
}

type SpecItem struct{ Label, Value string }
type SpecListProps struct {
	Items []SpecItem
	// Summary opts into a native, initially collapsed disclosure.
	Summary string
	Attrs   datastar.Attrs
}

func productLayoutClass(v ProductLayout) string {
	switch v {
	case ProductSplit:
		return "dw-product-split"
	case ProductExpansive:
		return "dw-product-stage"
	case ProductCover:
		return "dw-product-cover"
	case ProductPlate:
		return "dw-product-plate"
	case ProductTechnical:
		return "dw-product-technical"
	case ProductMonument:
		return "dw-product-monument"
	case ProductShowcase:
		return "dw-product-showcase"
	}
	return "dw-product-auto"
}

func editorialProduct(v ProductLayout) bool {
	return v == ProductCover || v == ProductPlate || v == ProductTechnical || v == ProductMonument || v == ProductShowcase
}

// EditorialSpread composes a heading, figure, numbered copy columns, and an
// action slot on an asymmetric publication grid. It never replaces Card/Grid.
type EditorialColumn struct{ Number, Heading, Body string }
type EditorialSpreadProps struct {
	Heading DisplayHeadingProps
	Image   ImageProps
	Caption string
	Columns []EditorialColumn
	Reverse bool
	Attrs   datastar.Attrs
}

// Publication opts a page into a full-width editorial frame. App Shell and
// Container retain their existing defaults for dashboards and conventional apps.
type PublicationProps struct {
	// Lighting adds a quiet, directional wash to the page surface.
	Lighting bool
	Attrs    datastar.Attrs
}

// NavigationLayout overrides the selected chrome's shell arrangement.
type NavigationLayout string

const (
	NavigationAuto NavigationLayout = ""
	NavigationSide NavigationLayout = "side"
	NavigationTop  NavigationLayout = "top"
)

func navigationClass(v NavigationLayout) string {
	switch v {
	case NavigationSide:
		return "dw-navigation-side"
	case NavigationTop:
		return "dw-navigation-top"
	}
	return "dw-navigation-auto"
}
