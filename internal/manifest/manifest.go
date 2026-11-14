// Package manifest holds the manifest of one backup generation: what the
// archive holds, where the capture came from, and the verification result
// the generation was published with. See the design of the change
// archive-generation, decisions D4 and D5.
//
// The manifest holds IDs, sizes, hashes and attachment file names. File names
// are vault content, so a manifest is only ever stored inside the encryption
// and never printed.
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// FormatName is the fixed value of the format key.
const FormatName = "bitwarden-backup-generation"

// FormatVersion is the only format version this package writes and reads.
const FormatVersion = 1

// Kind classifies an archive entry.
type Kind string

// The entry kinds.
const (
	KindRecovery   Kind = "recovery"
	KindDocument   Kind = "document"
	KindAttachment Kind = "attachment"
)

// Manifest is the content of manifest.json.
type Manifest struct {
	Created time.Time
	Source  Source
	Tool    Tool
	BW      BW
	Counts  Counts
	// Verification is the serialized verification result, kept byte for
	// byte. It must be a JSON object without surrounding white space.
	Verification json.RawMessage
	Entries      []Entry
}

// Source names the server and account a capture came from. The caller
// supplies the values.
type Source struct {
	ServerURL    string `json:"server_url"`
	AccountEmail string `json:"account_email"`
	AccountID    string `json:"account_id"`
}

// Tool names the program that wrote the generation.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// BW names the version of the official CLI that made the capture.
type BW struct {
	Version string `json:"version"`
}

// Counts states the size of the capture.
type Counts struct {
	// Items is the number of items in the plaintext export.
	Items int `json:"items"`
	// Attachments is the number of attachment entries.
	Attachments int `json:"attachments"`
}

// Entry describes one archive entry other than the manifest.
type Entry struct {
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
	Size int64  `json:"size"`
	// SHA256 is the lowercase hex SHA-256 of the content.
	SHA256 string `json:"sha256"`
	// ItemID, AttachmentID and FileName are set for attachments only.
	ItemID       string `json:"item_id,omitempty"`
	AttachmentID string `json:"attachment_id,omitempty"`
	FileName     string `json:"file_name,omitempty"`
}

// UnsupportedVersionError reports a format version this package does not
// know.
type UnsupportedVersionError struct {
	Version int
}

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf("manifest format version %d is not supported (supported: %d)", e.Version, FormatVersion)
}

// ErrInvalid is wrapped by every error about a malformed manifest.
var ErrInvalid = errors.New("invalid manifest")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Marshal serializes the manifest. Top-level keys come in a fixed order. The
// verification result is copied unchanged, which encoding/json would not do,
// because it compacts embedded raw messages.
func Marshal(m *Manifest) ([]byte, error) {
	if err := validRaw(m.Verification); err != nil {
		return nil, err
	}
	if err := validEntries(m.Entries); err != nil {
		return nil, err
	}
	entries := m.Entries
	if entries == nil {
		entries = []Entry{}
	}
	fields := []struct {
		key   string
		value any
	}{
		{"format", FormatName},
		{"format_version", FormatVersion},
		{"created", m.Created.UTC().Format(time.RFC3339)},
		{"source", m.Source},
		{"tool", m.Tool},
		{"bw", m.BW},
		{"counts", m.Counts},
		{"verification", nil},
		{"entries", entries},
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, f := range fields {
		fmt.Fprintf(&b, "  %q: ", f.key)
		if f.key == "verification" {
			b.Write(m.Verification)
		} else {
			v, err := json.MarshalIndent(f.value, "  ", "  ")
			if err != nil {
				return nil, err
			}
			b.Write(v)
		}
		if i < len(fields)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return b.Bytes(), nil
}

type wire struct {
	Format        *string         `json:"format"`
	FormatVersion *int            `json:"format_version"`
	Created       string          `json:"created"`
	Source        Source          `json:"source"`
	Tool          Tool            `json:"tool"`
	BW            BW              `json:"bw"`
	Counts        Counts          `json:"counts"`
	Verification  json.RawMessage `json:"verification"`
	Entries       []Entry         `json:"entries"`
}

// Parse reads a manifest. It fails on an unknown format version with an
// *UnsupportedVersionError and on any other problem with an error that wraps
// ErrInvalid. Unknown additional keys are ignored. Error messages name keys,
// positions and entry paths, never values from the source or file names.
func Parse(data []byte) (*Manifest, error) {
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, invalid("not a JSON object of the expected form")
	}
	if w.Format == nil || *w.Format != FormatName {
		return nil, invalid("format is not %q", FormatName)
	}
	if w.FormatVersion == nil {
		return nil, invalid("format_version is missing")
	}
	if *w.FormatVersion != FormatVersion {
		return nil, &UnsupportedVersionError{Version: *w.FormatVersion}
	}
	created, err := time.Parse(time.RFC3339, w.Created)
	if err != nil {
		return nil, invalid("created is not an RFC 3339 time")
	}
	if err := validRaw(w.Verification); err != nil {
		return nil, err
	}
	if err := validEntries(w.Entries); err != nil {
		return nil, err
	}
	return &Manifest{
		Created:      created.UTC(),
		Source:       w.Source,
		Tool:         w.Tool,
		BW:           w.BW,
		Counts:       w.Counts,
		Verification: w.Verification,
		Entries:      w.Entries,
	}, nil
}

func validRaw(raw json.RawMessage) error {
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' || !json.Valid(raw) {
		return invalid("verification is not a JSON object")
	}
	return nil
}

func validEntries(entries []Entry) error {
	seen := make(map[string]bool, len(entries))
	for i, e := range entries {
		if e.Path == "" {
			return invalid("entry %d has no path", i)
		}
		if seen[e.Path] {
			return invalid("entry %q is listed more than once", e.Path)
		}
		seen[e.Path] = true
		switch e.Kind {
		case KindRecovery, KindDocument, KindAttachment:
		default:
			return invalid("entry %q has an unknown kind", e.Path)
		}
		if e.Size < 0 {
			return invalid("entry %q has a negative size", e.Path)
		}
		if !isSHA256(e.SHA256) {
			return invalid("entry %q has no lowercase hex SHA-256", e.Path)
		}
	}
	return nil
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
