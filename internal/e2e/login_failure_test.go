//go:build vaultwarden && linux

package e2e

import (
	"os"
	"strings"
	"testing"
	"time"
)

// loginFailureLimit bounds a run that ends at a refused login (design D5).
// Such a run starts about five bw processes and the logout. If bw asks
// again after a wrong answer, the reader skips the second prompt as a
// redraw, and this limit ends the run instead of backupLimit.
const loginFailureLimit = 90 * time.Second

// twoStepServer starts a server and registers an account with two-step
// login. It seeds no vault, because the run ends before the first document
// (design D5).
func twoStepServer(t *testing.T) (*vaultServer, *account) {
	t.Helper()
	s := startServer(t)
	a := newAccount(t)
	if err := a.register(s.Client, s.URL); err != nil {
		t.Fatal(err)
	}
	enableTwoStepChecked(t, s, a)
	return s, a
}

// wantLoginRefused checks what both failure scenarios assert (design D5).
// The class after "login:" and the result of the logout are logged, not
// asserted.
func wantLoginRefused(t *testing.T, r *backupRun, a *account) {
	t.Helper()
	failed, _ := r.reportLine("Failed:")
	logout, _ := r.reportLine("Logout:")
	t.Logf("duration %v, Failed: %s, Logout: %s", r.Duration.Round(time.Second), failed, logout)

	if r.ExitCode != 1 {
		t.Errorf("exit status %d, want 1", r.ExitCode)
	}
	if g, _ := r.reportLine("Generation:"); !strings.HasPrefix(g, "none published") {
		t.Errorf("generation line %q, want none published", g)
	}
	if !strings.HasPrefix(failed, "login:") {
		t.Errorf("failed line %q, want a login failure", failed)
	}
	if logout == "" || logout == "not needed, no login was started" {
		t.Errorf("logout line %q, want a logout that was attempted", logout)
	}
	if p, _ := r.reportLine("Profile:"); p != "removed" {
		t.Errorf("profile line %q, want removed", p)
	}
	entries, err := os.ReadDir(r.Target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the target holds %d entries, want none", len(entries))
	}
	r.wantEmptyProfileLocation(t)
}

// TestWrongPasswordFailsLogin types a wrong master password. bw must not
// ask for the second factor before it checked the password.
func TestWrongPasswordFailsLogin(t *testing.T) {
	s, a := twoStepServer(t)
	wrong := newAccount(t) // a second random password of the same form
	r := runBackupLimit(t, s, a, map[string]answerFunc{
		"Email address":   fixed(a.Email),
		"Master password": fixed(wrong.Password),
	}, loginFailureLimit)
	t.Logf("report:\n%s", r.redacted(a))

	if got, want := promptNames(r.Prompts), "Email address, Master password"; got != want {
		t.Errorf("bw showed the prompts %q, want %q", got, want)
	}
	wantLoginRefused(t, r, a)
}

// wrongCode returns a code that is wrong by construction (design D5): the
// code of a step 100 steps before the current one, moved back further
// while it equals the code of one of the three steps the server accepts or
// of the last used step.
func (a *account) wrongCode() string {
	current := totpStep(a.clock())
	accepted := map[string]bool{totpCode(a.totpSecret, a.lastStep): true}
	for step := current - 1; step <= current+1; step++ {
		accepted[totpCode(a.totpSecret, step)] = true
	}
	step := current - 100
	for accepted[totpCode(a.totpSecret, step)] {
		step--
	}
	return totpCode(a.totpSecret, step)
}

// TestWrongTwoStepCodeFailsLogin types the right password and a wrong code.
func TestWrongTwoStepCodeFailsLogin(t *testing.T) {
	s, a := twoStepServer(t)
	r := runBackupLimit(t, s, a, map[string]answerFunc{
		"Email address":   fixed(a.Email),
		"Master password": fixed(a.Password),
		codePrompt:        func() (string, error) { return a.wrongCode(), nil },
	}, loginFailureLimit)
	t.Logf("report:\n%s", r.redacted(a))

	if got, want := promptNames(r.Prompts), "Email address, Master password, "+codePrompt; got != want {
		t.Errorf("bw showed the prompts %q, want %q in this order", got, want)
	}
	wantLoginRefused(t, r, a)
}
