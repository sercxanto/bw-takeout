package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/verify"
)

// Offline verification of generations with a chosen organization (the
// offline-verification delta of the change organization-export).

// threeItemOrganization returns the intact organization fixture whose
// organization holds three items in both exports.
func threeItemOrganization(t *testing.T) *capturetest.Fixture {
	t.Helper()
	f := capturetest.IntactOrganization(t)
	copyOf := f.Clone().Organization(t, capturetest.ChosenOrganization)
	o := f.Organization(t, capturetest.ChosenOrganization)
	const third = "77777777-7777-4777-8777-777777777779"
	plain := copyOf.PlaintextItems()[1].(map[string]any)
	sealed := copyOf.EncryptedItems()[1].(map[string]any)
	plain["id"], sealed["id"] = third, third
	o.PlaintextExport["items"] = append(o.PlaintextItems(), plain)
	o.EncryptedExport["items"] = append(o.EncryptedItems(), sealed)
	listed := f.Clone().ItemList
	for _, raw := range listed {
		if item := raw.(map[string]any); item["id"] == capturetest.ItemOrgNote {
			item["id"] = third
			f.ItemList = append(f.ItemList, item)
		}
	}
	f.Reference.Items = append(f.Reference.Items, capturetest.ReferenceItem{ItemID: third})
	return f
}

// publishFixture publishes a generation of the fixture into a new target.
func publishFixture(t *testing.T, f *capturetest.Fixture) generation {
	t.Helper()
	v, _ := archivetest.Verification(t, f, true)
	id := archivetest.NewKey(t)
	target := t.TempDir()
	pub, err := (&publish.Publisher{}).Publish(target, func(w io.Writer, _ time.Time) ([]byte, error) {
		return archivetest.WriteTo(t, w, f, v, id.Recipient().String()), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return generation{f: f, target: target, dir: pub.Path, identity: writeIdentity(t, id), id: id}
}

type tarEntry struct {
	hdr     tar.Header
	content []byte
}

// rewriteEntries decrypts the archive of the generation, lets change edit
// its entry list and encrypts the result again.
func (g generation) rewriteEntries(t *testing.T, change func([]tarEntry) []tarEntry) {
	t.Helper()
	plain, err := age.Decrypt(bytes.NewReader(g.archive(t)), g.id)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(plain)
	var entries []tarEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		c, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, tarEntry{*hdr, c})
	}
	entries = change(entries)
	var out bytes.Buffer
	enc, err := age.Encrypt(&out, g.id.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(enc)
	for _, e := range entries {
		h := e.hdr
		h.Size = int64(len(e.content))
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	g.replaceArchive(t, out.Bytes())
}

// changeManifest edits the manifest of the generation as a JSON tree.
func (g generation) changeManifest(t *testing.T, change func(map[string]any)) {
	t.Helper()
	g.replaceArchive(t, archivetest.Rewrite(t, g.archive(t), g.id, func(name string, content []byte) []byte {
		if name != archive.ManifestPath {
			return content
		}
		var doc map[string]any
		if err := json.Unmarshal(content, &doc); err != nil {
			t.Fatal(err)
		}
		change(doc)
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}))
}

func (g generation) verify(t *testing.T) outcome {
	t.Helper()
	return runCommand(t, nil, "verify", "--identity", g.identity, g.dir)
}

// wantNoName fails when the output holds an organization name or a
// sentinel.
func wantNoName(t *testing.T, o outcome) {
	t.Helper()
	out := o.stdout + o.stderr
	if strings.Contains(out, archivetest.OrganizationName(capturetest.ChosenOrganization)) || strings.Contains(out, capturetest.Marker) {
		t.Errorf("output holds the organization name or a sentinel\n%s", o)
	}
}

// Requirement: Organization entries follow the manifest.

func TestIntactGenerationWithAnOrganization(t *testing.T) {
	g := publishFixture(t, capturetest.IntactOrganization(t))
	o := g.verify(t)
	if o.code != exitVerified || o.stderr != "" {
		t.Fatal(o)
	}
	wantNoName(t, o)
}

func TestOrganizationDocumentMissing(t *testing.T) {
	g := publishFixture(t, capturetest.IntactOrganization(t))
	path := archive.OrganizationEncryptedExportPath(capturetest.ChosenOrganization)
	g.rewriteEntries(t, func(es []tarEntry) []tarEntry {
		var kept []tarEntry
		for _, e := range es {
			if e.hdr.Name != path {
				kept = append(kept, e)
			}
		}
		return kept
	})
	o := g.verify(t)
	if o.code != exitFailed || !strings.Contains(o.stderr, path) || !strings.Contains(o.stderr, archive.ProblemMissing) {
		t.Fatal(o)
	}
}

func TestDocumentOfAnOrganizationThatIsNotListed(t *testing.T) {
	g := publishFixture(t, capturetest.IntactOrganization(t))
	path := archive.OrganizationPlaintextExportPath(capturetest.OrganizationFirst)
	g.rewriteEntries(t, func(es []tarEntry) []tarEntry {
		last := es[len(es)-1]
		extra := tarEntry{tar.Header{Typeflag: tar.TypeReg, Name: path, Mode: 0o600}, []byte(`{"encrypted":false,"items":[]}`)}
		return append(append(es[:len(es)-1:len(es)-1], extra), last)
	})
	o := g.verify(t)
	if o.code != exitFailed || !strings.Contains(o.stderr, path) || !strings.Contains(o.stderr, archive.ProblemOrganization) {
		t.Fatal(o)
	}
}

func TestOrganizationItemCountDiffers(t *testing.T) {
	g := publishFixture(t, threeItemOrganization(t))
	g.changeManifest(t, func(d map[string]any) {
		d["organizations"].([]any)[0].(map[string]any)["items"] = 2
	})
	o := g.verify(t)
	if o.code != exitFailed || !strings.Contains(o.stderr, capturetest.ChosenOrganization) {
		t.Fatal(o)
	}
	wantNoName(t, o)
}

// Requirement: The manifest lists the organizations.

func TestGenerationWithoutAnOrganization(t *testing.T) {
	g := publishGeneration(t, verify.Verified)
	o := g.verify(t)
	if o.code != exitVerified || strings.Contains(o.stdout, "Organization") {
		t.Fatal(o)
	}
}

func TestManifestWithoutAnOrganizationList(t *testing.T) {
	g := publishGeneration(t, verify.Verified)
	g.changeManifest(t, func(d map[string]any) { delete(d, "organizations") })
	o := g.verify(t)
	if o.code != exitFailed || !strings.Contains(o.stderr, "invalid manifest") {
		t.Fatal(o)
	}
}

// Requirement: The stored verdict is reported.

func TestGenerationWithAnOrganization(t *testing.T) {
	g := publishFixture(t, threeItemOrganization(t))
	o := g.verify(t)
	if o.code != exitVerified {
		t.Fatal(o)
	}
	want := "Attachments: 4\nOrganization: " + capturetest.ChosenOrganization + ", 3 items\nArchive:"
	if !strings.Contains(o.stdout, want) {
		t.Errorf("output lacks %q\n%s", want, o)
	}
	wantNoName(t, o)
}
