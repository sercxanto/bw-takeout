package archive_test

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/manifest"
)

type rawEntry struct {
	hdr     tar.Header
	content []byte
}

// decryptEntries opens an archive without the package's reader and returns
// every tar entry with its header.
func decryptEntries(t *testing.T, data []byte, id age.Identity) []rawEntry {
	t.Helper()
	r, err := age.Decrypt(bytes.NewReader(data), id)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(r)
	var out []rawEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, rawEntry{hdr: *hdr, content: content})
	}
}

// encryptEntries builds an archive from raw entries, for archives that the
// writer would refuse to produce.
func encryptEntries(t *testing.T, entries []rawEntry, recipient age.Recipient) []byte {
	t.Helper()
	var b bytes.Buffer
	enc, err := age.Encrypt(&b, recipient)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(enc)
	for _, e := range entries {
		h := e.hdr
		h.Size = int64(len(e.content))
		if h.Typeflag != tar.TypeReg {
			h.Size = 0
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func intactArchive(t *testing.T) (*capturetest.Fixture, *age.X25519Identity, []byte, []byte) {
	t.Helper()
	f := capturetest.Intact(t)
	id := archivetest.NewKey(t)
	v, _ := archivetest.Verification(t, f, true)
	return f, id, v, archivetest.Build(t, f, v, id.Recipient().String())
}

func readAll(t *testing.T, data []byte, ids ...age.Identity) (*manifest.Manifest, map[string][]byte, error) {
	t.Helper()
	got := map[string][]byte{}
	m, err := archive.Read(bytes.NewReader(data), ids, func(path string, r io.Reader) error {
		c, err := io.ReadAll(r)
		got[path] = c
		return err
	})
	return m, got, err
}

func TestRoundTripIntactFixture(t *testing.T) {
	f, id, v, data := intactArchive(t)
	m, got, err := readAll(t, data, id)
	if err != nil {
		t.Fatal(err)
	}
	docs := archivetest.Documents(t, f)
	want := map[string][]byte{
		archive.RecoveryPath:        []byte(archive.RecoveryText),
		archive.PlaintextExportPath: docs.PlaintextExport,
		archive.EncryptedExportPath: docs.EncryptedExport,
		archive.ItemListPath:        docs.ItemList,
	}
	for _, a := range f.Attachments {
		want[archive.AttachmentPath(a.ItemID, a.AttachmentID)] = a.Content
	}
	if len(got) != len(want) {
		t.Errorf("got %d entries, want %d", len(got), len(want))
	}
	for path, content := range want {
		if !bytes.Equal(got[path], content) {
			t.Errorf("entry %s differs", path)
		}
	}
	if sha256.Sum256(got[archive.PlaintextExportPath]) != sha256.Sum256(docs.PlaintextExport) {
		t.Error("plaintext export hash differs")
	}
	if !bytes.Equal(m.Verification, v) {
		t.Error("embedded verification result differs from the supplied bytes")
	}
	if m.Counts.Items != 3 || m.Counts.Attachments != 2 || m.Source != archivetest.Source || !m.Created.Equal(archivetest.Created) {
		t.Errorf("manifest metadata: %+v %+v %v", m.Counts, m.Source, m.Created)
	}
	if len(m.Entries) != len(want) {
		t.Errorf("manifest lists %d entries", len(m.Entries))
	}
}

func TestFinishReturnsTheEmbeddedResult(t *testing.T) {
	f := capturetest.Intact(t)
	id := archivetest.NewKey(t)
	v, _ := archivetest.Verification(t, f, true)
	var b bytes.Buffer
	embedded := archivetest.WriteTo(t, &b, f, v, id.Recipient().String())
	m, err := archive.Read(bytes.NewReader(b.Bytes()), []age.Identity{id}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, m.Verification) || !bytes.Equal(embedded, v) {
		t.Errorf("Finish returned %q, the manifest embeds %q", embedded, m.Verification)
	}
}

func TestEntryOrderAndAttributes(t *testing.T) {
	f, id, _, data := intactArchive(t)
	entries := decryptEntries(t, data, id)
	var names []string
	for _, e := range entries {
		names = append(names, e.hdr.Name)
		h := e.hdr
		if h.Typeflag != tar.TypeReg || h.Mode != 0o600 || h.Uid != 0 || h.Gid != 0 ||
			h.Uname != "" || h.Gname != "" || !h.ModTime.Equal(archivetest.Created) {
			t.Errorf("entry %s: attributes %+v", h.Name, h)
		}
	}
	want := []string{archive.RecoveryPath, archive.PlaintextExportPath, archive.EncryptedExportPath, archive.ItemListPath}
	for _, a := range f.Attachments {
		want = append(want, archive.AttachmentPath(a.ItemID, a.AttachmentID))
	}
	want = append(want, archive.ManifestPath)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("order:\n got %v\nwant %v", names, want)
	}
}

func TestTwoAttachmentsWithTheSameFileName(t *testing.T) {
	_, id, _, data := intactArchive(t)
	m, got, err := readAll(t, data, id)
	if err != nil {
		t.Fatal(err)
	}
	first := archive.AttachmentPath(capturetest.ItemPasskeyLogin, capturetest.AttachmentFirst)
	second := archive.AttachmentPath(capturetest.ItemPasskeyLogin, capturetest.AttachmentSecond)
	if bytes.Equal(got[first], got[second]) {
		t.Fatal("precondition: the two attachments should differ")
	}
	names := map[string]string{}
	for _, e := range m.Entries {
		names[e.Path] = e.FileName
	}
	if names[first] != capturetest.AttachmentFileName || names[second] != capturetest.AttachmentFileName {
		t.Errorf("file names: %q %q", names[first], names[second])
	}
	for _, e := range decryptEntries(t, data, id) {
		if e.hdr.Name != archive.ManifestPath && strings.Contains(e.hdr.Name, capturetest.AttachmentFileName) {
			t.Errorf("file name in entry name %s", e.hdr.Name)
		}
	}
}

func TestIDValidation(t *testing.T) {
	bad := map[string]string{
		"path separator":   "aaaa/bbbb",
		"backslash":        `aaaa\bbbb`,
		"parent reference": "..",
		"parent path":      "../aaaa",
		"empty":            "",
	}
	if !archive.ValidAttachmentID(capturetest.AttachmentFirst) || !archive.ValidAttachmentID("0123456789abcdef0123") ||
		!archive.ValidAttachmentID(strings.Repeat("a", 64)) {
		t.Fatal("valid attachment IDs refused")
	}
	if !archive.ValidItemID(capturetest.ItemPasskeyLogin) {
		t.Fatal("valid item ID refused")
	}
	for name, id := range bad {
		if archive.ValidItemID(id) || archive.ValidAttachmentID(id) {
			t.Errorf("%s accepted", name)
		}
	}
	if archive.ValidAttachmentID(strings.Repeat("a", 65)) {
		t.Error("attachment ID of wrong length accepted")
	}
	if archive.ValidItemID(capturetest.ItemPasskeyLogin[:35]) || archive.ValidItemID(capturetest.ItemPasskeyLogin+"0") {
		t.Error("item ID of wrong length accepted")
	}
	if archive.ValidItemID("11111111/1111-4111-8111-111111111111") || archive.ValidItemID("1111111..111-4111-8111-111111111111") {
		t.Error("item ID with separator or dots accepted")
	}

	cases := map[string]archive.Attachment{}
	for name, id := range bad {
		cases["item ID "+name] = archive.Attachment{ItemID: id, AttachmentID: capturetest.AttachmentFirst}
		cases["attachment ID "+name] = archive.Attachment{ItemID: capturetest.ItemPasskeyLogin, AttachmentID: id}
	}
	cases["item ID wrong length"] = archive.Attachment{ItemID: capturetest.ItemPasskeyLogin[:35], AttachmentID: capturetest.AttachmentFirst}
	cases["attachment ID wrong length"] = archive.Attachment{ItemID: capturetest.ItemPasskeyLogin, AttachmentID: strings.Repeat("a", 65)}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			f := capturetest.Intact(t)
			id := archivetest.NewKey(t)
			v, _ := archivetest.Verification(t, f, true)
			var b bytes.Buffer
			w, err := archive.NewWriter(&b, []string{id.Recipient().String()}, archivetest.Created, archivetest.Documents(t, f))
			if err != nil {
				t.Fatal(err)
			}
			a.Content = []byte("content")
			if err := w.AddAttachment(a); err == nil {
				t.Fatal("attachment accepted")
			}
			if embedded, err := w.Finish(archivetest.Meta(v)); !errors.Is(err, archive.ErrWriterFailed) || embedded != nil {
				t.Fatalf("finish after refusal: %v, returned %q", err, embedded)
			}
			if _, err := archive.Read(bytes.NewReader(b.Bytes()), []age.Identity{id}, nil); err == nil {
				t.Fatal("unfinished archive reads as valid")
			}
		})
	}
}

