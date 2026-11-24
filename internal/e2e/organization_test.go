//go:build vaultwarden && linux

package e2e

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/publish"
)

// The organization setup of design D12 of the change organization-export:
// an owner creates an organization with two collections over the API,
// invites an admin and a user assigned to C1 only, confirms both, and seeds
// one item with an attachment in each collection and one item in the trash
// through bw. These calls worked against Vaultwarden 1.37.4 in the spike
// of 21 November 2026 (docs/research/organization-export-spike-2026-11-21.md).

// Organization member types and statuses of the Vaultwarden API.
const (
	memberAdmin     = 1
	memberUser      = 2
	statusAccepted  = 1
	statusConfirmed = 2
)

// orgSetup is a seeded organization and its members.
type orgSetup struct {
	server             *vaultServer
	owner, admin, user *account
	orgID              string
	c1, c2             string
	// items are the item of C1 and the item of C2, each with one
	// attachment. trash is the deleted item of C1.
	items       []string
	trash       string
	attachments []seededAttachment
}

// wrapForMember encrypts the organization key for an account as an
// EncString of type 4: RSA-OAEP with SHA-1 under the account's public key.
func wrapForMember(orgKey []byte, a *account) (string, error) {
	der, err := base64.StdEncoding.DecodeString(a.keys.PublicKey)
	if err != nil {
		return "", err
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return "", err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("the public key is not RSA")
	}
	ct, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, rsaPub, orgKey, nil)
	if err != nil {
		return "", err
	}
	return "4." + base64.StdEncoding.EncodeToString(ct), nil
}

// registered creates and registers one account.
func registered(t *testing.T, s *vaultServer) *account {
	t.Helper()
	a := newAccount(t)
	if err := a.register(s.Client, s.URL); err != nil {
		t.Fatal(err)
	}
	return a
}

