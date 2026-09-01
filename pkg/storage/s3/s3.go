// Package s3 is a [storage.Store] over any S3-compatible object store. It is
// aimed at Cloudflare R2 — endpoint, key pair, bucket, done — but the wire
// protocol is plain S3, so AWS S3 and Google Cloud Storage (interop mode)
// work with the same config shape. Built on minio-go, which is a pure client
// library: no SDK service catalog, no credential-chain magic.
//
//	store, err := s3.New(s3.Config{
//		Endpoint:        "accountid.r2.cloudflarestorage.com",
//		Bucket:          "files",
//		AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
//		SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
//		PublicBaseURL:   "https://files.example.com", // optional
//	})
//
// With PublicBaseURL set (a public bucket behind a custom domain), URL()
// is pure string concatenation. Without it, URL() presigns a GET valid for
// one hour — private buckets work out of the box, just with uglier links.
package s3

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/storage"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config is everything New needs. All fields except PublicBaseURL are
// required.
type Config struct {
	// Endpoint is the S3 API host, no scheme: for R2 that is
	// "<accountid>.r2.cloudflarestorage.com". Connections are always TLS;
	// there is no knob for plaintext on purpose.
	Endpoint string

	// Bucket is the bucket all keys live in. One Store per bucket.
	Bucket string

	AccessKeyID     string
	SecretAccessKey string

	// PublicBaseURL, when set, is the base public URL the bucket is served
	// from (e.g. "https://files.example.com" for an R2 custom domain) and
	// URL() becomes base + "/" + key. When empty, URL() returns a presigned
	// GET link valid for an hour.
	PublicBaseURL string
}

// S3 implements [storage.Store] over an S3-compatible bucket.
type S3 struct {
	client     *minio.Client
	bucket     string
	publicBase string
}

var _ storage.Store = (*S3)(nil)

// New validates cfg and returns a ready store. It does not dial: bad
// credentials or a missing bucket surface on the first call, not here.
func New(cfg Config) (*S3, error) {
	// R2's dashboard shows the endpoint with a scheme; be forgiving about
	// pasting it verbatim, but never downgrade to plaintext.
	endpoint := strings.TrimSuffix(strings.TrimPrefix(cfg.Endpoint, "https://"), "/")
	if endpoint == "" || strings.Contains(endpoint, "://") {
		return nil, fmt.Errorf("storage: s3 endpoint must be a host, got %q", cfg.Endpoint)
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("storage: s3 bucket is required")
	}
	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("storage: s3 credentials are required")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: true,
		// "auto" is R2's pseudo-region and GCS accepts it too. Pinning it
		// also skips minio's GetBucketLocation probe, which bucket-scoped R2
		// tokens are not allowed to make. AWS wants the bucket's real region;
		// grow a Region field the day someone points this at AWS.
		Region: "auto",
	})
	if err != nil {
		return nil, fmt.Errorf("storage: s3 client: %w", err)
	}
	return &S3{
		client:     client,
		bucket:     cfg.Bucket,
		publicBase: strings.TrimSuffix(cfg.PublicBaseURL, "/"),
	}, nil
}

// cleanKey validates and normalizes a key the same way [storage.Local] does:
// traversal is rejected loudly rather than silently rewritten, and the result
// has no leading slash. Key hygiene belongs to the Store contract, not to
// whichever backend happens to be wired in.
func cleanKey(key string) (string, error) {
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return "", fmt.Errorf("storage: invalid key %q: traversal", key)
		}
	}
	clean := strings.TrimPrefix(path.Clean("/"+key), "/")
	if clean == "" {
		return "", fmt.Errorf("storage: empty key")
	}
	return clean, nil
}

// contentTypeFor mirrors Local's default: explicit type wins, then the key's
// extension, then octet-stream. Unlike Local the chosen type is stored with
// the object, so reads return exactly what Put decided.
func contentTypeFor(key, given string) string {
	if given != "" {
		return given
	}
	if ct := mime.TypeByExtension(path.Ext(key)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// wrapErr maps minio's NoSuchKey responses to [storage.ErrNotExist] so
// errors.Is works the same across backends, and tags everything else with
// the failing operation.
func wrapErr(op string, err error) error {
	resp := minio.ToErrorResponse(err)
	if resp.Code == minio.NoSuchKey || resp.StatusCode == http.StatusNotFound {
		return storage.ErrNotExist
	}
	return fmt.Errorf("storage: %s: %w", op, err)
}

// fileOf converts minio object info to the Store's File, substituting our
// normalized key for minio's (they are equal, but ours is the contract).
func fileOf(key string, info minio.ObjectInfo) storage.File {
	return storage.File{Key: key, Size: info.Size, ContentType: info.ContentType, ModTime: info.LastModified}
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, contentType string) (storage.File, error) {
	k, err := cleanKey(key)
	if err != nil {
		return storage.File{}, err
	}
	ct := contentTypeFor(k, contentType)
	// Size -1 streams in multipart chunks (16MiB parts by default), so Put
	// accepts any reader without buffering the whole object in memory.
	info, err := s.client.PutObject(ctx, s.bucket, k, r, -1, minio.PutObjectOptions{ContentType: ct})
	if err != nil {
		return storage.File{}, fmt.Errorf("storage: put: %w", err)
	}
	return storage.File{Key: k, Size: info.Size, ContentType: ct, ModTime: time.Now()}, nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, storage.File, error) {
	k, err := cleanKey(key)
	if err != nil {
		return nil, storage.File{}, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, k, minio.GetObjectOptions{})
	if err != nil {
		return nil, storage.File{}, wrapErr("open", err)
	}
	// GetObject is lazy — the request fires on first read. Stat here so a
	// missing key is an Open error, not a surprise mid-read.
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, storage.File{}, wrapErr("open", err)
	}
	return obj, fileOf(k, info), nil
}

func (s *S3) Stat(ctx context.Context, key string) (storage.File, error) {
	k, err := cleanKey(key)
	if err != nil {
		return storage.File{}, err
	}
	info, err := s.client.StatObject(ctx, s.bucket, k, minio.StatObjectOptions{})
	if err != nil {
		return storage.File{}, wrapErr("stat", err)
	}
	return fileOf(k, info), nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	k, err := cleanKey(key)
	if err != nil {
		return err
	}
	// S3 DELETE is idempotent — a missing key already succeeds, matching the
	// Store contract without a pre-check.
	if err := s.client.RemoveObject(ctx, s.bucket, k, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("storage: delete: %w", err)
	}
	return nil
}

func (s *S3) URL(ctx context.Context, key string) (string, error) {
	k, err := cleanKey(key)
	if err != nil {
		return "", err
	}
	if s.publicBase != "" {
		return s.publicBase + "/" + k, nil
	}
	// Presigning is local computation (no round trip), but the link expires:
	// fine for redirects and <img> tags rendered per request, wrong for
	// anything persisted. Set PublicBaseURL for stable URLs.
	u, err := s.client.PresignedGetObject(ctx, s.bucket, k, time.Hour, nil)
	if err != nil {
		return "", fmt.Errorf("storage: presign: %w", err)
	}
	return u.String(), nil
}
