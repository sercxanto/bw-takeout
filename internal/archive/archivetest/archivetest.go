// Package archivetest builds synthetic archives for tests from the fixtures
// of package capturetest. Age key pairs are generated on every call, so the
// repository holds no key.
package archivetest

import (
	"bytes"
	"io"
	"testing"
	"time"

	"filippo.io/age"

	"bitwarden-backup/internal/archive"
	"bitwarden-backup/internal/capture/capturetest"
	"bitwarden-backup/internal/manifest"
	"bitwarden-backup/internal/verify"
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
		PlaintextExport: in.PlaintextExport,
		EncryptedExport: in.EncryptedExport,
		ItemList:        in.ItemList,
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
		Tool:         manifest.Tool{Name: "bitwarden-backup", Version: "test"},
		Verification: verification,
	}
}

// WriteTo writes a complete archive of the fixture to w.
func WriteTo(t testing.TB, w io.Writer, f *capturetest.Fixture, verification []byte, recipients ...string) {
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
	if err := aw.Finish(Meta(verification)); err != nil {
		t.Fatal(err)
	}
}

// Build returns a complete archive of the fixture.
func Build(t testing.TB, f *capturetest.Fixture, verification []byte, recipients ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	WriteTo(t, &b, f, verification, recipients...)
	return b.Bytes()
}
