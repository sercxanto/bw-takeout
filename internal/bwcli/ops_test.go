package bwcli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"bw-takeout/internal/bwcli/bwclitest"
)

const testServer = "https://localhost:8087"

func sha256File(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func wantNoProcess(t *testing.T, f *fixture) {
	t.Helper()
	if n := len(f.records()); n != 0 {
		t.Errorf("%d bw processes started", n)
	}
}

// Requirement: Every process uses the run's data directory.

func TestVersionQuery(t *testing.T) {
	f := newFixture(t, scenario("--version", bwclitest.Behavior{Stdout: "2026.10.1\n"}))
	id, err := f.client.Version(bg)
	if err != nil {
		t.Fatal(err)
	}
	if id.Version != "2026.10.1" || id.SHA256 != sha256File(t, fakePath) {
		t.Errorf("got %+v", id)
	}
	rec := f.lastRecord()
	if !slices.Equal(rec.Args, []string{"--version"}) {
		t.Errorf("arguments %q", rec.Args)
	}
	if v, _ := rec.Lookup("BITWARDENCLI_APPDATA_DIR"); v != f.data {
		t.Errorf("BITWARDENCLI_APPDATA_DIR is %q", v)
	}
	if !f.client.profileUsed {
		t.Error("data directory not confirmed")
	}
}

func TestBwUsesAnotherDataDirectory(t *testing.T) {
	f := newFixture(t, scenario("--version", bwclitest.Behavior{Stdout: "2026.10.1\n", SkipData: true}))
	_, err := f.client.Version(bg)
	wantClass(t, err, ClassProfileNotUsed)
	if f.client.profileUsed {
		t.Error("data directory confirmed")
	}
}

func TestDataFileIsADirectory(t *testing.T) {
	f := newFixture(t, scenario("--version", bwclitest.Behavior{Stdout: "2026.10.1\n", SkipData: true}))
	if err := os.Mkdir(filepath.Join(f.data, dataFile), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := f.client.Version(bg)
	wantClass(t, err, ClassProfileNotUsed)
}

// Requirement: Identity of bw is reported and never judged.

func TestVersionUnknownToThisProject(t *testing.T) {
	stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	f := newFixture(t, scenario("--version", bwclitest.Behavior{Stdout: "9999.99.0-synthetic\n"}, "status", statusUnauthenticated),
		func(c *Config) { c.Stderr = stderr })
	id, err := f.client.Version(bg)
	if err != nil || id.Version != "9999.99.0-synthetic" {
		t.Fatalf("%+v, %v", id, err)
	}
	if _, err := f.client.Status(bg); err != nil {
		t.Errorf("status after an unknown version: %v", err)
	}
	if fi, err := stderr.Stat(); err != nil || fi.Size() != 0 {
		t.Errorf("a message was written: %v", err)
	}
}

func TestVersionOutputIsEmpty(t *testing.T) {
	f := newFixture(t, scenario("--version", bwclitest.Behavior{}))
	_, err := f.client.Version(bg)
	e := wantClass(t, err, ClassOutput)
	if e.ExitCode != 0 {
		t.Errorf("exit code %d", e.ExitCode)
	}
	if f.client.profileUsed {
		t.Error("data directory confirmed after a failed query")
	}
}

func TestVersionOutputForms(t *testing.T) {
	for out, ok := range map[string]bool{
		"2026.10.1":                     true,
		"2026.10.1\r\n":                 true,
		strings.Repeat("9", 64) + "\n":  true,
		strings.Repeat("9", 65):         false,
		"2026.10.1\n\n":                 false,
		"2026.10.1\nsecond line\n":      false,
		"\n":                            false,
		"2026.10.1\x1b[0m\n":            false,
		"2026.10.1\u00a0\n":             false,
		"Could not find data file\n1\n": false,
	} {
		if _, got := versionLine([]byte(out)); got != ok {
			t.Errorf("%q: accepted %v, want %v", out, got, ok)
		}
	}
}

func TestVersionWithoutExecutable(t *testing.T) {
	f := newFixture(t, scenario())
	f.client.cfg.Executable = filepath.Join(t.TempDir(), "missing-bw")
	_, err := f.client.Version(bg)
	wantClass(t, err, ClassStart)
	wantNoProcess(t, f)
}

// Requirement: Results are read from the output.

func TestStatusOfAProfileWithoutLogin(t *testing.T) {
	f := newFixture(t, scenario("status", statusUnauthenticated))
	st, err := f.client.Status(bg)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != StateUnauthenticated || st.ServerURL != "" || st.UserID != "" || st.UserEmail != "" || st.LastSync != nil {
		t.Errorf("got %+v", st)
	}
}

func TestStatusWithoutASyncTime(t *testing.T) {
	for _, doc := range []string{
		`{"serverUrl":"https://localhost:8087","userEmail":"a@example.invalid","userId":"` + testItem + `","status":"unlocked"}`,
		`{"serverUrl":"https://localhost:8087","lastSync":null,"status":"locked"}`,
	} {
		f := newFixture(t, scenario("status", bwclitest.Behavior{Stdout: doc}))
		st, err := f.client.Status(bg)
		if err != nil {
			t.Fatal(err)
		}
		if st.LastSync != nil {
			t.Errorf("%s: lastSync %v, want missing", doc, st.LastSync)
		}
	}
}

func TestStatusDocumentDoesNotParse(t *testing.T) {
	for _, out := range []string{"", "unauthenticated", "{", `{"status":"unlocked"}{"status":"locked"}`, "[]"} {
		f := newFixture(t, scenario("status", bwclitest.Behavior{Stdout: out}))
		_, err := f.client.Status(bg)
		e := wantClass(t, err, ClassOutput)
		if e.Op != OpStatus || e.ExitCode != 0 {
			t.Errorf("%q: got %v", out, e)
		}
	}
}

func TestUnknownStatusValue(t *testing.T) {
	f := newFixture(t, scenario("status", bwclitest.Behavior{Stdout: `{"serverUrl":null,"lastSync":null,"status":"logged_out"}`}))
	_, err := f.client.Status(bg)
	wantClass(t, err, ClassOutput)
}

func TestLastSyncThatIsNotATime(t *testing.T) {
	for _, v := range []string{`"yesterday"`, `"2026-11-14"`, `"2026-11-14 09:06:29"`, `1763111189`, `""`} {
		f := newFixture(t, scenario("status", bwclitest.Behavior{Stdout: `{"lastSync":` + v + `,"status":"unlocked"}`}))
		_, err := f.client.Status(bg)
		wantClass(t, err, ClassOutput)
	}
}

func TestStatusIsNotReadFromTheExitCode(t *testing.T) {
	f := newFixture(t, scenario("status", bwclitest.Behavior{Exit: 1, Stdout: statusUnlocked.Stdout}))
	_, err := f.client.Status(bg)
	wantClass(t, err, ClassExit)
}

// Requirement: Server address is checked before use.

func TestAddressWithoutHTTPS(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{}))
	err := f.client.SetServer(bg, "http://localhost:8087")
	e := wantClass(t, err, ClassInvalidArgument)
	if e.ExitCode != NoExitCode {
		t.Errorf("exit code %d", e.ExitCode)
	}
	wantNoProcess(t, f)
}

