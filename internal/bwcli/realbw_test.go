package bwcli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// realBWVariable holds the path of a real bw executable. The tests
// TestRealBW… run only when it is set (design D13). They need no account,
// but each fresh profile sends one anonymous configuration request to
// Bitwarden Cloud, so they are not offline and CI does not run them.
const realBWVariable = "BW_TAKEOUT_REAL_BW"

// realClient returns a client for the real bw with a fresh data and
// working directory and the terminal of the test process. It skips the
// test when realBWVariable is not set.
func realClient(t *testing.T) (*Client, string) {
	t.Helper()
	path := os.Getenv(realBWVariable)
	if path == "" {
		t.Skip("skipped: opt-in, set BW_TAKEOUT_REAL_BW to the path of a bw executable; contacts Bitwarden Cloud once per test")
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("%s must be an absolute path, got %q", realBWVariable, path)
	}
	root := t.TempDir()
	data, work := filepath.Join(root, "data"), filepath.Join(root, "work")
	for _, d := range []string{data, work} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdin.Close() })
	stderr, err := os.Create(filepath.Join(root, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stderr.Close() })
	// Terminal is nil, so the client uses the real terminal check on
	// standard input and standard error, which are not terminals here.
	c, err := New(Config{Executable: path, DataDir: data, WorkDir: work, Stdin: stdin, Stderr: stderr})
	if err != nil {
		t.Fatal(err)
	}
	return c, data
}

func realVersion(t *testing.T, c *Client) Identity {
	t.Helper()
	id, err := c.Version(bg)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	return id
}

// Corresponds to the fake behavior "version line and data.json" and the
// scenario "Version query".
func TestRealBWVersionAndHash(t *testing.T) {
	c, data := realClient(t)
	id := realVersion(t, c)
	if id.Version == "" {
		t.Error("empty version")
	}
	resolved, err := filepath.EvalSymlinks(c.cfg.Executable)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if id.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("hash %s, file %s", id.SHA256, resolved)
	}
	if fi, err := os.Stat(filepath.Join(data, dataFile)); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("data.json in the data directory: %v", err)
	}
	t.Logf("bw %s, SHA-256 %s of %s", id.Version, id.SHA256, resolved)
}

// Corresponds to the fake status "unauthenticated" without lastSync and
// the scenario "Status of a profile without login".
func TestRealBWStatusWithoutLogin(t *testing.T) {
	c, _ := realClient(t)
	realVersion(t, c)
	s, err := c.Status(bg)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != StateUnauthenticated || s.LastSync != nil || s.UserID != "" || s.UserEmail != "" {
		t.Errorf("got %+v", s)
	}
}

// Corresponds to the fake failure followed by an unauthenticated status
// and the scenario "Not logged in".
func TestRealBWNotLoggedIn(t *testing.T) {
	c, _ := realClient(t)
	realVersion(t, c)
	wantClass(t, c.Sync(bg), ClassNotLoggedIn)
	_, err := c.ListItems(bg)
	wantClass(t, err, ClassNotLoggedIn)
	_, err = c.ExportJSON(bg)
	wantClass(t, err, ClassNotLoggedIn)
}

// Corresponds to the fake exit code 0 under BW_CLEANEXIT and the scenario
// "Exit code override in the calling environment".
func TestRealBWCleanExitInCallingEnvironment(t *testing.T) {
	t.Setenv("BW_CLEANEXIT", "true")
	c, _ := realClient(t)
	realVersion(t, c)
	wantClass(t, c.Sync(bg), ClassNotLoggedIn)
}

// Corresponds to the scenario "Login without a terminal", with the real
// terminal check instead of the fake terminal.
func TestRealBWLoginWithoutTerminal(t *testing.T) {
	c, _ := realClient(t)
	realVersion(t, c)
	wantClass(t, c.Login(bg), ClassNoTerminal)
	if c.LoggedIn() {
		t.Error("client holds a session key")
	}
}
