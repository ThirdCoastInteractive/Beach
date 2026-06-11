package driftwood

import (
	"embed"
	"io/fs"
)

// Static holds the kit's own assets — its stylesheet at css/driftwood.css. An
// app mounts this under /static (alongside the generated stellar.css) so the
// document Page renders has both sheets. The kit sheet references only Stellar
// tokens; it carries no color literals of its own.
//
//go:embed static/css/driftwood.css
var Static embed.FS

// Assets implements ui.AssetKit: the App serves these under /static
// automatically, so css/driftwood.css is reachable at /static/css/driftwood.css
// without the app wiring it. Rooted so paths line up with the /static prefix.
func (Kit) Assets() fs.FS {
	sub, err := fs.Sub(Static, "static")
	if err != nil {
		return Static // embed.FS is always valid; this never happens
	}
	return sub
}