func TestRecipients(t *testing.T) {
	f := capturetest.Intact(t)
	docs := archivetest.Documents(t, f)
	var b bytes.Buffer
	if _, err := archive.NewWriter(&b, nil, archivetest.Created, docs); !errors.Is(err, archive.ErrNoRecipient) {
		t.Errorf("no recipient: %v", err)
	}
	if b.Len() != 0 {
		t.Error("bytes written without recipient")
	}
	secret := archivetest.NewKey(t)
	good := archivetest.NewKey(t).Recipient().String()
	for _, rejected := range []string{
		secret.String(), // an identity in place of a recipient
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI" + capturetest.Marker,
		capturetest.Marker + "-passphrase",
		"age1" + capturetest.Marker,
	} {
		b.Reset()
		_, err := archive.NewWriter(&b, []string{good, rejected}, archivetest.Created, docs)
		if err == nil {
			t.Fatalf("recipient accepted")
		}
		if !strings.Contains(err.Error(), "recipient 2") {
			t.Errorf("position not named: %v", err)
		}
		if strings.Contains(err.Error(), rejected) || strings.Contains(err.Error(), capturetest.Marker) ||
			strings.Contains(err.Error(), "AGE-SECRET-KEY") {
			t.Errorf("value in error: %v", err)
		}
		if b.Len() != 0 {
			t.Error("bytes written with a rejected recipient")
		}
	}
}

