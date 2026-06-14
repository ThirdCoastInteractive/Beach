// Package cfimages is a small client for Cloudflare Images — the hosted,
// paid product that stores originals and serves resized variants from
// imagedelivery.net. (Not the Image Resizing worker feature, which rewrites
// URLs on your own zone.)
//
// It covers exactly what an app needs to use the product: upload a file,
// delete it, and build a delivery URL. Cloudflare's API surface is much
// bigger (direct creator uploads, signed URLs, variant management); pieces
// get added here when a feature actually needs them.
//
//	images := cfimages.New(accountID, apiToken, accountHash)
//	id, err := images.Upload(ctx, file, "cat.png")
//	src := images.DeliveryURL(id, "public")
package cfimages

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

// apiBase is the Cloudflare v4 API root. Tests point base at an httptest
// server instead.
const apiBase = "https://api.cloudflare.com/client/v4"

// Client calls the Cloudflare Images API for one account.
type Client struct {
	AccountID   string // cloudflare account id (dashboard -> images -> overview)
	Token       string // api token with Cloudflare Images edit permission
	AccountHash string // delivery hash, the first path segment of imagedelivery.net urls

	// HTTP is the client requests go through. New sets http.DefaultClient;
	// swap it to add timeouts or to stub the transport in tests.
	HTTP *http.Client

	base string // api root override for tests; empty means apiBase
}

// New returns a ready client. accountID and token drive the API calls;
// accountHash only feeds DeliveryURL, so it may be empty if you never build
// delivery URLs.
func New(accountID, token, accountHash string) *Client {
	return &Client{AccountID: accountID, Token: token, AccountHash: accountHash, HTTP: http.DefaultClient}
}

// envelope is Cloudflare's uniform v4 response wrapper.
type envelope struct {
	Success bool `json:"success"`
	Result  struct {
		ID string `json:"id"`
	} `json:"result"`
	Errors []apiError `json:"errors"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// errs flattens the envelope's error list into one readable string.
func (e *envelope) errs() string {
	if len(e.Errors) == 0 {
		return "api reported failure with no errors"
	}
	parts := make([]string, len(e.Errors))
	for i, ae := range e.Errors {
		parts[i] = fmt.Sprintf("code %d: %s", ae.Code, ae.Message)
	}
	return strings.Join(parts, "; ")
}

// url builds an absolute API URL under this account.
func (c *Client) url(p string) string {
	base := c.base
	if base == "" {
		base = apiBase
	}
	return base + "/accounts/" + url.PathEscape(c.AccountID) + p
}

// do sends an authenticated request and decodes the envelope, turning both
// transport failures and success:false responses into errors.
func (c *Client) do(req *http.Request) (*envelope, error) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	httpc := c.HTTP
	if httpc == nil {
		httpc = http.DefaultClient
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Envelopes are tiny; the limit only guards against a misbehaving proxy
	// feeding us something enormous.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		// Not the JSON envelope at all (gateway error page, etc.) — surface
		// the status and a snippet so the failure is diagnosable from logs.
		s := string(body)
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		return nil, fmt.Errorf("status %d, non-json response: %s", resp.StatusCode, s)
	}
	if !env.Success {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, env.errs())
	}
	return &env, nil
}

// Upload stores an image and returns Cloudflare's id for it. filename is
// metadata only (it shows in the dashboard); the bytes come from r. Images
// are capped at 10MB by the product, so the multipart body is buffered in
// memory rather than streamed.
func (c *Client) Upload(ctx context.Context, r io.Reader, filename string) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("cfimages: upload: %w", err)
	}
	if _, err := io.Copy(fw, r); err != nil {
		return "", fmt.Errorf("cfimages: upload read: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("cfimages: upload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/images/v1"), &buf)
	if err != nil {
		return "", fmt.Errorf("cfimages: upload: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	env, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("cfimages: upload: %w", err)
	}
	if env.Result.ID == "" {
		return "", fmt.Errorf("cfimages: upload: success response without an id")
	}
	return env.Result.ID, nil
}

// Delete removes an uploaded image by id. Deleting an id that does not exist
// is an error — Cloudflare reports it and we pass that on.
func (c *Client) Delete(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.url("/images/v1/"+url.PathEscape(id)), nil)
	if err != nil {
		return fmt.Errorf("cfimages: delete: %w", err)
	}
	if _, err := c.do(req); err != nil {
		return fmt.Errorf("cfimages: delete %s: %w", id, err)
	}
	return nil
}

// DeliveryURL builds the public URL that serves an image through a named
// variant ("public" is the default variant every account starts with). Pure
// string assembly — no API call, safe to use in templates.
func (c *Client) DeliveryURL(id, variant string) string {
	return "https://imagedelivery.net/" + c.AccountHash + "/" + id + "/" + variant
}
