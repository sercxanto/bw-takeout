package run_test

import (
	"os"
	"strings"
	"testing"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/bwcli/bwclitest"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/report"
	"bw-takeout/internal/run"
)

// Runs with chosen organizations (the backup-run delta of the change
// organization-export).

const (
	secondOrg      = ct.OrganizationSecond
	emptyOrgPlain  = `{"encrypted":false,"collections":[],"items":[]}`
	emptyOrgSealed = `{"encrypted":true,"encKeyValidation_DO_NOT_EDIT":"2.a|b|c","collections":[],"items":[]}`
	orgName        = "Org of the test"
)

// orgEntry is one entry of bw list organizations.
func orgEntry(id, name string, typ int) string {
	return `{"object":"organization","id":"` + id + `","name":` + jsonString(name) + `,"status":2,"type":` + string(rune('0'+typ)) + `,"enabled":true}`
}

// orgScenario answers a run of the intact organization fixture. The list
// shows the chosen organization as admin and the second as owner, whose
// exports are empty.
func orgScenario(t *testing.T, name string) (bwclitest.Scenario, *ct.Fixture) {
	t.Helper()
	f := ct.IntactOrganization(t)
	s := scenarioOf(t, f)
	in := f.Input(t)
	o := in.Organizations[0]
	s.Commands[bwclitest.ListOrganizations] = bwclitest.Behavior{
		Stdout: "[" + orgEntry(o.ID, name, 1) + "," + orgEntry(secondOrg, "Second", 0) + "]", Stderr: stderrSentinel,
	}
	s.Commands[bwclitest.OrganizationExport(o.ID)] = bwclitest.Behavior{Stdout: string(o.PlaintextExport), Stderr: stderrSentinel}
	s.Commands[bwclitest.OrganizationEncryptedExport(o.ID)] = bwclitest.Behavior{Stdout: string(o.EncryptedExport), Stderr: stderrSentinel}
	s.Commands[bwclitest.OrganizationExport(secondOrg)] = bwclitest.Behavior{Stdout: emptyOrgPlain}
	s.Commands[bwclitest.OrganizationEncryptedExport(secondOrg)] = bwclitest.Behavior{Stdout: emptyOrgSealed}
	return s, f
}

func orgEnv(t *testing.T, s bwclitest.Scenario, orgs ...string) *env {
	t.Helper()
	e := newEnv(t, s)
	e.cfg.Organizations = orgs
	return e
}

// wantTargetUnchanged fails unless the target is still empty, as newEnv
// created it.
func (e *env) wantTargetUnchanged() {
	e.t.Helper()
	if names := e.targetEntries(); len(names) != 0 {
		e.t.Errorf("target holds %v", names)
	}
}

// Requirement: The target is untouched until the snapshot is complete.

func TestChosenOrganizationWithoutTheRole(t *testing.T) {
	s, _ := orgScenario(t, orgName)
	s.Commands[bwclitest.ListOrganizations] = bwclitest.Behavior{Stdout: "[" + orgEntry(ct.ChosenOrganization, orgName, 2) + "]"}
	e := orgEnv(t, s, ct.ChosenOrganization)
	r := e.runReported()
	if r.status != report.ExitFailed || r.o.Failure == nil || r.o.Failure.Phase != run.PhaseSnapshot {
		t.Fatalf("status %d, failure %v\n%s", r.status, r.o.Failure, r.stdout)
	}
	e.wantTargetUnchanged()
	for _, c := range e.commands() {
		if strings.Contains(c, "--organizationid") {
			t.Errorf("organization export started: %s", c)
		}
	}
	e.wantProfileRemoved()
}

func TestOrganizationExportFails(t *testing.T) {
	s, _ := orgScenario(t, orgName)
	s.Commands[bwclitest.OrganizationEncryptedExport(ct.ChosenOrganization)] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel}
	e := orgEnv(t, s, ct.ChosenOrganization)
	r := e.runReported()
	if r.status != report.ExitFailed || r.o.Published != nil {
		t.Fatalf("status %d\n%s", r.status, r.stdout)
	}
	e.wantTargetUnchanged()
	for _, c := range e.commands() {
		if strings.HasPrefix(c, "get attachment") {
			t.Errorf("attachment requested: %s", c)
		}
	}
}

// Requirement: Plaintext stays in memory.

func TestNoTemporaryFilesWithAnOrganization(t *testing.T) {
	tmp := emptyTempDir(t)
	s, _ := orgScenario(t, orgName)
	e := orgEnv(t, s, ct.ChosenOrganization)
	o := e.run()
	if o.Published == nil || len(o.Downloads) != 4 {
		t.Fatalf("published %v, downloads %d", o.Published, len(o.Downloads))
	}
	if left, err := os.ReadDir(tmp); err != nil || len(left) != 0 {
		t.Errorf("temporary directory holds %d entries (%v)", len(left), err)
	}
}