// rebuild decrypts the intact archive, applies change to its entries and
// encrypts the result again.
func rebuild(t *testing.T, change func([]rawEntry) []rawEntry) (*age.X25519Identity, []byte) {
	t.Helper()
	_, id, _, data := intactArchive(t)
	entries := change(decryptEntries(t, data, id))
	return id, encryptEntries(t, entries, id.Recipient())
}

func indexOf(entries []rawEntry, name string) int {
	for i, e := range entries {
		if e.hdr.Name == name {
			return i
		}
	}
	return -1
}

func insertBefore(entries []rawEntry, name string, e rawEntry) []rawEntry {
	i := indexOf(entries, name)
	out := append([]rawEntry{}, entries[:i]...)
	out = append(out, e)
	return append(out, entries[i:]...)
}

func regular(name string, content string) rawEntry {
	return rawEntry{hdr: tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o600}, content: []byte(content)}
}

func wantEntryError(t *testing.T, err error, path, problem string) {
	t.Helper()
	for _, e := range archive.EntryErrors(err) {
		if e.Path == path && e.Problem == problem {
			if !strings.Contains(err.Error(), path) {
				t.Errorf("message does not name the entry: %v", err)
			}
			return
		}
	}
	t.Errorf("want %s %q, got %v", path, problem, err)
}

var (
	firstAttachment = archive.AttachmentPath(capturetest.ItemPasskeyLogin, capturetest.AttachmentFirst)
	extraAttachment = archive.AttachmentPath(capturetest.ItemPlainLogin, "extra0000000000000000000000000001")
)

