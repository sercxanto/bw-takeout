// Package printable replaces characters that are not printable, so that a
// value from an archive, from bw or from a vault cannot steer the terminal.
// The offline command and the backup report share it (design D1 of the
// change interactive-backup-run).
package printable

import (
	"strings"
	"unicode"
)

// Clean replaces control and other non-printable characters with '?'.
// Printable characters, including non-ASCII letters, stay as they are.
func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return '?'
	}, s)
}
