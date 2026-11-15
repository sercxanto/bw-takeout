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
)

// TestRecoveryWithStandardTools decrypts an archive with the age command and
// unpacks it with the tar command, then compares every file with the
// captured bytes (design D12). It writes synthetic plaintext to a test
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
	f, id, _, data := intactArchive(t)
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

	listing := run(t, exec.Command(tarPath, "-tf", tarFile))
	names := strings.Fields(listing)
	if len(names) == 0 || names[0] != archive.RecoveryPath || names[len(names)-1] != archive.ManifestPath {
		t.Errorf("tar listing: %v", names)
	}

	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	run(t, exec.Command(tarPath, "-xf", tarFile, "-C", out))

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
