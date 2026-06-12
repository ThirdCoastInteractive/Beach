// hush's media wiring: which storage.Store holds files, and whether image
// posts route through Cloudflare Images. Both decisions are env-driven and
// optional — with nothing set, files live in a local temp dir served at
// /files and image posts are raw stored bytes, exactly the database-less
// developer experience. Production sets R2_* (durable file bytes) and
// CF_IMAGES_* (resized variants from imagedelivery.net) independently.
package main

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/ThirdCoastInteractive/the-beach/storage"
	"github.com/ThirdCoastInteractive/the-beach/storage/cfimages"
	"github.com/ThirdCoastInteractive/the-beach/storage/s3"
)

// initMedia wires a.files and a.images from the environment, once at boot.
//
// The file store is R2 when all four R2_* variables are set (PublicBaseURL
// from R2_PUBLIC_URL — unset means presigned links, which still work), Local
// under HUSH_FILES_DIR (default: OS temp) otherwise. Application photos and
// non-Cloudflare image posts flow through it either way.
//
// The Cloudflare Images client exists when the three CF_IMAGES_* variables
// are set; nil means image posts go through the file store. The variant
// names are dashboard-side config — CF_IMAGES_VARIANT is the full-size one
// feed cards render, CF_IMAGES_THUMB the square crop the grid tiles use.
func (a *app) initMedia() error {
	endpoint := os.Getenv("R2_ENDPOINT")
	bucket := os.Getenv("R2_BUCKET")
	keyID := os.Getenv("R2_ACCESS_KEY_ID")
	secret := os.Getenv("R2_SECRET_ACCESS_KEY")
	if endpoint != "" && bucket != "" && keyID != "" && secret != "" {
		files, err := s3.New(s3.Config{
			Endpoint:        endpoint,
			Bucket:          bucket,
			AccessKeyID:     keyID,
			SecretAccessKey: secret,
			PublicBaseURL:   os.Getenv("R2_PUBLIC_URL"),
		})
		if err != nil {
			return err
		}
		a.files = files
	} else {
		dir := os.Getenv("HUSH_FILES_DIR")
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "hush-files")
		}
		files, err := storage.NewLocal(dir, "/files")
		if err != nil {
			return err
		}
		a.files = files
	}

	if account, token, hash := os.Getenv("CF_IMAGES_ACCOUNT_ID"), os.Getenv("CF_IMAGES_TOKEN"), os.Getenv("CF_IMAGES_HASH"); account != "" && token != "" && hash != "" {
		a.images = cfimages.New(account, token, hash)
		a.imgVariant = envStr("CF_IMAGES_VARIANT", "public")
		a.imgThumb = envStr("CF_IMAGES_THUMB", "thumb")
	}
	return nil
}

// storeImage puts an uploaded post image where it belongs: Cloudflare Images
// when the client is wired (cfID comes back filled, mediaURL is the full
// variant's delivery URL — a stable fallback even if the client is later
// unwired), the file store otherwise (cfID stays "").
func (a *app) storeImage(ctx context.Context, r io.Reader, contentType string) (mediaURL, cfID string, err error) {
	if a.images != nil {
		id, err := a.images.Upload(ctx, r, randHex()+extFor(contentType))
		if err != nil {
			return "", "", err
		}
		return a.images.DeliveryURL(id, a.imgVariant), id, nil
	}
	key := "posts/" + randHex() + extFor(contentType)
	if _, err := a.files.Put(ctx, key, r, contentType); err != nil {
		return "", "", err
	}
	url, err := a.files.URL(ctx, key)
	return url, "", err
}

// postImageURL is a post's full-size image source (feed cards). For a
// Cloudflare-hosted image it's recomputed from the id at render time, so a
// variant rename in the env applies to old posts too; otherwise the stored
// URL stands.
func (a *app) postImageURL(p Post) string {
	if p.CFImageID != "" && a.images != nil {
		return a.images.DeliveryURL(p.CFImageID, a.imgVariant)
	}
	return p.ImageURL
}

// postThumbURL is a post's grid-tile source: the square-crop thumb variant
// (Cloudflare resizes server-side) when the image lives in Cloudflare
// Images, the full image otherwise (the tile's object-fit crops it locally).
func (a *app) postThumbURL(p Post) string {
	if p.CFImageID != "" && a.images != nil {
		return a.images.DeliveryURL(p.CFImageID, a.imgThumb)
	}
	return p.ImageURL
}
