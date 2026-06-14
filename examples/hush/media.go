// hush's media wiring: the R2 object store that holds files, and the Cloudflare
// Images client image posts route through. Both are required (config.go tags the
// R2_*/CF_IMAGES_* credentials `required`), so there is no local-temp-dir or
// raw-bytes fallback — production media is the only media. R2 holds durable file
// bytes (application photos, the Cloudflare-Images delivery URL fallback);
// CF_IMAGES_* serves resized variants from imagedelivery.net.
package main

import (
	"context"
	"io"

	"github.com/ThirdCoastInteractive/the-beach/pkg/storage/cfimages"
	"github.com/ThirdCoastInteractive/the-beach/pkg/storage/s3"
)

// initMedia wires a.files (R2) and a.images (Cloudflare Images) from the
// validated config, once at boot. PublicBaseURL comes from R2_PUBLIC_URL — unset
// means presigned links, which still work. The variant names are dashboard-side
// config: CF_IMAGES_VARIANT is the full-size one feed cards render,
// CF_IMAGES_THUMB the square crop the grid tiles use.
func (a *app) initMedia() error {
	files, err := s3.New(s3.Config{
		Endpoint:        a.cfg.R2Endpoint,
		Bucket:          a.cfg.R2Bucket,
		AccessKeyID:     a.cfg.R2KeyID,
		SecretAccessKey: a.cfg.R2Secret,
		PublicBaseURL:   a.cfg.R2PublicURL,
	})
	if err != nil {
		return err
	}
	a.files = files

	a.images = cfimages.New(a.cfg.CFImagesAccount, a.cfg.CFImagesToken, a.cfg.CFImagesHash)
	a.imgVariant = a.cfg.CFImagesVariant
	a.imgThumb = a.cfg.CFImagesThumb
	return nil
}

// storeImage uploads a post image to Cloudflare Images: cfID comes back filled
// and mediaURL is the full variant's delivery URL — a stable fallback stored on
// the post even if the client is later unwired.
func (a *app) storeImage(ctx context.Context, r io.Reader, contentType string) (mediaURL, cfID string, err error) {
	id, err := a.images.Upload(ctx, r, randHex()+extFor(contentType))
	if err != nil {
		return "", "", err
	}
	return a.images.DeliveryURL(id, a.imgVariant), id, nil
}

// postImageURL is a post's full-size image source (feed cards). For a
// Cloudflare-hosted image it's recomputed from the id at render time, so a
// variant rename in the env applies to old posts too; otherwise the stored
// URL stands.
func (a *app) postImageURL(p Post) string {
	if p.CFImageID != "" {
		return a.images.DeliveryURL(p.CFImageID, a.imgVariant)
	}
	return p.ImageURL
}

// postThumbURL is a post's grid-tile source: the square-crop thumb variant
// (Cloudflare resizes server-side) when the image lives in Cloudflare Images,
// the stored URL otherwise.
func (a *app) postThumbURL(p Post) string {
	if p.CFImageID != "" {
		return a.images.DeliveryURL(p.CFImageID, a.imgThumb)
	}
	return p.ImageURL
}
