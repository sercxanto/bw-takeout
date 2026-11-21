package archive_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/capture/capturetest"
)

// TestRecoveryWithStandardTools decrypts an archive with the age command and
// unpacks it with the tar command, then compares every file with the
// captured bytes (design D12), the documents of a chosen organization
// included (design D7 of organization-export). It writes synthetic plaintext to a test
// directory, the only test that does so on purpose.
func TestRecoveryWithStandardTools(t *testing.T) {
	agePath, err := exec.LookPath("age")
	if err != nil {
		t.Skip("skipped: the age command is not in the search path")
	}
	tarPath, err := exec.LookPath("tar")
	if err != nil {
		t.Skip("skipped: the tar command is not in the search path")
	}
	f := capturetest.IntactOrganization(t)
	id, data := organizationArchive(t, f)
	dir := t.TempDir()
	identity := filepath.Join(dir, "identity.txt")
	if err := os.WriteFile(identity, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	encrypted := filepath.Join(dir, "vault.tar.age")
	if err := os.WriteFile(encrypted, data, 0o600); err != nil {
		t.Fatal(err)
	}
	tarFile := filepath.Join(dir, "vault.tar")
	run(t, exec.Command(agePath, "-d", "-i", identity, "-o", tarFile, encrypted))

	// Relative names and the working directory keep a drive letter out of
	// the arguments: GNU tar on Windows reads "C:\..." as host:path.
	tarCmd := func(args ...string) *exec.Cmd {
		c := exec.Command(tarPath, args...)
		c.Dir = dir
		return c
	}
	listing := run(t, tarCmd("-tf", "vault.tar"))
	names := strings.Fields(listing)
	if len(names) == 0 || names[0] != archive.RecoveryPath || names[len(names)-1] != archive.ManifestPath {
		t.Errorf("tar listing: %v", names)
	}

	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	run(t, tarCmd("-xf", "vault.tar", "-C", "out"))

	docs := archivetest.Documents(t, f)
	want := map[string][]byte{
		archive.RecoveryPath:         []byte(archive.RecoveryText),
		archive.PlaintextExportPath:  docs.PlaintextExport,
		archive.EncryptedExportPath:  docs.EncryptedExport,
		archive.ItemListPath:         docs.ItemList,
		archive.ArchivedItemListPath: docs.ArchivedItemList,
	}
	for _, a := range f.Attachments {
		want[archive.AttachmentPath(a.ItemID, a.AttachmentID)] = a.Content
	}
	if len(docs.Organizations) != 1 {
		t.Fatalf("precondition: %d organizations", len(docs.Organizations))
	}
	for _, o := range docs.Organizations {
		want[archive.OrganizationPlaintextExportPath(o.ID)] = o.PlaintextExport
		want[archive.OrganizationEncryptedExportPath(o.ID)] = o.EncryptedExport
	}
	if len(names) != len(want)+1 {
		t.Errorf("tar lists %d entries, want %d and the manifest", len(names), len(want))
	}
	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !bytes.Equal(got, content) {
			t.Errorf("%s differs from the captured bytes", name)
		}
	}
	if _, err := os.Stat(filepath.Join(out, archive.ManifestPath)); err != nil {
		t.Errorf("manifest not unpacked: %v", err)
	}
}

func run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v\n%s", filepath.Base(cmd.Path), err, stderr.String())
	}
	return stdout.String()
}
