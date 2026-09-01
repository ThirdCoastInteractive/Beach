package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// pretokenTTL is the validity window of a login pretoken. GET /login embeds one
// as a hidden field; POST verifies it, killing form replays.
const pretokenTTL = 10 * time.Minute

// pretokenPurpose gates the HMAC so a pretoken minted for login cannot be reused
// for any other purpose-gated token.
const pretokenPurpose = "login"

// Pretoken is the issued login pretoken plus its expiry, embedded as a hidden
// form field on GET /login and verified on POST.
type Pretoken struct {
	Value     string
	ExpiresAt time.Time
}

// MintPretoken issues a purpose-gated HMAC-SHA256 pretoken valid for 10 minutes.
// The encoded form is "<expiryUnix>.<base64url(mac)>"; secret is the framework's
// signing key. now is injectable for tests.
func MintPretoken(secret []byte, now time.Time) Pretoken {
	exp := now.Add(pretokenTTL)
	expUnix := exp.Unix()
	mac := pretokenMAC(secret, expUnix)
	value := strconv.FormatInt(expUnix, 10) + "." + base64.RawURLEncoding.EncodeToString(mac)
	return Pretoken{Value: value, ExpiresAt: exp}
}

// VerifyPretoken reports whether value is a well-formed, unexpired pretoken
// signed by secret. It is constant-time on the MAC comparison. now is injectable
// for tests.
func VerifyPretoken(secret []byte, value string, now time.Time) bool {
	expStr, macB64, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}
	mac, err := base64.RawURLEncoding.DecodeString(macB64)
	if err != nil {
		return false
	}
	want := pretokenMAC(secret, expUnix)
	if !hmac.Equal(mac, want) {
		return false
	}
	// Valid signature; now enforce expiry.
	return now.Unix() <= expUnix
}

// pretokenMAC computes the purpose-gated HMAC over the expiry.
func pretokenMAC(secret []byte, expUnix int64) []byte {
	h := hmac.New(sha256.New, secret)
	fmt.Fprintf(h, "%s|%d", pretokenPurpose, expUnix)
	return h.Sum(nil)
}
