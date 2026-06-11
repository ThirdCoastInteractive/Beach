// Package page stubs the templ component constructors the generated handlers
// reference via @page / @fragment. Only the signatures matter for the compile
// check.
package page

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

func comp() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error { return nil })
}

func ItemDetail(row any) templ.Component { return comp() }
func ItemCard(row any) templ.Component   { return comp() }

// ItemList takes no row: it is the @fragment of a :exec DELETE, which returns no
// value, so the generated handler calls it with no arguments.
func ItemList() templ.Component { return comp() }