func TestHTTPSAddress(t *testing.T) {
	f := newFixture(t, scenario(
		"config server "+testServer, bwclitest.Behavior{Stdout: "Saved setting `config`.\n"},
		"status", bwclitest.Behavior{Stdout: `{"serverUrl":"` + testServer + `","lastSync":null,"status":"unauthenticated"}`}))
	if err := f.client.SetServer(bg, testServer); err != nil {
		t.Fatal(err)
	}
	rec := f.records()[0]
	if !slices.Equal(rec.Args, []string{"config", "server", testServer}) {
		t.Errorf("arguments %q", rec.Args)
	}
	st, err := f.client.Status(bg)
	if err != nil || st.ServerURL != testServer {
		t.Errorf("status %+v, %v", st, err)
	}
}

func TestRefusedServerAddresses(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{}))
	for _, a := range []string{
		"", "localhost:8087", "ftp://localhost", "-https://localhost", "https:localhost",
		"https://user@localhost", "https://user:synthetic@localhost:8087",
		"https://", "https:///vault", "https://:8087",
		"https://localhost\n", "https://localhost\r\n--raw", "https://local\nhost",
	} {
		err := f.client.SetServer(bg, a)
		wantClass(t, err, ClassInvalidArgument)
		assertNoText(t, err, "localhost")
	}
	wantNoProcess(t, f)
}

func TestAcceptedServerAddresses(t *testing.T) {
	for _, a := range []string{testServer, "https://vault.example.invalid", "https://[::1]:8443/bitwarden", "HTTPS://vault.example.invalid"} {
		if !ValidServerAddress(a) {
			t.Errorf("%q refused", a)
		}
	}
}

// Operations with the arguments of design D5.

