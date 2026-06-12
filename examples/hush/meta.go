package main

import (
	"net"
	"net/http"
	"strings"
)

// Meta is the request context captured with every post — admin eyes only. It
// rides the post row for moderation (who keeps posting this from where), but
// templates never render any of it; the wall stays anonymous.
type Meta struct {
	IP      string
	Country string // ISO code from Cloudflare's CF-IPCountry; empty in dev
	City    string // Cloudflare managed transforms (Cf-Ipcity); empty in dev
	UA      string
}

// uaCap keeps stored user agents bounded — past a couple hundred chars it's
// noise (or someone stuffing the header), not signal.
const uaCap = 256

// clientMeta extracts the poster's request context. Behind Cloudflare the
// CF-* headers are authoritative; otherwise the first X-Forwarded-For hop or
// the socket's own address is the best we have.
func clientMeta(r *http.Request) Meta {
	ip := r.Header.Get("CF-Connecting-IP")
	if ip == "" {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip, _, _ = strings.Cut(xff, ",")
			ip = strings.TrimSpace(ip)
		}
	}
	if ip == "" {
		ip = r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			ip = host
		}
	}
	ua := r.Header.Get("User-Agent")
	if len(ua) > uaCap {
		ua = ua[:uaCap]
	}
	return Meta{
		IP:      ip,
		Country: r.Header.Get("CF-IPCountry"),
		City:    r.Header.Get("Cf-Ipcity"),
		UA:      ua,
	}
}
