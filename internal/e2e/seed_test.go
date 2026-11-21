//go:build vaultwarden && linux

package e2e

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// seedPasswordVar carries the password to the seed bw. The password never
// appears in an argument (design D9).
const seedPasswordVar = "BW_TAKEOUT_E2E_SEED_PASSWORD"

const (
	seedCommandLimit = 2 * time.Minute
	attachmentSize   = 64 << 10
)

// inventory is the reference inventory of a seeded vault, known from
// outside the CLI (design D5).
type inventory struct {
	PasskeyItem  string // the login with the passkey
	CredentialID string
	Items        []string // all item IDs, the passkey item first
	ArchivedItem string
	Attachments  []seededAttachment
}

type seededAttachment struct {
	ItemID, AttachmentID, SHA256 string
}

// bwPath is the resolved path of the bw of the dev container.
func bwPath(t testing.TB) string {
	t.Helper()
	p, err := exec.LookPath("bw")
	if err != nil {
		t.Fatalf("bw not found: %v", err)
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// seedBW runs the seed bw in a profile of its own.
type seedBW struct {
	bw      string
	profile string
	env     []string
	session string
	fresh   bool // no command has run in the profile yet
}

func (b *seedBW) run(stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), seedCommandLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.bw, args...)
	cmd.Dir = b.profile
	cmd.Env = b.env
	if b.session != "" {
		cmd.Env = append(append([]string{}, b.env...), "BW_SESSION="+b.session)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	defer func() { b.fresh = false }()
	// Neither output of bw is printed (design D9). A failure names the
	// command and the exit status only.
	if err != nil {
		return nil, fmt.Errorf("bw %s: %v", args[0], err)
	}
	// The first start of a fresh profile reports on standard error that
	// it created its data file; every later command must write nothing
	// there.
	if errOut.Len() != 0 && !b.fresh {
		return nil, fmt.Errorf("bw %s wrote %d bytes to standard error", args[0], errOut.Len())
	}
	return out.Bytes(), nil
}

// seedVault fills the vault of a registered account with the three items
// of the backup exercise of 18 November 2026 through the official bw: a
// login with a passkey, a login with an attachment, and an archived login
// with an attachment. It checks the counts that bw then shows, logs out
// and removes the seed profile and the import file. The returned paths are
// those of the removed profile and import file, for the test of design D5.
func seedVault(t *testing.T, s *vaultServer, a *account) (inv *inventory, profile, importFile string) {
	t.Helper()
	root := t.TempDir()
	profile = filepath.Join(root, "seed-profile")
	home := filepath.Join(root, "home")
	files := filepath.Join(root, "files")
	for _, d := range []string{profile, home, files} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	b := &seedBW{
		bw:      bwPath(t),
		profile: profile,
		fresh:   true,
		env: []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + home,
			"BITWARDENCLI_APPDATA_DIR=" + profile,
			"NODE_EXTRA_CA_CERTS=" + s.CAFile,
			"BW_NOINTERACTION=true",
			seedPasswordVar + "=" + a.Password,
		},
	}
	removeProfile := func() {
		if err := os.RemoveAll(profile); err != nil {
			t.Errorf("seed profile: %v", err)
		}
	}
	t.Cleanup(removeProfile)

	must := func(out []byte, err error) []byte {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// config server has no --raw form, so its output is read and dropped.
	must(b.run(nil, "config", "server", s.URL))
	session := must(b.run(nil, "login", a.Email, "--passwordenv", seedPasswordVar, "--raw"))
	if len(session) != 88 {
		t.Fatalf("seed login: a session key of %d bytes, want 88", len(session))
	}
	b.session = string(session)
	keepOutOfLog("session key", b.session)

	inv = &inventory{}
	importFile = filepath.Join(files, "import.json")
	data, credentialID := passkeyImport(t)
	inv.CredentialID = credentialID
	if err := os.WriteFile(importFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := b.run(nil, "import", "bitwardenjson", importFile)
	if rerr := os.Remove(importFile); rerr != nil {
		t.Fatal(rerr)
	}
	if err != nil {
		t.Fatal(err)
	}
	items := listItems(t, must(b.run(nil, "list", "items", "--raw")))
	if len(items) != 1 {
		t.Fatalf("after the import: %d items, want 1", len(items))
	}
	inv.PasskeyItem = items[0].ID
	inv.Items = append(inv.Items, items[0].ID)

	for i, name := range []string{"e2e attachment", "e2e archived attachment"} {
		item, err := json.Marshal(map[string]any{
			"type": 1, "name": name, "notes": nil, "favorite": false, "reprompt": 0,
			"fields": []any{}, "folderId": nil, "organizationId": nil, "collectionIds": []any{},
			"login": map[string]any{"username": "e2e", "password": nil, "totp": nil, "uris": []any{}},
		})
		if err != nil {
			t.Fatal(err)
		}
		var created struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(must(b.run(nil, "create", "item", base64.StdEncoding.EncodeToString(item), "--raw")), &created); err != nil || created.ID == "" {
			t.Fatalf("create item: no item ID")
		}
		inv.Items = append(inv.Items, created.ID)

		content := randomBytes(t, attachmentSize)
		file := filepath.Join(files, fmt.Sprintf("attachment-%d.bin", i+1))
		if err := os.WriteFile(file, content, 0o600); err != nil {
			t.Fatal(err)
		}
		var withAttachment struct {
			Attachments []struct {
				ID string `json:"id"`
			} `json:"attachments"`
		}
		if err := json.Unmarshal(must(b.run(nil, "create", "attachment", "--file", file, "--itemid", created.ID, "--raw")), &withAttachment); err != nil || len(withAttachment.Attachments) != 1 {
			t.Fatalf("create attachment: want 1 attachment on the item")
		}
		inv.Attachments = append(inv.Attachments, seededAttachment{ItemID: created.ID, AttachmentID: withAttachment.Attachments[0].ID, SHA256: sha256Hex(content)})
	}
	inv.ArchivedItem = inv.Items[2]
	must(b.run(nil, "archive", "item", inv.ArchivedItem, "--raw"))

	// The counts of the spike: two items in the default list, one in the
	// archived list, three items and one passkey in the plaintext export.
	if got := listItems(t, must(b.run(nil, "list", "items", "--raw"))); len(got) != 2 {
		t.Fatalf("list items: %d, want 2", len(got))
	}
	if got := listItems(t, must(b.run(nil, "list", "items", "--archived", "--raw"))); len(got) != 1 || got[0].ID != inv.ArchivedItem {
		t.Fatalf("list items --archived: %d items, want the archived one", len(got))
	}
	exported := exportItems(t, must(b.run(nil, "export", "--format", "json", "--raw")))
	if len(exported) != 3 {
		t.Fatalf("export: %d items, want 3", len(exported))
	}
	passkeys := 0
	for _, it := range exported {
		passkeys += len(it.Login.Fido2Credentials)
	}
	if passkeys != 1 {
		t.Fatalf("export: %d passkeys, want 1", passkeys)
	}

	must(b.run(nil, "logout", "--raw"))
	b.session = ""
	removeProfile()
	return inv, profile, importFile
}

type listedItem struct {
	ID    string `json:"id"`
	Login struct {
		Fido2Credentials []struct {
			CredentialID string `json:"credentialId"`
		} `json:"fido2Credentials"`
	} `json:"login"`
}

func listItems(t *testing.T, out []byte) []listedItem {
	t.Helper()
	var items []listedItem
	if err := json.Unmarshal(out, &items); err != nil {
		t.Fatalf("item list: not JSON")
	}
	return items
}

func exportItems(t *testing.T, out []byte) []listedItem {
	t.Helper()
	var doc struct {
		Encrypted bool         `json:"encrypted"`
		Items     []listedItem `json:"items"`
	}
	if err := json.Unmarshal(out, &doc); err != nil || doc.Encrypted {
		t.Fatalf("export: not a plaintext JSON export")
	}
	return doc.Items
}

// passkeyImport returns a Bitwarden JSON import file with one login and one
// passkey of 13 fields, as in the manual backup exercise, and the
// credential ID. The key is a fresh P-256 key in PKCS#8, base64url.
func passkeyImport(t *testing.T) ([]byte, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	credentialID := newUUID(t)
	doc := map[string]any{
		"encrypted": false,
		"folders":   []any{},
		"items": []any{map[string]any{
			"type": 1, "name": "e2e passkey", "favorite": false, "reprompt": 0,
			"login": map[string]any{
				"username": "e2e", "password": nil, "totp": nil,
				"uris": []any{map[string]any{"uri": "https://passkey.example.invalid", "match": nil}},
				"fido2Credentials": []any{map[string]any{
					"credentialId":    credentialID,
					"keyType":         "public-key",
					"keyAlgorithm":    "ECDSA",
					"keyCurve":        "P-256",
					"keyValue":        base64.RawURLEncoding.EncodeToString(pkcs8),
					"rpId":            "passkey.example.invalid",
					"userHandle":      base64.RawURLEncoding.EncodeToString(randomBytes(t, 16)),
					"userName":        "e2e",
					"counter":         "0",
					"rpName":          "E2E",
					"userDisplayName": "E2E",
					"discoverable":    "true",
					"creationDate":    "2026-11-21T00:00:00.000Z",
				}},
			},
		}},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data, credentialID
}

func newUUID(t testing.TB) string {
	b := randomBytes(t, 16)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func TestSeedVault(t *testing.T) {
	s := startServer(t)
	a := newAccount(t)
	if err := a.register(s.Client, s.URL); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	inv, profile, importFile := seedVault(t, s, a)
	t.Logf("seeded %d items and %d attachments in %v", len(inv.Items), len(inv.Attachments), time.Since(start).Round(time.Millisecond))
	for _, p := range []string{profile, importFile} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists: %v", filepath.Base(p), err)
		}
	}
}
