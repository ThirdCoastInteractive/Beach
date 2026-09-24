package storage

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Local is a Store backed by a filesystem directory. It is the default backend:
// zero config, nothing to provision. It also serves its own files — mount it as
// an http.Handler under PublicPrefix so URL() links resolve.
//
//	fs, _ := storage.NewLocal("uploads", "/files")
//	app.Raw(http.MethodGet, "/files/", http.StripPrefix("/files/", fs))
//	url, _ := fs.URL(ctx, "avatars/42.png") // -> "/files/avatars/42.png"
//
// Content type is derived from the key's extension on read (the grug default for
// a disk store: no sidecar metadata files). Object-store backends keep the
// content type you Put.
type Local struct {
	root   string // absolute base directory
	prefix string // public URL prefix, e.g. "/files"
}

// NewLocal returns a Local store rooted at dir (created if missing) whose URLs
// are prefixed with publicPrefix (e.g. "/files").
func NewLocal(dir, publicPrefix string) (*Local, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("storage: local root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("storage: local mkdir: %w", err)
	}
	return &Local{root: abs, prefix: "/" + strings.Trim(publicPrefix, "/")}, nil
}

// cleanKey validates a key and maps it to a filesystem path inside root,
// rejecting traversal and absolute keys.
func (l *Local) cleanKey(key string) (string, error) {
	clean := path.Clean("/" + key) // forces a rooted path
	if clean == "/" {
		return "", fmt.Errorf("storage: empty key")
	}
	// Reject traversal outright rather than silently rewriting it.
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return "", fmt.Errorf("storage: invalid key %q: traversal", key)
		}
	}
	return filepath.Join(l.root, filepath.FromSlash(clean)), nil
}

func contentTypeFor(key, given string) string {
	if given != "" {
		return given
	}
	if ct := mime.TypeByExtension(path.Ext(key)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func (l *Local) Put(ctx context.Context, key string, r io.Reader, contentType string) (File, error) {
	full, err := l.cleanKey(key)
	if err != nil {
		return File{}, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return File{}, fmt.Errorf("storage: put mkdir: %w", err)
	}
	f, err := os.Create(full)
	if err != nil {
		return File{}, fmt.Errorf("storage: put create: %w", err)
	}
	n, err := io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(full) // don't leave a partial object
		return File{}, fmt.Errorf("storage: put write: %w", err)
	}
	return File{Key: keyOf(key), Size: n, ContentType: contentTypeFor(key, contentType), ModTime: time.Now()}, nil
}

func (l *Local) Open(ctx context.Context, key string) (io.ReadCloser, File, error) {
	full, err := l.cleanKey(key)
	if err != nil {
		return nil, File{}, err
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, File{}, ErrNotExist
		}
		return nil, File{}, fmt.Errorf("storage: open: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, File{}, fmt.Errorf("storage: open stat: %w", err)
	}
	return f, File{Key: keyOf(key), Size: info.Size(), ContentType: contentTypeFor(key, ""), ModTime: info.ModTime()}, nil
}

func (l *Local) Stat(ctx context.Context, key string) (File, error) {
	full, err := l.cleanKey(key)
	if err != nil {
		return File{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, ErrNotExist
		}
		return File{}, fmt.Errorf("storage: stat: %w", err)
	}
	return File{Key: keyOf(key), Size: info.Size(), ContentType: contentTypeFor(key, ""), ModTime: info.ModTime()}, nil
}

func (l *Local) Delete(ctx context.Context, key string) error {
	full, err := l.cleanKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete: %w", err)
	}
	return nil
}

func (l *Local) URL(ctx context.Context, key string) (string, error) {
	return l.prefix + "/" + keyOf(key), nil
}

// ServeHTTP serves objects from the store. Mount it under PublicPrefix with the
// prefix stripped: it expects the key in r.URL.Path. It sets Content-Type from
// the extension and a long immutable cache (content-addressed keys are the
// convention), and rejects traversal.
func (l *Local) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")
	rc, info, err := l.Open(r.Context(), key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", info.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	f, ok := rc.(io.ReadSeeker)
	if ok {
		http.ServeContent(w, r, key, info.ModTime, f)
		return
	}
	_, _ = io.Copy(w, rc)
}

// keyOf normalizes a key to a clean, leading-slash-free path for File.Key.
func keyOf(key string) string { return strings.TrimPrefix(path.Clean("/"+key), "/") }
