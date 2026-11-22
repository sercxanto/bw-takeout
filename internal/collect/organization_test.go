package collect_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/bwcli/bwclitest"
	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/collect"
	"bw-takeout/internal/publish"
)

// Tests of the chosen organizations against the fake bw (designs D2, D3,
// D6 and D8 of the change organization-export).

// secondOrganization is a second chosen organization with empty exports.
const secondOrganization = ct.OrganizationSecond

const (
	emptyOrgPlain  = `{"encrypted":false,"collections":[],"items":[]}`
	emptyOrgSealed = `{"encrypted":true,"encKeyValidation_DO_NOT_EDIT":"2.a|b|c","collections":[],"items":[]}`
)

// orgEntry renders one entry of bw list organizations in the form of
// 2026.10.1.
func orgEntry(id, name string, typ, status int) string {
	return fmt.Sprintf(`{"object":"organization","id":%q,"name":%q,"status":%d,"type":%d,"enabled":true}`, id, name, status, typ)
}

func orgList(entries ...string) bwclitest.Behavior {
	return bwclitest.Behavior{Stdout: "[" + strings.Join(entries, ",") + "]"}
}

// orgHarness runs the intact organization fixture with the given
// organization list and chosen organizations. A second chosen organization
// gets empty exports.
func orgHarness(t *testing.T, list bwclitest.Behavior, chosen ...string) (*harness, *ct.Fixture) {
	t.Helper()
	f := ct.IntactOrganization(t)
	s := fakeScenario(t, f)
	s.Commands[bwclitest.ListOrganizations] = list
	s.Commands[bwclitest.OrganizationExport(secondOrganization)] = bwclitest.Behavior{Stdout: emptyOrgPlain}
	s.Commands[bwclitest.OrganizationEncryptedExport(secondOrganization)] = bwclitest.Behavior{Stdout: emptyOrgSealed}
	h := newHarness(t, s)
	h.collector.Organizations = chosen
	return h, f
}

// wantNoDocument fails when a document or an attachment was requested.
func (h *harness) wantNoDocument() {
	h.t.Helper()
	for _, c := range h.commands() {
		if strings.HasPrefix(c, "export") || strings.HasPrefix(c, "list items") || strings.HasPrefix(c, "get") {
			h.t.Errorf("document requested after the refusal: %s", c)
		}
	}
}

func wantOrganizationError(t *testing.T, err error, id string, reason collect.OrganizationReason) {
	t.Helper()
	var e *collect.OrganizationError
	if !errors.As(err, &e) || e.ID != id || e.Reason != reason || !errors.Is(err, collect.ErrOrganization) {
		t.Fatalf("error %v, want organization %s refused for %s", err, id, reason)
	}
	if !strings.Contains(err.Error(), id) {
		t.Errorf("error text %q lacks the organization ID", err)
	}
}

// Requirement: Chosen organizations are checked before the documents.

func TestAdminOfTheChosenOrganization(t *testing.T) {
	h, _ := orgHarness(t, orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed)), ct.ChosenOrganization)
	o := h.run()
	if o.err != nil {
		t.Fatalf("run: %v", o.err)
	}
	if o.published.Verdict != publish.VerdictVerified {
		t.Errorf("verdict %s; result %s", o.published.Verdict, o.verification)
	}
}

func TestOwnerOfTheChosenOrganization(t *testing.T) {
	h, _ := orgHarness(t, orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationOwner, bwcli.MembershipConfirmed)), ct.ChosenOrganization)
	if o := h.run(); o.err != nil {
		t.Fatalf("run: %v", o.err)
	}
}

func TestMemberWithoutTheRole(t *testing.T) {
	h, _ := orgHarness(t, orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationUser, bwcli.MembershipConfirmed)), ct.ChosenOrganization)
	o := h.run()
	wantOrganizationError(t, o.err, ct.ChosenOrganization, collect.OrganizationRole)
	h.wantNoDocument()
	h.wantNoGeneration()
}

func TestChosenOrganizationNotInTheList(t *testing.T) {
	h, _ := orgHarness(t, orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed)),
		ct.ChosenOrganization, secondOrganization)
	o := h.run()
	wantOrganizationError(t, o.err, secondOrganization, collect.OrganizationNotMember)
	h.wantNoDocument()
	h.wantNoGeneration()
}

