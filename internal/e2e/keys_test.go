//go:build vaultwarden && linux

package e2e

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// The key derivation of the Bitwarden clients for a PBKDF2 account (design
// D4). This is test code that the program never links. An error in it
// makes the login of bw fail, which fails the test loudly.

const kdfIterations = 600_000

// symmetricKey is an AES-256-CBC key with an HMAC-SHA256 key.
type symmetricKey struct {
	enc, mac []byte
}

func splitKey(k []byte) symmetricKey {
	return symmetricKey{enc: k[:32], mac: k[32:64]}
}

// masterKey derives the master key: PBKDF2-SHA256 of the password with
// the trimmed lowercase address as salt.
func masterKey(password, email string, iterations int) ([]byte, error) {
	salt := strings.ToLower(strings.TrimSpace(email))
	return pbkdf2.Key(sha256.New, password, []byte(salt), iterations, 32)
}

// masterPasswordHash is one more PBKDF2 round of the master key with the
// password as salt, in base64. The server stores a hash of it.
func masterPasswordHash(master []byte, password string) (string, error) {
	h, err := pbkdf2.Key(sha256.New, string(master), []byte(password), 1, 32)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h), nil
}

// stretch expands the master key with HKDF-Expand and the infos enc and
// mac.
func stretch(master []byte) (symmetricKey, error) {
	enc, err := hkdf.Expand(sha256.New, master, "enc", 32)
	if err != nil {
		return symmetricKey{}, err
	}
	mac, err := hkdf.Expand(sha256.New, master, "mac", 32)
	if err != nil {
		return symmetricKey{}, err
	}
	return symmetricKey{enc: enc, mac: mac}, nil
}

// encString encrypts plaintext as an EncString of type 2: AES-256-CBC with
// PKCS#7 padding and HMAC-SHA256 over IV and ciphertext, in the form
// 2.<iv>|<ct>|<mac>.
func encString(k symmetricKey, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(k.enc)
	if err != nil {
		return "", err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	pad := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte{}, plaintext...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)
	m := hmac.New(sha256.New, k.mac)
	m.Write(iv)
	m.Write(ct)
	b64 := base64.StdEncoding.EncodeToString
	return "2." + b64(iv) + "|" + b64(ct) + "|" + b64(m.Sum(nil)), nil
}

// decryptEncString opens an EncString of type 2. It checks the MAC before
// it decrypts.
func decryptEncString(k symmetricKey, s string) ([]byte, error) {
	body, ok := strings.CutPrefix(s, "2.")
	if !ok {
		return nil, errors.New("not an EncString of type 2")
	}
	parts := strings.Split(body, "|")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%d parts, want 3", len(parts))
	}
	var raw [3][]byte
	for i, p := range parts {
		b, err := base64.StdEncoding.DecodeString(p)
		if err != nil {
			return nil, err
		}
		raw[i] = b
	}
	iv, ct, tag := raw[0], raw[1], raw[2]
	if len(iv) != aes.BlockSize || len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return nil, errors.New("malformed IV or ciphertext")
	}
	m := hmac.New(sha256.New, k.mac)
	m.Write(iv)
	m.Write(ct)
	if !hmac.Equal(m.Sum(nil), tag) {
		return nil, errors.New("MAC mismatch")
	}
	block, err := aes.NewCipher(k.enc)
	if err != nil {
		return nil, err
	}
	pt := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, ct)
	pad := int(pt[len(pt)-1])
	if pad == 0 || pad > aes.BlockSize || !bytes.Equal(pt[len(pt)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, errors.New("bad padding")
	}
	return pt[:len(pt)-pad], nil
}

// accountKeys is everything the registration sends, and the user key.
type accountKeys struct {
	PasswordHash        string // master password hash, base64
	WrappedUserKey      string // user key under the stretched master key
	PublicKey           string // SPKI DER, base64
	EncryptedPrivateKey string // PKCS#8 DER under the user key

	userKey symmetricKey
}

// deriveAccountKeys creates the keys of a new account with the given
// iteration count.
func deriveAccountKeys(email, password string, iterations int) (*accountKeys, error) {
	master, err := masterKey(password, email, iterations)
	if err != nil {
		return nil, err
	}
	hash, err := masterPasswordHash(master, password)
	if err != nil {
		return nil, err
	}
	stretched, err := stretch(master)
	if err != nil {
		return nil, err
	}
	user := make([]byte, 64)
	if _, err := rand.Read(user); err != nil {
		return nil, err
	}
	wrapped, err := encString(stretched, user)
	if err != nil {
		return nil, err
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	pub, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	if err != nil {
		return nil, err
	}
	priv, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		return nil, err
	}
	userKey := splitKey(user)
	wrappedPriv, err := encString(userKey, priv)
	if err != nil {
		return nil, err
	}
	return &accountKeys{
		PasswordHash:        hash,
		WrappedUserKey:      wrapped,
		PublicKey:           base64.StdEncoding.EncodeToString(pub),
		EncryptedPrivateKey: wrappedPriv,
		userKey:             userKey,
	}, nil
}
