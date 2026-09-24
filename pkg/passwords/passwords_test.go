package passwords

import (
	"strings"
	"testing"
)

func TestHashRoundtrip(t *testing.T) {
	cases := []struct {
		name string
		pw   string
	}{
		{"simple", "correct horse"},
		{"unicode", "pásswörd-日本語-1234"},
		{"min length", "12345678"},
		{"long", strings.Repeat("a", 200)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			phc, err := Hash(tc.pw)
			if err != nil {
				t.Fatalf("Hash: %v", err)
			}
			if !IsArgonEncoded(phc) {
				t.Fatalf("hash not argon-encoded: %q", phc)
			}
			ok, err := Verify(tc.pw, phc)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if !ok {
				t.Fatalf("Verify returned false for correct password")
			}
		})
	}
}

func TestHashUniqueSalt(t *testing.T) {
	a, err := Hash("same-password-here")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Hash("same-password-here")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two hashes of the same password are identical; salt not random")
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	phc, err := Hash("the-right-password")
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"the-wrong-password",
		"the-right-passwor", // truncated
		"The-Right-Password",
		"",
	}
	for _, wrong := range cases {
		t.Run(wrong, func(t *testing.T) {
			ok, err := Verify(wrong, phc)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if ok {
				t.Fatalf("Verify accepted wrong password %q", wrong)
			}
		})
	}
}

func TestHashLengthValidation(t *testing.T) {
	cases := []struct {
		name    string
		pw      string
		wantErr error
	}{
		{"too short", "1234567", ErrPasswordTooShort},
		{"empty", "", ErrPasswordTooShort},
		{"exactly min", "12345678", nil},
		{"too long", strings.Repeat("x", maxLength+1), ErrPasswordTooLong},
		{"exactly max", strings.Repeat("x", maxLength), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Hash(tc.pw)
			if err != tc.wantErr {
				t.Fatalf("Hash err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestVerifyMalformed(t *testing.T) {
	cases := []struct {
		name string
		phc  string
	}{
		{"empty", ""},
		{"plain", "not-a-hash"},
		{"wrong algo", "$argon2i$v=19$m=131072,t=4,p=4$c2FsdA$aGFzaA"},
		{"missing fields", "$argon2id$v=19$m=131072,t=4,p=4"},
		{"bad params", "$argon2id$v=19$m=abc,t=4,p=4$c2FsdA$aGFzaA"},
		{"bad version", "$argon2id$v=99$m=131072,t=4,p=4$c2FsdA$aGFzaA"},
		{"bad base64 salt", "$argon2id$v=19$m=131072,t=4,p=4$!!!!$aGFzaA"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := Verify("anything", tc.phc)
			if err == nil {
				t.Fatalf("Verify(%q) expected error, got nil (ok=%v)", tc.phc, ok)
			}
			if ok {
				t.Fatalf("Verify(%q) returned ok=true on error", tc.phc)
			}
		})
	}
}

func TestNeedsRehash(t *testing.T) {
	current, err := Hash("rehash-me-please")
	if err != nil {
		t.Fatal(err)
	}
	if NeedsRehash(current) {
		t.Fatal("freshly-hashed password should not need rehash")
	}

	cases := []struct {
		name string
		phc  string
		want bool
	}{
		{"current params", current, false},
		{"weaker memory", "$argon2id$v=19$m=65536,t=4,p=4$" + sampleSaltKey(), true},
		{"weaker iterations", "$argon2id$v=19$m=131072,t=2,p=4$" + sampleSaltKey(), true},
		{"weaker parallelism", "$argon2id$v=19$m=131072,t=4,p=2$" + sampleSaltKey(), true},
		{"malformed", "garbage", true},
		{"stronger memory", "$argon2id$v=19$m=262144,t=4,p=4$" + sampleSaltKey(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NeedsRehash(tc.phc); got != tc.want {
				t.Fatalf("NeedsRehash = %v, want %v", got, tc.want)
			}
		})
	}
}

// sampleSaltKey returns a base64 salt$key pair of the default lengths
// (32-byte salt, 64-byte key) so synthetic PHC strings only differ in params.
func sampleSaltKey() string {
	phc, err := Hash("sample-for-saltkey")
	if err != nil {
		panic(err)
	}
	parts := strings.Split(phc, "$")
	return parts[4] + "$" + parts[5]
}

func TestIsArgonEncoded(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"$argon2id$v=19$m=131072,t=4,p=4$x$y", true},
		{"$argon2i$v=19$...", false},
		{"plaintext", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsArgonEncoded(tc.in); got != tc.want {
			t.Errorf("IsArgonEncoded(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestEncodeFormat(t *testing.T) {
	phc, err := Hash("format-check-password")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=131072,t=4,p=4$") {
		t.Fatalf("unexpected PHC prefix: %q", phc)
	}
}