func TestMembershipNotConfirmed(t *testing.T) {
	for _, status := range []int{bwcli.MembershipInvited, bwcli.MembershipAccepted, -1} {
		h, _ := orgHarness(t, orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationOwner, status)), ct.ChosenOrganization)
		o := h.run()
		wantOrganizationError(t, o.err, ct.ChosenOrganization, collect.OrganizationNotConfirmed)
		h.wantNoDocument()
	}
}

func TestCustomRole(t *testing.T) {
	for _, typ := range []int{bwcli.OrganizationManager, bwcli.OrganizationCustom, 5} {
		h, _ := orgHarness(t, orgList(orgEntry(ct.ChosenOrganization, "Org", typ, bwcli.MembershipConfirmed)), ct.ChosenOrganization)
		o := h.run()
		wantOrganizationError(t, o.err, ct.ChosenOrganization, collect.OrganizationRole)
		h.wantNoDocument()
	}
}

func TestNoOrganizationChosen(t *testing.T) {
	h, _ := orgHarness(t, bwclitest.Behavior{Exit: 1})
	o := h.run()
	if o.err != nil {
		t.Fatalf("run: %v", o.err)
	}
	for _, c := range h.commands() {
		if c == bwclitest.ListOrganizations || strings.Contains(c, "--organizationid") {
			t.Errorf("organization command started: %s", c)
		}
	}
}

func TestOrganizationListFails(t *testing.T) {
	h, _ := orgHarness(t, bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel + "\n"}, ct.ChosenOrganization)
	o := h.run()
	wantOp(t, o.err, bwcli.OpListOrganizations, bwcli.ClassExit)
	if o.err != nil && strings.Contains(o.err.Error(), ct.Marker) {
		t.Errorf("error %q carries the sentinel", o.err)
	}
	h.wantNoDocument()
	h.wantNoGeneration()
}

// The list is read after the freshness check and before the first
// document, once.
func TestOrganizationListAfterTheFreshnessCheck(t *testing.T) {
	h, _ := orgHarness(t, orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed)), ct.ChosenOrganization)
	if o := h.run(); o.err != nil {
		t.Fatalf("run: %v", o.err)
	}
	cmds := h.commands()
	// version and login come first.
	want := []string{"sync --force --raw", "status", bwclitest.ListOrganizations, "export --format json --raw"}
	if len(cmds) < 6 || strings.Join(cmds[2:6], "\n") != strings.Join(want, "\n") {
		t.Errorf("commands %q", cmds)
	}
}

// Requirement: Organization source comes from the organization list.

func TestSourceWithTwoOrganizations(t *testing.T) {
	list := orgList(
		orgEntry(secondOrganization, "Second "+ct.Marker, bwcli.OrganizationOwner, bwcli.MembershipConfirmed),
		orgEntry(ct.ChosenOrganization, "First", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed),
	)
	h, _ := orgHarness(t, list, ct.ChosenOrganization, secondOrganization)
	snap, err := h.collector.Snapshot(context.Background(), h.client)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Wipe()
	var got []string
	for _, o := range snap.Organizations {
		got = append(got, o.ID+"="+o.Name)
	}
	want := []string{ct.ChosenOrganization + "=First", secondOrganization + "=Second " + ct.Marker}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("source %q, want %q", got, want)
	}
}

// A name the list does not hold is empty.
func TestOrganizationWithoutAName(t *testing.T) {
	list := bwclitest.Behavior{Stdout: `[{"id":"` + ct.ChosenOrganization + `","status":2,"type":1}]`}
	h, _ := orgHarness(t, list, ct.ChosenOrganization)
	snap, err := h.collector.Snapshot(context.Background(), h.client)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Wipe()
	if len(snap.Organizations) != 1 || snap.Organizations[0].Name != "" {
		t.Errorf("organizations %+v", snap.Organizations)
	}
}

// Requirement: Two documents per chosen organization are collected.

