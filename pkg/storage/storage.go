// Package storage is the framework's unified file store: one Store interface,
// swappable backends. The default is the local filesystem ([Local]); the same
// interface is implemented over S3-compatible object stores — AWS S3,
// Cloudflare R2, and Google Cloud Storage — by the storage/s3 sub-package.
//
// Apps depend only on the Store interface, so a feature that stores avatars or
// post images works the same whether files live on a disk in development or in
// R2 in production. Swapping backends is a one-line wiring change.
package storage

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"time"
)

// ErrNotExist is returned by Open/Stat/Delete when the key has no object. It
// wraps fs.ErrNotExist so errors.Is(err, fs.ErrNotExist) also holds.
var ErrNotExist = fmt.Errorf("storage: object does not exist: %w", fs.ErrNotExist)

// File is an object's metadata.
type File struct {
	Key         string    // the object's key (a slash-separated path, no leading slash)
	Size        int64     // size in bytes
	ContentType string    // MIME type, e.g. "image/png"
	ModTime     time.Time // last modification time
}

// Store is a unified file store. Keys are slash-separated paths
// ("avatars/42.png"); a backend maps them to disk paths or object keys.
// Implementations must reject path traversal ("..") and absolute keys.
type Store interface {
	// Put stores the contents of r under key with the given content type,
	// overwriting any existing object, and returns the stored file's metadata.
	Put(ctx context.Context, key string, r io.Reader, contentType string) (File, error)

	// Open opens the object at key for reading. The caller closes the reader.
	// Returns ErrNotExist if key has no object.
	Open(ctx context.Context, key string) (io.ReadCloser, File, error)

	// Stat returns an object's metadata without its body. Returns ErrNotExist
	// if key has no object.
	Stat(ctx context.Context, key string) (File, error)

	// Delete removes the object at key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error

	// URL returns a URL a browser can fetch the object from: a local /files
	// route for [Local], a public or presigned URL for an object store.
	URL(ctx context.Context, key string) (string, error)
}
