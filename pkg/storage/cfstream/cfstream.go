// Package cfstream is a small client for Cloudflare Stream — the hosted
// video product that accepts tus resumable uploads and serves playback from
// cloudflarestream.com / videodelivery.net.
//
// It covers exactly what an app needs to use the product: create a
// creator-scoped tus upload (the browser PATCHes the file; the API token
// never leaves the server), delete a video, and build iframe / thumbnail
// URLs. Cloudflare's API surface is much bigger (live inputs, signing keys,
// analytics); pieces get added here when a feature actually needs them.
//
//	stream := cfstream.New(accountID, apiToken, customerCode)
//	tus, err := stream.CreateTus(ctx, fileSize, map[string]string{"name": "clip"})
//	iframe := stream.IframeURL(tus.UID)
package cfstream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// apiBase is the Cloudflare v4 API root. Tests point base at an httptest
// server instead.
const apiBase = "https://api.cloudflare.com/client/v4"

// tusResumable is the tus protocol version Cloudflare Stream speaks.
const tusResumable = "1.0.0"

// streamMediaIDHeader is the response header Cloudflare returns the new
// video's id in on a tus creation request. Preferred over parsing Location.
const streamMediaIDHeader = "stream-media-id"

// Client calls the Cloudflare Stream API for one account.
type Client struct {
	AccountID    string // cloudflare account id
	Token        string // api token with Cloudflare Stream edit permission
	CustomerCode string // subdomain code for customer-*.cloudflarestream.com iframe urls

	// HTTP is the client requests go through. New sets http.DefaultClient;
	// swap it to add timeouts or to stub the transport in tests.
	HTTP *http.Client

	base string // api root override for tests; empty means apiBase
}

// New returns a ready client. accountID and token drive the API calls;
// customerCode only feeds IframeURL, so it may be empty if you never build
// iframe URLs.
func New(accountID, token, customerCode string) *Client {
	return &Client{AccountID: accountID, Token: token, CustomerCode: customerCode, HTTP: http.DefaultClient}
}

// TusUpload is a freshly created Cloudflare Stream resumable upload.
type TusUpload struct {
	// URL is the Location header exactly as Cloudflare returned it, query
	// string included. Cloudflare's tus v2 Location carries "?tusv2=true";
	// stripping it (or rebuilding the URL from UID) yields a different
	// resource that cannot be resumed. Store and hand out this string
	// unmodified.
	URL string
	// UID is the Stream video id, taken from the stream-media-id response
	// header. Cloudflare recommends this over parsing Location. When the
	// header is absent, the last path segment of Location is used instead
	// (query string stripped so "?tusv2=true" never becomes part of the id).
	UID string
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

func (c *Client) httpc() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// do sends an authenticated request and decodes the envelope, turning both
// transport failures and success:false responses into errors.
func (c *Client) do(req *http.Request) (*envelope, error) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.httpc().Do(req)
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

// CreateTus creates a resumable Stream upload of exactly uploadLength bytes
// and returns the creator-scoped tus URL to hand to the browser.
//
// The creation POST is account-authenticated and happens server-side so the
// API token never reaches a browser. What comes back — the Location header
// — is a creator-scoped upload URL the browser can PATCH to directly.
//
// uploadLength must be the real file size: Cloudflare holds the upload to
// it. metadata becomes the Upload-Metadata header (tus "key base64(value)"
// pairs). Cloudflare special-cases a documented set of keys (name,
// maxDurationSeconds, …); spelling matters or they silently degrade into
// display-only meta.
func (c *Client) CreateTus(ctx context.Context, uploadLength int64, metadata map[string]string) (*TusUpload, error) {
	if uploadLength <= 0 {
		return nil, fmt.Errorf("cfstream: tus upload length must be positive, got %d", uploadLength)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/stream?direct_user=true"), nil)
	if err != nil {
		return nil, fmt.Errorf("cfstream: tus create: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Tus-Resumable", tusResumable)
	req.Header.Set("Upload-Length", strconv.FormatInt(uploadLength, 10))
	if encoded := encodeTusMetadata(metadata); encoded != "" {
		req.Header.Set("Upload-Metadata", encoded)
	}

	resp, err := c.httpc().Do(req)
	if err != nil {
		return nil, fmt.Errorf("cfstream: tus create: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		s := strings.TrimSpace(string(body))
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		return nil, fmt.Errorf("cfstream: tus create returned %d: %s", resp.StatusCode, s)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return nil, fmt.Errorf("cfstream: tus create returned no Location header")
	}

	uid := resp.Header.Get(streamMediaIDHeader)
	if uid == "" {
		uid = uidFromTusLocation(location)
	}
	if uid == "" {
		return nil, fmt.Errorf("cfstream: tus create returned no usable video id")
	}

	return &TusUpload{URL: location, UID: uid}, nil
}

// Delete removes a Stream video by uid. Deleting an id that does not exist
// is an error — Cloudflare reports it and we pass that on.
func (c *Client) Delete(ctx context.Context, uid string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.url("/stream/"+url.PathEscape(uid)), nil)
	if err != nil {
		return fmt.Errorf("cfstream: delete: %w", err)
	}
	if _, err := c.do(req); err != nil {
		return fmt.Errorf("cfstream: delete %s: %w", uid, err)
	}
	return nil
}

// IframeURL builds the public embed URL for a video. Pure string assembly —
// no API call, safe to use in templates.
func (c *Client) IframeURL(uid string) string {
	return "https://customer-" + c.CustomerCode + ".cloudflarestream.com/" + uid + "/iframe"
}

// ThumbnailURL builds Cloudflare's default still for a video. Pure string
// assembly — no API call.
func (c *Client) ThumbnailURL(uid string) string {
	return "https://videodelivery.net/" + uid + "/thumbnails/thumbnail.jpg"
}

// encodeTusMetadata renders an Upload-Metadata header value: comma-separated
// "key base64(value)" pairs, or a bare key for a valueless flag. Keys are
// emitted in sorted order so the header is stable across calls.
func encodeTusMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		if v := metadata[k]; v == "" {
			pairs = append(pairs, k)
		} else {
			pairs = append(pairs, k+" "+base64.StdEncoding.EncodeToString([]byte(v)))
		}
	}
	return strings.Join(pairs, ",")
}

// uidFromTusLocation extracts the Stream video uid from a tus Location URL.
// The query string is dropped first: Cloudflare's tus v2 Location ends in
// "?tusv2=true", and taking the last path segment without stripping it
// yields "<uid>?tusv2=true", which makes every playback and thumbnail URL 404.
func uidFromTusLocation(location string) string {
	if i := strings.IndexByte(location, '?'); i >= 0 {
		location = location[:i]
	}
	location = strings.TrimSuffix(location, "/")
	if i := strings.LastIndexByte(location, '/'); i >= 0 {
		return location[i+1:]
	}
	return location
}
