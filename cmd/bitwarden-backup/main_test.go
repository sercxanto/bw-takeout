package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"filippo.io/age"

	"bitwarden-backup/internal/archive"
	"bitwarden-backup/internal/archive/archivetest"
	"bitwarden-backup/internal/capture/capturetest"
	"bitwarden-backup/internal/publish"
	"bitwarden-backup/internal/verify"
)

// binary is the command built once for all tests, so that they run the
// real program.
var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bitwarden-backup-cmd-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "bitwarden-backup")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type outcome struct {
	code           int
	stdout, stderr string
}

func (o outcome) String() string {
	return fmt.Sprintf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
}

// runCommand runs the built command. env replaces the environment when it
// is not nil.
func runCommand(t *testing.T, env []string, args ...string) outcome {
	t.Helper()
	cmd := exec.Command(binary, args...)
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return outcome{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func writeIdentity(t *testing.T, id *age.X25519Identity) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "identity.txt")
	if err := os.WriteFile(path, []byte("# synthetic test identity\n"+id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type generation struct {
	f        *capturetest.Fixture
	target   string
	dir      string
	identity string
	id       *age.X25519Identity
}

func fixtureFor(t *testing.T, verdict verify.Verdict) (*capturetest.Fixture, []byte) {
	t.Helper()
	var f *capturetest.Fixture
	switch verdict {
	case verify.Verified:
		f = capturetest.Intact(t)
	case verify.Unverified:
		f = capturetest.VariantByName(t, capturetest.BlobLoginItem).Build(t)
	}
	v, got := archivetest.Verification(t, f, true)
	if got != verdict {
		t.Fatalf("precondition: verdict %s, want %s", got, verdict)
	}
	return f, v
}

// publishGeneration publishes a generation of a fixture with the given
// verdict into a new target.
func publishGeneration(t *testing.T, verdict verify.Verdict) generation {
	t.Helper()
	f, v := fixtureFor(t, verdict)
	id := archivetest.NewKey(t)
	target := t.TempDir()
	g, err := (&publish.Publisher{}).Begin(target)
	if err != nil {
		t.Fatal(err)
	}
	archivetest.WriteTo(t, g.Writer(), f, v, id.Recipient().String())
	pub, err := g.Commit(v)
	if err != nil {
		t.Fatal(err)
	}
	return generation{f: f, target: target, dir: pub.Path, identity: writeIdentity(t, id), id: id}
}

func (g generation) archivePath() string { return filepath.Join(g.dir, publish.ArchiveName) }

func (g generation) replaceArchive(t *testing.T, data []byte) {
	t.Helper()
	if err := os.WriteFile(g.archivePath(), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (g generation) archive(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(g.archivePath())
	if err != nil {
		t.Fatal(err)
	}
	return data
}

var damagedEntry = archive.AttachmentPath(capturetest.ItemPasskeyLogin, capturetest.AttachmentFirst)

// damage replaces the content of one attachment, so it no longer matches
// the manifest.
func (g generation) damage(t *testing.T) {
	t.Helper()
	g.replaceArchive(t, archivetest.Rewrite(t, g.archive(t), g.id, func(name string, content []byte) []byte {
		if name == damagedEntry {
			return []byte("replaced content")
		}
		return content
	}))
}

func TestUsageErrors(t *testing.T) {
	id := writeIdentity(t, archivetest.NewKey(t))
	dir := t.TempDir()
	for name, args := range map[string][]string{
		"no arguments":       nil,
		"unknown subcommand": {"backup"},
		"verify alone":       {"verify"},
		"no identity":        {"verify", dir},
		"no directory":       {"verify", "--identity", id},
		"two directories":    {"verify", "--identity", id, dir, dir},
		"unknown flag":       {"verify", "--passphrase", "x", dir},
	} {
		t.Run(name, func(t *testing.T) {
			o := runCommand(t, nil, args...)
			if o.code != exitUsage || o.stdout != "" || !strings.Contains(o.stderr, "usage:") {
				t.Fatal(o)
			}
		})
	}
}

func TestVerifiedCapture(t *testing.T) {
	g := publishGeneration(t, verify.Verified)
	o := runCommand(t, nil, "verify", "--identity", g.identity, g.dir)
	if o.code != exitVerified || o.stderr != "" {
		t.Fatal(o)
	}
	for _, want := range []string{
		"Generation:  " + filepath.Base(g.dir),
		"Created:     2026-11-14T17:00:00Z",
		"Server:      " + archivetest.Source.ServerURL,
		"Account:     " + archivetest.Source.AccountEmail + " (" + archivetest.Source.AccountID + ")",
		"Items:       3",
		"Attachments: 2",
		"Archive:     intact",
		"Verdict:     verified",
		"Findings:    0",
		"Warnings:    0",
	} {
		if !strings.Contains(o.stdout, want) {
			t.Errorf("output lacks %q\n%s", want, o)
		}
	}
}

func TestUnverifiedCapture(t *testing.T) {
	g := publishGeneration(t, verify.Unverified)
	if !strings.HasSuffix(g.dir, publish.UnverifiedMarker) {
		t.Fatalf("precondition: %s", g.dir)
	}
	o := runCommand(t, nil, "verify", "--identity", g.identity, g.dir)
	if o.code != exitUnverified || !strings.Contains(o.stdout, "Verdict:     unverified") {
		t.Fatal(o)
	}
	_, v := fixtureFor(t, verify.Unverified)
	var r verify.Result
	if err := json.Unmarshal(v, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) == 0 || !strings.Contains(o.stdout, fmt.Sprintf("Findings:    %d\n", len(r.Findings))) {
		t.Errorf("findings not reported: %d\n%s", len(r.Findings), o)
	}
}

func TestFailedVerification(t *testing.T) {
	cases := map[string]struct {
		change func(t *testing.T, g *generation)
		want   string
	}{
		"damaged entry": {
			change: func(t *testing.T, g *generation) { g.damage(t) },
			want:   damagedEntry,
		},
		"truncated archive": {
			change: func(t *testing.T, g *generation) {
				data := g.archive(t)
				g.replaceArchive(t, data[:len(data)-100])
			},
			want: "damaged or truncated",
		},
		"altered byte": {
			change: func(t *testing.T, g *generation) {
				data := g.archive(t)
				data[len(data)/2] ^= 1
				g.replaceArchive(t, data)
			},
			want: "damaged or truncated",
		},
		"identity of another key pair": {
			change: func(t *testing.T, g *generation) { g.identity = writeIdentity(t, archivetest.NewKey(t)) },
			want:   "no identity",
		},
		"newer format version": {
			change: func(t *testing.T, g *generation) {
				g.replaceArchive(t, archivetest.Rewrite(t, g.archive(t), g.id, func(name string, content []byte) []byte {
					if name == archive.ManifestPath {
						return bytes.Replace(content, []byte(`"format_version": 1`), []byte(`"format_version": 2`), 1)
					}
					return content
				}))
			},
			want: "format version 2 is not supported",
		},
		"missing directory": {
			change: func(t *testing.T, g *generation) { g.dir = filepath.Join(g.target, "missing") },
			want:   "not a generation directory",
		},
		"unreadable identity file": {
			change: func(t *testing.T, g *generation) { g.identity = filepath.Join(g.target, "missing.txt") },
			want:   "identity file",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			g := publishGeneration(t, verify.Verified)
			c.change(t, &g)
			o := runCommand(t, nil, "verify", "--identity", g.identity, g.dir)
			if o.code != exitFailed || !strings.Contains(o.stderr, "verification failed") || !strings.Contains(o.stderr, c.want) {
				t.Fatal(o)
			}
			if strings.Contains(o.stdout, "Verdict:") {
				t.Errorf("partial success reported\n%s", o)
			}
		})
	}
}

func TestPartialGeneration(t *testing.T) {
	target := t.TempDir()
	id := archivetest.NewKey(t)
	g, err := (&publish.Publisher{}).Begin(target)
	if err != nil {
		t.Fatal(err)
	}
	f, v := fixtureFor(t, verify.Verified)
	// A complete archive inside a directory that still carries the marker.
	archivetest.WriteTo(t, g.Writer(), f, v, id.Recipient().String())
	o := runCommand(t, nil, "verify", "--identity", writeIdentity(t, id), filepath.Join(target, g.PartialName()))
	if o.code != exitFailed || !strings.Contains(o.stderr, "incomplete") || !strings.Contains(o.stderr, publish.PartialMarker) {
		t.Fatal(o)
	}
}

func TestDirectoryWithoutArchive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), publish.NamePrefix+"20261114T170000Z-0000abcd")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	o := runCommand(t, nil, "verify", "--identity", writeIdentity(t, archivetest.NewKey(t)), dir)
	if o.code != exitFailed || !strings.Contains(o.stderr, "incomplete") || !strings.Contains(o.stderr, publish.ArchiveName) {
		t.Fatal(o)
	}
}

func TestSystemWithoutBW(t *testing.T) {
	g := publishGeneration(t, verify.Verified)
	if runtime.GOOS == "windows" {
		t.Skip("skipped: an empty search path is tested on Unix only")
	}
	o := runCommand(t, []string{"PATH="}, "verify", "--identity", g.identity, g.dir)
	if o.code != exitVerified {
		t.Fatal(o)
	}
}

func TestNoNetworkOrProcessPackage(t *testing.T) {
	out, err := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	deps := strings.Fields(string(out))
	if len(deps) < 10 {
		t.Fatalf("dependency list too short: %v", deps)
	}
	for _, d := range deps {
		if d == "net" || strings.HasPrefix(d, "net/") || d == "os/exec" || d == "bitwarden-backup/internal/verify" {
			t.Errorf("the command depends on %s", d)
		}
	}
}

// snapshot hashes every file below root.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			out[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestGenerationWithSentinelValues(t *testing.T) {
	for _, damaged := range []bool{false, true} {
		t.Run(fmt.Sprintf("damaged=%v", damaged), func(t *testing.T) {
			g := publishGeneration(t, verify.Verified)
			if damaged {
				g.damage(t)
			}
			tmp := t.TempDir()
			before := snapshot(t, g.target)
			env := append(os.Environ(), "TMPDIR="+tmp, "TMP="+tmp, "TEMP="+tmp)
			o := runCommand(t, env, "verify", "--identity", g.identity, g.dir)
			if damaged != (o.code == exitFailed) || (!damaged && o.code != exitVerified) {
				t.Fatal(o)
			}
			output := o.stdout + o.stderr
			if strings.Contains(output, capturetest.Marker) {
				t.Errorf("output contains the sentinel marker\n%s", o)
			}
			for _, s := range g.f.Sensitive() {
				if strings.Contains(output, s) {
					t.Errorf("output contains sentinel value %q", s)
				}
			}
			if strings.Contains(output, "AGE-SECRET-KEY") || strings.Contains(output, capturetest.AttachmentFileName) {
				t.Errorf("output contains key material or a file name\n%s", o)
			}
			if left, _ := os.ReadDir(tmp); len(left) != 0 {
				t.Errorf("temporary directory holds %d entries", len(left))
			}
			if !sameSnapshot(before, snapshot(t, g.target)) {
				t.Error("the target changed")
			}
		})
	}
}

