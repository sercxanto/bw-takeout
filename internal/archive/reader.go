package archive

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"

	"filippo.io/age"

	"bitwarden-backup/internal/manifest"
)

// maxManifestSize bounds the memory the manifest may take.
const maxManifestSize = 64 << 20

// Errors of the reader. An entry-specific problem is an *EntryError. An
// unknown format version is a *manifest.UnsupportedVersionError.
var (
	// ErrNoIdentityMatch: none of the identities opens the archive. A
	// damaged recipient stanza looks the same to the decryption.
	ErrNoIdentityMatch = errors.New("no identity matches a recipient of the archive")
	// ErrDamaged: the decryption or the tar stream failed. This covers
	// truncation and altered bytes.
	ErrDamaged = errors.New("archive is damaged or truncated")
	// ErrStructure: the archive decrypts but does not have the structure
	// the format defines.
	ErrStructure = errors.New("archive structure is not valid")
)

// EntryError names an entry whose structure or content is wrong. Path is
// the entry name, which holds only fixed names and IDs in a valid archive.
type EntryError struct {
	Path    string
	Problem string
}

func (e *EntryError) Error() string {
	return fmt.Sprintf("entry %q: %s", e.Path, e.Problem)
}

// Problems of an EntryError.
const (
	ProblemNotRegular     = "is not a regular file"
	ProblemUnexpectedName = "has a name the format does not define"
	ProblemDuplicate      = "occurs more than once"
	ProblemAfterManifest  = "follows the manifest"
	ProblemDiffers        = "does not match the size and SHA-256 in the manifest"
	ProblemNotListed      = "is not listed in the manifest"
	ProblemMissing        = "is listed in the manifest but missing from the archive"
	ProblemManifestPath   = "is listed in the manifest with a kind or IDs that do not match its name"
	ProblemRequired       = "is required by the format but not listed in the manifest"
)

type seenEntry struct {
	size   int64
	sha256 string
}

// Read decrypts an archive with the identities, reads it sequentially to its
// end and checks it against its manifest (the offline-verification spec). It
// returns the manifest only when every check passed.
//
// visit, when not nil, is called for every entry before the manifest with a
// reader of its content. The content is not trustworthy until Read returns
// without error. visit must not keep the reader.
func Read(src io.Reader, identities []age.Identity, visit func(path string, content io.Reader) error) (*manifest.Manifest, error) {
	plain, err := age.Decrypt(src, identities...)
	if err != nil {
		var noMatch *age.NoIdentityMatchError
		if errors.As(err, &noMatch) {
			return nil, ErrNoIdentityMatch
		}
		return nil, fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	tr := tar.NewReader(plain)
	seen := map[string]seenEntry{}
	var order []string
	var m *manifest.Manifest
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDamaged, err)
		}
		if m != nil {
			return nil, &EntryError{Path: hdr.Name, Problem: ProblemAfterManifest}
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil, &EntryError{Path: hdr.Name, Problem: ProblemNotRegular}
		}
		if hdr.Name == ManifestPath {
			m, err = readManifest(tr, hdr.Size)
			if err != nil {
				return nil, err
			}
			continue
		}
		if _, _, _, ok := kindOf(hdr.Name); !ok {
			return nil, &EntryError{Path: hdr.Name, Problem: ProblemUnexpectedName}
		}
		if _, dup := seen[hdr.Name]; dup {
			return nil, &EntryError{Path: hdr.Name, Problem: ProblemDuplicate}
		}
		h := sha256.New()
		if visit != nil {
			if err := visit(hdr.Name, io.TeeReader(tr, h)); err != nil {
				return nil, err
			}
		}
		if _, err := io.Copy(h, tr); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDamaged, err)
		}
		seen[hdr.Name] = seenEntry{size: hdr.Size, sha256: hex.EncodeToString(h.Sum(nil))}
		order = append(order, hdr.Name)
	}
	// The tar stream ends before the encryption does. Truncation and
	// altered bytes in the last chunk show only when the decryption is read
	// to its end. Bytes after the end of the tar stream must be zero.
	if err := drainZeros(plain); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("%w: the manifest is missing", ErrStructure)
	}
	if err := compare(m, seen, order); err != nil {
		return nil, err
	}
	return m, nil
}

func readManifest(r io.Reader, size int64) (*manifest.Manifest, error) {
	if size > maxManifestSize {
		return nil, fmt.Errorf("%w: the manifest is larger than %d bytes", ErrStructure, maxManifestSize)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		var uv *manifest.UnsupportedVersionError
		if errors.As(err, &uv) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrStructure, err)
	}
	return m, nil
}

type zeroCheck struct{ nonZero bool }

func (z *zeroCheck) Write(p []byte) (int, error) {
	for _, b := range p {
		if b != 0 {
			z.nonZero = true
		}
	}
	return len(p), nil
}

func drainZeros(r io.Reader) error {
	var z zeroCheck
	if _, err := io.Copy(&z, r); err != nil {
		return fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	if z.nonZero {
		return fmt.Errorf("%w: data follows the end of the tar stream", ErrStructure)
	}
	return nil
}

// compare checks the entries read against the manifest and reports every
// deviation, sorted by entry name.
func compare(m *manifest.Manifest, seen map[string]seenEntry, order []string) error {
	var problems []*EntryError
	listed := map[string]bool{}
	attachments := 0
	for _, e := range m.Entries {
		listed[e.Path] = true
		kind, itemID, attachmentID, ok := kindOf(e.Path)
		if !ok || kind != e.Kind || itemID != e.ItemID || attachmentID != e.AttachmentID ||
			(kind != manifest.KindAttachment && e.FileName != "") {
			problems = append(problems, &EntryError{Path: e.Path, Problem: ProblemManifestPath})
			continue
		}
		if kind == manifest.KindAttachment {
			attachments++
		}
		got, ok := seen[e.Path]
		switch {
		case !ok:
			problems = append(problems, &EntryError{Path: e.Path, Problem: ProblemMissing})
		case got.size != e.Size || got.sha256 != e.SHA256:
			problems = append(problems, &EntryError{Path: e.Path, Problem: ProblemDiffers})
		}
	}
	for _, name := range order {
		if !listed[name] {
			problems = append(problems, &EntryError{Path: name, Problem: ProblemNotListed})
		}
	}
	for _, name := range []string{RecoveryPath, PlaintextExportPath, EncryptedExportPath, ItemListPath} {
		if _, present := seen[name]; !listed[name] && !present {
			problems = append(problems, &EntryError{Path: name, Problem: ProblemRequired})
		}
	}
	if len(problems) > 0 {
		sort.SliceStable(problems, func(i, j int) bool { return problems[i].Path < problems[j].Path })
		errs := make([]error, len(problems))
		for i, p := range problems {
			errs[i] = p
		}
		return errors.Join(errs...)
	}
	if attachments != m.Counts.Attachments {
		return fmt.Errorf("%w: the manifest counts %d attachments and lists %d", ErrStructure, m.Counts.Attachments, attachments)
	}
	return nil
}

// EntryErrors returns every *EntryError in err, including those joined with
// errors.Join.
func EntryErrors(err error) []*EntryError {
	var out []*EntryError
	var walk func(error)
	walk = func(e error) {
		if e == nil {
			return
		}
		if ee, ok := e.(*EntryError); ok {
			out = append(out, ee)
			return
		}
		if j, ok := e.(interface{ Unwrap() []error }); ok {
			for _, x := range j.Unwrap() {
				walk(x)
			}
			return
		}
		walk(errors.Unwrap(e))
	}
	walk(err)
	return out
}
