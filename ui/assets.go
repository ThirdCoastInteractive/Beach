package ui

import "io/fs"

// AssetKit is an optional Kit capability. A kit that ships its own static
// assets — a stylesheet, fonts, an icon sprite — implements Assets, and the App
// serves them under /static automatically (overlaid on the app's own static
// tree). A kit without assets simply doesn't implement this interface.
//
// The returned FS is rooted at the /static prefix: a file at "css/driftwood.css"
// is served as /static/css/driftwood.css.
type AssetKit interface {
	Assets() fs.FS
}