func TestIdentityFileWithSentinelLine(t *testing.T) {
	g := publishGeneration(t, verify.Verified)
	for name, content := range map[string]string{
		"sentinel line":    capturetest.Marker + "-identity-line\n",
		"valid then wrong": g.id.String() + "\n" + capturetest.Marker + "-identity-line\n",
		"no identity":      "# only a comment\n",
		"empty":            "",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "identity.txt")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			o := runCommand(t, nil, "verify", "--identity", path, g.dir)
			if o.code != exitFailed || !strings.Contains(o.stderr, "identity file") {
				t.Fatal(o)
			}
			if strings.Contains(o.stdout+o.stderr, capturetest.Marker) || strings.Contains(o.stdout+o.stderr, "AGE-SECRET-KEY") {
				t.Errorf("identity file content in output\n%s", o)
			}
		})
	}
}

func TestStoredResultMatchesPackageVerify(t *testing.T) {
	_, v := fixtureFor(t, verify.Unverified)
	var full verify.Result
	var part storedResult
	if err := json.Unmarshal(v, &full); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(v, &part); err != nil {
		t.Fatal(err)
	}
	if part.Verdict != string(full.Verdict) || len(part.Findings) != len(full.Findings) || len(part.Warnings) != len(full.Warnings) {
		t.Fatalf("stored result %+v, full %+v", part, full)
	}
	if publish.VerdictVerified != string(verify.Verified) || publish.VerdictUnverified != string(verify.Unverified) {
		t.Fatal("verdict constants differ")
	}
}