// Requirement: Report shows the setup and the leftovers.

func TestChosenOrganizations(t *testing.T) {
	s, f := orgScenario(t, orgName)
	e := orgEnv(t, s, ct.ChosenOrganization, secondOrg)
	r := e.runReported()
	if r.o.Published == nil || r.o.Published.Verdict != publish.VerdictVerified {
		t.Fatalf("not published as verified\n%s", r.stdout)
	}
	r.want(t, "Recipients:  1\nOrganizations: "+ct.ChosenOrganization+", "+secondOrg+"\nCrash dumps:")
	m, got := e.open(r.o.Published)
	if len(m.Organizations) != 2 || m.Organizations[0].ID != ct.ChosenOrganization || m.Organizations[0].Name != orgName ||
		m.Organizations[0].Items != 2 || m.Organizations[1].ID != secondOrg || m.Organizations[1].Items != 0 {
		t.Errorf("manifest organizations %+v", m.Organizations)
	}
	in := f.Input(t)
	if string(got[archive.OrganizationEncryptedExportPath(ct.ChosenOrganization)]) != string(in.Organizations[0].EncryptedExport) ||
		string(got[archive.OrganizationPlaintextExportPath(secondOrg)]) != emptyOrgPlain {
		t.Error("organization documents differ from what bw wrote")
	}
	if n := countOf(e.commands(), attachmentCommand(ct.ItemOrgLogin, ct.AttachmentOrgLogin)); n != 1 {
		t.Errorf("organization attachment requested %d times", n)
	}
}

// Without the flag, the run reads no organization list and the setup lines
// name no organization.
func TestNoOrganizationFlag(t *testing.T) {
	s, _ := orgScenario(t, orgName)
	e := newEnv(t, s)
	r := e.runReported()
	if r.o.Published == nil {
		t.Fatalf("not published\n%s", r.stdout)
	}
	for _, c := range e.commands() {
		if c == bwclitest.ListOrganizations || strings.Contains(c, "--organizationid") {
			t.Errorf("organization command started: %s", c)
		}
	}
	if strings.Contains(r.stdout, "Organizations:") {
		t.Errorf("setup names organizations\n%s", r.stdout)
	}
}

// Requirement: Report shows how the run ended.

func TestOrganizationCheckFailed(t *testing.T) {
	s, _ := orgScenario(t, orgName)
	s.Commands[bwclitest.ListOrganizations] = bwclitest.Behavior{Stdout: "[]"}
	e := orgEnv(t, s, ct.ChosenOrganization)
	r := e.runReported()
	r.want(t, "Failed:      snapshot: organization "+ct.ChosenOrganization+": the account is not a member")
	if r.status != report.ExitFailed {
		t.Errorf("status %d", r.status)
	}
}

func TestOrganizationExportFailed(t *testing.T) {
	s, _ := orgScenario(t, orgName)
	s.Commands[bwclitest.OrganizationExport(ct.ChosenOrganization)] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel}
	e := orgEnv(t, s, ct.ChosenOrganization)
	r := e.runReported()
	r.want(t, "Failed:      snapshot: bw export_organization_json failed: exit, exit code 1, organization="+ct.ChosenOrganization)
	if found := r.leaks(ct.IntactOrganization(t)); len(found) != 0 {
		t.Errorf("output contains %q", found)
	}
}

// The organization export without the role logs bw out on Vaultwarden. If
// the role changed after the check, the failure names the export, the
// class not_logged_in and the organization (design D2).
func TestOrganizationExportLogsOut(t *testing.T) {
	s, _ := orgScenario(t, orgName)
	s.Commands[bwclitest.OrganizationExport(ct.ChosenOrganization)] = bwclitest.Behavior{Exit: 1, LogOut: true}
	e := orgEnv(t, s, ct.ChosenOrganization)
	r := e.runReported()
	r.want(t, "Failed:      snapshot: bw export_organization_json failed: not_logged_in, exit code 1, organization="+ct.ChosenOrganization)
	e.wantTargetUnchanged()
}

// Requirement: Report holds no vault content and no bw text.

func TestSentinelInAnOrganizationName(t *testing.T) {
	name := ct.Marker + "-organization-name \x1b[31m"
	s, f := orgScenario(t, name)
	e := orgEnv(t, s, ct.ChosenOrganization)
	r := e.runReported()
	if r.o.Published == nil {
		t.Fatalf("not published\n%s", r.stdout)
	}
	if found := r.leaks(f); len(found) != 0 {
		t.Errorf("output contains %q\nstdout:\n%s\nstderr:\n%s", found, r.stdout, r.stderr)
	}
	if strings.Contains(r.stdout+r.stderr, "organization-name") {
		t.Error("output contains the organization name")
	}
	m, _ := e.open(r.o.Published)
	if m.Organizations[0].Name != name {
		t.Errorf("manifest name %q", m.Organizations[0].Name)
	}
}
