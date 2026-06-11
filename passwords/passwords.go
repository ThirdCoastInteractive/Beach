// Package passwords provides argon2id password hashing in the PHC string
// format, built directly on golang.org/x/crypto/argon2 with zero dependencies
// beyond x/crypto.
//
// A hash is stored as a PHC-encoded string:
//
//	$argon2id$v=19$m=131072,t=4,p=4$<base64 salt>$<base64 key>
//
// The parameters are deliberately heavy (128 MB, 4 iterations, 4 lanes) because
// login is infrequent. Compare/Verify run in constant time, and NeedsRehash
// reports when a stored hash used weaker parameters so the login handler can
// transparently rehash on a successful login (parameter upgrades roll out one
// login at a time).
package passwords

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params holds the argon2id cost parameters and salt/key sizes.
type Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32 // bytes
	KeyLength   uint32 // bytes
}

// defaultParams are the framework defaults: 128 MB, 4 iterations, 4 lanes,
// 256-bit salt, 512-bit key.
var defaultParams = Params{
	Memory:      128 * 1024,
	Iterations:  4,
	Parallelism: 4,
	SaltLength:  32,
	KeyLength:   64,
}

// argon2Version is the version encoded in the PHC string (0x13 == 19).
const argon2Version = argon2.Version

// Length bounds for plaintext passwords.
const (
	minLength = 8
	maxLength = 512
)

// Errors returned by this package.
var (
	ErrPasswordTooShort = fmt.Errorf("password too short: minimum %d bytes", minLength)
	ErrPasswordTooLong  = fmt.Errorf("password too long: maximum %d bytes", maxLength)
	ErrInvalidHash      = errors.New("argon2id: invalid PHC hash format")
	ErrIncompatible     = errors.New("argon2id: incompatible algorithm or version")
)

// Hash hashes plaintext with the default parameters and returns a PHC-encoded
// string. It validates the plaintext length (8..512 bytes).
func Hash(plaintext string) (string, error) {
	return hashWith(plaintext, defaultParams)
}

func hashWith(plaintext string, p Params) (string, error) {
	if len(plaintext) < minLength {
		return "", ErrPasswordTooShort
	}
	if len(plaintext) > maxLength {
		return "", ErrPasswordTooLong
	}

	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2id: read salt: %w", err)
	}

	key := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	return encode(p, salt, key), nil
}

// encode renders the PHC string for the given params, salt and key.
func encode(p Params, salt, key []byte) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		p.Memory, p.Iterations, p.Parallelism,
		b64.EncodeToString(salt),
		b64.EncodeToString(key),
	)
}

// Verify reports whether plaintext matches the PHC-encoded hash, in constant
// time. A mismatch returns (false, nil); a malformed hash returns an error.
func Verify(plaintext, phc string) (bool, error) {
	p, salt, key, err := decode(phc)
	if err != nil {
		return false, err
	}

	other := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, uint32(len(key)))
	if subtle.ConstantTimeEq(int32(len(other)), int32(len(key))) == 0 {
		return false, nil
	}
	return subtle.ConstantTimeCompare(other, key) == 1, nil
}

// NeedsRehash reports whether the PHC-encoded hash used parameters weaker than
// the current defaults and should be re-hashed on the next successful login.
// A malformed hash returns true (it should be replaced).
func NeedsRehash(phc string) bool {
	p, salt, key, err := decode(phc)
	if err != nil {
		return true
	}
	return p.Memory < defaultParams.Memory ||
		p.Iterations < defaultParams.Iterations ||
		p.Parallelism < defaultParams.Parallelism ||
		uint32(len(salt)) < defaultParams.SaltLength ||
		uint32(len(key)) < defaultParams.KeyLength
}

// IsArgonEncoded reports whether input looks like an argon2id PHC string.
func IsArgonEncoded(input string) bool {
	return strings.HasPrefix(input, "$argon2id$")
}

// decode parses a PHC-encoded argon2id hash into its parameters, salt and key.
func decode(phc string) (Params, []byte, []byte, error) {
	parts := strings.Split(phc, "$")
	// "" / argon2id / v=19 / m=..,t=..,p=.. / salt / key
	if len(parts) != 6 || parts[0] != "" {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrIncompatible
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if version != argon2Version {
		return Params{}, nil, nil, ErrIncompatible
	}

	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}

	p.SaltLength = uint32(len(salt))
	p.KeyLength = uint32(len(key))
	return p, salt, key, nil
}