func TestTwoOrganizations(t *testing.T) {
	list := orgList(
		orgEntry(ct.ChosenOrganization, "First", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed),
		orgEntry(secondOrganization, "Second", bwcli.OrganizationOwner, bwcli.MembershipConfirmed),
	)
	h, f := orgHarness(t, list, ct.ChosenOrganization, secondOrganization)
	snap, err := h.collector.Snapshot(context.Background(), h.client)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Wipe()
	in := f.Input(t)
	if len(snap.Organizations) != 2 ||
		!bytes.Equal(snap.Organizations[0].PlaintextExport, in.Organizations[0].PlaintextExport) ||
		!bytes.Equal(snap.Organizations[0].EncryptedExport, in.Organizations[0].EncryptedExport) ||
		string(snap.Organizations[1].PlaintextExport) != emptyOrgPlain ||
		string(snap.Organizations[1].EncryptedExport) != emptyOrgSealed {
		t.Fatalf("organization documents differ from what bw wrote")
	}
	if !bytes.Equal(snap.PlaintextExport, in.PlaintextExport) || !bytes.Equal(snap.ArchivedItemList, in.ArchivedItemList) {
		t.Error("personal documents differ")
	}
	cmds := h.commands()
	want := []string{
		"export --format json --raw",
		"export --format encrypted_json --raw",
		"list items --raw",
		"list items --archived --raw",
		bwclitest.OrganizationExport(ct.ChosenOrganization),
		bwclitest.OrganizationEncryptedExport(ct.ChosenOrganization),
		bwclitest.OrganizationExport(secondOrganization),
		bwclitest.OrganizationEncryptedExport(secondOrganization),
	}
	if len(cmds) != 5+len(want) || strings.Join(cmds[5:], "\n") != strings.Join(want, "\n") {
		t.Errorf("commands %q, want after the list %q", cmds, want)
	}
}

func TestOrganizationExportFails(t *testing.T) {
	list := orgList(
		orgEntry(ct.ChosenOrganization, "First", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed),
		orgEntry(secondOrganization, "Second", bwcli.OrganizationOwner, bwcli.MembershipConfirmed),
	)
	f := ct.IntactOrganization(t)
	s := fakeScenario(t, f)
	s.Commands[bwclitest.ListOrganizations] = list
	s.Commands[bwclitest.OrganizationExport(secondOrganization)] = bwclitest.Behavior{Stdout: emptyOrgPlain}
	s.Commands[bwclitest.OrganizationEncryptedExport(secondOrganization)] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel + "\n"}
	h := newHarness(t, s)
	h.collector.Organizations = []string{ct.ChosenOrganization, secondOrganization}
	o := h.run()
	wantOp(t, o.err, bwcli.OpExportOrganizationEncrypted, bwcli.ClassExit)
	var e *bwcli.Error
	if !errors.As(o.err, &e) || e.OrganizationID != secondOrganization {
		t.Errorf("failure %v does not name organization %s", o.err, secondOrganization)
	}
	if o.err != nil && strings.Contains(o.err.Error(), ct.Marker) {
		t.Errorf("error %q carries the sentinel", o.err)
	}
	for _, c := range h.commands() {
		if strings.HasPrefix(c, "get attachment") {
			t.Errorf("attachment requested: %s", c)
		}
	}
	h.wantNoGeneration()
}

func TestSessionLostOnAnOrganizationExport(t *testing.T) {
	f := ct.IntactOrganization(t)
	s := fakeScenario(t, f)
	s.Commands[bwclitest.ListOrganizations] = orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed))
	s.Commands[bwclitest.OrganizationExport(ct.ChosenOrganization)] = bwclitest.Behavior{Exit: 1, LogOut: true}
	h := newHarness(t, s)
	h.collector.Organizations = []string{ct.ChosenOrganization}
	o := h.run()
	wantOp(t, o.err, bwcli.OpExportOrganizationJSON, bwcli.ClassNotLoggedIn)
	var e *bwcli.Error
	if !errors.As(o.err, &e) || e.OrganizationID != ct.ChosenOrganization {
		t.Errorf("failure %v does not name the organization", o.err)
	}
	cmds := h.commands()
	// The failed export is followed by the status query of the client and
	// by nothing else.
	last := cmds[len(cmds)-2:]
	if last[0] != bwclitest.OrganizationExport(ct.ChosenOrganization) || last[1] != "status" {
		t.Errorf("commands %q", cmds)
	}
	if n := countOf(cmds, bwclitest.OrganizationEncryptedExport(ct.ChosenOrganization)); n != 0 {
		t.Errorf("encrypted organization export started %d times", n)
	}
	h.wantNoGeneration()
}

