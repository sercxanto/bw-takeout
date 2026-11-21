package archive_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/capture/capturetest"
)

// Tests of the organization documents (design D7 of the change
// organization-export).

// twoOrganizations returns the intact organization fixture with a second
// organization whose exports are a copy of the first.
func twoOrganizations(t *testing.T) *capturetest.Fixture {
	t.Helper()
	f := capturetest.IntactOrganization(t)
	second := f.Clone().Organizations[0]
	second.ID = capturetest.OrganizationSecond
	f.Organizations = append(f.Organizations, second)
	return f
}

// organizationArchive writes an archive of the fixture and returns its
// identity and bytes.
func organizationArchive(t *testing.T, f *capturetest.Fixture) (*age.X25519Identity, []byte) {
	t.Helper()
	id := archivetest.NewKey(t)
	v, _ := archivetest.Verification(t, f, true)
	return id, archivetest.Build(t, f, v, id.Recipient().String())
}

// rewriteManifest decrypts an archive, lets change edit the manifest as a
// JSON tree and encrypts the result again.
func rewriteManifest(t *testing.T, id *age.X25519Identity, data []byte, change func(map[string]any)) []byte {
	t.Helper()
	entries := decryptEntries(t, data, id)
	i := indexOf(entries, archive.ManifestPath)
	var doc map[string]any
	if err := json.Unmarshal(entries[i].content, &doc); err != nil {
		t.Fatal(err)
	}
	change(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	entries[i].content = out
	return encryptEntries(t, entries, id.Recipient())
}

// Requirement: Captured documents are stored unchanged.

func TestOrganizationDocumentsRoundTrip(t *testing.T) {
	f := twoOrganizations(t)
	id, data := organizationArchive(t, f)
	m, got, err := readAll(t, data, id)
	if err != nil {
		t.Fatal(err)
	}
	docs := archivetest.Documents(t, f)
	if len(docs.Organizations) != 2 {
		t.Fatalf("%d organizations", len(docs.Organizations))
	}
	for _, o := range docs.Organizations {
		for path, want := range map[string][]byte{
			archive.OrganizationPlaintextExportPath(o.ID): o.PlaintextExport,
			archive.OrganizationEncryptedExportPath(o.ID): o.EncryptedExport,
		} {
			if sha256.Sum256(got[path]) != sha256.Sum256(want) {
				t.Errorf("%s differs from the captured bytes", path)
			}
		}
	}
	if len(m.Organizations) != 2 || m.Organizations[0].ID != capturetest.ChosenOrganization || m.Organizations[1].ID != capturetest.OrganizationSecond {
		t.Errorf("manifest organizations %+v", m.Organizations)
	}
}

// The organization documents follow the four documents, in the order the
// organizations were chosen.
func TestOrganizationDocumentOrder(t *testing.T) {
	f := twoOrganizations(t)
	id, data := organizationArchive(t, f)
	var names []string
	for _, e := range decryptEntries(t, data, id)[:9] {
		names = append(names, e.hdr.Name)
	}
	want := []string{
		archive.RecoveryPath, archive.PlaintextExportPath, archive.EncryptedExportPath,
		archive.ItemListPath, archive.ArchivedItemListPath,
		archive.OrganizationPlaintextExportPath(capturetest.ChosenOrganization),
		archive.OrganizationEncryptedExportPath(capturetest.ChosenOrganization),
		archive.OrganizationPlaintextExportPath(capturetest.OrganizationSecond),
		archive.OrganizationEncryptedExportPath(capturetest.OrganizationSecond),
	}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Errorf("entries %q", names)
	}
	if archive.OrganizationPlaintextExportPath(capturetest.ChosenOrganization) != "documents/organizations/"+capturetest.ChosenOrganization+"/export.json" ||
		archive.OrganizationEncryptedExportPath(capturetest.ChosenOrganization) != "documents/organizations/"+capturetest.ChosenOrganization+"/export.encrypted.json" {
		t.Error("entry names differ from design D7")
	}
}

// refused fails unless NewWriter refuses the documents and writes nothing.
func refused(t *testing.T, docs archive.Documents) error {
	t.Helper()
	var b bytes.Buffer
	_, err := archive.NewWriter(&b, []string{archivetest.NewKey(t).Recipient().String()}, archivetest.Created, docs)
	if err == nil {
		t.Fatal("writing was not refused")
	}
	if b.Len() != 0 {
		t.Errorf("%d bytes written before the refusal", b.Len())
	}
	return err
}

