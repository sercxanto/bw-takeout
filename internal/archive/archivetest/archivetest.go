// Package archivetest builds synthetic archives for tests from the fixtures
// of package capturetest. Age key pairs are generated on every call, so the
// repository holds no key.
package archivetest

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"
	"time"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/verify"
)

// Created is the creation time of the archives this package builds.
var Created = time.Date(2026, 11, 14, 17, 0, 0, 0, time.UTC)

// Source is the synthetic source of the archives this package builds.
var Source = manifest.Source{
	ServerURL:    "https://vault.example.test",
	AccountEmail: "backup-user@example.test",
	AccountID:    "a0000000-0000-4000-8000-000000000001",
}

// NewKey generates an age X25519 key pair.
func NewKey(t testing.TB) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Documents returns the rendered documents of the fixture.
func Documents(t testing.TB, f *capturetest.Fixture) archive.Documents {
	t.Helper()
	in := f.Input(t)
	return archive.Documents{
		PlaintextExport:  in.PlaintextExport,
		EncryptedExport:  in.EncryptedExport,
		ItemList:         in.ItemList,
		ArchivedItemList: in.ArchivedItemList,
	}
}

// Attachments returns the attachments of the fixture. All of them carry the
// fixture's shared file name.
func Attachments(f *capturetest.Fixture) []archive.Attachment {
	var out []archive.Attachment
	for _, a := range f.Attachments {
		out = append(out, archive.Attachment{
			ItemID: a.ItemID, AttachmentID: a.AttachmentID,
			FileName: capturetest.AttachmentFileName, Content: a.Content,
		})
	}
	return out
}

// Verification returns the serialized verification result of the fixture,
// compared with its reference inventory when withReference is true.
func Verification(t testing.TB, f *capturetest.Fixture, withReference bool) ([]byte, verify.Verdict) {
	t.Helper()
	var ref *capturetest.Reference
	if withReference {
		ref = &f.Reference
	}
	r := verify.Verify(f.Capture(t), ref)
	raw, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	return raw, r.Verdict
}

// Meta returns the manifest metadata for a verification result.
func Meta(verification []byte) archive.Meta {
	return archive.Meta{
		Source:       Source,
		Tool:         manifest.Tool{Name: "bw-takeout", Version: "test"},
		Verification: verification,
	}
}

// WriteTo writes a complete archive of the fixture to w and returns the
// verification result as the archive embeds it.
func WriteTo(t testing.TB, w io.Writer, f *capturetest.Fixture, verification []byte, recipients ...string) []byte {
	t.Helper()
	aw, err := archive.NewWriter(w, recipients, Created, Documents(t, f))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range Attachments(f) {
		if err := aw.AddAttachment(a); err != nil {
			t.Fatal(err)
		}
	}
	embedded, err := aw.Finish(Meta(verification))
	if err != nil {
		t.Fatal(err)
	}
	return embedded
}

// Build returns a complete archive of the fixture.
func Build(t testing.TB, f *capturetest.Fixture, verification []byte, recipients ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	WriteTo(t, &b, f, verification, recipients...)
	return b.Bytes()
}

// Rewrite decrypts an archive, lets change replace the content of each
// entry, and encrypts the result again to the identity's recipient. The
// manifest is kept as it is, so a changed entry no longer matches it.
func Rewrite(t testing.TB, data []byte, id *age.X25519Identity, change func(name string, content []byte) []byte) []byte {
	t.Helper()
	plain, err := age.Decrypt(bytes.NewReader(data), id)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(plain)
	var out bytes.Buffer
	enc, err := age.Encrypt(&out, id.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(enc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		content = change(hdr.Name, content)
		hdr.Size = int64(len(content))
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
