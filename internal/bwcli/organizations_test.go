package bwcli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"bw-takeout/internal/bwcli/bwclitest"
)

const (
	testOrg      = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	testOtherOrg = "abababab-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

// Requirement: Organization exports are requested by ID.

func TestExportByOrganizationID(t *testing.T) {
	plain := `{"encrypted":false,"collections":[],"items":[]}`
	sealed := `{"encrypted":true,"encKeyValidation_DO_NOT_EDIT":"2.x|y|z","collections":[],"items":[]}`
	f := newFixture(t, scenario(
		bwclitest.OrganizationExport(testOrg), bwclitest.Behavior{Stdout: plain},
		bwclitest.OrganizationEncryptedExport(testOrg), bwclitest.Behavior{Stdout: sealed}))
	out, err := f.client.ExportOrganizationEncryptedJSON(bg, testOrg)
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Data) != sealed {
		t.Errorf("got %q", out.Data)
	}
	want := []string{"export", "--organizationid", testOrg, "--format", "encrypted_json", "--raw"}
	if rec := f.lastRecord(); !slices.Equal(rec.Args, want) {
		t.Errorf("arguments %q", rec.Args)
	}
	out, err = f.client.ExportOrganizationJSON(bg, testOrg)
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Data) != plain {
		t.Errorf("got %q", out.Data)
	}
	want = []string{"export", "--organizationid", testOrg, "--format", "json", "--raw"}
	if rec := f.lastRecord(); !slices.Equal(rec.Args, want) {
		t.Errorf("arguments %q", rec.Args)
	}
}

func TestOrganizationIDThatStartsWithADash(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stdout: "{}"}))
	for _, id := range []string{"-" + testOrg[1:], "--raw", "../x", "", testOrg[:35], testOrg + "\n"} {
		for _, fn := range []func(string) (Output, error){
			func(id string) (Output, error) { return f.client.ExportOrganizationJSON(bg, id) },
			func(id string) (Output, error) { return f.client.ExportOrganizationEncryptedJSON(bg, id) },
		} {
			_, err := fn(id)
			e := wantClass(t, err, ClassInvalidArgument)
			if e.OrganizationID != "" {
				t.Errorf("refused ID in the failure: %v", e)
			}
		}
	}
	wantNoProcess(t, f)
}

func TestFailedOrganizationExportWithANameInTheErrorText(t *testing.T) {
	f := newFixture(t, scenario(
		bwclitest.OrganizationExport(testOrg), bwclitest.Behavior{Exit: 1, Stdout: sentinel + "\n", Stderr: "Organization " + sentinel + " failed\n"},
		"status", statusUnlocked))
	_, err := f.client.ExportOrganizationJSON(bg, testOrg)
	e := wantClass(t, err, ClassExit)
	if e.Op != OpExportOrganizationJSON || e.OrganizationID != testOrg {
		t.Errorf("failure %+v", e)
	}
	if !strings.Contains(err.Error(), testOrg) {
		t.Errorf("failure text %q lacks the organization ID", err.Error())
	}
	assertNoText(t, err, sentinel)
}

func TestEmptyOrganizationExportNamesTheOrganization(t *testing.T) {
	f := newFixture(t, scenario(bwclitest.OrganizationEncryptedExport(testOrg), bwclitest.Behavior{Stdout: "\n"}))
	_, err := f.client.ExportOrganizationEncryptedJSON(bg, testOrg)
	e := wantClass(t, err, ClassOutput)
	if e.Op != OpExportOrganizationEncrypted || e.OrganizationID != testOrg {
		t.Errorf("failure %+v", e)
	}
}

// Requirement: Organization list is read from its output.

func TestTwoMemberships(t *testing.T) {
	list := `[{"object":"organization","id":"` + testOrg + `","name":"First é","status":2,"type":1,"enabled":true},` +
		`{"id":"` + testOtherOrg + `","name":"Second","status":1,"type":0}]` + "\n"
	f := newFixture(t, scenario(bwclitest.ListOrganizations, bwclitest.Behavior{Stdout: list}))
	orgs, err := f.client.ListOrganizations(bg)
	if err != nil {
		t.Fatal(err)
	}
	want := []Organization{
		{ID: testOrg, Name: "First é", Type: OrganizationAdmin, Status: MembershipConfirmed},
		{ID: testOtherOrg, Name: "Second", Type: OrganizationOwner, Status: MembershipAccepted},
	}
	if !slices.Equal(orgs, want) {
		t.Errorf("got %+v", orgs)
	}
	if rec := f.lastRecord(); !slices.Equal(rec.Args, []string{"list", "organizations", "--raw"}) {
		t.Errorf("arguments %q", rec.Args)
	}
}

