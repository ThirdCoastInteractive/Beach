// Package media is hush's media wiring: the R2 object store that holds files,
// and the Cloudflare Images client image posts route through. Both are required
// (the web config tags the R2_*/CF_IMAGES_* credentials `required`), so there is
// no local-temp-dir or raw-bytes fallback — production media is the only media.
// R2 holds durable file bytes (application photos, the Cloudflare-Images
// delivery URL fallback); CF_IMAGES_* serves resized variants from
// imagedelivery.net.
package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"

	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/store"
	"github.com/ThirdCoastInteractive/the-beach/pkg/storage"
	"github.com/ThirdCoastInteractive/the-beach/pkg/storage/cfimages"
	"github.com/ThirdCoastInteractive/the-beach/pkg/storage/s3"
)

// Config carries the validated R2 + Cloudflare Images credentials and variant
// names from the web layer's env config.
type Config struct {
	R2Endpoint  string
	R2Bucket    string
	R2KeyID     string
	R2Secret    string
	R2PublicURL string

	CFImagesAccount string
	CFImagesToken   string
	CFImagesHash    string
	CFImagesVariant string
	CFImagesThumb   string
}

// Media wires the R2 object store and the Cloudflare Images client. Files is
// exported because application photos (apply.go) store and resolve keys through
// the same R2 store.
type Media struct {
	Files   storage.Store    // R2 object store (post images, application photos)
	images  *cfimages.Client // Cloudflare Images — image posts route through it
	variant string           // full-size variant feed cards render
	thumb   string           // square-crop thumb variant grid tiles use
}

// New wires Files (R2) and the Cloudflare Images client from the validated
// config. PublicBaseURL comes from R2_PUBLIC_URL — unset means presigned links,
// which still work. The variant names are dashboard-side config:
// CF_IMAGES_VARIANT is the full-size one feed cards render, CF_IMAGES_THUMB the
// square crop the grid tiles use.
func New(cfg Config) (*Media, error) {
	files, err := s3.New(s3.Config{
		Endpoint:        cfg.R2Endpoint,
		Bucket:          cfg.R2Bucket,
		AccessKeyID:     cfg.R2KeyID,
		SecretAccessKey: cfg.R2Secret,
		PublicBaseURL:   cfg.R2PublicURL,
	})
	if err != nil {
		return nil, err
	}
	return &Media{
		Files:   files,
		images:  cfimages.New(cfg.CFImagesAccount, cfg.CFImagesToken, cfg.CFImagesHash),
		variant: cfg.CFImagesVariant,
		thumb:   cfg.CFImagesThumb,
	}, nil
}

// StoreImage uploads a post image to Cloudflare Images: cfID comes back filled
// and mediaURL is the full variant's delivery URL — a stable fallback stored on
// the post even if the client is later unwired.
func (m *Media) StoreImage(ctx context.Context, r io.Reader, contentType string) (mediaURL, cfID string, err error) {
	id, err := m.images.Upload(ctx, r, RandHex()+ExtFor(contentType))
	if err != nil {
		return "", "", err
	}
	return m.images.DeliveryURL(id, m.variant), id, nil
}

// PostImageURL is a post's full-size image source (feed cards). For a
// Cloudflare-hosted image it's recomputed from the id at render time, so a
// variant rename in the env applies to old posts too; otherwise the stored
// URL stands.
func (m *Media) PostImageURL(p store.Post) string {
	if p.CFImageID != "" {
		return m.images.DeliveryURL(p.CFImageID, m.variant)
	}
	return p.ImageURL
}

// PostThumbURL is a post's grid-tile source: the square-crop thumb variant
// (Cloudflare resizes server-side) when the image lives in Cloudflare Images,
// the stored URL otherwise.
func (m *Media) PostThumbURL(p store.Post) string {
	if p.CFImageID != "" {
		return m.images.DeliveryURL(p.CFImageID, m.thumb)
	}
	return p.ImageURL
}

// FullURL is a Cloudflare Images delivery URL for the full-size variant.
func (m *Media) FullURL(cfID string) string {
	return m.images.DeliveryURL(cfID, m.variant)
}

// ThumbURL is a Cloudflare Images delivery URL for the square-crop variant.
func (m *Media) ThumbURL(cfID string) string {
	return m.images.DeliveryURL(cfID, m.thumb)
}

// RandHex returns 8 random bytes as hex, a collision-free object key stem.
func RandHex() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ExtFor maps an image content type to a file extension.
func ExtFor(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".bin"
	}
}