func TestOperationArguments(t *testing.T) {
	docs := map[string]string{
		"export --format json --raw":           `{"encrypted":false,"items":[{"name":"é"}]}` + "\n",
		"export --format encrypted_json --raw": `{"encrypted":true,"data":"2.x|y|z"}`,
		"list items --raw":                     "[]\r\n",
		"list items --archived --raw":          ` [{"id":"` + testItem + `"}] `,
	}
	s := scenario("sync --force --raw", bwclitest.Behavior{})
	for args, doc := range docs {
		s.Commands[args] = bwclitest.Behavior{Stdout: doc}
	}
	f := newFixture(t, s)
	if err := f.client.Sync(bg); err != nil {
		t.Fatal(err)
	}
	for _, op := range []struct {
		args string
		fn   func() (Output, error)
	}{
		{"export --format json --raw", func() (Output, error) { return f.client.ExportJSON(bg) }},
		{"export --format encrypted_json --raw", func() (Output, error) { return f.client.ExportEncryptedJSON(bg) }},
		{"list items --raw", func() (Output, error) { return f.client.ListItems(bg) }},
		{"list items --archived --raw", func() (Output, error) { return f.client.ListArchivedItems(bg) }},
	} {
		out, err := op.fn()
		if err != nil {
			t.Fatalf("%s: %v", op.args, err)
		}
		if string(out.Data) != docs[op.args] {
			t.Errorf("%s: got %q", op.args, out.Data)
		}
	}
	var got []string
	for _, r := range f.records() {
		got = append(got, strings.Join(r.Args, "\x00"))
	}
	want := []string{
		"sync\x00--force\x00--raw",
		"export\x00--format\x00json\x00--raw",
		"export\x00--format\x00encrypted_json\x00--raw",
		"list\x00items\x00--raw",
		"list\x00items\x00--archived\x00--raw",
	}
	if !slices.Equal(got, want) {
		t.Errorf("arguments %q, want %q", got, want)
	}
}

func TestEmptyDocument(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stdout: " \n"}))
	_, err := f.client.ExportJSON(bg)
	wantClass(t, err, ClassOutput)
	_, err = f.client.ListItems(bg)
	wantClass(t, err, ClassOutput)
}

func TestDocumentWithDecryptionErrors(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stdout: "[]", Stderr: decryptionLines}))
	out, err := f.client.ListItems(bg)
	if err != nil || out.DecryptionErrors != 3 {
		t.Errorf("%+v, %v", out, err)
	}
}

// Requirement: Attachments are requested by ID.

func TestDownloadByIDPair(t *testing.T) {
	content := "\x00syntheticé\r\n\n"
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stdout: content}))
	out, err := f.client.Attachment(bg, testItem, testAttachment)
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Data) != content {
		t.Errorf("got %q", out.Data)
	}
	want := []string{"get", "attachment", testAttachment, "--itemid", testItem, "--raw"}
	if rec := f.lastRecord(); !slices.Equal(rec.Args, want) {
		t.Errorf("arguments %q", rec.Args)
	}
}

func TestEmptyAttachment(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{}))
	out, err := f.client.Attachment(bg, testItem, testAttachment)
	if err != nil || len(out.Data) != 0 {
		t.Errorf("%q, %v", out.Data, err)
	}
}

func TestIDThatStartsWithADash(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{}))
	for _, ids := range [][2]string{
		{testItem, "-" + testAttachment[1:]},
		{"-" + testItem[1:], testAttachment},
		{testItem, "--raw"},
	} {
		_, err := f.client.Attachment(bg, ids[0], ids[1])
		e := wantClass(t, err, ClassInvalidArgument)
		if e.ItemID != "" || e.AttachmentID != "" {
			t.Errorf("refused IDs in the failure: %v", e)
		}
	}
	wantNoProcess(t, f)
}

func TestRefusedAttachmentIDs(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{}))
	for _, ids := range [][2]string{
		{testItem, "a/b"},
		{testItem, `a\b`},
		{testItem, ".."},
		{testItem, ""},
		{"", testAttachment},
		{testItem, strings.Repeat("a", 65)},
		{testItem[:35], testAttachment},
		{testItem + "0", testAttachment},
		{strings.ReplaceAll(testItem, "-", "/"), testAttachment},
		{testItem, testAttachment + "\n"},
	} {
		_, err := f.client.Attachment(bg, ids[0], ids[1])
		wantClass(t, err, ClassInvalidArgument)
	}
	wantNoProcess(t, f)
}

// Requirement: Session key stays in the child environment.

// login stands in for a successful login until the login is used: it
// gives the client the synthetic session key.
func login(f *fixture) {
	f.client.session = []byte(testKey)
}

func wantNoKeyInArguments(t *testing.T, recs []bwclitest.Record) {
	t.Helper()
	for _, r := range recs {
		for _, a := range r.Args {
			if strings.Contains(a, testKey) {
				t.Errorf("%q: the session key is an argument", r.Args)
			}
		}
	}
}