// createOrganization runs steps 2 to 5 of design D12 as the owner and
// returns the organization ID and the IDs of C1 and C2.
func createOrganization(t *testing.T, s *vaultServer, owner *account, members map[*account]int) (orgID, c1, c2 string) {
	t.Helper()
	api, err := owner.login(s.Client, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	orgKeyBytes := randomBytes(t, 64)
	keepOutOfLog("organization key", base64.StdEncoding.EncodeToString(orgKeyBytes))
	orgKey := splitKey(orgKeyBytes)

	collectionName, err := encString(orgKey, []byte("C1"))
	if err != nil {
		t.Fatal(err)
	}
	ownerKey, err := wrapForMember(orgKeyBytes, owner)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	keepOutOfLog("organization private key", base64.StdEncoding.EncodeToString(priv))
	encPriv, err := encString(orgKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	var org struct {
		ID string `json:"id"`
	}
	if err := api.do(http.MethodPost, "/api/organizations", map[string]any{
		"name":           "e2e organization",
		"billingEmail":   owner.Email,
		"collectionName": collectionName,
		"key":            ownerKey,
		"keys":           map[string]any{"publicKey": base64.StdEncoding.EncodeToString(pub), "encryptedPrivateKey": encPriv},
		"planType":       0,
	}, &org); err != nil {
		t.Fatalf("create the organization: %v", err)
	}
	if org.ID == "" {
		t.Fatal("create the organization: no ID")
	}
	orgID = org.ID
	base := "/api/organizations/" + url.PathEscape(orgID)

	var collections struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := api.do(http.MethodGet, base+"/collections", nil, &collections); err != nil {
		t.Fatalf("list the collections: %v", err)
	}
	if len(collections.Data) != 1 {
		t.Fatalf("%d collections after the creation, want the default one", len(collections.Data))
	}
	c1 = collections.Data[0].ID
	secondName, err := encString(orgKey, []byte("C2"))
	if err != nil {
		t.Fatal(err)
	}
	var second struct {
		ID string `json:"id"`
	}
	if err := api.do(http.MethodPost, base+"/collections", map[string]any{"name": secondName, "groups": []any{}, "users": []any{}}, &second); err != nil {
		t.Fatalf("create C2: %v", err)
	}
	if second.ID == "" {
		t.Fatal("create C2: no ID")
	}
	c2 = second.ID

	for a, typ := range members {
		if err := api.do(http.MethodPost, base+"/users/invite", map[string]any{
			"emails": []string{a.Email}, "groups": []any{}, "type": typ, "accessAll": false, "permissions": map[string]any{},
			"collections": []any{map[string]any{"id": c1, "readOnly": false, "hidePasswords": false, "manage": false}},
		}, nil); err != nil {
			t.Fatalf("invite a member of type %d: %v", typ, err)
		}
	}
	var users struct {
		Data []struct {
			ID     string `json:"id"`
			Email  string `json:"email"`
			Status int    `json:"status"`
		} `json:"data"`
	}
	if err := api.do(http.MethodGet, base+"/users", nil, &users); err != nil {
		t.Fatalf("list the members: %v", err)
	}
	confirmed := 0
	for _, u := range users.Data {
		for a := range members {
			if u.Email != a.Email {
				continue
			}
			if u.Status != statusAccepted {
				t.Fatalf("an invited member has status %d, want %d (accepted)", u.Status, statusAccepted)
			}
			key, err := wrapForMember(orgKeyBytes, a)
			if err != nil {
				t.Fatal(err)
			}
			if err := api.do(http.MethodPost, base+"/users/"+url.PathEscape(u.ID)+"/confirm", map[string]any{"key": key}, nil); err != nil {
				t.Fatalf("confirm a member: %v", err)
			}
			confirmed++
		}
	}
	if confirmed != len(members) {
		t.Fatalf("%d members confirmed, want %d", confirmed, len(members))
	}
	return orgID, c1, c2
}

// loggedInBW logs a seed bw in as the account, in a profile of its own,
// and logs it out and removes the profile at the end of the test.
func loggedInBW(t *testing.T, s *vaultServer, a *account) *seedBW {
	t.Helper()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	home := filepath.Join(root, "home")
	for _, d := range []string{profile, home} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	b := &seedBW{
		bw: bwPath(t), profile: profile, fresh: true,
		env: []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + home,
			"BITWARDENCLI_APPDATA_DIR=" + profile,
			"NODE_EXTRA_CA_CERTS=" + s.CAFile,
			"BW_NOINTERACTION=true",
			seedPasswordVar + "=" + a.Password,
		},
	}
	if _, err := b.run(nil, "config", "server", s.URL); err != nil {
		t.Fatal(err)
	}
	session, err := b.run(nil, "login", a.Email, "--passwordenv", seedPasswordVar, "--raw")
	if err != nil {
		t.Fatal(err)
	}
	if len(session) != 88 {
		t.Fatalf("login: a session key of %d bytes, want 88", len(session))
	}
	b.session = string(session)
	keepOutOfLog("session key", b.session)
	t.Cleanup(func() {
		if _, err := b.run(nil, "logout", "--raw"); err != nil {
			t.Errorf("logout: %v", err)
		}
		if err := os.RemoveAll(profile); err != nil {
			t.Errorf("profile: %v", err)
		}
	})
	return b
}

// createOrgItem creates a login of the organization in a collection and
// returns its ID.
func createOrgItem(t *testing.T, b *seedBW, orgID, collection, name string) string {
	t.Helper()
	item, err := json.Marshal(map[string]any{
		"type": 1, "name": name, "notes": nil, "favorite": false, "reprompt": 0,
		"fields": []any{}, "folderId": nil, "organizationId": orgID, "collectionIds": []string{collection},
		"login": map[string]any{"username": "e2e", "password": nil, "totp": nil, "uris": []any{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := b.run(nil, "create", "item", base64.StdEncoding.EncodeToString(item), "--organizationid", orgID, "--raw")
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID             string `json:"id"`
		OrganizationID string `json:"organizationId"`
	}
	if err := json.Unmarshal(out, &created); err != nil || created.ID == "" || created.OrganizationID != orgID {
		t.Fatalf("create item: no item of the organization")
	}
	return created.ID
}

// seedOrganization runs the whole setup of design D12 with an owner, an
// admin and a user.
func seedOrganization(t *testing.T) *orgSetup {
	t.Helper()
	s := startServer(t)
	o := &orgSetup{server: s, owner: registered(t, s), admin: registered(t, s), user: registered(t, s)}
	o.orgID, o.c1, o.c2 = createOrganization(t, s, o.owner, map[*account]int{o.admin: memberAdmin, o.user: memberUser})

	b := loggedInBW(t, s, o.owner)
	files := t.TempDir()
	for i, collection := range []string{o.c1, o.c2} {
		id := createOrgItem(t, b, o.orgID, collection, fmt.Sprintf("e2e organization item %d", i+1))
		o.items = append(o.items, id)
		content := randomBytes(t, attachmentSize)
		file := filepath.Join(files, fmt.Sprintf("org-attachment-%d.bin", i+1))
		if err := os.WriteFile(file, content, 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := b.run(nil, "create", "attachment", "--file", file, "--itemid", id, "--raw")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		var withAttachment struct {
			Attachments []struct {
				ID string `json:"id"`
			} `json:"attachments"`
		}
		if err := json.Unmarshal(out, &withAttachment); err != nil || len(withAttachment.Attachments) != 1 {
			t.Fatal("create attachment: want 1 attachment on the item")
		}
		o.attachments = append(o.attachments, seededAttachment{ItemID: id, AttachmentID: withAttachment.Attachments[0].ID, SHA256: sha256Hex(content)})
	}
	o.trash = createOrgItem(t, b, o.orgID, o.c1, "e2e organization item in the trash")
	if _, err := b.run(nil, "delete", "item", o.trash); err != nil {
		t.Fatal(err)
	}
	if _, err := b.run(nil, "sync"); err != nil {
		t.Fatal(err)
	}
	return o
}

// listedOrganization is one entry of bw list organizations.
type listedOrganization struct {
	ID     string `json:"id"`
	Type   int    `json:"type"`
	Status int    `json:"status"`
}

// TestOrganizationSetup checks the setup of design D12 from the view of
// the admin: both items in the local list, and the role admin with a
// confirmed membership.
func TestOrganizationSetup(t *testing.T) {
	o := seedOrganization(t)
	b := loggedInBW(t, o.server, o.admin)
	out, err := b.run(nil, "list", "items", "--raw")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, it := range listItems(t, out) {
		seen[it.ID] = true
	}
	if !seen[o.items[0]] {
		t.Fatal("the admin does not see the item of C1")
	}
	if !seen[o.items[1]] {
		t.Fatal("the admin does not see the item of C2, which the spike observed: stop and ask the maintainer")
	}
	if seen[o.trash] || len(seen) != 2 {
		t.Fatalf("the admin's list holds %d items, want the two items outside the trash", len(seen))
	}
	out, err = b.run(nil, "list", "organizations", "--raw")
	if err != nil {
		t.Fatal(err)
	}
	var orgs []listedOrganization
	if err := json.Unmarshal(out, &orgs); err != nil {
		t.Fatal("list organizations: not JSON")
	}
	if len(orgs) != 1 || orgs[0].ID != o.orgID || orgs[0].Type != memberAdmin || orgs[0].Status != statusConfirmed {
		t.Fatalf("list organizations: %+v, want the organization with type %d and status %d", orgs, memberAdmin, statusConfirmed)
	}
}

// orgGeneration is what the organization scenario reads from a generation:
// the item IDs of each organization document, the SHA-256 of each
// attachment entry and the item IDs of the personal plaintext export.
type orgGeneration struct {
	plainIDs, sealedIDs []string
	personalIDs         []string
	attachments         map[string]string
}

func readOrgGeneration(t *testing.T, gen, identityFile, orgID string) *orgGeneration {
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
	g := &orgGeneration{attachments: map[string]string{}}
	itemIDs := func(data []byte, encrypted bool) ([]string, error) {
		var doc struct {
			Encrypted bool `json:"encrypted"`
			Items     []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &doc); err != nil || doc.Encrypted != encrypted {
			return nil, errors.New("not an export of the expected form")
		}
		var out []string
		for _, it := range doc.Items {
			out = append(out, it.ID)
		}
		return out, nil
	}
	seen := map[string]bool{}
	_, err = archive.Read(f, ids, func(path string, content io.Reader) error {
		data, err := io.ReadAll(content)
		if err != nil {
			return err
		}
		seen[path] = true
		switch {
		case strings.HasPrefix(path, "attachments/"):
			g.attachments[path] = sha256Hex(data)
		case path == archive.PlaintextExportPath:
			g.personalIDs, err = itemIDs(data, false)
		case path == archive.OrganizationPlaintextExportPath(orgID):
			g.plainIDs, err = itemIDs(data, false)
		case path == archive.OrganizationEncryptedExportPath(orgID):
			g.sealedIDs, err = itemIDs(data, true)
		}
		if err != nil {
			return fmt.Errorf("%s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read the generation: %v", err)
	}
	for _, p := range []string{archive.OrganizationPlaintextExportPath(orgID), archive.OrganizationEncryptedExportPath(orgID)} {
		if !seen[p] {
			t.Errorf("the generation holds no %s", p)
		}
	}
	return g
}

func sameIDs(got, want []string) bool {
	a, b := slices.Clone(got), slices.Clone(want)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// TestOrganizationBackupIsVerified is the admin run of design D12: the
// admin, assigned to C1 only, backs up the whole organization.
func TestOrganizationBackupIsVerified(t *testing.T) {
	o := seedOrganization(t)
	r := runBackupWith(t, o.server, o.admin, nil, backupLimit, bwPath(t), "--organization", o.orgID)
	t.Logf("report:\n%s", r.redacted(o.admin))
	if r.ExitCode != 0 {
		t.Fatalf("exit status %d, want 0", r.ExitCode)
	}
	if v, _ := r.reportLine("Verdict:"); v != "verified" {
		t.Fatalf("verdict %q, want verified", v)
	}
	if v, _ := r.reportLine("Organizations:"); v != o.orgID {
		t.Errorf("setup line names %q, want the organization", v)
	}
	gen, _ := r.reportLine("Generation:")
	if filepath.Dir(gen) != r.Target {
		t.Fatalf("the generation line names %q, not a path in the target", gen)
	}
	verify := exec.Command(takeout, "verify", "--identity", r.Identity, gen)
	out, err := verify.CombinedOutput()
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, redactAddress(string(out), o.admin.Email))
	}
	if want := "Organization: " + o.orgID + ", 2 items\n"; !strings.Contains(string(out), want) {
		t.Errorf("verify does not print %q", want)
	}

	g := readOrgGeneration(t, gen, r.Identity, o.orgID)
	// The reference inventory of the scenario: both organization items.
	if !sameIDs(g.plainIDs, o.items) || !sameIDs(g.sealedIDs, o.items) {
		t.Errorf("the organization exports hold %d and %d items, want the two seeded ones", len(g.plainIDs), len(g.sealedIDs))
	}
	for _, list := range [][]string{g.plainIDs, g.sealedIDs} {
		if slices.Contains(list, o.trash) {
			t.Error("an organization export holds the item in the trash")
		}
	}
	if len(g.personalIDs) != 0 {
		t.Errorf("the personal export of the admin holds %d items, want none", len(g.personalIDs))
	}
	for _, want := range o.attachments {
		p := archive.AttachmentPath(want.ItemID, want.AttachmentID)
		if sum, ok := g.attachments[p]; !ok || sum != want.SHA256 {
			t.Errorf("%s: missing or with another SHA-256", p)
		}
	}
	if len(g.attachments) != len(o.attachments) {
		t.Errorf("%d attachment entries, want %d", len(g.attachments), len(o.attachments))
	}
	r.wantNoUnknownExportKeys(t)
	r.wantEmptyProfileLocation(t)
	t.Logf("backup of the organization in %v", r.Duration.Round(time.Millisecond))
}

// recordingBW writes a shell script that appends the argument list of each
// start to a file and then runs the real bw. The arguments of bw-takeout
// hold no secret: the session key is in the environment.
func recordingBW(t *testing.T) (script, record string) {
	t.Helper()
	dir := t.TempDir()
	script = filepath.Join(dir, "bw")
	record = filepath.Join(dir, "starts.txt")
	text := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + record + "'\nexec '" + bwPath(t) + "' \"$@\"\n"
	if err := os.WriteFile(script, []byte(text), 0o700); err != nil {
		t.Fatal(err)
	}
	return script, record
}

// TestOrganizationRoleIsRefused is the user run of design D12: the role
// check ends the run before any organization export, so bw is never logged
// out by the 401 of Vaultwarden.
func TestOrganizationRoleIsRefused(t *testing.T) {
	o := seedOrganization(t)
	bw, record := recordingBW(t)
	r := runBackupWith(t, o.server, o.user, nil, backupLimit, bw, "--organization", o.orgID)
	t.Logf("report:\n%s", r.redacted(o.user))
	if r.ExitCode != 1 {
		t.Fatalf("exit status %d, want 1", r.ExitCode)
	}
	want := "snapshot: organization " + o.orgID + ": the account is neither owner nor admin"
	if v, _ := r.reportLine("Failed:"); v != want {
		t.Fatalf("failure line %q, want %q", v, want)
	}
	entries, err := os.ReadDir(r.Target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the target holds %d entries, want none", len(entries))
	}
	starts, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(starts), "list organizations --raw\n") {
		t.Error("the organization list was not read")
	}
	if strings.Contains(string(starts), "--organizationid") {
		t.Error("an organization export was started")
	}
	r.wantEmptyProfileLocation(t)
}
