//go:build vaultwarden && linux

package e2e

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // TOTP of RFC 6238 is defined over SHA-1.
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
)

// totpStepSeconds and totpKeyBytes are the values of the authenticator
// method of Vaultwarden: 30-second steps and a key of 20 random bytes.
const (
	totpStepSeconds = 30
	totpKeyBytes    = 20
)

// decodeTOTPKey decodes a key in the form the server returns: base32
// without padding. It refuses a key that does not decode to 20 bytes, as the
// server does when it activates the method.
func decodeTOTPKey(key string) ([]byte, error) {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(key))
	if err != nil {
		return nil, fmt.Errorf("totp key is not base32")
	}
	if len(raw) != totpKeyBytes {
		return nil, fmt.Errorf("totp key decodes to %d bytes, want %d", len(raw), totpKeyBytes)
	}
	return raw, nil
}

// totpCode returns the 6-digit code of a step: HMAC-SHA1 over the step as an
// 8-byte big-endian number, dynamic truncation as in RFC 4226, modulo 10^6.
// The secret and the code exist in memory only (design D1, D6).
func totpCode(secret []byte, step uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], step)
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1_000_000)
}