// Requirement: Decryption error counts are passed on.

func TestOrganizationExportWithDecryptionErrors(t *testing.T) {
	f := ct.IntactOrganization(t)
	s := fakeScenario(t, f)
	s.Commands[bwclitest.ListOrganizations] = orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed))
	b := s.Commands[bwclitest.OrganizationExport(ct.ChosenOrganization)]
	b.Stderr, b.StderrRepeat = decryptionLine, 2
	s.Commands[bwclitest.OrganizationExport(ct.ChosenOrganization)] = b
	h := newHarness(t, s)
	h.collector.Organizations = []string{ct.ChosenOrganization}
	snap, err := h.collector.Snapshot(context.Background(), h.client)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Wipe()
	want := capture.OrganizationDecryptionErrors{PlaintextExport: 2}
	if snap.Organizations[0].DecryptionErrors != want {
		t.Errorf("counts %+v, want %+v", snap.Organizations[0].DecryptionErrors, want)
	}
	if in := snap.Input(nil); in.Organizations[0].DecryptionErrors != want {
		t.Errorf("input counts %+v", in.Organizations[0].DecryptionErrors)
	}
}

// Wipe clears the organization documents, also through the slices that
// Documents and Input share.
func TestWipeClearsTheOrganizationDocuments(t *testing.T) {
	h, _ := orgHarness(t, orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed)), ct.ChosenOrganization)
	snap, err := h.collector.Snapshot(context.Background(), h.client)
	if err != nil {
		t.Fatal(err)
	}
	docs, in := snap.Documents(), snap.Input(nil)
	snap.Wipe()
	for _, b := range [][]byte{
		snap.Organizations[0].PlaintextExport, snap.Organizations[0].EncryptedExport,
		docs.Organizations[0].PlaintextExport, docs.Organizations[0].EncryptedExport,
		in.Organizations[0].PlaintextExport, in.Organizations[0].EncryptedExport,
	} {
		if len(b) == 0 || !allZero(b) {
			t.Error("organization document not wiped")
		}
	}
}

// A failed organization export wipes every document collected before it.
func TestOrganizationExportFailureWipesEarlierDocuments(t *testing.T) {
	f := ct.IntactOrganization(t)
	docs := docsOf(t, f)
	in := f.Input(t)
	v := newVault(t).
		on(mSync, answer{}).
		on(mStatus, answer{status: freshStatus()}).
		on(mOrgs, answer{orgs: []bwcli.Organization{{ID: ct.ChosenOrganization, Type: bwcli.OrganizationAdmin, Status: bwcli.MembershipConfirmed}}}).
		on(mExport, answer{data: docs[0]}).
		on(mEncrypted, answer{data: docs[1]}).
		on(mList, answer{data: docs[2]}).
		on(mArchived, answer{data: docs[3]}).
		on(mOrgExport, answer{data: in.Organizations[0].PlaintextExport}).
		on(mOrgSealed, answer{err: bwErr(bwcli.OpExportOrganizationEncrypted, bwcli.ClassExit)})
	wrapped := &keepingVault{memVault: v}
	c, _ := collector()
	c.Organizations = []string{ct.ChosenOrganization}
	if _, err := c.Snapshot(context.Background(), wrapped); err == nil {
		t.Fatal("snapshot succeeded")
	}
	if len(wrapped.kept) != 5 {
		t.Fatalf("kept %d documents", len(wrapped.kept))
	}
	for i, b := range wrapped.kept {
		if !allZero(b) {
			t.Errorf("document %d not wiped after the failure", i)
		}
	}
	wantCalls(t, v, mSync, mStatus, mOrgs, mExport, mEncrypted, mList, mArchived,
		mOrgExport+" "+ct.ChosenOrganization, mOrgSealed+" "+ct.ChosenOrganization)
}

// keepingVault keeps every document slice it returns.
type keepingVault struct {
	*memVault
	kept [][]byte
}

func (v *keepingVault) keep(out bwcli.Output, err error) (bwcli.Output, error) {
	if err == nil {
		v.kept = append(v.kept, out.Data)
	}
	return out, err
}

func (v *keepingVault) ExportJSON(ctx context.Context) (bwcli.Output, error) {
	return v.keep(v.memVault.ExportJSON(ctx))
}

