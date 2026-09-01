package beach

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed view/static
var staticEmbedFS embed.FS

// embeddedStatic returns the embedded view/static tree, rooted so that
// "css/app.css" resolves directly (the embed.FS keeps the full path prefix).
func embeddedStatic() fs.FS {
	sub, err := fs.Sub(staticEmbedFS, "view/static")
	if err != nil {
		// The embed directive guarantees the path exists at build time.
		panic("beach: embedded static tree missing: " + err.Error())
	}
	return sub
}

// staticHandler serves a static FS with boot-time SHA256 ETags. Every file's
// ETag is computed once at construction; serving is then a map lookup plus a
// conditional-request check. Asset URLs carry a ?v=<version> query so each
// deploy busts caches without a manifest: a released binary uses a stable
// commit-based version, a dev build a boot timestamp. Versioned requests are
// served immutable for a year; bare requests (e.g. browser-issued ES module
// sub-imports, which don't carry ?v=) revalidate via ETag so a deploy's changed
// bytes are picked up rather than pinned by a stale immutable cache.
type staticHandler struct {
	fsys    fs.FS
	etags   map[string]string // clean path -> quoted ETag
	version string            // global asset version for ?v=
}

// overlayFS serves files from an ordered list of filesystems: the first layer
// that has a file wins for reads, and directory listings are the union of all
// layers (so fs.WalkDir over the overlay visits every file). It lets the App
// serve an app's static tree and the framework's own assets under one /static prefix.
type overlayFS struct{ layers []fs.FS }

func (o overlayFS) Open(name string) (fs.File, error) {
	for _, l := range o.layers {
		if f, err := l.Open(name); err == nil {
			return f, nil
		}
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (o overlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	seen := map[string]bool{}
	var out []fs.DirEntry
	var found bool
	for _, l := range o.layers {
		entries, err := fs.ReadDir(l, name)
		if err != nil {
			continue
		}
		found = true
		for _, e := range entries {
			if !seen[e.Name()] {
				seen[e.Name()] = true
				out = append(out, e)
			}
		}
	}
	if !found {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

// buildVersion is overridable at link time (-ldflags "-X ...buildVersion=abc123")
// so released binaries get stable commit-based asset versioning. Empty in dev.
var buildVersion string

// newStaticHandler walks fsys, computes an ETag per file, and picks the global
// asset version. release selects commit-based versioning (stable across restarts
// of the same binary) over a per-boot timestamp (unique per dev rebuild).
func newStaticHandler(fsys fs.FS, release bool) (*staticHandler, error) {
	sh := &staticHandler{
		fsys:  fsys,
		etags: map[string]string{},
	}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		sh.etags[path.Clean(p)] = `"` + hex.EncodeToString(sum[:16]) + `"`
		return nil
	})
	if err != nil {
		return nil, err
	}

	switch {
	case release && buildVersion != "":
		sh.version = buildVersion
	case release:
		sh.version = "rel"
	default:
		sh.version = strconv.FormatInt(time.Now().Unix(), 36)
	}
	return sh, nil
}

// assetURL builds the cache-busted public URL for an asset path. The version is
// the global one (a single deploy busts everything together); per-file ETags
// still give correct 304s within a version.
func (s *staticHandler) assetURL(p string) string {
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	return "/static/" + p + "?v=" + s.version
}

// ServeHTTP serves a single static file. The path has already been stripped of
// the /static/ prefix by the App's StripPrefix. Missing files are a plain 404;
// directory traversal is rejected by path.Clean plus the leading-dot guard.
func (s *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if name == "." || name == "" || strings.HasPrefix(name, "../") {
		http.NotFound(w, r)
		return
	}

	etag, ok := s.etags[name]
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("ETag", etag)
	// Cache policy hinges on the ?v= cache-buster. A request that carries a
	// version is content-addressed — its bytes never change for that URL — so it
	// may be cached for a year as immutable. A bare request must revalidate
	// instead: that is how the browser fetches ES module sub-imports (a module
	// doing import "/static/js/foo.js" does not inherit the importing module's
	// ?v=), and marking those immutable would pin a stale copy across a deploy.
	// Revalidation is cheap here — the boot-time ETag yields a 304 until the file
	// actually changes, at which point the next deploy's bytes are served.
	if r.URL.Query().Get("v") != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	if match := r.Header.Get("If-None-Match"); match != "" && etagMatch(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	b, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctype := contentTypeFor(name)
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}

// etagMatch reports whether the comma-separated If-None-Match header contains
// etag (or a wildcard).
func etagMatch(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || part == etag {
			return true
		}
	}
	return false
}

// contentTypeFor returns a Content-Type for a handful of asset extensions. The
// stdlib mime package covers most, but we pin the web-critical ones so a minimal
// container without a mime database still serves CSS/JS correctly.
func contentTypeFor(name string) string {
	switch {
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".mjs"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	default:
		return ""
	}
}

// compile-time assert staticHandler satisfies http.Handler.
var _ http.Handler = (*staticHandler)(nil)
