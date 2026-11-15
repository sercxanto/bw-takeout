package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/bwcli/bwclitest"
	"bw-takeout/internal/profile"
)

var fakePath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "loginprobe-fake-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakePath, err = bwclitest.Build(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

const (
	// testKey is a synthetic session key of 88 base64 characters.
	testKey  = "c3ludGhldGljLXNlc3Npb24ta2V5LXN5bnRoZXRpYy1zZXNzaW9uLWtleS1zeW50aGV0aWMtc2Vzc2lvbi0wMQ=="
	sentinel = "SENTINEL-loginprobe"
)

var testStart = time.Date(2026, 11, 15, 17, 0, 0, 0, time.UTC)

type terminal struct{}

func (terminal) IsTerminal() bool    { return true }
func (terminal) SaveState() error    { return nil }
func (terminal) RestoreState() error { return nil }

func statusDoc(state string) bwclitest.Behavior {
	return bwclitest.Behavior{Stdout: `{"serverUrl":"https://` + sentinel + `.example","lastSync":"2026-11-15T17:00:05.000Z","userEmail":"` + sentinel + `@example.com","userId":"11111111-2222-4333-8444-555555555555","status":"` + state + `"}`}
}

// probeWith runs the tool against the fake with the scenario and returns
// its exit status and everything it wrote.
func probeWith(t *testing.T, s bwclitest.Scenario) (int, string, *profile.Profile) {
	t.Helper()
	loc := filepath.Join(t.TempDir(), "profiles")
	var created *profile.Profile
	newProfile := func() (*profile.Profile, error) {
		p, err := (&profile.Creator{Location: loc}).Create()
		if err != nil {
			return nil, err
		}
		created = p
		return p, bwclitest.WriteScenario(p.DataDir, s)
	}
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	childErr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer childErr.Close()
	var out bytes.Buffer
	code := run([]string{"-bw", fakePath, "-server", "https://localhost:8443"}, &out, &out, deps{
		newProfile: newProfile, terminal: terminal{}, stdin: stdin, stderr: childErr,
		now: func() time.Time { return testStart },
	})
	return code, out.String(), created
}

func assertClean(t *testing.T, out string) {
	t.Helper()
	for _, s := range []string{testKey, testKey[:16], sentinel} {
		if strings.Contains(out, s) {
			t.Errorf("output contains %q:\n%s", s, out)
		}
	}
}

func TestProbeAgainstFake(t *testing.T) {
	noisy := bwclitest.Behavior{Stdout: sentinel, Stderr: sentinel + "\n"}
	code, out, p := probeWith(t, bwclitest.Scenario{Commands: map[string]bwclitest.Behavior{
		"--version": {Stdout: "2026.10.1-" + sentinel + "\n", Stderr: "Could not find data file, \"" + sentinel + "\"\n"},
		"config":    noisy,
		"login":     {Stdout: testKey, Prompt: "Master password (" + sentinel + "): "},
		"status":    statusDoc("unlocked"),
		"sync":      noisy,
		"logout":    noisy,
	}})
	t.Log("\n" + out)
	if code != 0 {
		t.Errorf("exit status %d", code)
	}
	for _, want := range []string{
		"login: ok, session key form valid",
		"status after sync: ok, state unlocked, lastSync set, later than the start",
		"data file after logout: ",
		"logout: ok",
		"teardown: profile removed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	assertClean(t, out)
	if _, err := os.Stat(p.Path); err == nil {
		t.Error("profile not removed")
	}
}

func TestProbeLoginWithOtherText(t *testing.T) {
	code, out, _ := probeWith(t, bwclitest.Scenario{Commands: map[string]bwclitest.Behavior{
		"--version": {Stdout: "2026.10.1\n"},
		"config":    {},
		"login":     {Stdout: sentinel + " " + testKey + "\n", Stderr: sentinel + "\n"},
		"status":    statusDoc("unauthenticated"),
		"logout":    {Stdout: sentinel, Stderr: sentinel + "\n"},
	}})
	if code != 1 {
		t.Errorf("exit status %d", code)
	}
	// bw exited with code 0, so it may be logged in although the key was
	// refused. The logout must run all the same.
	for _, want := range []string{"session key form invalid", "data file after logout: ", "logout after failed login: ok", "teardown: profile removed"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "not read") {
		t.Errorf("data file not read after a login attempt:\n%s", out)
	}
	assertClean(t, out)
}

func TestProbeWithoutLoginAttempt(t *testing.T) {
	code, out, _ := probeWith(t, bwclitest.Scenario{Commands: map[string]bwclitest.Behavior{
		"--version": {Exit: 1, Stderr: sentinel + "\n"},
		"logout":    {Stdout: sentinel},
	}})
	if code != 1 {
		t.Errorf("exit status %d", code)
	}
	for _, want := range []string{"version: failed (class exit)", "data file after logout: not read, no login attempted", "teardown: profile removed"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "logout") {
			t.Errorf("logout ran without a login attempt:\n%s", out)
		}
	}
	assertClean(t, out)
}

func TestProbeSyncFails(t *testing.T) {
	code, out, _ := probeWith(t, bwclitest.Scenario{Commands: map[string]bwclitest.Behavior{
		"--version": {Stdout: "2026.10.1\n"},
		"config":    {},
		"login":     {Stdout: testKey},
		"status":    statusDoc("locked"),
		"sync":      {Exit: 1, Stdout: sentinel, Stderr: "Vault is locked. " + sentinel + "\n"},
		"logout":    {Exit: 1, Stderr: sentinel + "\n"},
	}})
	if code != 1 {
		t.Errorf("exit status %d", code)
	}
	for _, want := range []string{"sync: failed (class locked)", "logout: failed (class exit)", "teardown: profile removed"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	assertClean(t, out)
}

func TestUsage(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out, &out, deps{}); code != 2 {
		t.Errorf("exit status %d", code)
	}
}
