package bwcli

import (
	"bytes"
	"errors"
)

// The sentences of the 2026.10.1 bundle that select a failure class (design
// D7). A sentence that changes in a later version falls back to ClassExit.
var sentences = map[string]Class{
	"You are not logged in.":                          ClassNotLoggedIn,
	"Vault is locked.":                                ClassLocked,
	"Premium status is required to use this feature.": ClassPremiumRequired,
}

// decryptionMarker starts each decryption error line of the bundled SDK.
var decryptionMarker = []byte("ERROR bitwarden_crypto::")

// maxStderrLine bounds what is held of one line of standard error. The
// rest of a longer line is dropped.
const maxStderrLine = 4096

// stderrScanner reads standard error as a stream. It counts decryption
// error lines and remembers the class of the first recognized sentence.
// It keeps no other text: each line is cleared once it is judged.
type stderrScanner struct {
	line             []byte
	decryptionErrors int
	recognized       Class
}

func (s *stderrScanner) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		chunk := p
		if i >= 0 {
			chunk = p[:i]
		}
		if room := maxStderrLine - len(s.line); room > 0 {
			if len(chunk) > room {
				chunk = chunk[:room]
			}
			s.line = append(s.line, chunk...)
		}
		if i < 0 {
			break
		}
		s.endLine()
		p = p[i+1:]
	}
	return n, nil
}

// flush judges a last line without a line ending.
func (s *stderrScanner) flush() {
	if len(s.line) > 0 {
		s.endLine()
	}
}

func (s *stderrScanner) endLine() {
	text := stripANSI(s.line)
	if bytes.Contains(text, decryptionMarker) {
		s.decryptionErrors++
	}
	if s.recognized == "" {
		if c, ok := sentences[string(bytes.TrimSpace(text))]; ok {
			s.recognized = c
		}
	}
	clear(text)
	clear(s.line)
	s.line = s.line[:0]
}

// stripANSI returns a copy of b without ANSI escape sequences, so that a
// coloured sentence is recognized.
func stripANSI(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] != 0x1b {
			out = append(out, b[i])
			continue
		}
		if i+1 < len(b) && b[i+1] == '[' {
			// Control sequence: parameters and intermediates up to a
			// final byte in 0x40 to 0x7e.
			j := i + 2
			for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		// A two-byte escape sequence.
		i++
	}
	return out
}

var errOutputLimit = errors.New("output limit exceeded")

// limitedBuffer holds standard output in memory up to a limit. When a write
// would pass the limit, it calls onExceed once and refuses the write.
type limitedBuffer struct {
	buf      []byte
	limit    int64
	exceeded bool
	onExceed func()
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.exceeded {
		return 0, errOutputLimit
	}
	if int64(len(b.buf))+int64(len(p)) > b.limit {
		b.exceeded = true
		if b.onExceed != nil {
			b.onExceed()
		}
		return 0, errOutputLimit
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// wipe overwrites the held output.
func (b *limitedBuffer) wipe() {
	clear(b.buf)
	b.buf = nil
}
