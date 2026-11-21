//go:build vaultwarden && linux

package e2e

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncStringForm(t *testing.T) {
	k := splitKey(randomBytes(t, 64))
	s, err := encString(k, []byte("synthetic plaintext"))
	if err != nil {
		t.Fatal(err)
	}
	body, ok := strings.CutPrefix(s, "2.")
	if !ok {
		t.Fatalf("no type 2 prefix: %.2s", s)
	}
	parts := strings.Split(body, "|")
	if len(parts) != 3 {
		t.Fatalf("%d parts, want 3", len(parts))
	}
	sizes := []int{16, 32, 32}
	for i, p := range parts {
		b, err := base64.StdEncoding.DecodeString(p)
		if err != nil {
			t.Fatalf("part %d: %v", i, err)
		}
		if len(b) != sizes[i] {
			t.Fatalf("part %d has %d bytes, want %d", i, len(b), sizes[i])
		}
	}
	got, err := decryptEncString(k, s)
	if err != nil || string(got) != "synthetic plaintext" {
		t.Fatalf("round trip: %q, %v", got, err)
	}
	wrong := symmetricKey{enc: k.enc, mac: randomBytes(t, 32)}
	if _, err := decryptEncString(wrong, s); err == nil || !strings.Contains(err.Error(), "MAC") {
		t.Fatalf("a wrong MAC key opened the EncString: %v", err)
	}
}

func TestAccountKeysDecryptWithDerivedKeys(t *testing.T) {
	const email, password = "e2e-unit@example.invalid", "a synthetic password"
	keys, err := deriveAccountKeys(email, password, 1000)
	if err != nil {
		t.Fatal(err)
	}
	master, err := masterKey(password, email, 1000)
	if err != nil {
		t.Fatal(err)
	}
	stretched, err := stretch(master)
	if err != nil {
		t.Fatal(err)
	}
	user, err := decryptEncString(stretched, keys.WrappedUserKey)
	if err != nil {
		t.Fatalf("user key: %v", err)
	}
	if len(user) != 64 || !bytes.Equal(user[:32], keys.userKey.enc) || !bytes.Equal(user[32:], keys.userKey.mac) {
		t.Fatal("the wrapped user key differs from the user key")
	}
	priv, err := decryptEncString(splitKey(user), keys.EncryptedPrivateKey)
	if err != nil {
		t.Fatalf("private key: %v", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok || rsaKey.N.BitLen() != 2048 {
		t.Fatal("not an RSA-2048 key")
	}
	pub, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if base64.StdEncoding.EncodeToString(pub) != keys.PublicKey {
		t.Fatal("the public key does not belong to the private key")
	}
	hash, err := masterPasswordHash(master, password)
	if err != nil || hash != keys.PasswordHash {
		t.Fatal("the master password hash differs")
	}
}

// The known-answer vectors are the unit tests of the Bitwarden SDK,
// https://github.com/bitwarden/sdk-internal at commit
// c5c08aebe8830f3c0fa54348b63f895137f2b827, files
// crates/bitwarden-crypto/src/keys/kdf.rs (test_master_key_derive_pbkdf2),
// crates/bitwarden-crypto/src/keys/utils.rs (test_stretch_kdf_key) and
// crates/bitwarden-crypto/src/keys/master_key.rs (test_password_hash_pbkdf2).

func TestKnownAnswerMasterKeyAndStretch(t *testing.T) {
	master, err := masterKey("67t9b5g67$%Dh89n", "test_key", 10000)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{31, 79, 104, 226, 150, 71, 177, 90, 194, 80, 172, 209, 17, 129, 132, 81, 138, 167,
		69, 167, 254, 149, 2, 27, 39, 197, 64, 42, 22, 195, 86, 75}
	if !bytes.Equal(master, want) {
		t.Fatalf("master key %v", master)
	}
	stretched, err := stretch(master)
	if err != nil {
		t.Fatal(err)
	}
	wantEnc := []byte{111, 31, 178, 45, 238, 152, 37, 114, 143, 215, 124, 83, 135, 173, 195, 23, 142,
		134, 120, 249, 61, 132, 163, 182, 113, 197, 189, 204, 188, 21, 237, 96}
	wantMac := []byte{221, 127, 206, 234, 101, 27, 202, 38, 86, 52, 34, 28, 78, 28, 185, 16, 48, 61, 127,
		166, 209, 247, 194, 87, 232, 26, 48, 85, 193, 249, 179, 155}
	if !bytes.Equal(stretched.enc, wantEnc) || !bytes.Equal(stretched.mac, wantMac) {
		t.Fatal("stretched key differs")
	}
}

func TestKnownAnswerMasterPasswordHash(t *testing.T) {
	const password = "asdfasdf"
	for _, salt := range []string{"test@bitwarden.com", "TEST@bitwarden.com", " test@bitwarden.com"} {
		master, err := masterKey(password, salt, 100_000)
		if err != nil {
			t.Fatal(err)
		}
		hash, err := masterPasswordHash(master, password)
		if err != nil {
			t.Fatal(err)
		}
		if hash != "wmyadRMyBZOH7P/a/ucTCbSghKgdzDpPqUnu/DAVtSw=" {
			t.Fatalf("salt %q: hash %s", salt, hash)
		}
	}
}

func randomBytes(t testing.TB, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}