func TestEntryWithoutARole(t *testing.T) {
	list := `[{"id":"` + testOrg + `","name":"A","status":2,"type":1},{"id":"` + testOtherOrg + `","name":"B","status":2}]`
	f := newFixture(t, scenario(bwclitest.ListOrganizations, bwclitest.Behavior{Stdout: list}))
	_, err := f.client.ListOrganizations(bg)
	e := wantClass(t, err, ClassOutput)
	if e.Op != OpListOrganizations {
		t.Errorf("operation %s", e.Op)
	}
}

func TestNoMembership(t *testing.T) {
	f := newFixture(t, scenario(bwclitest.ListOrganizations, bwclitest.Behavior{Stdout: "[]"}))
	orgs, err := f.client.ListOrganizations(bg)
	if err != nil || orgs == nil || len(orgs) != 0 {
		t.Errorf("%+v, %v", orgs, err)
	}
}

func TestOrganizationListEntryWithAnIDThatIsNotAUUID(t *testing.T) {
	f := newFixture(t, scenario(bwclitest.ListOrganizations, bwclitest.Behavior{Stdout: `[{"id":"../x","status":2,"type":0}]`}))
	_, err := f.client.ListOrganizations(bg)
	wantClass(t, err, ClassOutput)
}

func TestOrganizationListForms(t *testing.T) {
	entry := func(fields string) string { return `[{"id":"` + testOrg + `"` + fields + `}]` }
	for list, ok := range map[string]bool{
		entry(`,"type":0,"status":2`):             true,
		entry(`,"type":0,"status":2,"name":null`): true,
		entry(`,"type":0,"status":2`) + " \n":     true,
		entry(`,"type":"0","status":2`):           false,
		entry(`,"type":0,"status":2.5`):           false,
		entry(`,"type":0`):                        false,
		entry(`,"type":0,"status":2,"name":1`):    false,
		entry(`,"type":0,"status":2`) + "[]":      false,
		`[null]`:                                  false,
		`null`:                                    false,
		`{}`:                                      false,
		`[{"type":0,"status":2}]`:                 false,
		`[{"id":null,"type":0,"status":2}]`:       false,
		"You are not logged in.":                  false,
	} {
		orgs, got := parseOrganizationList([]byte(list))
		if got != ok {
			t.Errorf("%q: got %v (%+v), want %v", list, got, orgs, ok)
		}
	}
}

func TestOrganizationListOutputIsWiped(t *testing.T) {
	for _, list := range []string{
		`[{"id":"` + testOrg + `","name":"` + sentinel + `","type":1,"status":2}]`,
		`[{"id":"` + testOrg + `","name":"` + sentinel + `","status":2}]`,
	} {
		b := []byte(list)
		parseOrganizationList(b)
		if !bytes.Equal(b, make([]byte, len(b))) {
			t.Errorf("%q: raw output not wiped", list)
		}
	}
}

// The logout hook of the fake (design D9): an organization export without
// the role is answered with an error, bw logs out, and every later command
// fails as not logged in.
func TestOrganizationExportThatLogsOut(t *testing.T) {
	f := newFixture(t, scenario(
		bwclitest.OrganizationExport(testOrg), bwclitest.Behavior{Exit: 1, LogOut: true},
		"status", statusUnlocked,
		"list items --raw", bwclitest.Behavior{Stdout: "[]"}))
	login(f)
	_, err := f.client.ExportOrganizationJSON(bg, testOrg)
	e := wantClass(t, err, ClassNotLoggedIn)
	if e.Op != OpExportOrganizationJSON || e.OrganizationID != testOrg || e.ExitCode != 1 {
		t.Errorf("failure %+v", e)
	}
	_, err = f.client.ListItems(bg)
	wantClass(t, err, ClassNotLoggedIn)
	st, err := f.client.Status(bg)
	if err != nil || st.State != StateUnauthenticated {
		t.Errorf("status %+v, %v", st, err)
	}
}

func TestOrganizationFailureText(t *testing.T) {
	e := &Error{Op: OpExportOrganizationEncrypted, Class: ClassNotLoggedIn, ExitCode: 1, OrganizationID: testOrg}
	want := "bw export_organization_encrypted_json failed: not_logged_in, exit code 1, organization " + testOrg
	if got := e.Error(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
