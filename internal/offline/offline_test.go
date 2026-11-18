package offline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/verify"
)

// The tests of the command line run the built binary in cmd/bw-takeout.
// The tests here call the moved functions directly.

func writeIdentity(t *testing.T, id *age.X25519Identity) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "identity.txt")
	if err := os.WriteFile(path, []byte("# synthetic test identity\n"+id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtureFor(t *testing.T, verdict verify.Verdict) (*capturetest.Fixture, []byte) {
	t.Helper()
	var f *capturetest.Fixture
	switch verdict {
	case verify.Verified:
		f = capturetest.Intact(t)
	case verify.Unverified:
		f = capturetest.VariantByName(t, capturetest.BlobLoginItem).Build(t)
	}
	v, got := archivetest.Verification(t, f, true)
	if got != verdict {
		t.Fatalf("precondition: verdict %s, want %s", got, verdict)
	}
	return f, v
}

func TestPartialGenerationByAnotherPath(t *testing.T) {
	target := t.TempDir()
	id := archivetest.NewKey(t)
	g, err := (&publish.Publisher{}).Begin(target)
	if err != nil {
		t.Fatal(err)
	}
	// Windows cannot remove a directory that holds an open file.
	t.Cleanup(func() {
		if err := g.Abort(); err != nil {
			t.Error(err)
		}
	})
	f, v := fixtureFor(t, verify.Verified)
	archivetest.WriteTo(t, g.Writer(), f, v, id.Recipient().String())
	partial := filepath.Join(target, g.PartialName())
	link := filepath.Join(t.TempDir(), "latest")
	if err := os.Symlink(partial, link); err != nil {
		t.Skipf("symbolic links are not available: %v", err)
	}
	identity := writeIdentity(t, id)
	for _, c := range []struct{ name, cwd, dir string }{
		{"current directory", partial, "."},
		{"symbolic link", "", link},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.cwd != "" {
				t.Chdir(c.cwd)
			}
			var stdout, stderr bytes.Buffer
			code := verifyGeneration(identity, c.dir, &stdout, &stderr)
			if code != ExitFailed || !strings.Contains(stderr.String(), "incomplete") {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestStoredResultMatchesPackageVerify(t *testing.T) {
	_, v := fixtureFor(t, verify.Unverified)
	var full verify.Result
	var part storedResult
	if err := json.Unmarshal(v, &full); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(v, &part); err != nil {
		t.Fatal(err)
	}
	if part.Verdict != string(full.Verdict) || len(part.Findings) != len(full.Findings) || len(part.Warnings) != len(full.Warnings) {
		t.Fatalf("stored result %+v, full %+v", part, full)
	}
	if publish.VerdictVerified != string(verify.Verified) || publish.VerdictUnverified != string(verify.Unverified) {
		t.Fatal("verdict constants differ")
	}
}
