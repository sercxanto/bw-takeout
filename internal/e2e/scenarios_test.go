//go:build vaultwarden && linux

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/publish"
)

// seededServer starts a server, registers an account and seeds it.
func seededServer(t *testing.T) (*vaultServer, *account, *inventory) {
	t.Helper()
	s := startServer(t)
	a := newAccount(t)
	if err := a.register(s.Client, s.URL); err != nil {
		t.Fatal(err)
	}
	inv, _, _ := seedVault(t, s, a)
	return s, a, inv
}

// TestIntactVaultIsVerified is the intact scenario of design D8.
func TestIntactVaultIsVerified(t *testing.T) {
	s, a, inv := seededServer(t)
	r := runBackup(t, s, a)
	t.Logf("report:\n%s", r.redacted(a))

	if r.ExitCode != 0 {
		t.Fatalf("exit status %d, want 0", r.ExitCode)
	}
	if v, _ := r.reportLine("Verdict:"); v != "verified" {
		t.Fatalf("verdict %q, want verified", v)
	}
	gen, _ := r.reportLine("Generation:")
	if filepath.Dir(gen) != r.Target {
		t.Fatalf("the generation line names %q, not a path in the target", gen)
	}

	entries, err := os.ReadDir(r.Target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(gen) {
		t.Fatalf("the target holds %d entries, want the one generation", len(entries))
	}
	if name := entries[0].Name(); !strings.HasPrefix(name, publish.NamePrefix) || strings.Contains(name, ".") {
		t.Fatalf("the target entry %q is not a final generation", name)
	}

	verify := exec.Command(takeout, "verify", "--identity", r.Identity, gen)
	if out, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("verify: %v\n%s", err, redactAddress(string(out), a.Email))
	}

	got := readGeneration(t, gen, r.Identity)
	for _, want := range inv.Attachments {
		p := archive.AttachmentPath(want.ItemID, want.AttachmentID)
		sum, ok := got.attachments[p]
		if !ok {
			t.Errorf("no entry %s", p)
			continue
		}
		if sum != want.SHA256 {
			t.Errorf("%s: SHA-256 %s, want the seeded %s", p, sum, want.SHA256)
		}
	}
	if len(got.attachments) != len(inv.Attachments) {
		t.Errorf("%d attachment entries, want %d", len(got.attachments), len(inv.Attachments))
	}
	archived := false
	for _, at := range inv.Attachments {
		if at.ItemID == inv.ArchivedItem {
			_, archived = got.attachments[archive.AttachmentPath(at.ItemID, at.AttachmentID)]
		}
	}
	if !archived {
		t.Error("the attachment of the archived item is not in the generation")
	}
	if len(got.passkeys) != 1 || got.passkeys[0] != inv.CredentialID {
		t.Errorf("the plaintext export holds %d passkeys, want the seeded one", len(got.passkeys))
	}
	r.wantEmptyProfileLocation(t)
}

// TestTamperedPasskeyFails is the tampered scenario of design D8: the
// keyValue of the passkey is replaced on the server with a well-formed
// ciphertext under a foreign key.
func TestTamperedPasskeyFails(t *testing.T) {
	s, a, inv := seededServer(t)
	api, err := a.login(s.Client, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := api.tamperPasskey(inv.PasskeyItem); err != nil {
		t.Fatal(err)
	}
	r := runBackup(t, s, a)
	t.Logf("report:\n%s", r.redacted(a))

	if r.ExitCode != 1 {
		t.Fatalf("exit status %d, want 1", r.ExitCode)
	}
	if g, _ := r.reportLine("Generation:"); g != "none published, the verdict is failed" {
		t.Fatalf("generation line %q", g)
	}
	want := fmt.Sprintf("failed passkey_count_mismatch item=%s expected=1 found=0", inv.PasskeyItem)
	if !strings.Contains(r.Report, "  "+want+"\n") {
		t.Fatalf("the report has no finding %q", want)
	}
	if n := strings.Count(r.Report, "\n  failed "); n != 1 {
		t.Fatalf("%d failed findings, want only the passkey count", n)
	}
	entries, err := os.ReadDir(r.Target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the target holds %d entries, want none", len(entries))
	}
	r.wantEmptyProfileLocation(t)
}

// generationContent is what the scenario reads from a generation: the
// SHA-256 of each attachment entry and the credential IDs of the passkeys
// in the plaintext export. Content stays in memory.
type generationContent struct {
	attachments map[string]string
	passkeys    []string
}

func readGeneration(t *testing.T, gen, identityFile string) *generationContent {
	t.Helper()
	idf, err := os.Open(identityFile)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := age.ParseIdentities(idf)
	idf.Close()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(gen, publish.ArchiveName))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c := &generationContent{attachments: map[string]string{}}
	_, err = archive.Read(f, ids, func(path string, content io.Reader) error {
		data, err := io.ReadAll(content)
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(path, "attachments/"):
			c.attachments[path] = sha256Hex(data)
		case path == archive.PlaintextExportPath:
			var doc struct {
				Items []listedItem `json:"items"`
			}
			if err := json.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("plaintext export: not JSON")
			}
			for _, it := range doc.Items {
				for _, p := range it.Login.Fido2Credentials {
					c.passkeys = append(c.passkeys, p.CredentialID)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read the generation: %v", err)
	}
	return c
}
