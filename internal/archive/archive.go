// Package archive writes and reads the encrypted archive of one backup
// generation: a tar stream encrypted with age that holds the captured
// documents, the attachments by ID, a manifest and recovery instructions.
// See the design of the change archive-generation, decisions D2 to D7 and
// D11.
//
// The package works on an io.Writer and an io.Reader and never touches a
// path. Plaintext exists only in memory and inside the encryption stream.
// Neither the writer nor the reader creates a file.
package archive

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"

	"bw-takeout/internal/manifest"
	"bw-takeout/internal/vaultid"
)

// Fixed entry names of the layout (design D2).
const (
	RecoveryPath        = "RECOVERY.txt"
	PlaintextExportPath = "documents/export.json"
	EncryptedExportPath = "documents/export.encrypted.json"
	ItemListPath        = "documents/items.json"
	// ArchivedItemListPath holds the output of "bw list items --archived"
	// (design D5 of archived-item-list).
	ArchivedItemListPath = "documents/items.archived.json"
	ManifestPath         = "manifest.json"
	attachmentsDir       = "attachments/"
)

// AttachmentPath returns the entry name of one attachment. It does not
// validate the IDs.
func AttachmentPath(itemID, attachmentID string) string {
	return attachmentsDir + itemID + "/" + attachmentID
}

// ValidItemID reports whether id has the UUID form of 8-4-4-4-12
// hexadecimal digits (design D3). The rule lives in package vaultid.
func ValidItemID(id string) bool { return vaultid.ValidItemID(id) }

// ValidAttachmentID reports whether id consists of 1 to 64 ASCII letters
// and digits (design D3). The rule lives in package vaultid.
func ValidAttachmentID(id string) bool { return vaultid.ValidAttachmentID(id) }

// kindOf returns the kind an entry name stands for, and for an attachment
// its IDs. ok is false for a name the format does not define.
func kindOf(name string) (kind manifest.Kind, itemID, attachmentID string, ok bool) {
	switch name {
	case RecoveryPath:
		return manifest.KindRecovery, "", "", true
	case PlaintextExportPath, EncryptedExportPath, ItemListPath, ArchivedItemListPath:
		return manifest.KindDocument, "", "", true
	}
	rest, found := strings.CutPrefix(name, attachmentsDir)
	if !found {
		return "", "", "", false
	}
	itemID, attachmentID, found = strings.Cut(rest, "/")
	if !found || !ValidItemID(itemID) || !ValidAttachmentID(attachmentID) {
		return "", "", "", false
	}
	return manifest.KindAttachment, itemID, attachmentID, true
}

// ErrNoRecipient is returned when writing is requested without a recipient.
var ErrNoRecipient = errors.New("no recipient given")

// ParseRecipients parses native age X25519 recipient strings. An error
// names the one-based position of a rejected recipient and never its value.
func ParseRecipients(recipients []string) ([]age.Recipient, error) {
	if len(recipients) == 0 {
		return nil, ErrNoRecipient
	}
	out := make([]age.Recipient, 0, len(recipients))
	for i, s := range recipients {
		r, err := age.ParseX25519Recipient(s)
		if err != nil {
			// The age error quotes the value, so it is not wrapped.
			return nil, fmt.Errorf("recipient %d is not a native age X25519 recipient", i+1)
		}
		out = append(out, r)
	}
	return out, nil
}

// maxKeyFile bounds how much of an identity or recipients file is read.
const maxKeyFile = 1 << 20

// ErrIdentityFile is wrapped by every error about an identity file. The
// errors name line numbers and never content.
var ErrIdentityFile = errors.New("identity file")

// ErrRecipientFile is wrapped by every error about a recipients file. The
// errors name line numbers and never content (design D10 of
// interactive-backup-run).
var ErrRecipientFile = errors.New("recipients file")

// keyLine is one line of a key file that is neither empty nor a comment.
type keyLine struct {
	n    int
	text string
}

// readKeyLines reads a file of one key per line. Empty lines and lines
// starting with # are skipped, and whitespace at both ends of a line is
// trimmed. Errors wrap base and name no content.
func readKeyLines(r io.Reader, base error) ([]keyLine, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxKeyFile+1))
	if err != nil {
		return nil, fmt.Errorf("%w: cannot be read", base)
	}
	if len(data) > maxKeyFile {
		return nil, fmt.Errorf("%w: larger than %d bytes", base, maxKeyFile)
	}
	var lines []keyLine
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 0, 4096), maxKeyFile)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, keyLine{n, line})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%w: cannot be read", base)
	}
	return lines, nil
}

// ParseIdentityFile reads unencrypted age X25519 identities, one per line.
// Empty lines and lines starting with # are skipped. Any other line that is
// not an identity, and a file without any identity, are refused.
func ParseIdentityFile(r io.Reader) ([]age.Identity, error) {
	lines, err := readKeyLines(r, ErrIdentityFile)
	if err != nil {
		return nil, err
	}
	var ids []age.Identity
	for _, l := range lines {
		id, err := age.ParseX25519Identity(l.text)
		if err != nil {
			// The age error may quote the line, so it is not wrapped.
			return nil, fmt.Errorf("%w: line %d is not an unencrypted age X25519 identity", ErrIdentityFile, l.n)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: holds no identity", ErrIdentityFile)
	}
	return ids, nil
}

// ParseRecipientFile reads native age X25519 recipients, one per line, in
// the form of an identity file: empty lines and lines starting with # are
// skipped, and whitespace at both ends of a line is trimmed. Any other line
// that is not a recipient, and a file without any recipient, are refused.
// It returns the recipient strings, which NewWriter parses again.
func ParseRecipientFile(r io.Reader) ([]string, error) {
	lines, err := readKeyLines(r, ErrRecipientFile)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range lines {
		if _, err := age.ParseX25519Recipient(l.text); err != nil {
			// The age error quotes the value, so it is not wrapped.
			return nil, fmt.Errorf("%w: line %d is not a native age X25519 recipient", ErrRecipientFile, l.n)
		}
		out = append(out, l.text)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: holds no recipient", ErrRecipientFile)
	}
	return out, nil
}
