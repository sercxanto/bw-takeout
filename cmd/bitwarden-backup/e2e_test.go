package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bitwarden-backup/internal/archive"
	"bitwarden-backup/internal/archive/archivetest"
	"bitwarden-backup/internal/capture"
	"bitwarden-backup/internal/capture/capturetest"
	"bitwarden-backup/internal/manifest"
	"bitwarden-backup/internal/publish"
	"bitwarden-backup/internal/verify"
)

// backup runs the whole chain the later backup command will run: parse the
// captured documents, verify them, check the recipients, write the archive
// into a new generation and publish it by verdict.
func backup(t *testing.T, target string, f *capturetest.Fixture, recipients []string) (*publish.Published, error) {
	t.Helper()
	in := f.Input(t)
	result := verify.Verify(capture.Parse(in), &f.Reference)
	verification, err := result.JSON()
	if err != nil {
		return nil, err
	}
	// Recipients are checked before the target is touched.
	if _, err := archive.ParseRecipients(recipients); err != nil {
		return nil, err
	}
	g, err := (&publish.Publisher{}).Begin(target)
	if err != nil {
		return nil, err
	}
	abort := func(err error) (*publish.Published, error) {
		return nil, errors.Join(err, g.Abort())
	}
	w, err := archive.NewWriter(g.Writer(), recipients, g.Created(), archive.Documents{
		PlaintextExport: in.PlaintextExport,
		EncryptedExport: in.EncryptedExport,
		ItemList:        in.ItemList,
	})
	if err != nil {
		return abort(err)
	}
	for _, a := range archivetest.Attachments(f) {
		if err := w.AddAttachment(a); err != nil {
			return abort(err)
		}
	}
	err = w.Finish(archive.Meta{
		Source:       archivetest.Source,
		Tool:         manifest.Tool{Name: "bitwarden-backup", Version: "test"},
		Verification: verification,
	})
	if err != nil {
		return abort(err)
	}
	return g.Commit(verification)
}

func TestEndToEnd(t *testing.T) {
	cases := []struct {
		name    string
		fixture func(t *testing.T) *capturetest.Fixture
		marker  string
		exit    int
		verdict string
	}{
		{"verified", func(t *testing.T) *capturetest.Fixture { return capturetest.Intact(t) }, "", exitVerified, "verified"},
		{"unverified", func(t *testing.T) *capturetest.Fixture {
			return capturetest.VariantByName(t, capturetest.BlobLoginItem).Build(t)
		}, publish.UnverifiedMarker, exitUnverified, "unverified"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target := t.TempDir()
			id := archivetest.NewKey(t)
			pub, err := backup(t, target, c.fixture(t), []string{id.Recipient().String()})
			if err != nil {
				t.Fatal(err)
			}
			_, marker, ok := publish.ParseName(pub.Name)
			if !ok || marker != c.marker {
				t.Fatalf("published as %s", pub.Name)
			}
			o := runCommand(t, nil, "verify", "--identity", writeIdentity(t, id), pub.Path)
			if o.code != c.exit || !strings.Contains(o.stdout, "Verdict:     "+c.verdict) {
				t.Fatal(o)
			}
		})
	}
	t.Run("failed capture", func(t *testing.T) {
		target := t.TempDir()
		id := archivetest.NewKey(t)
		f := capturetest.VariantByName(t, capturetest.MissingItem).Build(t)
		if _, err := backup(t, target, f, []string{id.Recipient().String()}); !errors.Is(err, publish.ErrFailedCapture) {
			t.Fatalf("got %v", err)
		}
		if left, _ := os.ReadDir(target); len(left) != 0 {
			t.Errorf("target holds %d entries", len(left))
		}
	})
	t.Run("attachment ID with a path separator", func(t *testing.T) {
		target := t.TempDir()
		id := archivetest.NewKey(t)
		f := capturetest.Intact(t)
		f.Attachments[0].AttachmentID = "../escape"
		if _, err := backup(t, target, f, []string{id.Recipient().String()}); err == nil {
			t.Fatal("capture with an unsafe attachment ID was published")
		}
		if left, _ := os.ReadDir(target); len(left) != 0 {
			t.Errorf("target holds %d entries", len(left))
		}
	})
	t.Run("no recipient", func(t *testing.T) {
		target := t.TempDir()
		if _, err := backup(t, target, capturetest.Intact(t), nil); !errors.Is(err, archive.ErrNoRecipient) {
			t.Fatalf("got %v", err)
		}
		if left, _ := os.ReadDir(target); len(left) != 0 {
			t.Errorf("target holds %d entries", len(left))
		}
	})
}

// Environment variables of the manual offline exercise (design D12).
const (
	exerciseTargetEnv    = "BITWARDEN_BACKUP_EXERCISE_TARGET"
	exerciseRecipientEnv = "BITWARDEN_BACKUP_EXERCISE_RECIPIENT"
)

// TestLeaveGenerationForOfflineExercise writes one synthetic generation for
// the manual offline exercise on a second system. It runs only when both
// environment variables are set: the target directory and the age recipient
// whose identity is kept on the second system.
func TestLeaveGenerationForOfflineExercise(t *testing.T) {
	target, recipient := os.Getenv(exerciseTargetEnv), os.Getenv(exerciseRecipientEnv)
	if target == "" || recipient == "" {
		t.Skipf("skipped: opt-in, set %s and %s to leave a synthetic generation", exerciseTargetEnv, exerciseRecipientEnv)
	}
	pub, err := backup(t, target, capturetest.Intact(t), []string{recipient})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("synthetic generation left at %s", filepath.Clean(pub.Path))
}
