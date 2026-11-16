package archive_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/manifest"
)

// Requirement: Captured documents are stored unchanged.

func TestBothItemListsRoundTrip(t *testing.T) {
	f, id, _, data := intactArchive(t)
	docs := archivetest.Documents(t, f)
	if bytes.Equal(docs.ItemList, docs.ArchivedItemList) {
		t.Fatal("the fixture's two item lists are equal")
	}
	_, got, err := readAll(t, data, id)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]byte{
		archive.ItemListPath:         docs.ItemList,
		archive.ArchivedItemListPath: docs.ArchivedItemList,
	} {
		if sha256.Sum256(got[path]) != sha256.Sum256(want) {
			t.Errorf("%s: SHA-256 differs from the captured list", path)
		}
	}
}

func TestEmptyArchivedItemList(t *testing.T) {
	f := capturetest.Intact(t)
	id := archivetest.NewKey(t)
	docs := archivetest.Documents(t, f)
	docs.ArchivedItemList = []byte("[]")
	var b bytes.Buffer
	w, err := archive.NewWriter(&b, []string{id.Recipient().String()}, archivetest.Created, docs)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := archivetest.Verification(t, f, false)
	if _, err := w.Finish(archivetest.Meta(v)); err != nil {
		t.Fatal(err)
	}
	m, got, err := readAll(t, b.Bytes(), id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got[archive.ArchivedItemListPath]) != "[]" {
		t.Errorf("archived item list = %q, want []", got[archive.ArchivedItemListPath])
	}
	listed := false
	for _, e := range m.Entries {
		if e.Path == archive.ArchivedItemListPath {
			listed = e.Kind == manifest.KindDocument && e.Size == 2
		}
	}
	if !listed {
		t.Error("manifest does not list the empty archived item list as a document of two bytes")
	}
}

// Scenario "Archived item list absent", and the same refusal for each of
// the other three documents: the destination receives no byte.
func TestArchivedItemListAbsent(t *testing.T) {
	recipient := archivetest.NewKey(t).Recipient().String()
	for _, tc := range []struct {
		path  string
		clear func(d *archive.Documents)
	}{
		{archive.ArchivedItemListPath, func(d *archive.Documents) { d.ArchivedItemList = nil }},
		{archive.ItemListPath, func(d *archive.Documents) { d.ItemList = []byte{} }},
		{archive.EncryptedExportPath, func(d *archive.Documents) { d.EncryptedExport = nil }},
		{archive.PlaintextExportPath, func(d *archive.Documents) { d.PlaintextExport = nil }},
	} {
		t.Run(tc.path, func(t *testing.T) {
			docs := archivetest.Documents(t, capturetest.Intact(t))
			tc.clear(&docs)
			var b bytes.Buffer
			w, err := archive.NewWriter(&b, []string{recipient}, archivetest.Created, docs)
			if !errors.Is(err, archive.ErrDocumentAbsent) || w != nil {
				t.Fatalf("writer %v, error %v, want a refusal", w, err)
			}
			if !strings.Contains(err.Error(), tc.path) {
				t.Errorf("error does not name %s: %v", tc.path, err)
			}
			if b.Len() != 0 {
				t.Errorf("destination received %d bytes", b.Len())
			}
		})
	}
}

// Requirement: Unsafe or unknown archive structure is rejected.

// Scenario "Archive without the archived item list": an otherwise
// consistent archive whose manifest lists no such entry.
func TestArchiveWithoutTheArchivedItemList(t *testing.T) {
	id, data := rebuild(t, func(es []rawEntry) []rawEntry {
		i := indexOf(es, archive.ArchivedItemListPath)
		es = append(es[:i:i], es[i+1:]...)
		mi := indexOf(es, archive.ManifestPath)
		m, err := manifest.Parse(es[mi].content)
		if err != nil {
			t.Fatal(err)
		}
		var kept []manifest.Entry
		for _, e := range m.Entries {
			if e.Path != archive.ArchivedItemListPath {
				kept = append(kept, e)
			}
		}
		m.Entries = kept
		if es[mi].content, err = manifest.Marshal(m); err != nil {
			t.Fatal(err)
		}
		return es
	})
	_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
	wantEntryError(t, err, archive.ArchivedItemListPath, archive.ProblemRequired)
	if got := archive.EntryErrors(err); len(got) != 1 {
		t.Errorf("entry errors = %v, want only the missing list", got)
	}
}

// The scenarios "Entry not listed", "Listed entry missing" and "Entry
// differs from the manifest", applied to the archived item list.
func TestReaderScenariosForTheArchivedItemList(t *testing.T) {
	path := archive.ArchivedItemListPath
	t.Run("entry differs from the manifest", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			i := indexOf(es, path)
			es[i].content = append([]byte{}, es[i].content...)
			es[i].content[1] ^= 1
			return es
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, path, archive.ProblemDiffers)
	})
	t.Run("entry not listed", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			mi := indexOf(es, archive.ManifestPath)
			m, err := manifest.Parse(es[mi].content)
			if err != nil {
				t.Fatal(err)
			}
			var kept []manifest.Entry
			for _, e := range m.Entries {
				if e.Path != path {
					kept = append(kept, e)
				}
			}
			m.Entries = kept
			if es[mi].content, err = manifest.Marshal(m); err != nil {
				t.Fatal(err)
			}
			return es
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, path, archive.ProblemNotListed)
	})
	t.Run("listed entry missing", func(t *testing.T) {
		id, data := rebuild(t, func(es []rawEntry) []rawEntry {
			i := indexOf(es, path)
			return append(es[:i:i], es[i+1:]...)
		})
		_, err := archive.Read(bytes.NewReader(data), []age.Identity{id}, nil)
		wantEntryError(t, err, path, archive.ProblemMissing)
	})
}
