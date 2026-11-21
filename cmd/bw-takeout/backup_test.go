package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/bwcli/bwclitest"
)

// sandbox is the environment of a backup command run: a program search
// path that holds no bw, and user cache directories that must stay empty,
// so that a profile would show up there.
type sandbox struct {
	env        []string
	cache      string
	target     string
	recipients string
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	root := t.TempDir()
	s := sandbox{cache: filepath.Join(root, "cache"), target: filepath.Join(root, "target"), recipients: filepath.Join(root, "recipients.txt")}
	for _, d := range []string{s.cache, s.target, filepath.Join(root, "path")} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(s.recipients, []byte(archivetest.NewKey(t).Recipient().String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "PATH", "HOME", "XDG_CACHE_HOME", "LOCALAPPDATA":
			continue
		}
		s.env = append(s.env, kv)
	}
	s.env = append(s.env, "PATH="+filepath.Join(root, "path"), "HOME="+s.cache, "XDG_CACHE_HOME="+s.cache, "LocalAppData="+s.cache)
	return s
}

// wantNoProfile checks that no profile location was created.
func (s sandbox) wantNoProfile(t *testing.T) {
	t.Helper()
	if entries, _ := os.ReadDir(s.cache); len(entries) != 0 {
		t.Errorf("the cache directory holds %d entries", len(entries))
	}
}

func (s sandbox) args(extra ...string) []string {
	return append([]string{"backup", "--target", s.target, "--recipients-file", s.recipients}, extra...)
}

func TestServerFlagMissing(t *testing.T) {
	s := newSandbox(t)
	o := runCommand(t, s.env, s.args()...)
	if o.code != exitUsage || o.stdout != "" || !strings.Contains(o.stderr, "usage: bw-takeout backup") {
		t.Fatal(o)
	}
	s.wantNoProfile(t)
}

func TestServerAddressNotHTTPS(t *testing.T) {
	s := newSandbox(t)
	for _, server := range []string{"http://vault.example.org", "https://", "https://user@vault.example.org"} {
		o := runCommand(t, s.env, s.args("--server", server)...)
		if o.code != exitUsage || o.stdout != "" || !strings.Contains(o.stderr, "usage: bw-takeout backup") {
			t.Fatalf("%s: %v", server, o)
		}
	}
	s.wantNoProfile(t)
}

func TestBackupUsageErrors(t *testing.T) {
	s := newSandbox(t)
	for name, args := range map[string][]string{
		"unknown flag":           s.args("--server", "https://vault.example.org", "--passphrase", "x"),
		"extra argument":         s.args("--server", "https://vault.example.org", "extra"),
		"target missing":         {"backup", "--server", "https://vault.example.org", "--recipients-file", s.recipients},
		"recipients missing":     {"backup", "--server", "https://vault.example.org", "--target", s.target},
		"flag after an argument": {"backup", "extra", "--server", "https://vault.example.org", "--target", s.target, "--recipients-file", s.recipients},
	} {
		t.Run(name, func(t *testing.T) {
			o := runCommand(t, s.env, args...)
			if o.code != exitUsage || o.stdout != "" || !strings.Contains(o.stderr, "usage: bw-takeout backup") {
				t.Fatal(o)
			}
		})
	}
	s.wantNoProfile(t)
}

// A target that does not exist ends the run before any bw is needed: the
// search path holds none.
func TestBackupTargetMissingWithoutBW(t *testing.T) {
	s := newSandbox(t)
	o := runCommand(t, s.env, "backup", "--server", "https://vault.example.org",
		"--target", filepath.Join(s.target, "missing"), "--recipients-file", s.recipients)
	if o.code != exitFailed || !strings.Contains(o.stdout, "Failed:      checks: target is not an existing directory") {
		t.Fatal(o)
	}
	if strings.Contains(o.stdout, "bw:") || o.stderr != "" {
		t.Errorf("setup lines or standard error\n%s", o)
	}
	s.wantNoProfile(t)
}

func TestBackupBWNotFound(t *testing.T) {
	s := newSandbox(t)
	o := runCommand(t, s.env, s.args("--server", "https://vault.example.org")...)
	if o.code != exitFailed || !strings.Contains(o.stdout, "Failed:      checks: bw executable not found") {
		t.Fatal(o)
	}
	s.wantNoProfile(t)
}

func TestUsageNamesBothSubcommands(t *testing.T) {
	o := runCommand(t, nil)
	if o.code != exitUsage || !strings.Contains(o.stderr, "usage: bw-takeout backup") || !strings.Contains(o.stderr, "usage: bw-takeout verify") {
		t.Fatal(o)
	}
}

// Requirement: Organizations are chosen by flag.

const (
	flagOrgA = "0c000000-0000-4000-8000-00000000000a"
	flagOrgB = "0c000000-0000-4000-8000-00000000000b"
)

func TestTwoOrganizations(t *testing.T) {
	cfg := backupFlags([]string{"--server", "https://vault.example.org", "--target", "t", "--recipients-file", "r",
		"--organization", strings.ToUpper(flagOrgB), "--organization", flagOrgA})
	if cfg == nil {
		t.Fatal("flags refused")
	}
	if !slices.Equal(cfg.Organizations, []string{flagOrgB, flagOrgA}) {
		t.Errorf("organizations %q, want both in the order given and lowercase", cfg.Organizations)
	}
}

// fakeBW builds the fake bw, so that a usage error is shown to start no
// bw process: the fake records each start in the data directory of a
// profile, and no profile may exist.
func fakeBW(t *testing.T) string {
	t.Helper()
	path, err := bwclitest.Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOrganizationIDNotAUUID(t *testing.T) {
	s := newSandbox(t)
	bw := fakeBW(t)
	for _, id := range []string{"../x", "-" + flagOrgA[1:], "", flagOrgA[:35], "--server"} {
		o := runCommand(t, s.env, s.args("--server", "https://vault.example.org", "--bw", bw, "--organization", id)...)
		if o.code != exitUsage || o.stdout != "" || !strings.Contains(o.stderr, "usage: bw-takeout backup") {
			t.Fatalf("%q: %v", id, o)
		}
	}
	s.wantNoProfile(t)
}

func TestSameOrganizationTwice(t *testing.T) {
	s := newSandbox(t)
	bw := fakeBW(t)
	for _, second := range []string{flagOrgA, strings.ToUpper(flagOrgA)} {
		o := runCommand(t, s.env, s.args("--server", "https://vault.example.org", "--bw", bw,
			"--organization", flagOrgA, "--organization", flagOrgB, "--organization", second)...)
		if o.code != exitUsage || o.stdout != "" || !strings.Contains(o.stderr, "usage: bw-takeout backup") {
			t.Fatalf("%s: %v", second, o)
		}
	}
	s.wantNoProfile(t)
}

func TestNoOrganizationFlag(t *testing.T) {
	cfg := backupFlags([]string{"--server", "https://vault.example.org", "--target", "t", "--recipients-file", "r"})
	if cfg == nil || cfg.Organizations != nil {
		t.Fatalf("config %+v", cfg)
	}
}
