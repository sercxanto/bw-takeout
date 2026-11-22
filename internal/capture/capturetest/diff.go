package capturetest

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

// Diff lists the places where two fixtures differ. It returns one line per
// place, sorted:
//
//	~ path   the value at path changed
//	- path   path exists only in a
//	+ path   path exists only in b
//
// A path starts with "plaintext", "encrypted", "list" or "archived" (the
// archived item list). Object keys follow
// after a slash. Array elements that all carry a distinct "id" are addressed
// as [id], all other array elements by index. A subtree that exists on one
// side only is reported once, at its root. Changed attachment contents and a
// changed reference inventory are reported as "~ attachments" and
// "~ reference", a changed byte override of the archived item list as
// "~ archived-bytes", and changed decryption error counts of the documents
// as "~ decryption-errors". The exports of an organization start with
// "organization[<id>]/plaintext" and "organization[<id>]/encrypted", their
// byte overrides and decryption error counts are reported as
// "~ organization[<id>]/plaintext-bytes", ".../encrypted-bytes" and
// ".../decryption-errors", and an organization on one side only as
// "- organization[<id>]" or "+ organization[<id>]".
func Diff(t testing.TB, a, b *Fixture) []string {
	t.Helper()
	var out []string
	diffTree("plaintext", normalize(t, a.PlaintextExport), normalize(t, b.PlaintextExport), &out)
	diffTree("encrypted", normalize(t, a.EncryptedExport), normalize(t, b.EncryptedExport), &out)
	diffTree("list", normalize(t, a.ItemList), normalize(t, b.ItemList), &out)
	diffTree("archived", normalize(t, a.ArchivedItemList), normalize(t, b.ArchivedItemList), &out)
	if (a.ArchivedItemListBytes == nil) != (b.ArchivedItemListBytes == nil) ||
		!bytes.Equal(a.ArchivedItemListBytes, b.ArchivedItemListBytes) {
		out = append(out, "~ archived-bytes")
	}
	if !reflect.DeepEqual(a.Attachments, b.Attachments) {
		out = append(out, "~ attachments")
	}
	if !reflect.DeepEqual(a.Reference, b.Reference) {
		out = append(out, "~ reference")
	}
	if a.DecryptionErrors != b.DecryptionErrors {
		out = append(out, "~ decryption-errors")
	}
	diffOrganizations(t, a.Organizations, b.Organizations, &out)
	sort.Strings(out)
	return out
}

func diffOrganizations(t testing.TB, a, b []OrganizationFixture, out *[]string) {
	t.Helper()
	index := func(orgs []OrganizationFixture) map[string]*OrganizationFixture {
		m := make(map[string]*OrganizationFixture, len(orgs))
		for i := range orgs {
			m[orgs[i].ID] = &orgs[i]
		}
		return m
	}
	am, bm := index(a), index(b)
	for id, ao := range am {
		path := "organization[" + id + "]"
		bo, ok := bm[id]
		if !ok {
			*out = append(*out, "- "+path)
			continue
		}
		diffTree(path+"/plaintext", normalize(t, ao.PlaintextExport), normalize(t, bo.PlaintextExport), out)
		diffTree(path+"/encrypted", normalize(t, ao.EncryptedExport), normalize(t, bo.EncryptedExport), out)
		for _, bytesOf := range []struct {
			name string
			a, b []byte
		}{
			{"plaintext-bytes", ao.PlaintextExportBytes, bo.PlaintextExportBytes},
			{"encrypted-bytes", ao.EncryptedExportBytes, bo.EncryptedExportBytes},
		} {
			if (bytesOf.a == nil) != (bytesOf.b == nil) || !bytes.Equal(bytesOf.a, bytesOf.b) {
				*out = append(*out, "~ "+path+"/"+bytesOf.name)
			}
		}
		if ao.DecryptionErrors != bo.DecryptionErrors {
			*out = append(*out, "~ "+path+"/decryption-errors")
		}
	}
	for id := range bm {
		if _, ok := am[id]; !ok {
			*out = append(*out, "+ organization["+id+"]")
		}
	}
}

// normalize passes a document through JSON so that both sides of a
// comparison use the same Go types.
func normalize(t testing.TB, doc any) any {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("normalize fixture document: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("normalize fixture document: %v", err)
	}
	return out
}

func diffTree(path string, a, b any, out *[]string) {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			*out = append(*out, "~ "+path)
			return
		}
		for k, child := range av {
			if other, ok := bv[k]; ok {
				diffTree(path+"/"+k, child, other, out)
			} else {
				*out = append(*out, "- "+path+"/"+k)
			}
		}
		for k := range bv {
			if _, ok := av[k]; !ok {
				*out = append(*out, "+ "+path+"/"+k)
			}
		}
	case []any:
		bv, ok := b.([]any)
		if !ok {
			*out = append(*out, "~ "+path)
			return
		}
		aIDs, aKeyed := elementIDs(av)
		bIDs, bKeyed := elementIDs(bv)
		if aKeyed && bKeyed {
			diffKeyed(path, av, bv, aIDs, bIDs, out)
			return
		}
		for i := range av {
			if i < len(bv) {
				diffTree(path+"/"+strconv.Itoa(i), av[i], bv[i], out)
			} else {
				*out = append(*out, "- "+path+"/"+strconv.Itoa(i))
			}
		}
		for i := len(av); i < len(bv); i++ {
			*out = append(*out, "+ "+path+"/"+strconv.Itoa(i))
		}
	default:
		if !reflect.DeepEqual(a, b) {
			*out = append(*out, "~ "+path)
		}
	}
}

func diffKeyed(path string, a, b []any, aIDs, bIDs []string, out *[]string) {
	bIndex := make(map[string]int, len(bIDs))
	for i, id := range bIDs {
		bIndex[id] = i
	}
	aSeen := make(map[string]bool, len(aIDs))
	for i, id := range aIDs {
		aSeen[id] = true
		if j, ok := bIndex[id]; ok {
			diffTree(path+"["+id+"]", a[i], b[j], out)
		} else {
			*out = append(*out, "- "+path+"["+id+"]")
		}
	}
	for _, id := range bIDs {
		if !aSeen[id] {
			*out = append(*out, "+ "+path+"["+id+"]")
		}
	}
}

// elementIDs returns the "id" of every element. The second result is false
// when an element has no string ID or when two elements share one, and for
// an empty array.
func elementIDs(elements []any) ([]string, bool) {
	if len(elements) == 0 {
		return nil, false
	}
	ids := make([]string, 0, len(elements))
	seen := make(map[string]bool, len(elements))
	for _, raw := range elements {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		id, ok := m["id"].(string)
		if !ok || seen[id] {
			return nil, false
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, true
}
