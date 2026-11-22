package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/verify"
)

const hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func verificationJSON(t *testing.T) []byte {
	t.Helper()
	f := capturetest.Intact(t)
	raw, err := verify.Verify(f.Capture(t), &f.Reference).JSON()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sample(t *testing.T) *Manifest {
	t.Helper()
	return &Manifest{
		Created: time.Date(2026, 11, 14, 17, 0, 0, 0, time.UTC),
		Source:  Source{ServerURL: "https://vault.example.test", AccountEmail: "user@example.test", AccountID: "a0000000-0000-4000-8000-000000000001"},
		Tool:    Tool{Name: "bw-takeout", Version: "test"},
		Counts:  Counts{Items: 3, Attachments: 1},
		// The result is indented, so the test also shows that Marshal does
		// not compact it.
		Verification: verificationJSON(t),
		Entries: []Entry{
			{Path: "RECOVERY.txt", Kind: KindRecovery, Size: 10, SHA256: hashA},
			{Path: "documents/export.json", Kind: KindDocument, Size: 0, SHA256: hashA},
			{
				Path: "attachments/" + capturetest.ItemPasskeyLogin + "/" + capturetest.AttachmentFirst,
				Kind: KindAttachment, Size: 5, SHA256: hashA,
				ItemID: capturetest.ItemPasskeyLogin, AttachmentID: capturetest.AttachmentFirst,
				FileName: capturetest.AttachmentFileName,
			},
		},
	}
}

func TestRoundTripKeepsVerificationBytes(t *testing.T) {
	m := sample(t)
	if !bytes.Contains(m.Verification, []byte("\n  ")) {
		t.Fatal("precondition: the supplied result should be indented")
	}
	data, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, m.Verification) {
		t.Error("serialized manifest does not contain the result byte for byte")
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Verification, m.Verification) {
		t.Error("parsed result differs from the supplied bytes")
	}
	if !got.Created.Equal(m.Created) || got.Source != m.Source || got.Tool != m.Tool ||
		got.BW != m.BW || got.Counts != m.Counts || len(got.Entries) != len(m.Entries) {
		t.Errorf("round trip changed the manifest: %+v", got)
	}
	for i := range m.Entries {
		if got.Entries[i] != m.Entries[i] {
			t.Errorf("entry %d: got %+v, want %+v", i, got.Entries[i], m.Entries[i])
		}
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"format", "format_version", "created", "source", "tool", "bw", "counts", "organizations", "verification", "entries"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("key %s missing", k)
		}
	}
	if string(keys["format"]) != `"bw-takeout-generation"` || string(keys["format_version"]) != "1" {
		t.Errorf("format keys: %s %s", keys["format"], keys["format_version"])
	}
	if string(keys["created"]) != `"2026-11-14T17:00:00Z"` {
		t.Errorf("created: %s", keys["created"])
	}
}

func TestMarshalRejectsVerificationThatIsNotAnObject(t *testing.T) {
	for _, raw := range []string{"", "[]", `"verified"`, " {}", "{", `{"a":}`} {
		m := sample(t)
		m.Verification = json.RawMessage(raw)
		if _, err := Marshal(m); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: got %v", raw, err)
		}
	}
}