func TestOrganizationDocumentAbsent(t *testing.T) {
	for _, which := range []string{"plaintext", "encrypted"} {
		docs := archivetest.Documents(t, capturetest.IntactOrganization(t))
		path := archive.OrganizationEncryptedExportPath(capturetest.ChosenOrganization)
		if which == "plaintext" {
			docs.Organizations[0].PlaintextExport = nil
			path = archive.OrganizationPlaintextExportPath(capturetest.ChosenOrganization)
		} else {
			docs.Organizations[0].EncryptedExport = []byte{}
		}
		err := refused(t, docs)
		if !errors.Is(err, archive.ErrDocumentAbsent) || !strings.Contains(err.Error(), path) {
			t.Errorf("%s: %v", which, err)
		}
	}
}

// Requirement: Entry names come from fixed names and validated IDs.

func TestOrganizationIDThatIsNotAUUID(t *testing.T) {
	for _, bad := range []string{"../x", "aaaaaaaa/bbbb-4ccc-8ddd-eeeeeeeeeeee", "", "-" + capturetest.ChosenOrganization[1:], capturetest.ChosenOrganization + "/"} {
		docs := archivetest.Documents(t, capturetest.IntactOrganization(t))
		docs.Organizations[0].ID = bad
		err := refused(t, docs)
		if !strings.Contains(err.Error(), "not a UUID") || (bad != "" && strings.Contains(err.Error(), bad)) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestDuplicateOrganizationID(t *testing.T) {
	f := twoOrganizations(t)
	f.Organizations[1].ID = capturetest.ChosenOrganization
	err := refused(t, archivetest.Documents(t, f))
	if !strings.Contains(err.Error(), "twice") {
		t.Errorf("got %v", err)
	}
}

// A plaintext organization export that does not parse cannot be counted.
func TestUnreadableOrganizationExport(t *testing.T) {
	docs := archivetest.Documents(t, capturetest.IntactOrganization(t))
	docs.Organizations[0].PlaintextExport = []byte(capturetest.Marker + " not json")
	err := refused(t, docs)
	if !strings.Contains(err.Error(), capturetest.ChosenOrganization) || strings.Contains(err.Error(), capturetest.Marker) {
		t.Errorf("got %v", err)
	}
}

// Requirement: Manifest.

func TestChosenOrganizationInTheManifest(t *testing.T) {
	f := capturetest.IntactOrganization(t)
	o := f.Organization(t, capturetest.ChosenOrganization)
	third := f.Clone().Organization(t, capturetest.ChosenOrganization).PlaintextItems()[1].(map[string]any)
	third["id"] = "77777777-7777-4777-8777-777777777779"
	o.PlaintextExport["items"] = append(o.PlaintextItems(), third)
	id, data := organizationArchive(t, f)
	m, _, err := readAll(t, data, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Organizations) != 1 {
		t.Fatalf("organizations %+v", m.Organizations)
	}
	got := m.Organizations[0]
	if got.ID != capturetest.ChosenOrganization || got.Name != archivetest.OrganizationName(capturetest.ChosenOrganization) || got.Items != 3 {
		t.Errorf("organization %+v", got)
	}
	if m.Counts.Items != 4 {
		t.Errorf("personal count %d, want 4", m.Counts.Items)
	}
}

func TestNoOrganizationChosen(t *testing.T) {
	_, id, _, data := intactArchive(t)
	m, _, err := readAll(t, data, id)
	if err != nil {
		t.Fatal(err)
	}
	if m.Organizations == nil || len(m.Organizations) != 0 {
		t.Errorf("organizations %+v, want an empty list", m.Organizations)
	}
}

// A chosen organization without a name in Meta is stored with an empty
// name.
func TestOrganizationWithoutAName(t *testing.T) {
	f := capturetest.IntactOrganization(t)
	id := archivetest.NewKey(t)
	v, _ := archivetest.Verification(t, f, false)
	var b bytes.Buffer
	aw, err := archive.NewWriter(&b, []string{id.Recipient().String()}, archivetest.Created, archivetest.Documents(t, f))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range archivetest.Attachments(f) {
		if err := aw.AddAttachment(a); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := aw.Finish(archivetest.Meta(v)); err != nil {
		t.Fatal(err)
	}
	m, _, err := readAll(t, b.Bytes(), id)
	if err != nil {
		t.Fatal(err)
	}
	if m.Organizations[0].Name != "" || m.Organizations[0].Items != 2 {
		t.Errorf("organization %+v", m.Organizations[0])
	}
}

// Reader rules of "Organization entries follow the manifest" and "The
// manifest lists the organizations".

func TestReaderScenariosForOrganizations(t *testing.T) {
	org := capturetest.ChosenOrganization
	other := capturetest.OrganizationFirst
	read := func(id *age.X25519Identity, data []byte) error {
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		return err
	}

	t.Run("intact generation with an organization", func(t *testing.T) {
		id, data := organizationArchive(t, capturetest.IntactOrganization(t))
		if err := read(id, data); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("organization document missing", func(t *testing.T) {
		id, data := organizationArchive(t, capturetest.IntactOrganization(t))
		entries := decryptEntries(t, data, id)
		i := indexOf(entries, archive.OrganizationEncryptedExportPath(org))
		entries = append(entries[:i], entries[i+1:]...)
		wantEntryError(t, read(id, encryptEntries(t, entries, id.Recipient())), archive.OrganizationEncryptedExportPath(org), archive.ProblemMissing)
	})

	t.Run("organization document neither stored nor listed", func(t *testing.T) {
		id, data := organizationArchive(t, capturetest.IntactOrganization(t))
		path := archive.OrganizationEncryptedExportPath(org)
		data = rewriteManifest(t, id, data, func(d map[string]any) {
			var kept []any
			for _, e := range d["entries"].([]any) {
				if e.(map[string]any)["path"] != path {
					kept = append(kept, e)
				}
			}
			d["entries"] = kept
		})
		entries := decryptEntries(t, data, id)
		i := indexOf(entries, path)
		entries = append(entries[:i], entries[i+1:]...)
		wantEntryError(t, read(id, encryptEntries(t, entries, id.Recipient())), path, archive.ProblemRequired)
	})

	t.Run("document of an organization that is not listed", func(t *testing.T) {
		id, data := organizationArchive(t, capturetest.IntactOrganization(t))
		path := archive.OrganizationPlaintextExportPath(other)
		entries := decryptEntries(t, data, id)
		entries = insertBefore(entries, archive.ManifestPath, regular(path, `{"encrypted":false,"items":[]}`))
		wantEntryError(t, read(id, encryptEntries(t, entries, id.Recipient())), path, archive.ProblemOrganization)
	})

	t.Run("listed entry of an organization that is not listed", func(t *testing.T) {
		id, data := organizationArchive(t, capturetest.IntactOrganization(t))
		data = rewriteManifest(t, id, data, func(d map[string]any) { d["organizations"] = []any{} })
		wantEntryError(t, read(id, data), archive.OrganizationPlaintextExportPath(org), archive.ProblemOrganization)
	})

	t.Run("organization item count differs", func(t *testing.T) {
		id, data := organizationArchive(t, capturetest.IntactOrganization(t))
		data = rewriteManifest(t, id, data, func(d map[string]any) {
			d["organizations"].([]any)[0].(map[string]any)["items"] = 3
		})
		err := read(id, data)
		if !errors.Is(err, archive.ErrStructure) || !strings.Contains(err.Error(), org) {
			t.Errorf("got %v", err)
		}
		if strings.Contains(err.Error(), capturetest.Marker) {
			t.Errorf("error names the organization: %v", err)
		}
	})

	t.Run("organization entry with another file name", func(t *testing.T) {
		id, data := organizationArchive(t, capturetest.IntactOrganization(t))
		path := "documents/organizations/" + org + "/items.json"
		entries := decryptEntries(t, data, id)
		entries = insertBefore(entries, archive.ManifestPath, regular(path, "[]"))
		wantEntryError(t, read(id, encryptEntries(t, entries, id.Recipient())), path, archive.ProblemUnexpectedName)
	})

	t.Run("manifest without an organization list", func(t *testing.T) {
		_, id, _, data := intactArchive(t)
		data = rewriteManifest(t, id, data, func(d map[string]any) { delete(d, "organizations") })
		err := read(id, data)
		if !errors.Is(err, archive.ErrStructure) || !strings.Contains(err.Error(), "invalid manifest") {
			t.Errorf("got %v", err)
		}
	})
}

// An archive in the layout of ADR 0005 before this change, written entry by
// entry with a manifest of that layout, is refused.
func TestArchiveOfTheEarlierLayout(t *testing.T) {
	_, id, _, data := intactArchive(t)
	entries := decryptEntries(t, data, id)
	i := indexOf(entries, archive.ManifestPath)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(entries[i].content, &doc); err != nil {
		t.Fatal(err)
	}
	// The earlier manifest had exactly these keys, in this order.
	var b bytes.Buffer
	b.WriteString("{\n")
	keys := []string{"format", "format_version", "created", "source", "tool", "bw", "counts", "verification", "entries"}
	for k, key := range keys {
		b.WriteString("  \"" + key + "\": ")
		b.Write(doc[key])
		if k < len(keys)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	entries[i].content = b.Bytes()
	_, err := archive.Read(bytes.NewReader(encryptEntries(t, entries, id.Recipient())), []age.Identity{id}, nil)
	if !errors.Is(err, archive.ErrStructure) || !strings.Contains(err.Error(), "organizations is missing") {
		t.Errorf("got %v", err)
	}
}