func (v *keepingVault) ExportEncryptedJSON(ctx context.Context) (bwcli.Output, error) {
	return v.keep(v.memVault.ExportEncryptedJSON(ctx))
}

func (v *keepingVault) ListItems(ctx context.Context) (bwcli.Output, error) {
	return v.keep(v.memVault.ListItems(ctx))
}

func (v *keepingVault) ListArchivedItems(ctx context.Context) (bwcli.Output, error) {
	return v.keep(v.memVault.ListArchivedItems(ctx))
}

func (v *keepingVault) ExportOrganizationJSON(ctx context.Context, id string) (bwcli.Output, error) {
	return v.keep(v.memVault.ExportOrganizationJSON(ctx, id))
}

// Requirement: Attachments in scope are downloaded once.

func TestAttachmentOfAChosenOrganization(t *testing.T) {
	h, _ := orgHarness(t, orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed)), ct.ChosenOrganization)
	o := h.run()
	if o.err != nil {
		t.Fatalf("run: %v", o.err)
	}
	if n := countOf(h.commands(), attachmentCommand(ct.ItemOrgLogin, ct.AttachmentOrgLogin)); n != 1 {
		t.Errorf("organization attachment requested %d times, want 1", n)
	}
	found := false
	for _, d := range o.downloads {
		found = found || d.ItemID == ct.ItemOrgLogin && d.AttachmentID == ct.AttachmentOrgLogin && d.Error == ""
	}
	if !found {
		t.Errorf("no download result for the organization attachment: %+v", o.downloads)
	}
}

// Without the organization chosen, its attachment is not requested.
func TestAttachmentOfAnOrganizationThatIsNotChosen(t *testing.T) {
	f := ct.VariantByName(t, ct.OrgNotChosenItems).Build(t)
	s := fakeScenario(t, f)
	s.Commands[bwclitest.ListOrganizations] = orgList(orgEntry(ct.ChosenOrganization, "Org", bwcli.OrganizationAdmin, bwcli.MembershipConfirmed))
	s.Commands[attachmentCommand(ct.ItemUnchosenA, ct.AttachmentUnchosen)] = bwclitest.Behavior{Stdout: "content"}
	h := newHarness(t, s)
	h.collector.Organizations = []string{ct.ChosenOrganization}
	o := h.run()
	if o.err != nil {
		t.Fatalf("run: %v", o.err)
	}
	if n := countOf(h.commands(), attachmentCommand(ct.ItemUnchosenA, ct.AttachmentUnchosen)); n != 0 {
		t.Errorf("attachment of an organization that is not chosen requested %d times", n)
	}
	for _, d := range o.downloads {
		if d.ItemID == ct.ItemUnchosenA {
			t.Errorf("download result %+v", d)
		}
	}
	if o.published.Verdict != publish.VerdictVerified {
		t.Errorf("verdict %s: %s", o.published.Verdict, o.verification)
	}
}

// An item of a chosen organization that both lists show adds its
// attachment to the plan once.
func TestOrganizationAttachmentShownByBothLists(t *testing.T) {
	f := ct.IntactOrganization(t)
	for _, raw := range f.ItemList {
		if item := raw.(map[string]any); item["id"] == ct.ItemOrgLogin {
			f.ArchivedItemList = append(f.ArchivedItemList, item)
		}
	}
	v := snapshotVault(t, f, freshStatus()).
		on(mOrgs, answer{orgs: []bwcli.Organization{{ID: ct.ChosenOrganization, Type: bwcli.OrganizationAdmin, Status: bwcli.MembershipConfirmed}}})
	in := f.Input(t)
	v.on(mOrgExport, answer{data: in.Organizations[0].PlaintextExport}).on(mOrgSealed, answer{data: in.Organizations[0].EncryptedExport})
	c, _ := collector()
	c.Organizations = []string{ct.ChosenOrganization}
	s, err := c.Snapshot(context.Background(), v)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, p := range s.Plan {
		if p.ItemID == ct.ItemOrgLogin && p.AttachmentID == ct.AttachmentOrgLogin {
			n++
			if p.FileName != ct.AttachmentFileName {
				t.Errorf("file name %q", p.FileName)
			}
		}
	}
	if n != 1 || len(s.Plan) != 4 {
		t.Errorf("plan %v", planRefs(s))
	}
}