func TestCommandBeforeLogin(t *testing.T) {
	f := newFixture(t, scenario("--version", bwclitest.Behavior{Stdout: "2026.10.1\n"}, "status", statusUnauthenticated, "config", bwclitest.Behavior{}))
	if _, err := f.client.Version(bg); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client.Status(bg); err != nil {
		t.Fatal(err)
	}
	if err := f.client.SetServer(bg, testServer); err != nil {
		t.Fatal(err)
	}
	recs := f.records()
	if len(recs) != 3 {
		t.Fatalf("%d processes", len(recs))
	}
	for _, r := range recs {
		if _, ok := r.Lookup("BW_SESSION"); ok {
			t.Errorf("%q received a session key", r.Args)
		}
	}
}

func TestCommandAfterLogin(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stdout: "[]"}, "status", statusUnlocked))
	login(f)
	if err := f.client.Sync(bg); err != nil {
		t.Fatal(err)
	}
	for _, fn := range []func() (Output, error){
		func() (Output, error) { return f.client.ExportJSON(bg) },
		func() (Output, error) { return f.client.ExportEncryptedJSON(bg) },
		func() (Output, error) { return f.client.ListItems(bg) },
		func() (Output, error) { return f.client.ListArchivedItems(bg) },
		func() (Output, error) { return f.client.Attachment(bg, testItem, testAttachment) },
	} {
		if _, err := fn(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.client.Status(bg); err != nil {
		t.Fatal(err)
	}
	recs := f.records()
	if len(recs) != 7 {
		t.Fatalf("%d processes", len(recs))
	}
	for _, r := range recs {
		if v, ok := r.Lookup("BW_SESSION"); !ok || v != testKey {
			t.Errorf("%q did not receive the session key", r.Args)
		}
	}
	wantNoKeyInArguments(t, recs)
}

func TestCommandAfterLogout(t *testing.T) {
	f := newFixture(t, scenario("logout", bwclitest.Behavior{Stdout: "You have logged out.\n"}, "status", statusUnauthenticated))
	login(f)
	key := f.client.session
	if err := f.client.Logout(bg); err != nil {
		t.Fatal(err)
	}
	if f.client.LoggedIn() {
		t.Error("still logged in")
	}
	if strings.Trim(string(key), "\x00") != "" {
		t.Error("session key not overwritten")
	}
	if _, err := f.client.Status(bg); err != nil {
		t.Fatal(err)
	}
	recs := f.records()
	if v, _ := recs[0].Lookup("BW_SESSION"); v != testKey {
		t.Error("the logout did not receive the session key")
	}
	if _, ok := recs[1].Lookup("BW_SESSION"); ok {
		t.Error("the status after the logout received a session key")
	}
	wantNoKeyInArguments(t, recs)
}

func TestFailedLogoutOverwritesTheSessionKey(t *testing.T) {
	for name, b := range map[string]bwclitest.Behavior{
		"exit code":  {Exit: 1, Stderr: "You are not logged in.\n"},
		"stray file": {WriteFile: "logout.txt"},
	} {
		f := newFixture(t, scenario("logout", b))
		login(f)
		key := f.client.session
		if err := f.client.Logout(bg); err == nil {
			t.Fatalf("%s: logout succeeded", name)
		}
		if f.client.LoggedIn() || strings.Trim(string(key), "\x00") != "" {
			t.Errorf("%s: session key kept", name)
		}
	}
}

func TestLogoutWithoutLogin(t *testing.T) {
	f := newFixture(t, scenario("logout", bwclitest.Behavior{Exit: 1, Stderr: "You are not logged in.\n"}))
	err := f.client.Logout(bg)
	wantClass(t, err, ClassExit)
	if rec := f.lastRecord(); !slices.Equal(rec.Args, []string{"logout"}) {
		t.Errorf("arguments %q", rec.Args)
	}
}

func TestFailureWithASessionKeyInUse(t *testing.T) {
	leak := bwclitest.Behavior{Exit: 1, Stdout: testKey + "\n", Stderr: "BW_SESSION=" + testKey + "\n"}
	f := newFixture(t, scenario("*", leak, "status", statusUnlocked))
	login(f)
	errs := []error{f.client.Sync(bg)}
	for _, fn := range []func() (Output, error){
		func() (Output, error) { return f.client.ExportJSON(bg) },
		func() (Output, error) { return f.client.Attachment(bg, testItem, testAttachment) },
	} {
		out, err := fn()
		if out.Data != nil {
			t.Error("output returned with a failure")
		}
		errs = append(errs, err)
	}
	errs = append(errs, f.client.Logout(bg))
	for _, err := range errs {
		if err == nil {
			t.Fatal("no failure")
		}
		assertNoText(t, err, testKey)
	}
}