func TestReaderScenarios(t *testing.T) {
	t.Run("entry differs from the manifest", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			i := indexOf(es, firstAttachment)
			es[i].content = append([]byte{}, es[i].content...)
			es[i].content[0] ^= 1
			return es
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, firstAttachment, archive.ProblemDiffers)
	})
	t.Run("entry not listed", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			return insertBefore(es, archive.ManifestPath, regular(extraAttachment, "extra"))
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, extraAttachment, archive.ProblemNotListed)
	})
	t.Run("listed entry missing", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			i := indexOf(es, firstAttachment)
			return append(es[:i:i], es[i+1:]...)
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, firstAttachment, archive.ProblemMissing)
	})
	t.Run("symbolic link entry", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			link := rawEntry{hdr: tar.Header{Typeflag: tar.TypeSymlink, Name: extraAttachment, Linkname: "/etc/passwd"}}
			return insertBefore(es, archive.ManifestPath, link)
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, extraAttachment, archive.ProblemNotRegular)
	})
	t.Run("directory entry", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			dir := rawEntry{hdr: tar.Header{Typeflag: tar.TypeDir, Name: "documents/", Mode: 0o700}}
			return insertBefore(es, archive.PlaintextExportPath, dir)
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, "documents/", archive.ProblemNotRegular)
	})
	t.Run("newer format version", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			i := indexOf(es, archive.ManifestPath)
			es[i].content = bytes.Replace(es[i].content, []byte(`"format_version": 1`), []byte(`"format_version": 2`), 1)
			return es
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		var uv *manifest.UnsupportedVersionError
		if !errors.As(err, &uv) || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("got %v", err)
		}
	})
	for _, name := range []string{"../escape", "/etc/passwd", "documents/other.json", "attachments/" + capturetest.ItemPasskeyLogin + "/../x", "attachments/x/y"} {
		t.Run("unexpected name "+name, func(t *testing.T) {
			id, data := rebuild(t, func(es []rawEntry) []rawEntry {
				return insertBefore(es, archive.ManifestPath, regular(name, "x"))
			})
			_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
			wantEntryError(t, err, name, archive.ProblemUnexpectedName)
		})
	}
	t.Run("duplicate entry", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			return insertBefore(es, archive.ManifestPath, es[indexOf(es, firstAttachment)])
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, firstAttachment, archive.ProblemDuplicate)
	})
	t.Run("manifest not last", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			return append(es, regular(extraAttachment, "late"))
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, extraAttachment, archive.ProblemAfterManifest)
	})
	t.Run("manifest missing", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry { return es[:len(es)-1] })
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		if !errors.Is(err, archive.ErrStructure) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("manifest invalid", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			es[len(es)-1].content = []byte("{}")
			return es
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		if !errors.Is(err, archive.ErrStructure) || !errors.Is(err, manifest.ErrInvalid) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("recovery instructions not first", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			es[0], es[1] = es[1], es[0]
			return es
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, archive.RecoveryPath, archive.ProblemNotFirst)
	})
	t.Run("item count differs from the export", func(t *testing.T) {
		var counted int
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			i := indexOf(es, archive.ManifestPath)
			m, err := manifest.Parse(es[i].content)
			if err != nil {
				t.Fatal(err)
			}
			counted = m.Counts.Items
			m.Counts.Items++
			if es[i].content, err = manifest.Marshal(m); err != nil {
				t.Fatal(err)
			}
			return es
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		if !errors.Is(err, archive.ErrStructure) || !strings.Contains(err.Error(), fmt.Sprintf("counts %d items", counted+1)) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestDataAfterTarEnd(t *testing.T) {
	_, id, _, data := intactArchive(t)
	plain, err := age.Decrypt(bytes.NewReader(data), id)
	if err != nil {
		t.Fatal(err)
	}
	all, err := io.ReadAll(plain)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	enc, _ := age.Encrypt(&b, id.Recipient())
	enc.Write(append(all, 'x'))
	enc.Close()
	if _, err := archive.Read(bytes.NewReader(b.Bytes()), []age.Identity{id}, nil); !errors.Is(err, archive.ErrStructure) {
		t.Fatalf("got %v", err)
	}
	b.Reset()
	enc, _ = age.Encrypt(&b, id.Recipient())
	enc.Write(append(all, make([]byte, 10240)...))
	enc.Close()
	if _, err := archive.Read(bytes.NewReader(b.Bytes()), []age.Identity{id}, nil); err != nil {
		t.Fatalf("zero padding refused: %v", err)
	}
}

// headerEnd returns the offset after the age header.
func headerEnd(t *testing.T, data []byte) int {
	t.Helper()
	i := bytes.Index(data, []byte("\n--- "))
	if i < 0 {
		t.Fatal("no header MAC line")
	}
	j := bytes.IndexByte(data[i+1:], '\n')
	return i + 1 + j + 1
}

// multiChunkArchive returns an archive whose payload spans several age
// chunks of 64 KiB, by adding a large synthetic attachment.
func multiChunkArchive(t *testing.T) (*age.X25519Identity, []byte) {
	t.Helper()
	f := capturetest.Intact(t)
	id := archivetest.NewKey(t)
	v, _ := archivetest.Verification(t, f, true)
	var b bytes.Buffer
	w, err := archive.NewWriter(&b, []string{id.Recipient().String()}, archivetest.Created, archivetest.Documents(t, f))
	if err != nil {
		t.Fatal(err)
	}
	attachments := append(archivetest.Attachments(f), archive.Attachment{
		ItemID: capturetest.ItemNote, AttachmentID: "large000000000000000000000000001",
		Content: bytes.Repeat([]byte("synthetic "), 20000),
	})
	for _, a := range attachments {
		if err := w.AddAttachment(a); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Finish(archivetest.Meta(v)); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	if _, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil); err != nil {
		t.Fatalf("precondition: %v", err)
	}
	return id, data
}

func TestTruncatedArchive(t *testing.T) {
	const chunk = 64*1024 + 16 // ciphertext of one full chunk
	for name, build := range map[string]func(*testing.T) (*age.X25519Identity, []byte){
		"single chunk": func(t *testing.T) (*age.X25519Identity, []byte) {
			_, id, _, data := intactArchive(t)
			return id, data
		},
		"several chunks": multiChunkArchive,
	} {
		t.Run(name, func(t *testing.T) {
			id, data := build(t)
			h := headerEnd(t, data)
			// Cuts inside the header, at its end, inside the first chunk, at
			// a chunk boundary, and inside the last chunk.
			cuts := []int{0, 1, 20, h - 30, h - 1, h, h + 8, h + 16, h + 100, len(data) / 2, len(data) - 17, len(data) - 16, len(data) - 1}
			if len(data)-h > chunk {
				cuts = append(cuts, h+chunk, h+chunk+1, h+2*chunk)
			}
			for _, n := range cuts {
				if n <= 0 || n >= len(data) {
					continue
				}
				if _, err := archive.Read(bytes.NewReader(data[:n]), []age.Identity{id}, nil); err == nil {
					t.Errorf("cut at %d of %d: verification passed", n, len(data))
				}
			}
		})
	}
}

func TestAlteredByte(t *testing.T) {
	id, data := multiChunkArchive(t)
	h := headerEnd(t, data)
	positions := []int{0, 5, 30, h - 20, h - 2, h, h + 16, h + 17, h + 500, len(data) / 2, len(data) - 17, len(data) - 1}
	for _, p := range positions {
		d := append([]byte{}, data...)
		d[p] ^= 0x01
		if _, err := archive.Read(bytes.NewReader(d), []age.Identity{id}, nil); err == nil {
			t.Errorf("byte %d of %d altered: verification passed", p, len(data))
		}
	}
}

func TestTwoRecipients(t *testing.T) {
	f := capturetest.Intact(t)
	a, b := archivetest.NewKey(t), archivetest.NewKey(t)
	v, _ := archivetest.Verification(t, f, true)
	data := archivetest.Build(t, f, v, a.Recipient().String(), b.Recipient().String())
	for name, id := range map[string]*age.X25519Identity{"first": a, "second": b} {
		if _, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil); err != nil {
			t.Errorf("%s identity alone: %v", name, err)
		}
	}
}

func TestIdentityOfAnotherKeyPair(t *testing.T) {
	_, _, _, data := intactArchive(t)
	other := archivetest.NewKey(t)
	_, err := archive.Read(bytes.NewReader(data), []age.Identity{other}, nil)
	if !errors.Is(err, archive.ErrNoIdentityMatch) || !strings.Contains(err.Error(), "no identity matches") {
		t.Fatalf("got %v", err)
	}
}

func TestRecoveryInstructions(t *testing.T) {
	f, id, _, data := intactArchive(t)
	entries := decryptEntries(t, data, id)
	if entries[0].hdr.Name != archive.RecoveryPath || string(entries[0].content) != archive.RecoveryText {
		t.Fatalf("first entry is %s", entries[0].hdr.Name)
	}
	text := archive.RecoveryText
	if strings.Contains(text, capturetest.Marker) {
		t.Error("recovery text contains the sentinel marker")
	}
	for _, s := range f.Sensitive() {
		if strings.Contains(text, s) {
			t.Errorf("recovery text contains sentinel value %q", s)
		}
	}
	for _, name := range []string{"RECOVERY.txt", "documents/export.json", "documents/export.encrypted.json", "documents/items.json", "attachments/", "manifest.json"} {
		if !strings.Contains(text, name) {
			t.Errorf("recovery text does not name %s", name)
		}
	}
}

func TestTemporaryLocationStaysEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	if os.TempDir() != tmp {
		t.Fatalf("temporary directory is %s", os.TempDir())
	}
	_, id, _, data := intactArchive(t)
	if _, _, err := readAll(t, data, id); err != nil {
		t.Fatal(err)
	}
	left, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("temporary directory holds %d entries", len(left))
	}
}

func TestParseIdentityFile(t *testing.T) {
	id := archivetest.NewKey(t)
	ids, err := archive.ParseIdentityFile(strings.NewReader("# comment\n\n" + id.String() + "\n"))
	if err != nil || len(ids) != 1 {
		t.Fatalf("got %d identities, %v", len(ids), err)
	}
	for name, content := range map[string]string{
		"sentinel line": id.String() + "\n" + capturetest.Marker + "-identity-line\n",
		"empty":         "",
		"only comments": "# a\n# b\n",
		"recipient":     id.Recipient().String() + "\n",
	} {
		_, err := archive.ParseIdentityFile(strings.NewReader(content))
		if !errors.Is(err, archive.ErrIdentityFile) {
			t.Errorf("%s: got %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), capturetest.Marker) || strings.Contains(err.Error(), "AGE-SECRET-KEY") || strings.Contains(err.Error(), "age1") {
			t.Errorf("%s: content in error: %v", name, err)
		}
	}
}
