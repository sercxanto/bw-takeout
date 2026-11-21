package collect_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/collect"
)

// Method names of the in-memory vault, as recorded in its calls.
const (
	mSync      = "sync"
	mStatus    = "status"
	mExport    = "export_json"
	mEncrypted = "export_encrypted_json"
	mList      = "list_items"
	mArchived  = "list_archived_items"
	mAttach    = "attachment"
	mOrgs      = "list_organizations"
	mOrgExport = "export_organization_json"
	mOrgSealed = "export_organization_encrypted_json"
)

// answer is one queued result of a vault call. Data is copied on every
// call, so that clearing the returned slice leaves the queued answer
// intact.
type answer struct {
	data   []byte
	count  int
	status bwcli.Status
	orgs   []bwcli.Organization
	err    error
	// before runs when the call is made, before the answer is returned.
	before func()
}

// call is one recorded call.
type call struct {
	method               string
	itemID, attachmentID string
	organizationID       string
}

func (c call) String() string {
	switch {
	case c.method == mAttach:
		return c.method + " " + c.itemID + "/" + c.attachmentID
	case c.organizationID != "":
		return c.method + " " + c.organizationID
	}
	return c.method
}

// memVault is the in-memory Vault of the unit tests (design D11). Each
// method answers from its own queue. A call without a queued answer is
// reported through fail and returns errNoAnswer.
type memVault struct {
	fail    func(format string, args ...any)
	answers map[string][]answer
	calls   []call
}

var errNoAnswer = errors.New("memVault: no queued answer")

func newVault(t testing.TB) *memVault {
	return &memVault{fail: t.Errorf, answers: map[string][]answer{}}
}

// on queues answers for a method.
func (v *memVault) on(method string, answers ...answer) *memVault {
	v.answers[method] = append(v.answers[method], answers...)
	return v
}

func (v *memVault) next(c call) answer {
	v.calls = append(v.calls, c)
	queue := v.answers[c.method]
	if len(queue) == 0 {
		v.fail("unexpected call %s: no queued answer left", c)
		return answer{err: errNoAnswer}
	}
	a := queue[0]
	v.answers[c.method] = queue[1:]
	if a.before != nil {
		a.before()
	}
	return a
}

func (v *memVault) output(c call) (bwcli.Output, error) {
	a := v.next(c)
	if a.err != nil {
		return bwcli.Output{}, a.err
	}
	return bwcli.Output{Data: bytes.Clone(a.data), DecryptionErrors: a.count}, nil
}

func (v *memVault) Sync(ctx context.Context) error { return v.next(call{method: mSync}).err }

func (v *memVault) Status(ctx context.Context) (bwcli.Status, error) {
	a := v.next(call{method: mStatus})
	return a.status, a.err
}

func (v *memVault) ExportJSON(ctx context.Context) (bwcli.Output, error) {
	return v.output(call{method: mExport})
}

func (v *memVault) ExportEncryptedJSON(ctx context.Context) (bwcli.Output, error) {
	return v.output(call{method: mEncrypted})
}

func (v *memVault) ListItems(ctx context.Context) (bwcli.Output, error) {
	return v.output(call{method: mList})
}

func (v *memVault) ListArchivedItems(ctx context.Context) (bwcli.Output, error) {
	return v.output(call{method: mArchived})
}

func (v *memVault) Attachment(ctx context.Context, itemID, attachmentID string) (bwcli.Output, error) {
	return v.output(call{method: mAttach, itemID: itemID, attachmentID: attachmentID})
}

func (v *memVault) ListOrganizations(ctx context.Context) ([]bwcli.Organization, error) {
	a := v.next(call{method: mOrgs})
	return a.orgs, a.err
}

func (v *memVault) ExportOrganizationJSON(ctx context.Context, organizationID string) (bwcli.Output, error) {
	return v.output(call{method: mOrgExport, organizationID: organizationID})
}

func (v *memVault) ExportOrganizationEncryptedJSON(ctx context.Context, organizationID string) (bwcli.Output, error) {
	return v.output(call{method: mOrgSealed, organizationID: organizationID})
}

// leftover fails when queued answers were not used.
func (v *memVault) leftover(t testing.TB) {
	t.Helper()
	for method, queue := range v.answers {
		if len(queue) != 0 {
			t.Errorf("%d queued answers for %s were not used", len(queue), method)
		}
	}
}

// methods returns the recorded calls as strings.
func (v *memVault) methods() []string {
	out := make([]string, 0, len(v.calls))
	for _, c := range v.calls {
		out = append(out, c.String())
	}
	return out
}

func wantCalls(t testing.TB, v *memVault, want ...string) {
	t.Helper()
	if got := v.methods(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n got %q\nwant %q", got, want)
	}
}

// t0 is the fixed clock of the unit tests. It has a sub-millisecond part,
// so that the truncation of design D3 matters.
var t0 = time.Date(2026, 11, 17, 10, 0, 0, 123_456_789, time.UTC)

// waits records the durations a collector waited.
type waits struct {
	got []time.Duration
	// onWait, when set, runs instead of returning at once.
	onWait func(ctx context.Context) error
}

func (w *waits) wait(ctx context.Context, d time.Duration) error {
	w.got = append(w.got, d)
	if w.onWait != nil {
		return w.onWait(ctx)
	}
	return nil
}

// collector returns a collector with the fixed clock and a recording wait.
func collector() (*collect.Collector, *waits) {
	w := &waits{}
	return &collect.Collector{Now: func() time.Time { return t0 }, Wait: w.wait}, w
}

func bwErr(op bwcli.Op, class bwcli.Class) error {
	return &bwcli.Error{Op: op, Class: class, ExitCode: 1}
}

// The test double reports a call that has no queued answer left, and the
// collection then ends with ErrUnexpected.
func TestMemVaultFailsWithoutQueuedAnswer(t *testing.T) {
	var reported []string
	v := &memVault{
		fail:    func(format string, args ...any) { reported = append(reported, fmt.Sprintf(format, args...)) },
		answers: map[string][]answer{},
	}
	v.on(mSync, answer{})

	if err := v.Sync(context.Background()); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if len(reported) != 0 {
		t.Fatalf("reported %q after a queued answer", reported)
	}
	if err := v.Sync(context.Background()); !errors.Is(err, errNoAnswer) {
		t.Errorf("second sync: %v, want errNoAnswer", err)
	}
	if len(reported) != 1 || !strings.Contains(reported[0], "no queued answer") {
		t.Errorf("reported %q, want one report of the missing answer", reported)
	}

	c, _ := collector()
	_, err := c.Snapshot(context.Background(), v)
	if !errors.Is(err, collect.ErrUnexpected) || strings.Contains(err.Error(), "memVault") {
		t.Errorf("snapshot error = %v, want ErrUnexpected without the text of the vault error", err)
	}
	if len(reported) != 2 {
		t.Errorf("reported %d times, want 2", len(reported))
	}
}

// The double hands out copies, so clearing an answer does not change the
// queued data.
func TestMemVaultReturnsCopies(t *testing.T) {
	data := []byte("content")
	v := newVault(t).on(mExport, answer{data: data})
	out, err := v.ExportJSON(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	clear(out.Data)
	if string(data) != "content" {
		t.Errorf("queued data changed to %q", data)
	}
}