// replace serializes the sample, applies a change to its generic form and
// returns the result.
func replace(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	data, err := Marshal(sample(t))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	change(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestParseRejectsUnknownFormatVersion(t *testing.T) {
	data := replace(t, func(d map[string]any) { d["format_version"] = 2 })
	_, err := Parse(data)
	var uv *UnsupportedVersionError
	if !errors.As(err, &uv) || uv.Version != 2 {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("message does not say the version is not supported: %v", err)
	}
}

func TestParseIgnoresUnknownKeys(t *testing.T) {
	data := replace(t, func(d map[string]any) {
		d["later_key"] = map[string]any{"x": 1}
		d["source"].(map[string]any)["later_source_key"] = "y"
		d["entries"].([]any)[0].(map[string]any)["later_entry_key"] = true
	})
	m, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 3 {
		t.Errorf("entries: %d", len(m.Entries))
	}
}

func TestParseRejectsInvalidManifests(t *testing.T) {
	entry := func(i int) func(map[string]any) map[string]any {
		return func(d map[string]any) map[string]any { return d["entries"].([]any)[i].(map[string]any) }
	}
	cases := map[string]func(map[string]any){
		"duplicate path": func(d map[string]any) {
			entry(1)(d)["path"] = "RECOVERY.txt"
		},
		"negative size":      func(d map[string]any) { entry(0)(d)["size"] = -1 },
		"uppercase hash":     func(d map[string]any) { entry(0)(d)["sha256"] = strings.ToUpper(hashA) },
		"short hash":         func(d map[string]any) { entry(0)(d)["sha256"] = hashA[:63] },
		"long hash":          func(d map[string]any) { entry(0)(d)["sha256"] = hashA + "a" },
		"non-hex hash":       func(d map[string]any) { entry(0)(d)["sha256"] = "g" + hashA[1:] },
		"empty hash":         func(d map[string]any) { entry(0)(d)["sha256"] = "" },
		"unknown kind":       func(d map[string]any) { entry(0)(d)["kind"] = "symlink" },
		"missing kind":       func(d map[string]any) { delete(entry(0)(d), "kind") },
		"empty path":         func(d map[string]any) { entry(0)(d)["path"] = "" },
		"wrong format":       func(d map[string]any) { d["format"] = "other" },
		"missing format":     func(d map[string]any) { delete(d, "format") },
		"missing version":    func(d map[string]any) { delete(d, "format_version") },
		"bad created":        func(d map[string]any) { d["created"] = "yesterday" },
		"no verification":    func(d map[string]any) { delete(d, "verification") },
		"verification list":  func(d map[string]any) { d["verification"] = []any{} },
		"size as text":       func(d map[string]any) { entry(0)(d)["size"] = "10" },
		"no organizations":   func(d map[string]any) { delete(d, "organizations") },
		"organizations null": func(d map[string]any) { d["organizations"] = nil },
		"organization ID not a UUID": func(d map[string]any) {
			d["organizations"] = []any{map[string]any{"id": "../x", "name": "a", "items": 0}}
		},
		"organization twice": func(d map[string]any) {
			o := map[string]any{"id": capturetest.ChosenOrganization, "name": "a", "items": 0}
			d["organizations"] = []any{o, o}
		},
		"negative organization count": func(d map[string]any) {
			d["organizations"] = []any{map[string]any{"id": capturetest.ChosenOrganization, "name": "a", "items": -1}}
		},
		"organization count as text": func(d map[string]any) {
			d["organizations"] = []any{map[string]any{"id": capturetest.ChosenOrganization, "name": "a", "items": "1"}}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(replace(t, change))
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
	if _, err := Parse([]byte("not json")); !errors.Is(err, ErrInvalid) {
		t.Errorf("not json: %v", err)
	}
}

func TestParseErrorsCarryNoFileName(t *testing.T) {
	data := replace(t, func(d map[string]any) {
		d["entries"].([]any)[2].(map[string]any)["size"] = -1
	})
	_, err := Parse(data)
	if err == nil || strings.Contains(err.Error(), capturetest.Marker) {
		t.Fatalf("got %v", err)
	}
}

// A manifest with and without organizations round-trips byte for byte, and
// the organizations follow the counts.
func TestRoundTripWithAndWithoutOrganizations(t *testing.T) {
	for name, orgs := range map[string][]Organization{
		"none": nil,
		"two": {
			{ID: capturetest.ChosenOrganization, Name: capturetest.Marker + " org é", Items: 3},
			{ID: capturetest.OrganizationSecond, Name: "", Items: 0},
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := sample(t)
			m.Organizations = orgs
			data, err := Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Organizations) != len(orgs) || got.Organizations == nil {
				t.Fatalf("organizations %+v, want %+v", got.Organizations, orgs)
			}
			for i := range orgs {
				if got.Organizations[i] != orgs[i] {
					t.Errorf("organization %d: %+v, want %+v", i, got.Organizations[i], orgs[i])
				}
			}
			again, err := Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(again, data) {
				t.Errorf("second serialization differs:\n%s\n%s", again, data)
			}
			counts := bytes.Index(data, []byte(`"counts":`))
			organizations := bytes.Index(data, []byte(`"organizations":`))
			verification := bytes.Index(data, []byte(`"verification":`))
			if !(counts < organizations && organizations < verification) {
				t.Errorf("organizations not between counts and verification")
			}
			if orgs == nil && !bytes.Contains(data, []byte(`"organizations": []`)) {
				t.Errorf("no empty organization list in %s", data)
			}
		})
	}
}

// A manifest of the layout before organizations is refused as invalid, and
// version 2 as unsupported.
func TestManifestWithoutOrganizationsIsRefused(t *testing.T) {
	_, err := Parse(replace(t, func(d map[string]any) { delete(d, "organizations") }))
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "organizations") {
		t.Errorf("got %v", err)
	}
	_, err = Parse(replace(t, func(d map[string]any) { delete(d, "organizations"); d["format_version"] = 2 }))
	var uv *UnsupportedVersionError
	if !errors.As(err, &uv) {
		t.Errorf("version 2: got %v", err)
	}
}

// Marshal refuses what Parse would refuse, and no message names an
// organization name.
func TestMarshalRefusesInvalidOrganizations(t *testing.T) {
	for _, orgs := range [][]Organization{
		{{ID: "not-a-uuid", Name: capturetest.Marker}},
		{{ID: capturetest.ChosenOrganization, Name: capturetest.Marker}, {ID: capturetest.ChosenOrganization, Name: capturetest.Marker}},
		{{ID: capturetest.ChosenOrganization, Name: capturetest.Marker, Items: -1}},
	} {
		m := sample(t)
		m.Organizations = orgs
		_, err := Marshal(m)
		if !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), capturetest.Marker) {
			t.Errorf("%+v: got %v", orgs, err)
		}
	}
}
