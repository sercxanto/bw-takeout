//go:build vaultwarden && linux

package e2e

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// TestTOTPKnownAnswers checks totpCode against the SHA-1 vectors of RFC 6238,
// Appendix B (secret "12345678901234567890"). The RFC lists 8 digits. A
// 6-digit code is the same truncated number modulo 10^6, so the expected
// values are the last 6 digits of the RFC values.
func TestTOTPKnownAnswers(t *testing.T) {
	secret := []byte("12345678901234567890")
	vectors := []struct {
		unixTime uint64
		rfc8     string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, v := range vectors {
		want := v.rfc8[2:]
		if got := totpCode(secret, v.unixTime/totpStepSeconds); got != want {
			t.Errorf("T=%d: code %s, want %s (RFC 6238, Appendix B: %s)", v.unixTime, got, want, v.rfc8)
		}
	}
}

// TestTOTPKeyDecoding checks the key form of the server: 32 base32
// characters without padding decode to 20 bytes, any other length is refused.
func TestTOTPKeyDecoding(t *testing.T) {
	raw := []byte("12345678901234567890")
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)

	key := enc.EncodeToString(raw)
	if len(key) != 32 {
		t.Fatalf("test key has %d characters, want 32", len(key))
	}
	got, err := decodeTOTPKey(key)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("decoded key differs from the original")
	}
	if _, err := decodeTOTPKey(strings.ToLower(key)); err != nil {
		t.Errorf("lowercase key refused: %v", err)
	}

	for _, n := range []int{10, 19, 21, 32} {
		other := enc.EncodeToString(make([]byte, n))
		if _, err := decodeTOTPKey(other); err == nil {
			t.Errorf("key of %d bytes accepted", n)
		}
	}
	if _, err := decodeTOTPKey("not base32 !"); err == nil {
		t.Errorf("non-base32 key accepted")
	}
}

// TestNextCodeFollowsReplayRule checks design D3 with a fixed clock: in the
// step of the activation the next step is typed, in a later step the
// current one, and two calls in the same step give two different,
// increasing steps.
func TestNextCodeFollowsReplayRule(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1_000_000_000, 0)
	step := totpStep(now)
	a := &account{totpSecret: secret, lastStep: step, now: func() time.Time { return now }}

	// The step of the activation: the code of the next step.
	got, err := a.nextCode()
	if err != nil {
		t.Fatal(err)
	}
	if want := totpCode(secret, step+1); got != want || a.lastStep != step+1 {
		t.Errorf("in the step of the activation: step %d typed, want %d", a.lastStep, step+1)
	}
	// The same step again: a different, later step.
	if _, err := a.nextCode(); err != nil {
		t.Fatal(err)
	}
	if a.lastStep != step+2 {
		t.Errorf("second call in the same step: step %d, want %d", a.lastStep, step+2)
	}

	// A later clock step beyond the last used step: the current step.
	now = now.Add(10 * totpStepSeconds * time.Second)
	got, err = a.nextCode()
	if err != nil {
		t.Fatal(err)
	}
	if want := totpCode(secret, step+10); got != want || a.lastStep != step+10 {
		t.Errorf("in a later step: step %d typed, want the current step %d", a.lastStep, step+10)
	}

	if _, err := (&account{}).nextCode(); err == nil {
		t.Error("nextCode without two-step login did not fail")
	}
}

// TestWrongCodeIsWrongByConstruction checks that the wrong code differs from
// the codes of the steps the server accepts and from the last used step,
// for many clock positions.
func TestWrongCodeIsWrongByConstruction(t *testing.T) {
	secret := []byte("12345678901234567890")
	for offset := uint64(0); offset < 2000; offset++ {
		now := time.Unix(int64(1_000_000_000+offset*totpStepSeconds), 0)
		a := &account{totpSecret: secret, lastStep: totpStep(now) + 1, now: func() time.Time { return now }}
		wrong := a.wrongCode()
		for step := totpStep(now) - 1; step <= totpStep(now)+1; step++ {
			if wrong == totpCode(secret, step) {
				t.Fatalf("offset %d: the wrong code equals the code of the accepted step %+d", offset, int64(step)-int64(totpStep(now)))
			}
		}
		if wrong == totpCode(secret, a.lastStep) {
			t.Fatalf("offset %d: the wrong code equals the code of the last used step", offset)
		}
	}
}
