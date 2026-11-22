//go:build vaultwarden && linux

package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
)

// backupLimit bounds one backup under the pseudo-terminal (design D6).
const backupLimit = 3 * time.Minute

// backupRun is what one bw-takeout backup left behind.
type backupRun struct {
	ExitCode int
	Report   string   // standard output, which holds no vault content
	Prompts  []string // names of the prompts bw showed
	Target   string
	Cache    string // XDG_CACHE_HOME of the run
	Identity string // age identity file, mode 0600
	Duration time.Duration
}

// runBackup runs the built bw-takeout backup against the server with the
// answers at the prompts of bw (design D6), by default the address and the
// password of the account (design D3). The age identity is
// generated for the run, and the profile location is isolated under a
// temporary XDG_CACHE_HOME.
func runBackup(t *testing.T, s *vaultServer, a *account, answers map[string]answerFunc) *backupRun {
	t.Helper()
	return runBackupLimit(t, s, a, answers, backupLimit)
}

// runBackupLimit is runBackup with its own time limit.
func runBackupLimit(t *testing.T, s *vaultServer, a *account, answers map[string]answerFunc, limit time.Duration) *backupRun {
	t.Helper()
	return runBackupWith(t, s, a, answers, limit, bwPath(t))
}

// runBackupWith is runBackupLimit with a bw executable of its own and
// extra flags, such as --organization.
func runBackupWith(t *testing.T, s *vaultServer, a *account, answers map[string]answerFunc, limit time.Duration, bw string, extra ...string) *backupRun {
	t.Helper()
	root := t.TempDir()
	r := &backupRun{
		Target:   filepath.Join(root, "target"),
		Cache:    filepath.Join(root, "cache"),
		Identity: filepath.Join(root, "identity.txt"),
	}
	home := filepath.Join(root, "home")
	for _, d := range []string{r.Target, r.Cache, home} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.Identity, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recipients := filepath.Join(root, "recipients.txt")
	if err := os.WriteFile(recipients, []byte(id.Recipient().String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(takeout, append([]string{"backup",
		"--server", s.URL,
		"--ca", s.CAFile,
		"--bw", bw,
		"--target", r.Target,
		"--recipients-file", recipients,
	}, extra...)...)
	cmd.Dir = root
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CACHE_HOME=" + r.Cache,
	}
	if answers == nil {
		answers = map[string]answerFunc{
			"Email address":   fixed(a.Email),
			"Master password": fixed(a.Password),
		}
	}
	start := time.Now()
	term, err := runWithPrompts(cmd, answers, limit)
	r.Duration = time.Since(start)
	if term != nil {
		r.Prompts = term.Prompts
		r.Report = string(term.Stdout)
		r.ExitCode = term.ExitCode
	}
	if err != nil {
		// The report holds no vault content (backup-run spec), so it may
		// be shown. The terminal text is not.
		t.Fatalf("backup: %v\nreport:\n%s", err, r.redacted(a))
	}
	t.Logf("backup: exit %d in %v, prompts: %s", r.ExitCode, r.Duration.Round(time.Millisecond), promptNames(r.Prompts))
	return r
}

// redacted is the report with the address of the account replaced. The
// report names the account, and the address is what was typed at the
// prompt (design D9).
func (r *backupRun) redacted(a *account) string {
	return redactAddress(r.Report, a.Email)
}

// redactAddress replaces the address in text that bw-takeout printed, such
// as the Account line of the backup report or of verify.
func redactAddress(text, email string) string {
	return strings.ReplaceAll(text, email, "<address>")
}

// reportLine returns the value of the report line with the given label,
// such as "Verdict:", and whether it is there.
func (r *backupRun) reportLine(label string) (string, bool) {
	for _, l := range strings.Split(r.Report, "\n") {
		if v, ok := strings.CutPrefix(l, label); ok {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

// wantEmptyProfileLocation checks that the profile location under the
// temporary XDG_CACHE_HOME of the run exists and is empty.
func (r *backupRun) wantEmptyProfileLocation(t *testing.T) {
	t.Helper()
	if err := profileLocationError(r.Cache); err != nil {
		t.Error(err)
	}
}

// profileLocationError checks the profile location under cache. The run
// creates the location and leaves it in place, so a missing location means
// that the run did not use the isolated one (review of 21 November 2026,
// R2).
func profileLocationError(cache string) error {
	entries, err := os.ReadDir(filepath.Join(cache, "bw-takeout", "profiles"))
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("the run did not use the profile location under the temporary XDG_CACHE_HOME")
	}
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("the profile location holds %d entries", len(entries))
	}
	return nil
}
