package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/capture"
	"bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/verify"
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
	// Publish takes the verdict from the bytes that Finish returns, which are
	// the bytes the manifest embeds.
	return (&publish.Publisher{}).Publish(target, func(dst io.Writer, created time.Time) ([]byte, error) {
		w, err := archive.NewWriter(dst, recipients, created, archive.Documents{
			PlaintextExport:  in.PlaintextExport,
			EncryptedExport:  in.EncryptedExport,
			ItemList:         in.ItemList,
			ArchivedItemList: in.ArchivedItemList,
		})
		if err != nil {
			return nil, err
		}
		for _, a := range archivetest.Attachments(f) {
			if err := w.AddAttachment(a); err != nil {
				return nil, err
			}
		}
		return w.Finish(archive.Meta{
			Source:       archivetest.Source,
			Tool:         manifest.Tool{Name: "bw-takeout", Version: "test"},
			Verification: verification,
		})
	})
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
	exerciseTargetEnv    = "BW_TAKEOUT_EXERCISE_TARGET"
	exerciseRecipientEnv = "BW_TAKEOUT_EXERCISE_RECIPIENT"
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
